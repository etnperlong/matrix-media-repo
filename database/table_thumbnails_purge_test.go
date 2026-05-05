package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/t2bot/matrix-media-repo/common/rcontext"
)

func TestGetOlderThanBatchLimitsAndUsesPerRowCutoff(t *testing.T) {
	db := openRowsLifecycleTestDB(t)
	useIsolatedSchema(t, db)

	_, err := db.Exec(`
		CREATE TABLE media (origin TEXT, media_id TEXT, upload_name TEXT, content_type TEXT, user_id TEXT, sha256_hash TEXT, size_bytes BIGINT, creation_ts BIGINT, quarantined BOOLEAN, datastore_id TEXT, location TEXT);
		CREATE TABLE thumbnails (origin TEXT, media_id TEXT, content_type TEXT, width INT, height INT, method TEXT, animated BOOLEAN, sha256_hash TEXT, size_bytes BIGINT, creation_ts BIGINT, datastore_id TEXT, location TEXT);
		INSERT INTO thumbnails
		SELECT 'example.com', 'old-' || n::TEXT, 'image/png', 64, 64, 'crop', FALSE, 'old-hash-' || n::TEXT, 1, 10, 'ds1', 'old-loc-' || n::TEXT
		FROM generate_series(1, 1005) AS n;
		INSERT INTO thumbnails VALUES ('example.com', 'protected', 'image/png', 64, 64, 'crop', FALSE, 'protected-hash', 1, 10, 'ds1', 'protected-loc');
		INSERT INTO media VALUES ('example.com', 'protected-media', 'name', 'image/png', '@u:example.com', 'protected-hash', 1, 10, FALSE, 'ds1', 'protected-loc');
		INSERT INTO thumbnails VALUES ('example.com', 'newer', 'image/png', 64, 64, 'crop', FALSE, 'newer-hash', 1, 10000, 'ds1', 'newer-loc');
		INSERT INTO thumbnails VALUES ('example.com', 'newer-same-hash', 'image/png', 64, 64, 'crop', FALSE, 'old-hash-1', 1, 10000, 'ds1', 'newer-same-hash-loc');
	`)
	require.NoError(t, err)

	stmts, err := prepareThumbnailsTables(db)
	require.NoError(t, err)
	thumbsDb := stmts.Prepare(rcontext.Initial())

	firstBatch, err := thumbsDb.GetOlderThanBatch(100, 1000)
	require.NoError(t, err)
	require.Len(t, firstBatch, 1000)
	for _, thumb := range firstBatch {
		assert.NotEqual(t, "newer", thumb.MediaId)
		assert.NotEqual(t, "newer-same-hash", thumb.MediaId)
	}

	for _, thumb := range firstBatch {
		require.NoError(t, thumbsDb.Delete(thumb))
	}

	secondBatch, err := thumbsDb.GetOlderThanBatch(100, 1000)
	require.NoError(t, err)
	require.Len(t, secondBatch, 6)
	for _, thumb := range secondBatch {
		assert.NotEqual(t, "newer", thumb.MediaId)
		assert.NotEqual(t, "newer-same-hash", thumb.MediaId)
	}

	remainingCount := 0
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM thumbnails WHERE media_id IN ('newer', 'newer-same-hash')`).Scan(&remainingCount))
	assert.Equal(t, 2, remainingCount)
}

func TestGetOlderThanBatchAfterAdvancesPastUndeletableRows(t *testing.T) {
	db := openRowsLifecycleTestDB(t)
	useIsolatedSchema(t, db)

	_, err := db.Exec(`
		CREATE TABLE thumbnails (origin TEXT, media_id TEXT, content_type TEXT, width INT, height INT, method TEXT, animated BOOLEAN, sha256_hash TEXT, size_bytes BIGINT, creation_ts BIGINT, datastore_id TEXT, location TEXT);
		INSERT INTO thumbnails VALUES ('example.com', 'old-1', 'image/png', 64, 64, 'crop', FALSE, 'hash-1', 1, 10, 'ds1', 'loc-1');
		INSERT INTO thumbnails VALUES ('example.com', 'old-2', 'image/png', 64, 64, 'crop', FALSE, 'hash-2', 1, 20, 'ds1', 'loc-2');
		INSERT INTO thumbnails VALUES ('example.com', 'old-3', 'image/png', 64, 64, 'crop', FALSE, 'hash-3', 1, 30, 'ds1', 'loc-3');
	`)
	require.NoError(t, err)

	stmts, err := prepareThumbnailsTables(db)
	require.NoError(t, err)
	thumbsDb := stmts.Prepare(rcontext.Initial())

	firstBatch, err := thumbsDb.GetOlderThanBatch(100, 2)
	require.NoError(t, err)
	require.Len(t, firstBatch, 2)
	assert.Equal(t, []string{"old-1", "old-2"}, []string{firstBatch[0].MediaId, firstBatch[1].MediaId})

	nextBatch, err := thumbsDb.GetOlderThanBatchAfter(100, 2, firstBatch[len(firstBatch)-1])
	require.NoError(t, err)
	require.Len(t, nextBatch, 1)
	assert.Equal(t, "old-3", nextBatch[0].MediaId)
}
