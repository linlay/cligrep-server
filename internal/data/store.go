package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/linlay/cligrep-server/internal/config"
	"github.com/linlay/cligrep-server/internal/models"
)

type Store struct {
	db *sql.DB
}

const cliSelectList = `
	SELECT c.SLUG_, c.DISPLAY_NAME_, c.SUMMARY_, c.TYPE_, c.TAGS_JSON_, c.HELP_TEXT_, c.VERSION_TEXT_,
	       c.POPULARITY_, c.RUNTIME_IMAGE_, c.ENABLED_, c.EXAMPLE_LINE_,
	       c.ENVIRONMENT_KIND_, c.SOURCE_TYPE_, c.AUTHOR_, c.GITHUB_URL_, c.GITEE_URL_,
	       c.LICENSE_, c.CREATED_AT_, c.ORIGINAL_COMMAND_, c.EXECUTABLE_,
	       (SELECT COUNT(*) FROM user_favorite f WHERE f.CLI_SLUG_ = c.SLUG_) AS FAVORITE_COUNT_,
	       (SELECT COUNT(*) FROM user_comment m WHERE m.CLI_SLUG_ = c.SLUG_) AS COMMENT_COUNT_,
	       (SELECT COUNT(*) FROM sandbox_execution_log e WHERE e.CLI_SLUG_ = c.SLUG_) AS RUN_COUNT_`

func Open(ctx context.Context, cfg config.Config) (*Store, error) {
	if err := ensureDatabase(ctx, cfg); err != nil {
		return nil, err
	}

	db, err := sql.Open("mysql", mysqlDSN(cfg, true))
	if err != nil {
		return nil, fmt.Errorf("open mysql database: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping mysql database: %w", err)
	}

	store := &Store{db: db}
	if err := store.init(ctx); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) init(ctx context.Context) error {
	for _, statement := range mysqlSchemaStatements() {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize mysql schema: %w", err)
		}
	}
	if err := s.upgradeAuthSchema(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Store) SeedCLIs(ctx context.Context, clis []models.CLI) error {
	query := `INSERT INTO cli_registry
		(SLUG_, DISPLAY_NAME_, SUMMARY_, TYPE_, TAGS_JSON_, HELP_TEXT_, VERSION_TEXT_, POPULARITY_, RUNTIME_IMAGE_, ENABLED_, EXAMPLE_LINE_,
		 ENVIRONMENT_KIND_, SOURCE_TYPE_, AUTHOR_, GITHUB_URL_, GITEE_URL_, LICENSE_, CREATED_AT_, ORIGINAL_COMMAND_, EXECUTABLE_)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			DISPLAY_NAME_ = VALUES(DISPLAY_NAME_),
			SUMMARY_ = VALUES(SUMMARY_),
			TYPE_ = VALUES(TYPE_),
			TAGS_JSON_ = VALUES(TAGS_JSON_),
			HELP_TEXT_ = VALUES(HELP_TEXT_),
			VERSION_TEXT_ = VALUES(VERSION_TEXT_),
			POPULARITY_ = VALUES(POPULARITY_),
			RUNTIME_IMAGE_ = VALUES(RUNTIME_IMAGE_),
			ENABLED_ = VALUES(ENABLED_),
			EXAMPLE_LINE_ = VALUES(EXAMPLE_LINE_),
			ENVIRONMENT_KIND_ = VALUES(ENVIRONMENT_KIND_),
			SOURCE_TYPE_ = VALUES(SOURCE_TYPE_),
			AUTHOR_ = VALUES(AUTHOR_),
			GITHUB_URL_ = VALUES(GITHUB_URL_),
			GITEE_URL_ = VALUES(GITEE_URL_),
			LICENSE_ = VALUES(LICENSE_),
			CREATED_AT_ = VALUES(CREATED_AT_),
			ORIGINAL_COMMAND_ = VALUES(ORIGINAL_COMMAND_),
			EXECUTABLE_ = VALUES(EXECUTABLE_)`

	for _, cli := range clis {
		tagsJSON, err := json.Marshal(cli.Tags)
		if err != nil {
			return fmt.Errorf("marshal cli tags: %w", err)
		}

		if _, err := s.db.ExecContext(ctx, query,
			cli.Slug,
			cli.DisplayName,
			cli.Summary,
			cli.Type,
			string(tagsJSON),
			cli.HelpText,
			cli.VersionText,
			cli.Popularity,
			cli.RuntimeImage,
			cli.Enabled,
			cli.ExampleLine,
			cli.EnvironmentKind,
			cli.SourceType,
			cli.Author,
			cli.GitHubURL,
			cli.GiteeURL,
			cli.License,
			cli.CreatedAt.UTC(),
			cli.OriginalCommand,
			cli.Executable,
		); err != nil {
			return fmt.Errorf("seed cli %s: %w", cli.Slug, err)
		}
	}

	return nil
}

