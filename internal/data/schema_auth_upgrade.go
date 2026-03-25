package data

import (
	"context"
	"database/sql"
	"fmt"
)

func (s *Store) upgradeAuthSchema(ctx context.Context) error {
	columnStatements := map[string]string{
		"DISPLAY_NAME_":  `ALTER TABLE auth_user ADD COLUMN DISPLAY_NAME_ VARCHAR(255) NOT NULL DEFAULT '' AFTER USERNAME_`,
		"EMAIL_":         `ALTER TABLE auth_user ADD COLUMN EMAIL_ VARCHAR(255) NOT NULL DEFAULT '' AFTER DISPLAY_NAME_`,
		"AVATAR_URL_":    `ALTER TABLE auth_user ADD COLUMN AVATAR_URL_ VARCHAR(1024) NOT NULL DEFAULT '' AFTER EMAIL_`,
		"AUTH_PROVIDER_": `ALTER TABLE auth_user ADD COLUMN AUTH_PROVIDER_ VARCHAR(32) NOT NULL DEFAULT 'mock' AFTER AVATAR_URL_`,
		"AUTH_SUB_":      `ALTER TABLE auth_user ADD COLUMN AUTH_SUB_ VARCHAR(255) NULL AFTER AUTH_PROVIDER_`,
		"UPDATED_AT_":    `ALTER TABLE auth_user ADD COLUMN UPDATED_AT_ DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) AFTER CREATED_AT_`,
		"LAST_LOGIN_AT_": `ALTER TABLE auth_user ADD COLUMN LAST_LOGIN_AT_ DATETIME(3) NULL AFTER UPDATED_AT_`,
	}

	for column, statement := range columnStatements {
		if err := s.ensureColumn(ctx, "auth_user", column, statement); err != nil {
			return err
		}
	}

	if _, err := s.db.ExecContext(ctx, `
		UPDATE auth_user
		SET DISPLAY_NAME_ = CASE WHEN DISPLAY_NAME_ = '' THEN USERNAME_ ELSE DISPLAY_NAME_ END,
			AUTH_PROVIDER_ = CASE WHEN AUTH_PROVIDER_ = '' THEN 'mock' ELSE AUTH_PROVIDER_ END,
			UPDATED_AT_ = COALESCE(UPDATED_AT_, CREATED_AT_),
			LAST_LOGIN_AT_ = COALESCE(LAST_LOGIN_AT_, CREATED_AT_)`); err != nil {
		return fmt.Errorf("backfill auth_user columns: %w", err)
	}

	if err := s.ensureIndex(ctx, "auth_user", "UK_AUTH_USER_PROVIDER_SUB", `CREATE UNIQUE INDEX UK_AUTH_USER_PROVIDER_SUB ON auth_user (AUTH_PROVIDER_, AUTH_SUB_)`); err != nil {
		return err
	}
	if err := s.ensureTable(ctx, "auth_session", `CREATE TABLE auth_session (
		ID_ BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
		USER_ID_ BIGINT UNSIGNED NOT NULL,
		TOKEN_HASH_ CHAR(64) NOT NULL,
		EXPIRES_AT_ DATETIME(3) NOT NULL,
		CREATED_AT_ DATETIME(3) NOT NULL,
		LAST_SEEN_AT_ DATETIME(3) NOT NULL,
		USER_AGENT_ VARCHAR(512) NOT NULL DEFAULT '',
		IP_ VARCHAR(64) NOT NULL DEFAULT '',
		PRIMARY KEY (ID_),
		UNIQUE KEY UK_AUTH_SESSION_TOKEN_HASH (TOKEN_HASH_),
		KEY IDX_AUTH_SESSION_USER_EXPIRES (USER_ID_, EXPIRES_AT_),
		CONSTRAINT FK_AUTH_SESSION_USER FOREIGN KEY (USER_ID_) REFERENCES auth_user (ID_) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		return err
	}
	if err := s.ensureTable(ctx, "auth_local_credential", `CREATE TABLE auth_local_credential (
		USER_ID_ BIGINT UNSIGNED NOT NULL,
		PASSWORD_HASH_ VARCHAR(255) NOT NULL,
		PASSWORD_UPDATED_AT_ DATETIME(3) NOT NULL,
		CREATED_AT_ DATETIME(3) NOT NULL,
		PRIMARY KEY (USER_ID_),
		CONSTRAINT FK_AUTH_LOCAL_CREDENTIAL_USER FOREIGN KEY (USER_ID_) REFERENCES auth_user (ID_) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		return err
	}
	if err := s.ensureTable(ctx, "auth_login_log", `CREATE TABLE auth_login_log (
		ID_ BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
		USER_ID_ BIGINT UNSIGNED NULL,
		USERNAME_ VARCHAR(128) NOT NULL DEFAULT '',
		DISPLAY_NAME_ VARCHAR(255) NOT NULL DEFAULT '',
		AUTH_METHOD_ VARCHAR(32) NOT NULL,
		LOGIN_RESULT_ VARCHAR(16) NOT NULL,
		FAILURE_REASON_ VARCHAR(128) NOT NULL DEFAULT '',
		IP_ VARCHAR(64) NOT NULL DEFAULT '',
		USER_AGENT_ VARCHAR(512) NOT NULL DEFAULT '',
		LOGIN_AT_ DATETIME(3) NOT NULL,
		PRIMARY KEY (ID_),
		KEY IDX_AUTH_LOGIN_LOG_USER_AT (USER_ID_, LOGIN_AT_),
		KEY IDX_AUTH_LOGIN_LOG_METHOD_AT (AUTH_METHOD_, LOGIN_AT_),
		CONSTRAINT FK_AUTH_LOGIN_LOG_USER FOREIGN KEY (USER_ID_) REFERENCES auth_user (ID_) ON DELETE SET NULL
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		return err
	}

	return nil
}

func (s *Store) ensureTable(ctx context.Context, tableName, createStatement string) error {
	exists, err := s.hasTable(ctx, tableName)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, createStatement); err != nil {
		return fmt.Errorf("create table %s: %w", tableName, err)
	}
	return nil
}

func (s *Store) ensureColumn(ctx context.Context, tableName, columnName, alterStatement string) error {
	exists, err := s.hasColumn(ctx, tableName, columnName)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, alterStatement); err != nil {
		return fmt.Errorf("add column %s.%s: %w", tableName, columnName, err)
	}
	return nil
}

func (s *Store) ensureIndex(ctx context.Context, tableName, indexName, createStatement string) error {
	exists, err := s.hasIndex(ctx, tableName, indexName)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, createStatement); err != nil {
		return fmt.Errorf("create index %s.%s: %w", tableName, indexName, err)
	}
	return nil
}

func (s *Store) hasTable(ctx context.Context, tableName string) (bool, error) {
	var name string
	err := s.db.QueryRowContext(ctx, `
		SELECT TABLE_NAME
		FROM information_schema.tables
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?`,
		tableName,
	).Scan(&name)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check table %s: %w", tableName, err)
	}
	return true, nil
}

func (s *Store) hasColumn(ctx context.Context, tableName, columnName string) (bool, error) {
	var name string
	err := s.db.QueryRowContext(ctx, `
		SELECT COLUMN_NAME
		FROM information_schema.columns
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`,
		tableName,
		columnName,
	).Scan(&name)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check column %s.%s: %w", tableName, columnName, err)
	}
	return true, nil
}

func (s *Store) hasIndex(ctx context.Context, tableName, indexName string) (bool, error) {
	var name string
	err := s.db.QueryRowContext(ctx, `
		SELECT INDEX_NAME
		FROM information_schema.statistics
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ?
		LIMIT 1`,
		tableName,
		indexName,
	).Scan(&name)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check index %s.%s: %w", tableName, indexName, err)
	}
	return true, nil
}
