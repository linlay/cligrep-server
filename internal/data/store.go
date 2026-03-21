package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/linlay/cligrep-server/internal/models"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	store := &Store{db: db}
	if err := store.init(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) init() error {
	statements := []string{
		`PRAGMA foreign_keys = ON;`,
		`PRAGMA journal_mode = WAL;`,
		`PRAGMA busy_timeout = 5000;`,
		`CREATE TABLE IF NOT EXISTS clis (
			slug TEXT PRIMARY KEY,
			display_name TEXT NOT NULL,
			summary TEXT NOT NULL,
			type TEXT NOT NULL,
			tags_json TEXT NOT NULL,
			help_text TEXT NOT NULL,
			version_text TEXT NOT NULL,
			popularity INTEGER NOT NULL,
			runtime_image TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			example_line TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			ip TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS favorites (
			user_id INTEGER NOT NULL,
			cli_slug TEXT NOT NULL,
			created_at TEXT NOT NULL,
			PRIMARY KEY (user_id, cli_slug),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY (cli_slug) REFERENCES clis(slug) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS comments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			cli_slug TEXT NOT NULL,
			user_id INTEGER NOT NULL,
			body TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY (cli_slug) REFERENCES clis(slug) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS execution_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER,
			cli_slug TEXT NOT NULL,
			line TEXT NOT NULL,
			mode TEXT NOT NULL,
			stdout TEXT NOT NULL,
			stderr TEXT NOT NULL,
			exit_code INTEGER NOT NULL,
			duration_ms INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL
		);`,
		`CREATE TABLE IF NOT EXISTS generated_assets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			kind TEXT NOT NULL,
			name TEXT NOT NULL,
			cli_slug TEXT,
			user_id INTEGER,
			content TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL
		);`,
	}

	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return fmt.Errorf("initialize sqlite schema: %w", err)
		}
	}

	return nil
}

func (s *Store) SeedCLIs(ctx context.Context, clis []models.CLI) error {
	query := `INSERT INTO clis
		(slug, display_name, summary, type, tags_json, help_text, version_text, popularity, runtime_image, enabled, example_line)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(slug) DO UPDATE SET
			display_name=excluded.display_name,
			summary=excluded.summary,
			type=excluded.type,
			tags_json=excluded.tags_json,
			help_text=excluded.help_text,
			version_text=excluded.version_text,
			popularity=excluded.popularity,
			runtime_image=excluded.runtime_image,
			enabled=excluded.enabled,
			example_line=excluded.example_line`

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
			boolToInt(cli.Enabled),
			cli.ExampleLine,
		); err != nil {
			return fmt.Errorf("seed cli %s: %w", cli.Slug, err)
		}
	}

	return nil
}

func (s *Store) ListTrending(ctx context.Context, limit int) ([]models.CLI, error) {
	if limit <= 0 {
		limit = 12
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT c.slug, c.display_name, c.summary, c.type, c.tags_json, c.help_text, c.version_text,
		       c.popularity, c.runtime_image, c.enabled, c.example_line,
		       (SELECT COUNT(*) FROM favorites f WHERE f.cli_slug = c.slug) AS favorite_count,
		       (SELECT COUNT(*) FROM comments m WHERE m.cli_slug = c.slug) AS comment_count
		FROM clis c
		WHERE c.enabled = 1 AND c.type != 'builtin'
		ORDER BY c.popularity DESC, c.display_name ASC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("query trending clis: %w", err)
	}
	defer rows.Close()

	return scanCLIs(rows)
}

func (s *Store) SearchCLIs(ctx context.Context, query string, limit int) ([]models.CLI, error) {
	if limit <= 0 {
		limit = 12
	}

	pattern := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.slug, c.display_name, c.summary, c.type, c.tags_json, c.help_text, c.version_text,
		       c.popularity, c.runtime_image, c.enabled, c.example_line,
		       (SELECT COUNT(*) FROM favorites f WHERE f.cli_slug = c.slug) AS favorite_count,
		       (SELECT COUNT(*) FROM comments m WHERE m.cli_slug = c.slug) AS comment_count
		FROM clis c
		WHERE c.enabled = 1
		  AND (
			  LOWER(c.slug) LIKE ?
			  OR LOWER(c.display_name) LIKE ?
			  OR LOWER(c.summary) LIKE ?
			  OR LOWER(c.help_text) LIKE ?
			  OR LOWER(c.tags_json) LIKE ?
		  )
		ORDER BY
			CASE WHEN c.type = 'builtin' THEN 0 ELSE 1 END,
			c.popularity DESC,
			c.display_name ASC
		LIMIT ?`, pattern, pattern, pattern, pattern, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search clis: %w", err)
	}
	defer rows.Close()

	return scanCLIs(rows)
}

func (s *Store) GetCLI(ctx context.Context, slug string) (models.CLI, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT c.slug, c.display_name, c.summary, c.type, c.tags_json, c.help_text, c.version_text,
		       c.popularity, c.runtime_image, c.enabled, c.example_line,
		       (SELECT COUNT(*) FROM favorites f WHERE f.cli_slug = c.slug) AS favorite_count,
		       (SELECT COUNT(*) FROM comments m WHERE m.cli_slug = c.slug) AS comment_count
		FROM clis c
		WHERE c.slug = ? AND c.enabled = 1`, slug)

	return scanCLI(row)
}