func (s *Store) ListHomepageCLIs(ctx context.Context, sort string, limit int) ([]models.CLI, int, error) {
	if limit <= 0 {
		limit = 12
	}

	total, err := s.CountHomepageCLIs(ctx)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf(`%s
		FROM cli_registry c
		WHERE c.ENABLED_ = 1 AND c.ENVIRONMENT_KIND_ != 'WEBSITE'
		ORDER BY %s
		LIMIT ?`, cliSelectList, homepageSortOrder(sort)),
		limit,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query homepage clis: %w", err)
	}
	defer rows.Close()

	clis, err := scanCLIs(rows)
	if err != nil {
		return nil, 0, err
	}
	return clis, total, nil
}

func (s *Store) CountHomepageCLIs(ctx context.Context) (int, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM cli_registry
		WHERE ENABLED_ = 1 AND ENVIRONMENT_KIND_ != 'WEBSITE'`)
	var total int
	if err := row.Scan(&total); err != nil {
		return 0, fmt.Errorf("count homepage clis: %w", err)
	}
	return total, nil
}

func (s *Store) SearchCLIs(ctx context.Context, query string, limit int) ([]models.CLI, error) {
	if limit <= 0 {
		limit = 12
	}

	pattern := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`%s
		FROM cli_registry c
		WHERE c.ENABLED_ = 1
		  AND (
			  LOWER(c.SLUG_) LIKE ?
			  OR LOWER(c.DISPLAY_NAME_) LIKE ?
			  OR LOWER(c.SUMMARY_) LIKE ?
			  OR LOWER(c.HELP_TEXT_) LIKE ?
			  OR LOWER(CAST(c.TAGS_JSON_ AS CHAR)) LIKE ?
		  )
		ORDER BY
			CASE WHEN c.TYPE_ = 'builtin' THEN 0 ELSE 1 END,
			(SELECT COUNT(*) FROM user_favorite f WHERE f.CLI_SLUG_ = c.SLUG_) DESC,
			c.DISPLAY_NAME_ ASC
		LIMIT ?`, cliSelectList), pattern, pattern, pattern, pattern, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search clis: %w", err)
	}
	defer rows.Close()

	return scanCLIs(rows)
}

func (s *Store) GetCLI(ctx context.Context, slug string) (models.CLI, error) {
	row := s.db.QueryRowContext(ctx, fmt.Sprintf(`%s
		FROM cli_registry c
		WHERE c.SLUG_ = ? AND c.ENABLED_ = 1`, cliSelectList), slug)
	return scanCLI(row)
}

func (s *Store) SeedMockUsers(ctx context.Context, usernames []string) error {
	for _, username := range usernames {
		if _, err := s.LoginMock(ctx, username); err != nil {
			return fmt.Errorf("seed mock user %s: %w", username, err)
		}
	}
	return nil
}

func (s *Store) SeedFavoritesByUsername(ctx context.Context, username, cliSlug string) error {
	user, err := s.LoginMock(ctx, username)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT IGNORE INTO user_favorite (USER_ID_, CLI_SLUG_, CREATED_AT_)
		VALUES (?, ?, ?)`,
		user.ID, cliSlug, time.Now().UTC(),
	); err != nil {
		return fmt.Errorf("seed favorite %s/%s: %w", username, cliSlug, err)
	}
	return nil
}

