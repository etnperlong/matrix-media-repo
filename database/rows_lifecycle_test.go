package database

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/t2bot/matrix-media-repo/common/rcontext"
)

func openRowsLifecycleTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("MMR_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("MMR_TEST_POSTGRES_DSN not set")
	}

	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = db.Close()
	})
	require.NoError(t, db.Ping())
	return db
}

func useIsolatedSchema(t *testing.T, db *sql.DB) string {
	t.Helper()
	schema := strings.NewReplacer("/", "_", "-", "_", " ", "_").Replace(strings.ToLower(t.Name()))
	_, err := db.Exec("CREATE SCHEMA " + pq.QuoteIdentifier(schema))
	require.NoError(t, err)
	_, err = db.Exec("SET search_path TO " + pq.QuoteIdentifier(schema))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("SET search_path TO public")
		_, _ = db.Exec("DROP SCHEMA IF EXISTS " + pq.QuoteIdentifier(schema) + " CASCADE")
	})
	return schema
}

func assertNoRowsLeak(t *testing.T, db *sql.DB) {
	t.Helper()
	assert.Equal(t, 0, db.Stats().InUse)
}

func TestMediaTableRowsAreClosed(t *testing.T) {
	db := openRowsLifecycleTestDB(t)

	t.Run("GetDistinctDatastoreIds", func(t *testing.T) {
		stmt, err := db.Prepare(`SELECT 'ds1'::TEXT`)
		require.NoError(t, err)
		defer stmt.Close()
		table := &MediaTableWithContext{statements: &mediaTableStatements{selectDistinctMediaDatastoreIds: stmt}, ctx: rcontext.Initial()}

		ids, err := table.GetDistinctDatastoreIds()
		require.NoError(t, err)
		assert.Equal(t, []string{"ds1"}, ids)
		assertNoRowsLeak(t, db)

		badStmt, err := db.Prepare(`SELECT NULL::TEXT`)
		require.NoError(t, err)
		defer badStmt.Close()
		table.statements.selectDistinctMediaDatastoreIds = badStmt

		_, err = table.GetDistinctDatastoreIds()
		require.Error(t, err)
		assertNoRowsLeak(t, db)
	})

	t.Run("scanRows", func(t *testing.T) {
		stmt, err := db.Prepare(`SELECT 'example.com'::TEXT, 'mid'::TEXT, 'name'::TEXT, 'image/png'::TEXT, '@u:example.com'::TEXT, $1::TEXT, 1::BIGINT, 1::BIGINT, FALSE, 'ds1'::TEXT, 'loc1'::TEXT`)
		require.NoError(t, err)
		defer stmt.Close()
		table := &MediaTableWithContext{statements: &mediaTableStatements{selectMediaByHash: stmt}, ctx: rcontext.Initial()}

		records, err := table.GetByHash("hash")
		require.NoError(t, err)
		require.Len(t, records, 1)
		assert.Equal(t, "loc1", records[0].Location)
		assertNoRowsLeak(t, db)

		badStmt, err := db.Prepare(`SELECT NULL::TEXT, 'mid'::TEXT, 'name'::TEXT, 'image/png'::TEXT, '@u:example.com'::TEXT, $1::TEXT, 1::BIGINT, 1::BIGINT, FALSE, 'ds1'::TEXT, 'loc1'::TEXT`)
		require.NoError(t, err)
		defer badStmt.Close()
		table.statements.selectMediaByHash = badStmt

		_, err = table.GetByHash("hash")
		require.Error(t, err)
		assertNoRowsLeak(t, db)
	})
}

func TestThumbnailsTableRowsAreClosed(t *testing.T) {
	db := openRowsLifecycleTestDB(t)
	stmt, err := db.Prepare(`SELECT $1::TEXT, $2::TEXT, 'image/png'::TEXT, 64::INT, 64::INT, 'crop'::TEXT, FALSE, 'hash'::TEXT, 1::BIGINT, 1::BIGINT, 'ds1'::TEXT, 'loc1'::TEXT`)
	require.NoError(t, err)
	defer stmt.Close()
	table := &thumbnailsTableWithContext{statements: &thumbnailsTableStatements{selectThumbnailsForMedia: stmt}, ctx: rcontext.Initial()}

	thumbs, err := table.GetForMedia("example.com", "mid")
	require.NoError(t, err)
	require.Len(t, thumbs, 1)
	assert.Equal(t, "loc1", thumbs[0].Location)
	assertNoRowsLeak(t, db)

	badStmt, err := db.Prepare(`SELECT $1::TEXT, $2::TEXT, 'image/png'::TEXT, NULL::INT, 64::INT, 'crop'::TEXT, FALSE, 'hash'::TEXT, 1::BIGINT, 1::BIGINT, 'ds1'::TEXT, 'loc1'::TEXT`)
	require.NoError(t, err)
	defer badStmt.Close()
	table.statements.selectThumbnailsForMedia = badStmt

	_, err = table.GetForMedia("example.com", "mid")
	require.Error(t, err)
	assertNoRowsLeak(t, db)
}

