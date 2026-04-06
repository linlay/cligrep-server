USE `cligrep`;

SET @ddl = IF(
  EXISTS(
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'cli_registry'
      AND column_name = 'OFFICIAL_URL_'
  ),
  'SELECT 1',
  'ALTER TABLE cli_registry ADD COLUMN OFFICIAL_URL_ VARCHAR(512) NOT NULL DEFAULT '''' AFTER AUTHOR_'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @ddl = IF(
  EXISTS(
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'cli_registry'
      AND column_name = 'GITHUB_URL_'
  ),
  'UPDATE cli_registry SET OFFICIAL_URL_ = CASE WHEN OFFICIAL_URL_ = '''' THEN GITHUB_URL_ ELSE OFFICIAL_URL_ END',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @ddl = IF(
  EXISTS(
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'cli_registry'
      AND column_name = 'GITHUB_URL_'
  ),
  'ALTER TABLE cli_registry DROP COLUMN GITHUB_URL_',
  'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