func (s *Store) SeedExecutionLog(ctx context.Context, seedKey, cliSlug, line, mode string, durationMS int64, createdAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		INSERT IGNORE INTO seed_execution_record (SEED_KEY_, CLI_SLUG_, CREATED_AT_)
		VALUES (?, ?, ?)`,
		seedKey, cliSlug, createdAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("seed execution marker %s: %w", seedKey, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("seed execution marker rows affected: %w", err)
	}
	if rows == 0 {
		return nil
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO sandbox_execution_log (USER_ID_, CLI_SLUG_, LINE_, MODE_, STDOUT_, STDERR_, EXIT_CODE_, DURATION_MS_, CREATED_AT_)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		nil, cliSlug, line, mode, "seeded execution", "", 0, durationMS, createdAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("seed execution log %s: %w", seedKey, err)
	}
	return nil
}

func (s *Store) SetFavorite(ctx context.Context, mutation models.FavoriteMutation) error {
	if mutation.Active {
		_, err := s.db.ExecContext(ctx, `
			INSERT IGNORE INTO user_favorite (USER_ID_, CLI_SLUG_, CREATED_AT_)
			VALUES (?, ?, ?)`,
			mutation.UserID, mutation.CLISlug, time.Now().UTC(),
		)
		if err != nil {
			return fmt.Errorf("set favorite: %w", err)
		}
		return nil
	}

	if _, err := s.db.ExecContext(ctx, `DELETE FROM user_favorite WHERE USER_ID_ = ? AND CLI_SLUG_ = ?`, mutation.UserID, mutation.CLISlug); err != nil {
		return fmt.Errorf("remove favorite: %w", err)
	}
	return nil
}

func (s *Store) ListFavorites(ctx context.Context, userID int64) ([]models.CLI, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`%s
		FROM user_favorite f
		JOIN cli_registry c ON c.SLUG_ = f.CLI_SLUG_
		WHERE f.USER_ID_ = ?
		ORDER BY f.CREATED_AT_ DESC`, cliSelectList), userID)
	if err != nil {
		return nil, fmt.Errorf("list favorites: %w", err)
	}
	defer rows.Close()

	return scanCLIs(rows)
}

func (s *Store) AddComment(ctx context.Context, mutation models.CommentMutation) (models.Comment, error) {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO user_comment (CLI_SLUG_, USER_ID_, BODY_, CREATED_AT_)
		VALUES (?, ?, ?, ?)`,
		mutation.CLISlug, mutation.UserID, strings.TrimSpace(mutation.Body), time.Now().UTC(),
	)
	if err != nil {
		return models.Comment{}, fmt.Errorf("insert comment: %w", err)
	}

	commentID, err := result.LastInsertId()
	if err != nil {
		return models.Comment{}, fmt.Errorf("comment id: %w", err)
	}

	row := s.db.QueryRowContext(ctx, `
		SELECT c.ID_, c.CLI_SLUG_, c.USER_ID_, u.USERNAME_, c.BODY_, c.CREATED_AT_
		FROM user_comment c
		JOIN auth_user u ON u.ID_ = c.USER_ID_
		WHERE c.ID_ = ?`, commentID)
	return scanComment(row)
}

func (s *Store) ListComments(ctx context.Context, cliSlug string) ([]models.Comment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.ID_, c.CLI_SLUG_, c.USER_ID_, u.USERNAME_, c.BODY_, c.CREATED_AT_
		FROM user_comment c
		JOIN auth_user u ON u.ID_ = c.USER_ID_
		WHERE c.CLI_SLUG_ = ?
		ORDER BY c.CREATED_AT_ DESC
		LIMIT 24`, cliSlug)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	defer rows.Close()

	var comments []models.Comment
	for rows.Next() {
		comment, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		comments = append(comments, comment)
	}
	return comments, rows.Err()
}

func (s *Store) SaveAsset(ctx context.Context, asset models.GeneratedAsset) (models.GeneratedAsset, error) {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO sandbox_generated_asset (KIND_, NAME_, CLI_SLUG_, USER_ID_, CONTENT_, CREATED_AT_)
		VALUES (?, ?, ?, ?, ?, ?)`,
		asset.Kind, asset.Name, nullableString(asset.CLISlug), asset.UserID, asset.Content, now,
	)
	if err != nil {
		return models.GeneratedAsset{}, fmt.Errorf("insert generated asset: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return models.GeneratedAsset{}, fmt.Errorf("generated asset id: %w", err)
	}

	asset.ID = id
	asset.CreatedAt = now
	return asset, nil
}