func TestTasksTableRowsAreClosed(t *testing.T) {
	db := openRowsLifecycleTestDB(t)
	stmt, err := db.Prepare(`SELECT 1::INT, 'demo'::TEXT, '{}'::JSON, 1::BIGINT, 0::BIGINT, ''::TEXT`)
	require.NoError(t, err)
	defer stmt.Close()
	table := &tasksTableWithContext{statements: &tasksTableStatements{selectAllTasks: stmt, selectIncompleteTasks: stmt}, ctx: rcontext.Initial()}

	tasks, err := table.GetAll(true)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "demo", tasks[0].Name)
	assertNoRowsLeak(t, db)

	badStmt, err := db.Prepare(`SELECT NULL::INT, 'demo'::TEXT, '{}'::JSON, 1::BIGINT, 0::BIGINT, ''::TEXT`)
	require.NoError(t, err)
	defer badStmt.Close()
	table.statements.selectAllTasks = badStmt

	_, err = table.GetAll(true)
	require.Error(t, err)
	assertNoRowsLeak(t, db)
}

func TestExportPartsRowsAreClosed(t *testing.T) {
	db := openRowsLifecycleTestDB(t)
	stmt, err := db.Prepare(`SELECT $1::TEXT, 1::INT, 5::BIGINT, 'part1'::TEXT, 'ds1'::TEXT, 'loc1'::TEXT`)
	require.NoError(t, err)
	defer stmt.Close()
	table := &exportPartsTableWithContext{statements: &exportPartsTableStatements{selectExportPartsById: stmt}, ctx: rcontext.Initial()}

	parts, err := table.GetForExport("exp")
	require.NoError(t, err)
	require.Len(t, parts, 1)
	assert.Equal(t, "loc1", parts[0].Location)
	assertNoRowsLeak(t, db)

	badStmt, err := db.Prepare(`SELECT $1::TEXT, NULL::INT, 5::BIGINT, 'part1'::TEXT, 'ds1'::TEXT, 'loc1'::TEXT`)
	require.NoError(t, err)
	defer badStmt.Close()
	table.statements.selectExportPartsById = badStmt

	_, err = table.GetForExport("exp")
	require.Error(t, err)
	assertNoRowsLeak(t, db)
}

func TestRestrictedMediaRowsAreClosed(t *testing.T) {
	db := openRowsLifecycleTestDB(t)
	stmt, err := db.Prepare(`SELECT $1::TEXT, $2::TEXT, 'io.t2bot.requires_authentication'::TEXT, 'true'::TEXT`)
	require.NoError(t, err)
	defer stmt.Close()
	table := &restrictedMediaTableWithContext{statements: &restrictedMediaTableStatements{selectRestrictedMedia: stmt}, ctx: rcontext.Initial()}

	restrictions, err := table.GetAllForId("example.com", "mid")
	require.NoError(t, err)
	require.Len(t, restrictions, 1)
	assert.Equal(t, RestrictedRequiresAuth, restrictions[0].Condition)
	assertNoRowsLeak(t, db)

	badStmt, err := db.Prepare(`SELECT NULLIF($1::TEXT, $1::TEXT), $2::TEXT, 'io.t2bot.requires_authentication'::TEXT, 'true'::TEXT`)
	require.NoError(t, err)
	defer badStmt.Close()
	table.statements.selectRestrictedMedia = badStmt

	_, err = table.GetAllForId("example.com", "mid")
	require.Error(t, err)
	assertNoRowsLeak(t, db)
}

func TestMetadataRowsAreClosed(t *testing.T) {
	db := openRowsLifecycleTestDB(t)
	useIsolatedSchema(t, db)

	_, err := db.Exec(`
		CREATE TABLE media (origin TEXT, media_id TEXT, upload_name TEXT, content_type TEXT, user_id TEXT, sha256_hash TEXT, size_bytes BIGINT, creation_ts BIGINT, quarantined BOOLEAN, datastore_id TEXT, location TEXT);
		CREATE TABLE last_access (sha256_hash TEXT, last_access_ts BIGINT);
		INSERT INTO media VALUES ('example.com','mid','name','image/png','@alice:example.com','hash',11,10,FALSE,'ds1','loc1');
		INSERT INTO last_access VALUES ('hash', 5);
	`)
	require.NoError(t, err)

	lastAccessStmt, err := db.Prepare(selectMediaForDatastoreWithLastAccess)
	require.NoError(t, err)
	defer lastAccessStmt.Close()
	table := &metadataVirtualTableWithContext{statements: &metadataVirtualTableStatements{db: db, selectMediaForDatastoreWithLastAccess: lastAccessStmt}, ctx: rcontext.Initial()}

	stats, total, err := table.UnoptimizedSynapseUserStatsPage("example.com", SynStatUserOrderByUserId, 0, 10, -1, -1, "", true)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, "@alice:example.com", stats[0].UserId)
	assertNoRowsLeak(t, db)

	entries, err := table.GetMediaForDatastoreByLastAccess("ds1", 6)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "loc1", entries[0].Location)
	assertNoRowsLeak(t, db)

	_, err = db.Exec(`TRUNCATE TABLE media; INSERT INTO media VALUES ('example.com','mid2','name','image/png','@bob:example.com','hash2',NULL,10,FALSE,'ds1','loc2');`)
	require.NoError(t, err)

	_, _, err = table.UnoptimizedSynapseUserStatsPage("example.com", SynStatUserOrderByUserId, 0, 10, -1, -1, "", true)
	require.Error(t, err)
	assertNoRowsLeak(t, db)

	badStmt, err := db.Prepare(`SELECT NULL::TEXT, 11::BIGINT, $2::TEXT, 'loc1'::TEXT, 10::BIGINT, $1::BIGINT, 'image/png'::TEXT`)
	require.NoError(t, err)
	defer badStmt.Close()
	table.statements.selectMediaForDatastoreWithLastAccess = badStmt

	_, err = table.GetMediaForDatastoreByLastAccess("ds1", 6)
	require.Error(t, err)
	assertNoRowsLeak(t, db)
}
