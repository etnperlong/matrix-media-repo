package url_preview

import (
	"errors"

	"github.com/getsentry/sentry-go"
	"github.com/t2bot/matrix-media-repo/common"
	"github.com/t2bot/matrix-media-repo/common/rcontext"
	"github.com/t2bot/matrix-media-repo/database"
	"github.com/t2bot/matrix-media-repo/datastores"
	"github.com/t2bot/matrix-media-repo/url_previewing/m"
)

func Process(ctx rcontext.RequestContext, previewUrl string, preview m.PreviewResult, err error, onHost string, userId string, languageHeader string, ts int64) (*database.DbUrlPreview, error) {
	previewDb := database.GetInstance().UrlPreviews.Prepare(ctx)

	if err != nil {
		if errors.Is(err, m.ErrPreviewUnsupported) {
			err = common.ErrMediaNotFound
		}

		if errors.Is(err, common.ErrMediaNotFound) {
			previewDb.InsertError(previewUrl, common.ErrCodeNotFound, ts, languageHeader)
		} else {
			previewDb.InsertError(previewUrl, common.ErrCodeUnknown, ts, languageHeader)
		}
		return nil, err
	} else {
		result := &database.DbUrlPreview{
			Url:            previewUrl,
			ErrorCode:      "",
			BucketTs:       ts, // already bucketed
			SiteUrl:        preview.Url,
			SiteName:       preview.SiteName,
			ResourceType:   preview.Type,
			Description:    preview.Description,
			Title:          preview.Title,
			LanguageHeader: languageHeader,
		}

		// Step 7: Store the thumbnail, if needed
		uploadedMedia := UploadImage(ctx, preview.Image, onHost, userId, result)

		// Step 8: Insert the record
		inserted, err := previewDb.Insert(result)
		if err != nil {
			ctx.Log.Warn("Non-fatal error caching URL preview: ", err)
			sentry.CaptureException(err)
			return result, nil
		}
		if inserted {
			return result, nil
		}

		existing, getErr := previewDb.Get(previewUrl, ts, languageHeader)
		if getErr != nil {
			ctx.Log.Warn("Non-fatal error reading existing URL preview after cache conflict: ", getErr)
			sentry.CaptureException(getErr)
			return result, nil
		}

		if existing != nil && existing.ErrorCode != "" {
			updated, updateErr := previewDb.UpdateIfError(result)
			if updateErr != nil {
				ctx.Log.Warn("Non-fatal error upgrading URL preview cache entry after conflict: ", updateErr)
				sentry.CaptureException(updateErr)
			} else if updated {
				return result, nil
			}

			existing, getErr = previewDb.Get(previewUrl, ts, languageHeader)
			if getErr != nil {
				ctx.Log.Warn("Non-fatal error re-reading upgraded URL preview cache entry: ", getErr)
				sentry.CaptureException(getErr)
				return result, nil
			}
		}

		if uploadedMedia != nil && (existing == nil || existing.ImageMxc != result.ImageMxc) {
			if cleanupErr := cleanupUnusedPreviewMedia(ctx, uploadedMedia); cleanupErr != nil {
				ctx.Log.Warn("Non-fatal error cleaning up unused URL preview media after cache conflict: ", cleanupErr)
				sentry.CaptureException(cleanupErr)
			}
		}

		if existing != nil {
			return existing, nil
		}

		return result, nil
	}
}

func cleanupUnusedPreviewMedia(ctx rcontext.RequestContext, record *database.DbMedia) error {
	mediaDb := database.GetInstance().Media.Prepare(ctx)
	if err := database.GetInstance().RestrictedMedia.Prepare(ctx).Delete(record.Origin, record.MediaId); err != nil {
		return err
	}
	if err := mediaDb.Delete(record.Origin, record.MediaId); err != nil {
		return err
	}

	exists, err := mediaDb.LocationExists(record.DatastoreId, record.Location)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	return datastores.RemoveWithDsId(ctx, record.DatastoreId, record.Location)
}
