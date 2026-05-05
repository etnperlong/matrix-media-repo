UPDATE url_previews SET language_header = '' WHERE language_header IS NULL;
DELETE FROM url_previews
WHERE ctid IN (
	SELECT ctid
	FROM (
		SELECT ctid,
		       ROW_NUMBER() OVER (
			       PARTITION BY url, bucket_ts, language_header
			       ORDER BY CASE WHEN error_code = '' THEN 0 ELSE 1 END, ctid
	       ) AS row_num
		FROM url_previews
	) ranked
	WHERE ranked.row_num > 1
);
ALTER TABLE url_previews ALTER COLUMN language_header SET DEFAULT '';
ALTER TABLE url_previews ALTER COLUMN language_header SET NOT NULL;
DROP INDEX IF EXISTS url_previews_index;
CREATE UNIQUE INDEX IF NOT EXISTS url_previews_index ON url_previews (url, bucket_ts, language_header);
