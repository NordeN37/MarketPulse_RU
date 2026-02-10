BEGIN;

-- Extract first HTTP(S) URL from Telegram news content into the url column.
-- Only updates rows where url is currently empty/null.
UPDATE news
SET url = substring(content FROM 'https?://[^\s<>"''\)\]]+')
WHERE source = 'telegram'
  AND (url IS NULL OR url = '')
  AND content ~ 'https?://';

COMMIT;