func (s *Store) LoginMock(ctx context.Context, username string) (models.User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		username = "operator"
	}

	ip := generateMockIP(username)
	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO users (username, ip, created_at)
		VALUES (?, ?, ?)
		ON CONFLICT(username) DO UPDATE SET ip=excluded.ip`, username, ip, now); err != nil {
		return models.User{}, fmt.Errorf("upsert mock user: %w", err)
	}

	row := s.db.QueryRowContext(ctx, `SELECT id, username, ip, created_at FROM users WHERE username = ?`, username)
	return scanUser(row)
}

func (s *Store) GetUser(ctx context.Context, userID int64) (models.User, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, username, ip, created_at FROM users WHERE id = ?`, userID)
	return scanUser(row)
}

func (s *Store) SetFavorite(ctx context.Context, mutation models.FavoriteMutation) error {
	if mutation.Active {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO favorites (user_id, cli_slug, created_at)
			VALUES (?, ?, ?)
			ON CONFLICT(user_id, cli_slug) DO NOTHING`,
			mutation.UserID, mutation.CLISlug, time.Now().UTC().Format(time.RFC3339),
		)
		if err != nil {
			return fmt.Errorf("set favorite: %w", err)
		}
		return nil
	}

	if _, err := s.db.ExecContext(ctx, `DELETE FROM favorites WHERE user_id = ? AND cli_slug = ?`, mutation.UserID, mutation.CLISlug); err != nil {
		return fmt.Errorf("remove favorite: %w", err)
	}
	return nil
}

func (s *Store) ListFavorites(ctx context.Context, userID int64) ([]models.CLI, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.slug, c.display_name, c.summary, c.type, c.tags_json, c.help_text, c.version_text,
		       c.popularity, c.runtime_image, c.enabled, c.example_line,
		       (SELECT COUNT(*) FROM favorites f WHERE f.cli_slug = c.slug) AS favorite_count,
		       (SELECT COUNT(*) FROM comments m WHERE m.cli_slug = c.slug) AS comment_count
		FROM favorites f
		JOIN clis c ON c.slug = f.cli_slug
		WHERE f.user_id = ?
		ORDER BY f.created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list favorites: %w", err)
	}
	defer rows.Close()

	return scanCLIs(rows)
}

func (s *Store) AddComment(ctx context.Context, mutation models.CommentMutation) (models.Comment, error) {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO comments (cli_slug, user_id, body, created_at)
		VALUES (?, ?, ?, ?)`,
		mutation.CLISlug, mutation.UserID, strings.TrimSpace(mutation.Body), time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return models.Comment{}, fmt.Errorf("insert comment: %w", err)
	}

	commentID, err := result.LastInsertId()
	if err != nil {
		return models.Comment{}, fmt.Errorf("comment id: %w", err)
	}

	row := s.db.QueryRowContext(ctx, `
		SELECT c.id, c.cli_slug, c.user_id, u.username, c.body, c.created_at
		FROM comments c
		JOIN users u ON u.id = c.user_id
		WHERE c.id = ?`, commentID)
	return scanComment(row)
}

func (s *Store) ListComments(ctx context.Context, cliSlug string) ([]models.Comment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.cli_slug, c.user_id, u.username, c.body, c.created_at
		FROM comments c
		JOIN users u ON u.id = c.user_id
		WHERE c.cli_slug = ?
		ORDER BY c.created_at DESC
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
		INSERT INTO generated_assets (kind, name, cli_slug, user_id, content, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		asset.Kind, asset.Name, asset.CLISlug, asset.UserID, asset.Content, now.Format(time.RFC3339),
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
		INSERT INTO execution_logs (user_id, cli_slug, line, mode, stdout, stderr, exit_code, duration_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, cliSlug, line, mode, result.Stdout, result.Stderr, result.ExitCode, result.DurationMS, time.Now().UTC().Format(time.RFC3339),
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
		cli     models.CLI
		tagsRaw string
		enabled int
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
		&cli.FavoriteCount,
		&cli.CommentCount,
	); err != nil {
		return models.CLI{}, err
	}

	if err := json.Unmarshal([]byte(tagsRaw), &cli.Tags); err != nil {
		return models.CLI{}, fmt.Errorf("unmarshal cli tags: %w", err)
	}

	cli.Enabled = enabled == 1
	return cli, nil
}

func scanUser(row scanner) (models.User, error) {
	var (
		user       models.User
		createdRaw string
	)
	if err := row.Scan(&user.ID, &user.Username, &user.IP, &createdRaw); err != nil {
		return models.User{}, err
	}
	parsed, err := time.Parse(time.RFC3339, createdRaw)
	if err != nil {
		return models.User{}, fmt.Errorf("parse user timestamp: %w", err)
	}
	user.CreatedAt = parsed
	return user, nil
}

func scanComment(row scanner) (models.Comment, error) {
	var (
		comment    models.Comment
		createdRaw string
	)
	if err := row.Scan(&comment.ID, &comment.CLISlug, &comment.UserID, &comment.Username, &comment.Body, &createdRaw); err != nil {
		return models.Comment{}, err
	}
	parsed, err := time.Parse(time.RFC3339, createdRaw)
	if err != nil {
		return models.Comment{}, fmt.Errorf("parse comment timestamp: %w", err)
	}
	comment.CreatedAt = parsed
	return comment, nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func generateMockIP(username string) string {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(strings.ToLower(username)))
	sum := hasher.Sum32()
	return fmt.Sprintf("10.24.%d.%d", (sum>>8)&0xff, sum&0xff)
}
