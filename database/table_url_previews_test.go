package database

import (
	"database/sql"
	"os"
	"sync"
	"testing"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/t2bot/matrix-media-repo/common/rcontext"
	"github.com/t2bot/matrix-media-repo/util"
)

const createUrlPreviewsTestTable = `
CREATE TABLE url_previews (
	url TEXT NOT NULL,
	error_code TEXT NOT NULL,
	bucket_ts BIGINT NOT NULL,
	site_url TEXT NOT NULL,
	site_name TEXT NOT NULL,
	resource_type TEXT NOT NULL,
	description TEXT NOT NULL,
	title TEXT NOT NULL,
	image_mxc TEXT NOT NULL,
	image_type TEXT NOT NULL,
	image_size BIGINT NOT NULL,
	image_width INT NOT NULL,
	image_height INT NOT NULL,
	language_header TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX url_previews_index ON url_previews (url, bucket_ts, language_header);
`

func openUrlPreviewsTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("MMR_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("MMR_TEST_POSTGRES_DSN not set")
	}

	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})

	require.NoError(t, db.Ping())
	_, err = db.Exec(`DROP TABLE IF EXISTS url_previews;`)
	require.NoError(t, err)
	_, err = db.Exec(createUrlPreviewsTestTable)
	require.NoError(t, err)

	return db
}

func makePreviewRecord(languageHeader string) *DbUrlPreview {
	return &DbUrlPreview{
		Url:            "https://example.com/test",
		ErrorCode:      "",
		BucketTs:       util.GetHourBucket(1714923456789),
		SiteUrl:        "https://example.com/test",
		SiteName:       "example",
		ResourceType:   "website",
		Description:    "description",
		Title:          "title",
		ImageMxc:       "mxc://example.com/image",
		ImageType:      "image/png",
		ImageSize:      42,
		ImageWidth:     8,
		ImageHeight:    8,
		LanguageHeader: languageHeader,
	}
}

func TestUrlPreviewInsertRespectsLanguageCacheKey(t *testing.T) {
	db := openUrlPreviewsTestDB(t)
	stmts, err := prepareUrlPreviewsTables(db)
	require.NoError(t, err)

	table := stmts.Prepare(rcontext.Initial())
	rawTs := int64(1714923456789)
	bucketTs := util.GetHourBucket(rawTs)
	const workers = 8
	inserted := make(chan bool, workers)
	errCh := make(chan error, workers)
	start := new(sync.WaitGroup)
	start.Add(1)
	waiter := new(sync.WaitGroup)

	for i := 0; i < workers; i++ {
		waiter.Add(1)
		go func() {
			defer waiter.Done()
			start.Wait()
			ok, err := table.Insert(makePreviewRecord("en"))
			inserted <- ok
			errCh <- err
		}()
	}

	start.Done()
	waiter.Wait()
	close(inserted)
	close(errCh)

	insertedCount := 0
	for ok := range inserted {
		if ok {
			insertedCount++
		}
	}
	for err := range errCh {
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, insertedCount)

	record, err := table.Get("https://example.com/test", rawTs, "en")
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, "en", record.LanguageHeader)

	count := 0
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM url_previews WHERE url = $1 AND bucket_ts = $2`, "https://example.com/test", bucketTs).Scan(&count))
	assert.Equal(t, 1, count)

	ok, err := table.Insert(makePreviewRecord("fr"))
	require.NoError(t, err)
	assert.True(t, ok)

	recordFr, err := table.Get("https://example.com/test", rawTs, "fr")
	require.NoError(t, err)
	require.NotNil(t, recordFr)
	assert.Equal(t, "fr", recordFr.LanguageHeader)

	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM url_previews WHERE url = $1 AND bucket_ts = $2`, "https://example.com/test", bucketTs).Scan(&count))
	assert.Equal(t, 2, count)
}

func TestUrlPreviewGetBucketsRawTimestamps(t *testing.T) {
	db := openUrlPreviewsTestDB(t)
	stmts, err := prepareUrlPreviewsTables(db)
	require.NoError(t, err)

	table := stmts.Prepare(rcontext.Initial())
	rawTs := int64(1714923456789)
	record := makePreviewRecord("en")
	record.BucketTs = util.GetHourBucket(rawTs)
	inserted, err := table.Insert(record)
	require.NoError(t, err)
	assert.True(t, inserted)

	fetched, err := table.Get(record.Url, rawTs, record.LanguageHeader)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, record.BucketTs, fetched.BucketTs)
	assert.Equal(t, record.LanguageHeader, fetched.LanguageHeader)
}

func TestUrlPreviewUpdateIfErrorPromotesExistingError(t *testing.T) {
	db := openUrlPreviewsTestDB(t)
	stmts, err := prepareUrlPreviewsTables(db)
	require.NoError(t, err)

	table := stmts.Prepare(rcontext.Initial())
	rawTs := int64(1714923456789)
	bucketTs := util.GetHourBucket(rawTs)
	table.InsertError("https://example.com/test", "M_NOT_FOUND", bucketTs, "en")

	updated, err := table.UpdateIfError(makePreviewRecord("en"))
	require.NoError(t, err)
	assert.True(t, updated)

	record, err := table.Get("https://example.com/test", rawTs, "en")
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, "", record.ErrorCode)
	assert.Equal(t, "title", record.Title)
	assert.Equal(t, "mxc://example.com/image", record.ImageMxc)
}
