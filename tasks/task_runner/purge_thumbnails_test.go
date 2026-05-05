package task_runner

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/t2bot/matrix-media-repo/common/rcontext"
	"github.com/t2bot/matrix-media-repo/database"
)

type fakeThumbnailStore struct {
	byLocation map[thumbnailLocation][]*database.DbThumbnail
	deleted    []*database.DbThumbnail
}

func (f *fakeThumbnailStore) GetByLocation(datastoreId string, location string) ([]*database.DbThumbnail, error) {
	return f.byLocation[thumbnailLocation{datastoreId: datastoreId, location: location}], nil
}

func (f *fakeThumbnailStore) Delete(record *database.DbThumbnail) error {
	f.deleted = append(f.deleted, record)
	return nil
}

type fakeMediaStore struct {
	exists map[thumbnailLocation]bool
}

func (f *fakeMediaStore) LocationExists(datastoreId string, location string) (bool, error) {
	return f.exists[thumbnailLocation{datastoreId: datastoreId, location: location}], nil
}

func testThumbnail(mediaID string, creationTs int64, datastoreID string, location string) *database.DbThumbnail {
	return &database.DbThumbnail{
		Locatable:   &database.Locatable{Sha256Hash: mediaID + "-hash", DatastoreId: datastoreID, Location: location},
		Origin:      "example.com",
		MediaId:     mediaID,
		ContentType: "image/png",
		Width:       64,
		Height:      64,
		Method:      "crop",
		Animated:    false,
		SizeBytes:   1,
		CreationTs:  creationTs,
	}
}

func TestPurgeThumbnailBatchRemovesObjectBeforeDeletingUnreferencedRows(t *testing.T) {
	oldOne := testThumbnail("old-one", 10, "ds1", "shared-loc")
	oldTwo := testThumbnail("old-two", 20, "ds1", "shared-loc")
	thumbsDb := &fakeThumbnailStore{byLocation: map[thumbnailLocation][]*database.DbThumbnail{
		{datastoreId: "ds1", location: "shared-loc"}: {oldOne, oldTwo},
	}}
	mediaDb := &fakeMediaStore{exists: map[thumbnailLocation]bool{}}
	removed := make([]thumbnailLocation, 0)

	deleted := purgeThumbnailBatch(rcontext.Initial(), thumbsDb, mediaDb, func(ctx rcontext.RequestContext, datastoreId string, location string) error {
		require.Empty(t, thumbsDb.deleted, "datastore object should be removed before DB references are deleted")
		removed = append(removed, thumbnailLocation{datastoreId: datastoreId, location: location})
		return nil
	}, []*database.DbThumbnail{oldOne, oldTwo}, make(map[thumbnailLocation]bool), 100)

	assert.Equal(t, 2, deleted)
	assert.Equal(t, []thumbnailLocation{{datastoreId: "ds1", location: "shared-loc"}}, removed)
	assert.ElementsMatch(t, []*database.DbThumbnail{oldOne, oldTwo}, thumbsDb.deleted)
}

func TestPurgeThumbnailBatchKeepsRowsForRetryWhenObjectDeleteFails(t *testing.T) {
	oldThumb := testThumbnail("old", 10, "ds1", "loc")
	thumbsDb := &fakeThumbnailStore{byLocation: map[thumbnailLocation][]*database.DbThumbnail{
		{datastoreId: "ds1", location: "loc"}: {oldThumb},
	}}
	mediaDb := &fakeMediaStore{exists: map[thumbnailLocation]bool{}}

	deleted := purgeThumbnailBatch(rcontext.Initial(), thumbsDb, mediaDb, func(ctx rcontext.RequestContext, datastoreId string, location string) error {
		return errors.New("remove failed")
	}, []*database.DbThumbnail{oldThumb}, make(map[thumbnailLocation]bool), 100)

	assert.Equal(t, 0, deleted)
	assert.Empty(t, thumbsDb.deleted)
}

func TestPurgeThumbnailBatchDeletesRowsWithoutRemovingSharedLiveObject(t *testing.T) {
	oldThumb := testThumbnail("old", 10, "ds1", "loc")
	newThumb := testThumbnail("new", 200, "ds1", "loc")
	thumbsDb := &fakeThumbnailStore{byLocation: map[thumbnailLocation][]*database.DbThumbnail{
		{datastoreId: "ds1", location: "loc"}: {oldThumb, newThumb},
	}}
	mediaDb := &fakeMediaStore{exists: map[thumbnailLocation]bool{}}
	removeCalled := false

	deleted := purgeThumbnailBatch(rcontext.Initial(), thumbsDb, mediaDb, func(ctx rcontext.RequestContext, datastoreId string, location string) error {
		removeCalled = true
		return nil
	}, []*database.DbThumbnail{oldThumb}, make(map[thumbnailLocation]bool), 100)

	assert.Equal(t, 1, deleted)
	assert.False(t, removeCalled)
	assert.Equal(t, []*database.DbThumbnail{oldThumb}, thumbsDb.deleted)
}

func TestPurgeThumbnailBatchDeletesRowsWithoutRemovingMediaObject(t *testing.T) {
	oldThumb := testThumbnail("old", 10, "ds1", "loc")
	thumbsDb := &fakeThumbnailStore{byLocation: map[thumbnailLocation][]*database.DbThumbnail{
		{datastoreId: "ds1", location: "loc"}: {oldThumb},
	}}
	mediaDb := &fakeMediaStore{exists: map[thumbnailLocation]bool{
		{datastoreId: "ds1", location: "loc"}: true,
	}}
	removeCalled := false

	deleted := purgeThumbnailBatch(rcontext.Initial(), thumbsDb, mediaDb, func(ctx rcontext.RequestContext, datastoreId string, location string) error {
		removeCalled = true
		return nil
	}, []*database.DbThumbnail{oldThumb}, make(map[thumbnailLocation]bool), 100)

	assert.Equal(t, 1, deleted)
	assert.False(t, removeCalled)
	assert.Equal(t, []*database.DbThumbnail{oldThumb}, thumbsDb.deleted)
}
