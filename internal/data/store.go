package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/linlay/cligrep-server/internal/config"
	"github.com/linlay/cligrep-server/internal/models"
	mysqlschema "github.com/linlay/cligrep-server/scripts/mysql"
)

type Store struct {
	db *sql.DB
}

const cliSelectList = `
	SELECT c.SLUG_, c.DISPLAY_NAME_, c.SUMMARY_, c.TYPE_, c.TAGS_JSON_, c.HELP_TEXT_, c.VERSION_TEXT_,
	       c.POPULARITY_, c.RUNTIME_IMAGE_, c.ENABLED_, c.EXAMPLE_LINE_,
	       c.ENVIRONMENT_KIND_, c.SOURCE_TYPE_, c.AUTHOR_, c.GITHUB_URL_, c.GITEE_URL_,
	       c.LICENSE_, c.CREATED_AT_, c.ORIGINAL_COMMAND_, c.EXECUTABLE_,
	       c.FAVORITE_COUNT_, c.COMMENT_COUNT_, c.RUN_COUNT_`

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
			c.FAVORITE_COUNT_ DESC,
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

func (s *Store) GetCLIReleases(ctx context.Context, slug string) ([]models.CLIRelease, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ID_, VERSION_, PUBLISHED_AT_, IS_CURRENT_, SOURCE_KIND_, SOURCE_URL_
		FROM cli_release
		WHERE CLI_SLUG_ = ?
		ORDER BY PUBLISHED_AT_ DESC, VERSION_ DESC`, slug)
	if err != nil {
		return nil, fmt.Errorf("query cli releases: %w", err)
	}
	defer rows.Close()

	var releases []models.CLIRelease
	releaseIndexByID := make(map[int64]int)
	for rows.Next() {
		var (
			release     models.CLIRelease
			publishedAt time.Time
			isCurrent   bool
		)
		if err := rows.Scan(
			&release.ID,
			&release.Version,
			&publishedAt,
			&isCurrent,
			&release.SourceKind,
			&release.SourceURL,
		); err != nil {
			return nil, fmt.Errorf("scan cli release: %w", err)
		}
		release.PublishedAt = publishedAt.UTC()
		release.IsCurrent = isCurrent
		release.Assets = []models.CLIReleaseAsset{}
		releaseIndexByID[release.ID] = len(releases)
		releases = append(releases, release)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cli releases: %w", err)
	}
	if len(releases) == 0 {
		return nil, nil
	}

	assetRows, err := s.db.QueryContext(ctx, `
		SELECT a.ID_, a.RELEASE_ID_, a.FILE_NAME_, a.DOWNLOAD_URL_, a.OS_, a.ARCH_, a.PACKAGE_KIND_, a.CHECKSUM_URL_, a.SIZE_BYTES_
		FROM cli_release_asset a
		JOIN cli_release r ON r.ID_ = a.RELEASE_ID_
		WHERE r.CLI_SLUG_ = ?
		ORDER BY r.PUBLISHED_AT_ DESC, a.FILE_NAME_ ASC`, slug)
	if err != nil {
		return nil, fmt.Errorf("query cli release assets: %w", err)
	}
	defer assetRows.Close()

	for assetRows.Next() {
		var asset models.CLIReleaseAsset
		if err := assetRows.Scan(
			&asset.ID,
			&asset.ReleaseID,
			&asset.FileName,
			&asset.DownloadURL,
			&asset.OS,
			&asset.Arch,
			&asset.PackageKind,
			&asset.ChecksumURL,
			&asset.SizeBytes,
		); err != nil {
			return nil, fmt.Errorf("scan cli release asset: %w", err)
		}
		index, ok := releaseIndexByID[asset.ReleaseID]
		if !ok {
			continue
		}
		releases[index].Assets = append(releases[index].Assets, asset)
	}
	if err := assetRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cli release assets: %w", err)
	}

	return releases, nil
}

func (s *Store) ReplaceCLIReleases(ctx context.Context, slug string, releases []models.CLIRelease) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replace cli releases: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if len(releases) == 0 {
		if _, err = tx.ExecContext(ctx, `DELETE FROM cli_release WHERE CLI_SLUG_ = ?`, slug); err != nil {
			return fmt.Errorf("delete cli releases: %w", err)
		}
		return tx.Commit()
	}

	versions := make([]string, 0, len(releases))
	for _, release := range releases {
		versions = append(versions, release.Version)
		now := time.Now().UTC()
		result, execErr := tx.ExecContext(ctx, `
			INSERT INTO cli_release (CLI_SLUG_, VERSION_, PUBLISHED_AT_, IS_CURRENT_, SOURCE_KIND_, SOURCE_URL_, CREATED_AT_, UPDATED_AT_)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				ID_ = LAST_INSERT_ID(ID_),
				PUBLISHED_AT_ = VALUES(PUBLISHED_AT_),
				IS_CURRENT_ = VALUES(IS_CURRENT_),
				SOURCE_KIND_ = VALUES(SOURCE_KIND_),
				SOURCE_URL_ = VALUES(SOURCE_URL_),
				UPDATED_AT_ = VALUES(UPDATED_AT_)`,
			slug,
			release.Version,
			release.PublishedAt.UTC(),
			release.IsCurrent,
			release.SourceKind,
			release.SourceURL,
			now,
			now,
		)
		if execErr != nil {
			err = fmt.Errorf("upsert cli release %s/%s: %w", slug, release.Version, execErr)
			return err
		}

		releaseID, idErr := result.LastInsertId()
		if idErr != nil {
			err = fmt.Errorf("cli release id %s/%s: %w", slug, release.Version, idErr)
			return err
		}
		if _, execErr = tx.ExecContext(ctx, `DELETE FROM cli_release_asset WHERE RELEASE_ID_ = ?`, releaseID); execErr != nil {
			err = fmt.Errorf("delete cli release assets %s/%s: %w", slug, release.Version, execErr)
			return err
		}

		for _, asset := range release.Assets {
			if _, execErr = tx.ExecContext(ctx, `
				INSERT INTO cli_release_asset (RELEASE_ID_, FILE_NAME_, DOWNLOAD_URL_, OS_, ARCH_, PACKAGE_KIND_, CHECKSUM_URL_, SIZE_BYTES_, CREATED_AT_)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				releaseID,
				asset.FileName,
				asset.DownloadURL,
				asset.OS,
				asset.Arch,
				asset.PackageKind,
				asset.ChecksumURL,
				asset.SizeBytes,
				now,
			); execErr != nil {
				err = fmt.Errorf("insert cli release asset %s/%s/%s: %w", slug, release.Version, asset.FileName, execErr)
				return err
			}
		}
	}

	deleteQuery := `DELETE FROM cli_release WHERE CLI_SLUG_ = ?`
	args := []any{slug}
	if len(versions) > 0 {
		deleteQuery += " AND VERSION_ NOT IN (" + strings.TrimRight(strings.Repeat("?,", len(versions)), ",") + ")"
		for _, version := range versions {
			args = append(args, version)
		}
	}
	if _, err = tx.ExecContext(ctx, deleteQuery, args...); err != nil {
		return fmt.Errorf("delete stale cli releases %s: %w", slug, err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit replace cli releases: %w", err)
	}
	return nil
}

