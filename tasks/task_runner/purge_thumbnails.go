package task_runner

import (
	"fmt"

	"github.com/getsentry/sentry-go"
	"github.com/t2bot/matrix-media-repo/common/config"
	"github.com/t2bot/matrix-media-repo/common/rcontext"
	"github.com/t2bot/matrix-media-repo/database"
	"github.com/t2bot/matrix-media-repo/datastores"
	"github.com/t2bot/matrix-media-repo/util"
)

const thumbnailPurgeBatchSize = 1000

type thumbnailLocation struct {
	datastoreId string
	location    string
}

type thumbnailStore interface {
	GetByLocation(datastoreId string, location string) ([]*database.DbThumbnail, error)
	Delete(record *database.DbThumbnail) error
}

type mediaStore interface {
	LocationExists(datastoreId string, location string) (bool, error)
}

type thumbnailObjectRemover func(ctx rcontext.RequestContext, datastoreId string, location string) error

func PurgeThumbnails(ctx rcontext.RequestContext) {
	// dev note: don't use ctx for config lookup to avoid misreading it

	if config.Get().Thumbnails.ExpireDays <= 0 {
		return
	}

	beforeTs := util.NowMillis() - int64(config.Get().Thumbnails.ExpireDays*24*60*60*1000)
	thumbsDb := database.GetInstance().Thumbnails.Prepare(ctx)
	deletedLocations := make(map[thumbnailLocation]bool)
	var after *database.DbThumbnail
	for {
		old, err := thumbsDb.GetOlderThanBatchAfter(beforeTs, thumbnailPurgeBatchSize, after)
		if err != nil {
			ctx.Log.Error("Error deleting thumbnails: ", err)
			sentry.CaptureException(err)
			return
		}
		if len(old) == 0 {
			return
		}

		deleted := doPurgeThumbnails(ctx, old, deletedLocations, beforeTs)
		if deleted == 0 {
			ctx.Log.Warn("Thumbnail purge made no progress in this batch; advancing to avoid retrying the same rows forever")
			after = old[len(old)-1]
			continue
		}
		after = nil
	}
}

func doPurgeThumbnails(ctx rcontext.RequestContext, thumbs []*database.DbThumbnail, deletedLocations map[thumbnailLocation]bool, beforeTs int64) int {
	thumbsDb := database.GetInstance().Thumbnails.Prepare(ctx)
	mediaDb := database.GetInstance().Media.Prepare(ctx)
	return purgeThumbnailBatch(ctx, thumbsDb, mediaDb, datastores.RemoveWithDsId, thumbs, deletedLocations, beforeTs)
}

func purgeThumbnailBatch(ctx rcontext.RequestContext, thumbsDb thumbnailStore, mediaDb mediaStore, removeObject thumbnailObjectRemover, thumbs []*database.DbThumbnail, deletedLocations map[thumbnailLocation]bool, beforeTs int64) int {
	deletedCount := 0
	touchedLocations := make(map[thumbnailLocation]*database.DbThumbnail)
	for _, thumb := range thumbs {
		loc := thumbnailLocation{datastoreId: thumb.DatastoreId, location: thumb.Location}
		if _, ok := touchedLocations[loc]; !ok {
			touchedLocations[loc] = thumb
		}
	}

	for loc, thumb := range touchedLocations {
		if _, ok := deletedLocations[loc]; ok {
			continue
		}

		mediaExists, err := mediaDb.LocationExists(thumb.DatastoreId, thumb.Location)
		if err != nil {
			ctx.Log.Error("Error checking for conflicting media: ", err)
			sentry.CaptureException(err)
			continue
		}
		locationThumbs, err := thumbsDb.GetByLocation(thumb.DatastoreId, thumb.Location)
		if err != nil {
			ctx.Log.Error("Error checking for thumbnail location references: ", err)
			sentry.CaptureException(err)
			continue
		}

		oldThumbs := make([]*database.DbThumbnail, 0)
		hasNewerThumb := false
		for _, locationThumb := range locationThumbs {
			if locationThumb.CreationTs < beforeTs {
				oldThumbs = append(oldThumbs, locationThumb)
			} else {
				hasNewerThumb = true
			}
		}
		if len(oldThumbs) == 0 {
			continue
		}

		removeDatastoreObject := !mediaExists && !hasNewerThumb
		if removeDatastoreObject {
			ctx.Log.Debugf("Trying to remove datastore object for %s/%s", loc.datastoreId, loc.location)
			if err = removeObject(ctx, thumb.DatastoreId, thumb.Location); err != nil {
				ctx.Log.Error("Error deleting thumbnail from datastore: ", err)
				sentry.CaptureException(err)
				continue
			}
			deletedLocations[loc] = true
		}

		for _, oldThumb := range oldThumbs {
			mxc := fmt.Sprintf("%s?w=%d&h=%d&m=%s&a=%t", util.MxcUri(oldThumb.Origin, oldThumb.MediaId), oldThumb.Width, oldThumb.Height, oldThumb.Method, oldThumb.Animated)
			ctx.Log.Debugf("Trying to delete database record for %s", mxc)
			if err = thumbsDb.Delete(oldThumb); err != nil {
				ctx.Log.Error("Error deleting thumbnail record: ", err)
				sentry.CaptureException(err)
				continue
			}
			deletedCount++
		}
	}

	return deletedCount
}
