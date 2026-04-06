package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/linlay/cligrep-server/internal/i18n"
	"github.com/linlay/cligrep-server/internal/models"
)

func (s *Store) ListAdminCLIs(ctx context.Context, user models.User) ([]models.CLI, error) {
	locale := i18n.LocaleFromContext(ctx)
	query := fmt.Sprintf(`%s
		FROM cli_registry c
		%s`, cliSelectList, cliLocaleJoin)
	args := []any{locale}
	if !hasRole(user, models.RolePlatformAdmin) {
		query += ` WHERE c.OWNER_USER_ID_ = ?`
		args = append(args, user.ID)
	}
	query += ` ORDER BY c.UPDATED_AT_ DESC, c.DISPLAY_NAME_ ASC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list admin clis: %w", err)
	}
	defer rows.Close()

	return scanCLIs(rows)
}

func (s *Store) GetAdminCLI(ctx context.Context, slug string) (models.CLI, error) {
	locale := i18n.LocaleFromContext(ctx)
	row := s.db.QueryRowContext(ctx, fmt.Sprintf(`%s
		FROM cli_registry c
		%s
		WHERE c.SLUG_ = ?`, cliSelectList, cliLocaleJoin), locale, slug)
	return scanCLI(row)
}

func (s *Store) CreateOwnedCLI(ctx context.Context, ownerID int64, cli models.CLI) (models.CLI, error) {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO cli_registry (
			SLUG_, DISPLAY_NAME_, SUMMARY_, TYPE_, TAGS_JSON_, HELP_TEXT_,
			VERSION_TEXT_, POPULARITY_, RUNTIME_IMAGE_, ENABLED_, EXAMPLE_LINE_,
			ENVIRONMENT_KIND_, SOURCE_TYPE_, AUTHOR_, OFFICIAL_URL_, GITEE_URL_, LICENSE_,
			CREATED_AT_, UPDATED_AT_, PUBLISHED_AT_, ORIGINAL_COMMAND_, EXECUTABLE_,
			FAVORITE_COUNT_, COMMENT_COUNT_, RUN_COUNT_, OWNER_USER_ID_, STATUS_, EXECUTION_TEMPLATE_
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, 0, ?, ?, ?)`,
		cli.Slug,
		cli.DisplayName,
		cli.Summary,
		string(cli.Type),
		mustJSON(cli.Tags),
		cli.HelpText,
		cli.VersionText,
		cli.Popularity,
		cli.RuntimeImage,
		cli.Enabled,
		cli.ExampleLine,
		string(cli.EnvironmentKind),
		cli.SourceType,
		cli.Author,
		cli.OfficialURL,
		cli.GiteeURL,
		cli.License,
		now,
		now,
		nullableTime(cli.PublishedAt),
		cli.OriginalCommand,
		cli.Executable,
		ownerID,
		cli.Status,
		cli.ExecutionTemplate,
	)
	if err != nil {
		if isDuplicateEntryError(err) {
			return models.CLI{}, models.ErrCLISlugTaken
		}
		return models.CLI{}, fmt.Errorf("create admin cli: %w", err)
	}
	return s.GetAdminCLI(ctx, cli.Slug)
}