func (s *Store) SetFavorite(ctx context.Context, mutation models.FavoriteMutation) error {
	if mutation.Active {
		result, err := s.db.ExecContext(ctx, `
			INSERT IGNORE INTO user_favorite (USER_ID_, CLI_SLUG_, CREATED_AT_)
			VALUES (?, ?, ?)`,
			mutation.UserID, mutation.CLISlug, time.Now().UTC(),
		)
		if err != nil {
			return fmt.Errorf("set favorite: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("set favorite rows affected: %w", err)
		}
		if rows > 0 {
			if err := s.incrementCLICounter(ctx, mutation.CLISlug, "FAVORITE_COUNT_", 1); err != nil {
				return err
			}
		}
		return nil
	}

	result, err := s.db.ExecContext(ctx, `DELETE FROM user_favorite WHERE USER_ID_ = ? AND CLI_SLUG_ = ?`, mutation.UserID, mutation.CLISlug)
	if err != nil {
		return fmt.Errorf("remove favorite: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("remove favorite rows affected: %w", err)
	}
	if rows > 0 {
		if err := s.incrementCLICounter(ctx, mutation.CLISlug, "FAVORITE_COUNT_", -1); err != nil {
			return err
		}
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
	if err := s.incrementCLICounter(ctx, mutation.CLISlug, "COMMENT_COUNT_", 1); err != nil {
		return models.Comment{}, err
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
	return s.incrementCLICounter(ctx, cliSlug, "RUN_COUNT_", 1)
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

func (s *Store) incrementCLICounter(ctx context.Context, cliSlug, column string, delta int) error {
	validColumns := map[string]struct{}{
		"FAVORITE_COUNT_": {},
		"COMMENT_COUNT_":  {},
		"RUN_COUNT_":      {},
	}
	if _, ok := validColumns[column]; !ok {
		return fmt.Errorf("unsupported cli counter column %s", column)
	}

	if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`
		UPDATE cli_registry
		SET %s = GREATEST(%s + ?, 0)
		WHERE SLUG_ = ?`, column, column), delta, cliSlug); err != nil {
		return fmt.Errorf("update cli counter %s for %s: %w", column, cliSlug, err)
	}
	return nil
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
	return parseSQLStatements(mysqlschema.Schema())
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
		return "c.RUN_COUNT_ DESC, c.DISPLAY_NAME_ ASC"
	case "favorites", "":
		return "c.FAVORITE_COUNT_ DESC, c.DISPLAY_NAME_ ASC"
	default:
		return "c.FAVORITE_COUNT_ DESC, c.DISPLAY_NAME_ ASC"
	}
}

func parseSQLStatements(raw string) []string {
	lines := strings.Split(raw, "\n")
	statements := make([]string, 0, 16)
	var builder strings.Builder

	flush := func() {
		statement := strings.TrimSpace(builder.String())
		if statement == "" {
			builder.Reset()
			return
		}
		statement = strings.TrimSuffix(statement, ";")
		statement = strings.TrimSpace(statement)
		if statement != "" {
			statements = append(statements, statement)
		}
		builder.Reset()
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}

		builder.WriteString(line)
		builder.WriteString("\n")

		if strings.HasSuffix(trimmed, ";") {
			flush()
		}
	}
	flush()

	return statements
}