func (s *Store) LogExecution(ctx context.Context, userID *int64, cliSlug, line, mode string, result models.ExecutionResult) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sandbox_execution_log (USER_ID_, CLI_SLUG_, LINE_, MODE_, STDOUT_, STDERR_, EXIT_CODE_, DURATION_MS_, CREATED_AT_)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, cliSlug, line, mode, result.Stdout, result.Stderr, result.ExitCode, result.DurationMS, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert execution log: %w", err)
	}
	return nil
}

func scanCLIs(rows *sql.Rows) ([]models.CLI, error) {
	var clis []models.CLI
	for rows.Next() {
		cli, err := scanCLI(rows)
		if err != nil {
			return nil, err
		}
		clis = append(clis, cli)
	}
	return clis, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanCLI(row scanner) (models.CLI, error) {
	var (
		cli        models.CLI
		tagsRaw    []byte
		enabled    bool
		executable bool
		createdAt  time.Time
	)

	if err := row.Scan(
		&cli.Slug,
		&cli.DisplayName,
		&cli.Summary,
		&cli.Type,
		&tagsRaw,
		&cli.HelpText,
		&cli.VersionText,
		&cli.Popularity,
		&cli.RuntimeImage,
		&enabled,
		&cli.ExampleLine,
		&cli.EnvironmentKind,
		&cli.SourceType,
		&cli.Author,
		&cli.GitHubURL,
		&cli.GiteeURL,
		&cli.License,
		&createdAt,
		&cli.OriginalCommand,
		&executable,
		&cli.FavoriteCount,
		&cli.CommentCount,
		&cli.RunCount,
	); err != nil {
		return models.CLI{}, err
	}

	if err := json.Unmarshal(tagsRaw, &cli.Tags); err != nil {
		return models.CLI{}, fmt.Errorf("unmarshal cli tags: %w", err)
	}

	cli.Enabled = enabled
	cli.Executable = executable
	cli.CreatedAt = createdAt.UTC()
	return cli, nil
}

func scanUser(row scanner) (models.User, error) {
	var (
		user      models.User
		createdAt time.Time
	)
	if err := row.Scan(
		&user.ID,
		&user.Username,
		&user.DisplayName,
		&user.Email,
		&user.AvatarURL,
		&user.AuthProvider,
		&user.IP,
		&createdAt,
	); err != nil {
		return models.User{}, err
	}
	user.CreatedAt = createdAt.UTC()
	return user, nil
}

func scanComment(row scanner) (models.Comment, error) {
	var (
		comment   models.Comment
		createdAt time.Time
	)
	if err := row.Scan(&comment.ID, &comment.CLISlug, &comment.UserID, &comment.Username, &comment.Body, &createdAt); err != nil {
		return models.Comment{}, err
	}
	comment.CreatedAt = createdAt.UTC()
	return comment, nil
}

func generateMockIP(username string) string {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(strings.ToLower(username)))
	sum := hasher.Sum32()
	return fmt.Sprintf("10.24.%d.%d", (sum>>8)&0xff, sum&0xff)
}

func ensureDatabase(ctx context.Context, cfg config.Config) error {
	db, err := sql.Open("mysql", mysqlDSN(cfg, false))
	if err != nil {
		return fmt.Errorf("open mysql server connection: %w", err)
	}
	defer db.Close()

	createCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.PingContext(createCtx); err != nil {
		return fmt.Errorf("ping mysql server: %w", err)
	}

	if _, err := db.ExecContext(createCtx, createDatabaseStatement(cfg.DBName)); err != nil {
		return fmt.Errorf("create database %s: %w", cfg.DBName, err)
	}

	return nil
}

func createDatabaseStatement(name string) string {
	return fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", quoteIdentifier(name))
}

func mysqlDSN(cfg config.Config, withDatabase bool) string {
	driverCfg := mysql.NewConfig()
	driverCfg.User = cfg.DBUser
	driverCfg.Passwd = cfg.DBPassword
	driverCfg.Net = "tcp"
	driverCfg.Addr = net.JoinHostPort(cfg.DBHost, fmt.Sprintf("%d", cfg.DBPort))
	driverCfg.Params = map[string]string{
		"charset": "utf8mb4",
	}
	driverCfg.Collation = "utf8mb4_unicode_ci"
	driverCfg.ParseTime = true
	if withDatabase {
		driverCfg.DBName = cfg.DBName
	}
	return driverCfg.FormatDSN()
}

