package data

import (
	"context"
	"database/sql"
	"fmt"
)

func (s *Store) upgradeReleaseSchema(ctx context.Context) error {
	if err := s.ensureTable(ctx, "cli_release", `CREATE TABLE cli_release (
		ID_ BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
		CLI_SLUG_ VARCHAR(128) NOT NULL,
		VERSION_ VARCHAR(128) NOT NULL,
		PUBLISHED_AT_ DATETIME(3) NOT NULL,
		IS_CURRENT_ TINYINT(1) NOT NULL DEFAULT 0,
		SOURCE_KIND_ VARCHAR(64) NOT NULL DEFAULT '',
		SOURCE_URL_ VARCHAR(1024) NOT NULL DEFAULT '',
		CREATED_AT_ DATETIME(3) NOT NULL,
		UPDATED_AT_ DATETIME(3) NOT NULL,
		PRIMARY KEY (ID_),
		UNIQUE KEY UK_CLI_RELEASE_SLUG_VERSION (CLI_SLUG_, VERSION_),
		KEY IDX_CLI_RELEASE_SLUG_PUBLISHED (CLI_SLUG_, PUBLISHED_AT_),
		KEY IDX_CLI_RELEASE_SLUG_CURRENT (CLI_SLUG_, IS_CURRENT_),
		CONSTRAINT FK_CLI_RELEASE_CLI FOREIGN KEY (CLI_SLUG_) REFERENCES cli_registry (SLUG_) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		return err
	}
	if err := s.ensureTable(ctx, "cli_release_asset", `CREATE TABLE cli_release_asset (
		ID_ BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
		RELEASE_ID_ BIGINT UNSIGNED NOT NULL,
		FILE_NAME_ VARCHAR(255) NOT NULL,
		DOWNLOAD_URL_ VARCHAR(1024) NOT NULL,
		OS_ VARCHAR(64) NOT NULL DEFAULT '',
		ARCH_ VARCHAR(64) NOT NULL DEFAULT '',
		PACKAGE_KIND_ VARCHAR(64) NOT NULL DEFAULT '',
		CHECKSUM_URL_ VARCHAR(1024) NOT NULL DEFAULT '',
		SIZE_BYTES_ BIGINT NOT NULL DEFAULT 0,
		CREATED_AT_ DATETIME(3) NOT NULL,
		PRIMARY KEY (ID_),
		KEY IDX_CLI_RELEASE_ASSET_RELEASE (RELEASE_ID_),
		CONSTRAINT FK_CLI_RELEASE_ASSET_RELEASE FOREIGN KEY (RELEASE_ID_) REFERENCES cli_release (ID_) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		return err
	}

	indexStatements := map[string]string{
		"UK_CLI_RELEASE_SLUG_VERSION":    `CREATE UNIQUE INDEX UK_CLI_RELEASE_SLUG_VERSION ON cli_release (CLI_SLUG_, VERSION_)`,
		"IDX_CLI_RELEASE_SLUG_PUBLISHED": `CREATE INDEX IDX_CLI_RELEASE_SLUG_PUBLISHED ON cli_release (CLI_SLUG_, PUBLISHED_AT_)`,
		"IDX_CLI_RELEASE_SLUG_CURRENT":   `CREATE INDEX IDX_CLI_RELEASE_SLUG_CURRENT ON cli_release (CLI_SLUG_, IS_CURRENT_)`,
	}
	for indexName, statement := range indexStatements {
		if err := s.ensureIndex(ctx, "cli_release", indexName, statement); err != nil {
			return err
		}
	}
	if err := s.ensureIndex(ctx, "cli_release_asset", "IDX_CLI_RELEASE_ASSET_RELEASE", `CREATE INDEX IDX_CLI_RELEASE_ASSET_RELEASE ON cli_release_asset (RELEASE_ID_)`); err != nil {
		return err
	}

	return s.ensureReleaseForeignKeys(ctx)
}

func (s *Store) ensureReleaseForeignKeys(ctx context.Context) error {
	if err := s.ensureForeignKey(ctx, "cli_release", "FK_CLI_RELEASE_CLI", `ALTER TABLE cli_release ADD CONSTRAINT FK_CLI_RELEASE_CLI FOREIGN KEY (CLI_SLUG_) REFERENCES cli_registry (SLUG_) ON DELETE CASCADE`); err != nil {
		return err
	}
	if err := s.ensureForeignKey(ctx, "cli_release_asset", "FK_CLI_RELEASE_ASSET_RELEASE", `ALTER TABLE cli_release_asset ADD CONSTRAINT FK_CLI_RELEASE_ASSET_RELEASE FOREIGN KEY (RELEASE_ID_) REFERENCES cli_release (ID_) ON DELETE CASCADE`); err != nil {
		return err
	}
	return nil
}

func (s *Store) ensureForeignKey(ctx context.Context, tableName, constraintName, alterStatement string) error {
	exists, err := s.hasForeignKey(ctx, tableName, constraintName)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, alterStatement); err != nil {
		return fmt.Errorf("create foreign key %s.%s: %w", tableName, constraintName, err)
	}
	return nil
}

func (s *Store) hasForeignKey(ctx context.Context, tableName, constraintName string) (bool, error) {
	var name string
	err := s.db.QueryRowContext(ctx, `
		SELECT CONSTRAINT_NAME
		FROM information_schema.referential_constraints
		WHERE CONSTRAINT_SCHEMA = DATABASE() AND TABLE_NAME = ? AND CONSTRAINT_NAME = ?
		LIMIT 1`,
		tableName,
		constraintName,
	).Scan(&name)
	if err == nil {
		return true, nil
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	return false, fmt.Errorf("check foreign key %s.%s: %w", tableName, constraintName, err)
}
