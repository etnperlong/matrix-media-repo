package database

import (
	"database/sql"
	"errors"

	"github.com/t2bot/matrix-media-repo/common/rcontext"
	"github.com/t2bot/matrix-media-repo/util"
)

type DbUrlPreview struct {
	Url            string
	ErrorCode      string
	BucketTs       int64
	SiteUrl        string
	SiteName       string
	ResourceType   string
	Description    string
	Title          string
	ImageMxc       string
	ImageType      string
	ImageSize      int64
	ImageWidth     int
	ImageHeight    int
	LanguageHeader string
}

const selectUrlPreview = "SELECT url, error_code, bucket_ts, site_url, site_name, resource_type, description, title, image_mxc, image_type, image_size, image_width, image_height, language_header FROM url_previews WHERE url = $1 AND bucket_ts = $2 AND COALESCE(language_header, '') = $3 LIMIT 1;"
const insertUrlPreview = "INSERT INTO url_previews (url, error_code, bucket_ts, site_url, site_name, resource_type, description, title, image_mxc, image_type, image_size, image_width, image_height, language_header) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) ON CONFLICT (url, bucket_ts, language_header) DO NOTHING;"
const updateUrlPreviewIfError = "UPDATE url_previews SET error_code = $4, site_url = $5, site_name = $6, resource_type = $7, description = $8, title = $9, image_mxc = $10, image_type = $11, image_size = $12, image_width = $13, image_height = $14, language_header = $3 WHERE url = $1 AND bucket_ts = $2 AND COALESCE(language_header, '') = $3 AND error_code <> '';"
const deleteOldUrlPreviews = "DELETE FROM url_previews WHERE bucket_ts <= $1;"

type urlPreviewsTableStatements struct {
	selectUrlPreview        *sql.Stmt
	insertUrlPreview        *sql.Stmt
	updateUrlPreviewIfError *sql.Stmt
	deleteOldUrlPreviews    *sql.Stmt
}

type urlPreviewsTableWithContext struct {
	statements *urlPreviewsTableStatements
	ctx        rcontext.RequestContext
}

func prepareUrlPreviewsTables(db *sql.DB) (*urlPreviewsTableStatements, error) {
	var err error
	var stmts = &urlPreviewsTableStatements{}

	if stmts.selectUrlPreview, err = db.Prepare(selectUrlPreview); err != nil {
		return nil, errors.New("error preparing selectUrlPreview: " + err.Error())
	}
	if stmts.insertUrlPreview, err = db.Prepare(insertUrlPreview); err != nil {
		return nil, errors.New("error preparing insertUrlPreview: " + err.Error())
	}
	if stmts.updateUrlPreviewIfError, err = db.Prepare(updateUrlPreviewIfError); err != nil {
		return nil, errors.New("error preparing updateUrlPreviewIfError: " + err.Error())
	}
	if stmts.deleteOldUrlPreviews, err = db.Prepare(deleteOldUrlPreviews); err != nil {
		return nil, errors.New("error preparing deleteOldUrlPreviews: " + err.Error())
	}

	return stmts, nil
}

func (s *urlPreviewsTableStatements) Prepare(ctx rcontext.RequestContext) *urlPreviewsTableWithContext {
	return &urlPreviewsTableWithContext{
		statements: s,
		ctx:        ctx,
	}
}

func (s *urlPreviewsTableWithContext) Get(url string, ts int64, languageHeader string) (*DbUrlPreview, error) {
	row := s.statements.selectUrlPreview.QueryRowContext(s.ctx, url, util.GetHourBucket(ts), languageHeader)
	val := &DbUrlPreview{}
	err := row.Scan(&val.Url, &val.ErrorCode, &val.BucketTs, &val.SiteUrl, &val.SiteName, &val.ResourceType, &val.Description, &val.Title, &val.ImageMxc, &val.ImageType, &val.ImageSize, &val.ImageWidth, &val.ImageHeight, &val.LanguageHeader)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return val, err
}

func (s *urlPreviewsTableWithContext) Insert(p *DbUrlPreview) (bool, error) {
	r, err := s.statements.insertUrlPreview.ExecContext(s.ctx, p.Url, p.ErrorCode, p.BucketTs, p.SiteUrl, p.SiteName, p.ResourceType, p.Description, p.Title, p.ImageMxc, p.ImageType, p.ImageSize, p.ImageWidth, p.ImageHeight, p.LanguageHeader)
	if err != nil {
		return false, err
	}
	affected, err := r.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (s *urlPreviewsTableWithContext) UpdateIfError(p *DbUrlPreview) (bool, error) {
	r, err := s.statements.updateUrlPreviewIfError.ExecContext(s.ctx, p.Url, p.BucketTs, p.LanguageHeader, p.ErrorCode, p.SiteUrl, p.SiteName, p.ResourceType, p.Description, p.Title, p.ImageMxc, p.ImageType, p.ImageSize, p.ImageWidth, p.ImageHeight)
	if err != nil {
		return false, err
	}
	affected, err := r.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (s *urlPreviewsTableWithContext) InsertError(url string, errorCode string, ts int64, languageHeader string) {
	_, _ = s.Insert(&DbUrlPreview{
		Url:            url,
		ErrorCode:      errorCode,
		BucketTs:       ts,
		LanguageHeader: languageHeader,
		// remainder of fields don't matter
	})
}

func (s *urlPreviewsTableWithContext) DeleteOlderThan(ts int64) error {
	_, err := s.statements.deleteOldUrlPreviews.ExecContext(s.ctx, ts)
	return err
}