func mysqlSchemaStatements() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS cli_registry (
			SLUG_ VARCHAR(128) NOT NULL,
			DISPLAY_NAME_ VARCHAR(255) NOT NULL,
			SUMMARY_ TEXT NOT NULL,
			TYPE_ VARCHAR(64) NOT NULL,
			TAGS_JSON_ JSON NOT NULL,
			HELP_TEXT_ MEDIUMTEXT NOT NULL,
			VERSION_TEXT_ VARCHAR(255) NOT NULL,
			POPULARITY_ INT NOT NULL,
			RUNTIME_IMAGE_ VARCHAR(255) NOT NULL,
			ENABLED_ TINYINT(1) NOT NULL DEFAULT 1,
			EXAMPLE_LINE_ VARCHAR(512) NOT NULL,
			ENVIRONMENT_KIND_ VARCHAR(32) NOT NULL,
			SOURCE_TYPE_ VARCHAR(64) NOT NULL,
			AUTHOR_ VARCHAR(255) NOT NULL DEFAULT '',
			GITHUB_URL_ VARCHAR(512) NOT NULL DEFAULT '',
			GITEE_URL_ VARCHAR(512) NOT NULL DEFAULT '',
			LICENSE_ VARCHAR(128) NOT NULL DEFAULT '',
			CREATED_AT_ DATETIME(3) NOT NULL,
			ORIGINAL_COMMAND_ VARCHAR(512) NOT NULL DEFAULT '',
			EXECUTABLE_ TINYINT(1) NOT NULL DEFAULT 1,
			PRIMARY KEY (SLUG_),
			KEY IDX_CLI_REGISTRY_ENABLED_ENV_TYPE (ENABLED_, ENVIRONMENT_KIND_, TYPE_)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS auth_user (
			ID_ BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			USERNAME_ VARCHAR(128) NOT NULL,
			DISPLAY_NAME_ VARCHAR(255) NOT NULL DEFAULT '',
			EMAIL_ VARCHAR(255) NOT NULL DEFAULT '',
			AVATAR_URL_ VARCHAR(1024) NOT NULL DEFAULT '',
			AUTH_PROVIDER_ VARCHAR(32) NOT NULL DEFAULT 'mock',
			AUTH_SUB_ VARCHAR(255) NULL,
			IP_ VARCHAR(64) NOT NULL,
			CREATED_AT_ DATETIME(3) NOT NULL,
			UPDATED_AT_ DATETIME(3) NOT NULL,
			LAST_LOGIN_AT_ DATETIME(3) NULL,
			PRIMARY KEY (ID_),
			UNIQUE KEY UK_AUTH_USER_USERNAME (USERNAME_),
			UNIQUE KEY UK_AUTH_USER_PROVIDER_SUB (AUTH_PROVIDER_, AUTH_SUB_)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS auth_local_credential (
			USER_ID_ BIGINT UNSIGNED NOT NULL,
			PASSWORD_HASH_ VARCHAR(255) NOT NULL,
			PASSWORD_UPDATED_AT_ DATETIME(3) NOT NULL,
			CREATED_AT_ DATETIME(3) NOT NULL,
			PRIMARY KEY (USER_ID_),
			CONSTRAINT FK_AUTH_LOCAL_CREDENTIAL_USER FOREIGN KEY (USER_ID_) REFERENCES auth_user (ID_) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS auth_session (
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS auth_login_log (
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS user_favorite (
			USER_ID_ BIGINT UNSIGNED NOT NULL,
			CLI_SLUG_ VARCHAR(128) NOT NULL,
			CREATED_AT_ DATETIME(3) NOT NULL,
			PRIMARY KEY (USER_ID_, CLI_SLUG_),
			KEY IDX_USER_FAVORITE_USER_CREATED (USER_ID_, CREATED_AT_),
			CONSTRAINT FK_USER_FAVORITE_USER FOREIGN KEY (USER_ID_) REFERENCES auth_user (ID_) ON DELETE CASCADE,
			CONSTRAINT FK_USER_FAVORITE_CLI FOREIGN KEY (CLI_SLUG_) REFERENCES cli_registry (SLUG_) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS user_comment (
			ID_ BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			CLI_SLUG_ VARCHAR(128) NOT NULL,
			USER_ID_ BIGINT UNSIGNED NOT NULL,
			BODY_ TEXT NOT NULL,
			CREATED_AT_ DATETIME(3) NOT NULL,
			PRIMARY KEY (ID_),
			KEY IDX_USER_COMMENT_CLI_CREATED (CLI_SLUG_, CREATED_AT_),
			KEY IDX_USER_COMMENT_USER_CREATED (USER_ID_, CREATED_AT_),
			CONSTRAINT FK_USER_COMMENT_USER FOREIGN KEY (USER_ID_) REFERENCES auth_user (ID_) ON DELETE CASCADE,
			CONSTRAINT FK_USER_COMMENT_CLI FOREIGN KEY (CLI_SLUG_) REFERENCES cli_registry (SLUG_) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS sandbox_execution_log (
			ID_ BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			USER_ID_ BIGINT UNSIGNED NULL,
			CLI_SLUG_ VARCHAR(128) NOT NULL,
			LINE_ TEXT NOT NULL,
			MODE_ VARCHAR(64) NOT NULL,
			STDOUT_ MEDIUMTEXT NOT NULL,
			STDERR_ MEDIUMTEXT NOT NULL,
			EXIT_CODE_ INT NOT NULL,
			DURATION_MS_ BIGINT NOT NULL,
			CREATED_AT_ DATETIME(3) NOT NULL,
			PRIMARY KEY (ID_),
			KEY IDX_SANDBOX_EXECUTION_LOG_CLI_CREATED (CLI_SLUG_, CREATED_AT_),
			CONSTRAINT FK_SANDBOX_EXECUTION_LOG_USER FOREIGN KEY (USER_ID_) REFERENCES auth_user (ID_) ON DELETE SET NULL,
			CONSTRAINT FK_SANDBOX_EXECUTION_LOG_CLI FOREIGN KEY (CLI_SLUG_) REFERENCES cli_registry (SLUG_) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS sandbox_generated_asset (
			ID_ BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			KIND_ VARCHAR(64) NOT NULL,
			NAME_ VARCHAR(255) NOT NULL,
			CLI_SLUG_ VARCHAR(128) NULL,
			USER_ID_ BIGINT UNSIGNED NULL,
			CONTENT_ MEDIUMTEXT NOT NULL,
			CREATED_AT_ DATETIME(3) NOT NULL,
			PRIMARY KEY (ID_),
			KEY IDX_SANDBOX_GENERATED_ASSET_KIND_CREATED (KIND_, CREATED_AT_),
			CONSTRAINT FK_SANDBOX_GENERATED_ASSET_USER FOREIGN KEY (USER_ID_) REFERENCES auth_user (ID_) ON DELETE SET NULL,
			CONSTRAINT FK_SANDBOX_GENERATED_ASSET_CLI FOREIGN KEY (CLI_SLUG_) REFERENCES cli_registry (SLUG_) ON DELETE SET NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
		`CREATE TABLE IF NOT EXISTS seed_execution_record (
			SEED_KEY_ VARCHAR(128) NOT NULL,
			CLI_SLUG_ VARCHAR(128) NOT NULL,
			CREATED_AT_ DATETIME(3) NOT NULL,
			PRIMARY KEY (SEED_KEY_),
			CONSTRAINT FK_SEED_EXECUTION_RECORD_CLI FOREIGN KEY (CLI_SLUG_) REFERENCES cli_registry (SLUG_) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	}
}

func quoteIdentifier(value string) string {
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func homepageSortOrder(sort string) string {
	switch strings.ToLower(strings.TrimSpace(sort)) {
	case "newest":
		return "c.CREATED_AT_ DESC, c.DISPLAY_NAME_ ASC"
	case "runs":
		return "(SELECT COUNT(*) FROM sandbox_execution_log e WHERE e.CLI_SLUG_ = c.SLUG_) DESC, c.DISPLAY_NAME_ ASC"
	case "favorites", "":
		return "(SELECT COUNT(*) FROM user_favorite f WHERE f.CLI_SLUG_ = c.SLUG_) DESC, c.DISPLAY_NAME_ ASC"
	default:
		return "(SELECT COUNT(*) FROM user_favorite f WHERE f.CLI_SLUG_ = c.SLUG_) DESC, c.DISPLAY_NAME_ ASC"
	}
}
