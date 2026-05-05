DROP INDEX IF EXISTS url_previews_index;
DELETE FROM url_previews
WHERE ctid IN (
	SELECT ctid
	FROM (
		SELECT ctid,
		       ROW_NUMBER() OVER (
			       PARTITION BY url, error_code, bucket_ts
			       ORDER BY ctid
	       ) AS row_num
		FROM url_previews
	) ranked
	WHERE ranked.row_num > 1
);
CREATE UNIQUE INDEX IF NOT EXISTS url_previews_index ON url_previews (url, error_code, bucket_ts);
ALTER TABLE url_previews ALTER COLUMN language_header DROP NOT NULL;
ALTER TABLE url_previews ALTER COLUMN language_header DROP DEFAULT;