func (s *Store) UpdateAdminCLI(ctx context.Context, cli models.CLI) (models.CLI, error) {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		UPDATE cli_registry
		SET DISPLAY_NAME_ = ?,
		    SUMMARY_ = ?,
		    TAGS_JSON_ = ?,
		    HELP_TEXT_ = ?,
		    VERSION_TEXT_ = ?,
		    RUNTIME_IMAGE_ = ?,
		    ENABLED_ = ?,
		    EXAMPLE_LINE_ = ?,
		    ENVIRONMENT_KIND_ = ?,
		    SOURCE_TYPE_ = ?,
		    AUTHOR_ = ?,
		    OFFICIAL_URL_ = ?,
		    GITEE_URL_ = ?,
		    LICENSE_ = ?,
		    UPDATED_AT_ = ?,
		    PUBLISHED_AT_ = ?,
		    ORIGINAL_COMMAND_ = ?,
		    EXECUTABLE_ = ?,
		    STATUS_ = ?,
		    EXECUTION_TEMPLATE_ = ?
		WHERE SLUG_ = ?`,
		cli.DisplayName,
		cli.Summary,
		mustJSON(cli.Tags),
		cli.HelpText,
		cli.VersionText,
		cli.RuntimeImage,
		cli.Enabled,
		cli.ExampleLine,
		string(cli.EnvironmentKind),
		cli.SourceType,
		cli.Author,
		cli.OfficialURL,
		cli.GiteeURL,
		cli.License,
		now,
		nullableTime(cli.PublishedAt),
		cli.OriginalCommand,
		cli.Executable,
		cli.Status,
		cli.ExecutionTemplate,
		cli.Slug,
	)
	if err != nil {
		return models.CLI{}, fmt.Errorf("update admin cli: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return models.CLI{}, fmt.Errorf("update admin cli rows affected: %w", err)
	}
	if rows == 0 {
		return models.CLI{}, sql.ErrNoRows
	}
	return s.GetAdminCLI(ctx, cli.Slug)
}

func (s *Store) SetCLIStatus(ctx context.Context, slug string, status models.CLIStatus, publishedAt *time.Time) error {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		UPDATE cli_registry
		SET STATUS_ = ?, PUBLISHED_AT_ = ?, UPDATED_AT_ = ?
		WHERE SLUG_ = ?`,
		string(status),
		nullableTime(publishedAt),
		now,
		slug,
	)
	if err != nil {
		return fmt.Errorf("set cli status: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("set cli status rows affected: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteCLI(ctx context.Context, slug string) ([]string, error) {
	paths, err := s.collectStoragePathsForCLI(ctx, slug)
	if err != nil {
		return nil, err
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM cli_registry WHERE SLUG_ = ?`, slug)
	if err != nil {
		return nil, fmt.Errorf("delete admin cli: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("delete admin cli rows affected: %w", err)
	}
	if rows == 0 {
		return nil, sql.ErrNoRows
	}
	return paths, nil
}

func (s *Store) CreateCLIRelease(ctx context.Context, slug string, release models.CLIRelease) (models.CLIRelease, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.CLIRelease{}, fmt.Errorf("begin create release tx: %w", err)
	}
	defer tx.Rollback()

	if release.IsCurrent {
		if _, err := tx.ExecContext(ctx, `UPDATE cli_release SET IS_CURRENT_ = 0, UPDATED_AT_ = ? WHERE CLI_SLUG_ = ?`, time.Now().UTC(), slug); err != nil {
			return models.CLIRelease{}, fmt.Errorf("clear current release: %w", err)
		}
	}

	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO cli_release (CLI_SLUG_, VERSION_, PUBLISHED_AT_, IS_CURRENT_, SOURCE_KIND_, SOURCE_URL_, CREATED_AT_, UPDATED_AT_)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		slug,
		release.Version,
		release.PublishedAt.UTC(),
		release.IsCurrent,
		release.SourceKind,
		release.SourceURL,
		now,
		now,
	)
	if err != nil {
		return models.CLIRelease{}, fmt.Errorf("create release: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `UPDATE cli_registry SET UPDATED_AT_ = ? WHERE SLUG_ = ?`, now, slug); err != nil {
		return models.CLIRelease{}, fmt.Errorf("touch cli after release create: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return models.CLIRelease{}, fmt.Errorf("commit create release tx: %w", err)
	}

	return s.GetCLIRelease(ctx, slug, release.Version)
}

func (s *Store) UpdateCLIRelease(ctx context.Context, slug, version string, release models.CLIRelease) (models.CLIRelease, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.CLIRelease{}, fmt.Errorf("begin update release tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	if release.IsCurrent {
		if _, err := tx.ExecContext(ctx, `UPDATE cli_release SET IS_CURRENT_ = 0, UPDATED_AT_ = ? WHERE CLI_SLUG_ = ?`, now, slug); err != nil {
			return models.CLIRelease{}, fmt.Errorf("clear current release: %w", err)
		}
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE cli_release
		SET PUBLISHED_AT_ = ?, IS_CURRENT_ = ?, SOURCE_KIND_ = ?, SOURCE_URL_ = ?, UPDATED_AT_ = ?
		WHERE CLI_SLUG_ = ? AND VERSION_ = ?`,
		release.PublishedAt.UTC(),
		release.IsCurrent,
		release.SourceKind,
		release.SourceURL,
		now,
		slug,
		version,
	)
	if err != nil {
		return models.CLIRelease{}, fmt.Errorf("update release: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return models.CLIRelease{}, fmt.Errorf("update release rows affected: %w", err)
	}
	if rows == 0 {
		return models.CLIRelease{}, sql.ErrNoRows
	}

	if _, err := tx.ExecContext(ctx, `UPDATE cli_registry SET UPDATED_AT_ = ? WHERE SLUG_ = ?`, now, slug); err != nil {
		return models.CLIRelease{}, fmt.Errorf("touch cli after release update: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return models.CLIRelease{}, fmt.Errorf("commit update release tx: %w", err)
	}

	return s.GetCLIRelease(ctx, slug, version)
}

func (s *Store) GetCLIRelease(ctx context.Context, slug, version string) (models.CLIRelease, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT ID_, VERSION_, PUBLISHED_AT_, IS_CURRENT_, SOURCE_KIND_, SOURCE_URL_
		FROM cli_release
		WHERE CLI_SLUG_ = ? AND VERSION_ = ?`, slug, version)

	var (
		release     models.CLIRelease
		publishedAt time.Time
		isCurrent   bool
	)
	if err := row.Scan(
		&release.ID,
		&release.Version,
		&publishedAt,
		&isCurrent,
		&release.SourceKind,
		&release.SourceURL,
	); err != nil {
		return models.CLIRelease{}, err
	}
	release.PublishedAt = publishedAt.UTC()
	release.IsCurrent = isCurrent

	assets, err := s.listReleaseAssets(ctx, release.ID)
	if err != nil {
		return models.CLIRelease{}, err
	}
	release.Assets = assets
	return release, nil
}

func (s *Store) DeleteCLIRelease(ctx context.Context, slug, version string) ([]string, error) {
	paths, err := s.collectStoragePathsForRelease(ctx, slug, version)
	if err != nil {
		return nil, err
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM cli_release WHERE CLI_SLUG_ = ? AND VERSION_ = ?`, slug, version)
	if err != nil {
		return nil, fmt.Errorf("delete release: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("delete release rows affected: %w", err)
	}
	if rows == 0 {
		return nil, sql.ErrNoRows
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE cli_registry SET UPDATED_AT_ = ? WHERE SLUG_ = ?`, time.Now().UTC(), slug); err != nil {
		return nil, fmt.Errorf("touch cli after release delete: %w", err)
	}
	return paths, nil
}

func (s *Store) CreateCLIReleaseAsset(ctx context.Context, slug, version string, asset models.CLIReleaseAsset) (models.CLIReleaseAsset, error) {
	release, err := s.GetCLIRelease(ctx, slug, version)
	if err != nil {
		return models.CLIReleaseAsset{}, err
	}

	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO cli_release_asset (
			RELEASE_ID_, FILE_NAME_, DOWNLOAD_URL_, OS_, ARCH_, PACKAGE_KIND_, CHECKSUM_URL_,
			SIZE_BYTES_, STORAGE_KIND_, STORAGE_PATH_, CREATED_AT_
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		release.ID,
		asset.FileName,
		asset.DownloadURL,
		asset.OS,
		asset.Arch,
		asset.PackageKind,
		asset.ChecksumURL,
		asset.SizeBytes,
		asset.StorageKind,
		asset.StoragePath,
		now,
	)
	if err != nil {
		return models.CLIReleaseAsset{}, fmt.Errorf("create release asset: %w", err)
	}
	assetID, err := result.LastInsertId()
	if err != nil {
		return models.CLIReleaseAsset{}, fmt.Errorf("release asset id: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE cli_registry SET UPDATED_AT_ = ? WHERE SLUG_ = ?`, now, slug); err != nil {
		return models.CLIReleaseAsset{}, fmt.Errorf("touch cli after asset create: %w", err)
	}
	asset.ID = assetID
	asset.ReleaseID = release.ID
	return asset, nil
}

func (s *Store) DeleteCLIReleaseAsset(ctx context.Context, slug, version string, assetID int64) (models.CLIReleaseAsset, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT a.ID_, a.RELEASE_ID_, a.FILE_NAME_, a.DOWNLOAD_URL_, a.OS_, a.ARCH_, a.PACKAGE_KIND_, a.CHECKSUM_URL_, a.SIZE_BYTES_, a.STORAGE_KIND_, a.STORAGE_PATH_
		FROM cli_release_asset a
		JOIN cli_release r ON r.ID_ = a.RELEASE_ID_
		WHERE r.CLI_SLUG_ = ? AND r.VERSION_ = ? AND a.ID_ = ?`, slug, version, assetID)

	var asset models.CLIReleaseAsset
	if err := row.Scan(
		&asset.ID,
		&asset.ReleaseID,
		&asset.FileName,
		&asset.DownloadURL,
		&asset.OS,
		&asset.Arch,
		&asset.PackageKind,
		&asset.ChecksumURL,
		&asset.SizeBytes,
		&asset.StorageKind,
		&asset.StoragePath,
	); err != nil {
		return models.CLIReleaseAsset{}, err
	}

	result, err := s.db.ExecContext(ctx, `DELETE FROM cli_release_asset WHERE ID_ = ?`, assetID)
	if err != nil {
		return models.CLIReleaseAsset{}, fmt.Errorf("delete release asset: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return models.CLIReleaseAsset{}, fmt.Errorf("delete release asset rows affected: %w", err)
	}
	if rows == 0 {
		return models.CLIReleaseAsset{}, sql.ErrNoRows
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE cli_registry SET UPDATED_AT_ = ? WHERE SLUG_ = ?`, time.Now().UTC(), slug); err != nil {
		return models.CLIReleaseAsset{}, fmt.Errorf("touch cli after asset delete: %w", err)
	}
	return asset, nil
}

func (s *Store) listReleaseAssets(ctx context.Context, releaseID int64) ([]models.CLIReleaseAsset, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ID_, RELEASE_ID_, FILE_NAME_, DOWNLOAD_URL_, OS_, ARCH_, PACKAGE_KIND_, CHECKSUM_URL_, SIZE_BYTES_, STORAGE_KIND_, STORAGE_PATH_
		FROM cli_release_asset
		WHERE RELEASE_ID_ = ?
		ORDER BY FILE_NAME_ ASC`, releaseID)
	if err != nil {
		return nil, fmt.Errorf("list release assets: %w", err)
	}
	defer rows.Close()

	var assets []models.CLIReleaseAsset
	for rows.Next() {
		var asset models.CLIReleaseAsset
		if err := rows.Scan(
			&asset.ID,
			&asset.ReleaseID,
			&asset.FileName,
			&asset.DownloadURL,
			&asset.OS,
			&asset.Arch,
			&asset.PackageKind,
			&asset.ChecksumURL,
			&asset.SizeBytes,
			&asset.StorageKind,
			&asset.StoragePath,
		); err != nil {
			return nil, fmt.Errorf("scan release asset: %w", err)
		}
		assets = append(assets, asset)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate release assets: %w", err)
	}
	return assets, nil
}

func (s *Store) collectStoragePathsForCLI(ctx context.Context, slug string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.STORAGE_PATH_
		FROM cli_release_asset a
		JOIN cli_release r ON r.ID_ = a.RELEASE_ID_
		WHERE r.CLI_SLUG_ = ? AND a.STORAGE_PATH_ <> ''`, slug)
	if err != nil {
		return nil, fmt.Errorf("collect cli asset paths: %w", err)
	}
	defer rows.Close()
	return scanStoragePaths(rows)
}

func (s *Store) collectStoragePathsForRelease(ctx context.Context, slug, version string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.STORAGE_PATH_
		FROM cli_release_asset a
		JOIN cli_release r ON r.ID_ = a.RELEASE_ID_
		WHERE r.CLI_SLUG_ = ? AND r.VERSION_ = ? AND a.STORAGE_PATH_ <> ''`, slug, version)
	if err != nil {
		return nil, fmt.Errorf("collect release asset paths: %w", err)
	}
	defer rows.Close()
	return scanStoragePaths(rows)
}

func scanStoragePaths(rows *sql.Rows) ([]string, error) {
	paths := make([]string, 0, 4)
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, fmt.Errorf("scan storage path: %w", err)
		}
		paths = append(paths, path)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate storage paths: %w", err)
	}
	return paths, nil
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func nullableTime(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return value.UTC()
}

func hasRole(user models.User, role models.Role) bool {
	for _, item := range user.Roles {
		if item == string(role) {
			return true
		}
	}
	return false
}
