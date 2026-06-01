package data

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/linlay/cligrep-server/internal/models"
)

const (
	adminAPIKeySecretPrefix = "cg_admin_"
	adminAPIKeyPrefixLength = 20
)

func GenerateAdminAPIKeySecret() (string, error) {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("generate admin api key: %w", err)
	}
	return adminAPIKeySecretPrefix + base64.RawURLEncoding.EncodeToString(randomBytes), nil
}

func (s *Store) ListAdminAPIKeys(ctx context.Context) ([]models.AdminAPIKey, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ID_, NAME_, KEY_PREFIX_, CREATED_BY_USER_ID_, CREATED_AT_, LAST_USED_AT_, REVOKED_AT_
		FROM admin_api_key
		ORDER BY COALESCE(REVOKED_AT_, '9999-12-31') DESC, CREATED_AT_ DESC, ID_ DESC`)
	if err != nil {
		return nil, fmt.Errorf("list admin api keys: %w", err)
	}
	defer rows.Close()

	var items []models.AdminAPIKey
	for rows.Next() {
		item, err := scanAdminAPIKey(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admin api keys: %w", err)
	}
	return items, nil
}

func (s *Store) CreateAdminAPIKey(ctx context.Context, name string, createdByUserID int64) (models.AdminAPIKeyCreateResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return models.AdminAPIKeyCreateResult{}, models.ErrInvalidAPIKeyName
	}
	if createdByUserID <= 0 {
		return models.AdminAPIKeyCreateResult{}, models.ErrUnauthorized
	}

	secret, err := GenerateAdminAPIKeySecret()
	if err != nil {
		return models.AdminAPIKeyCreateResult{}, err
	}

	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO admin_api_key (NAME_, KEY_PREFIX_, KEY_HASH_, CREATED_BY_USER_ID_, CREATED_AT_)
		VALUES (?, ?, ?, ?, ?)`,
		name,
		adminAPIKeyPrefix(secret),
		adminAPIKeyHash(secret),
		createdByUserID,
		now,
	)
	if err != nil {
		return models.AdminAPIKeyCreateResult{}, fmt.Errorf("create admin api key: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return models.AdminAPIKeyCreateResult{}, fmt.Errorf("admin api key id: %w", err)
	}
	item, err := s.GetAdminAPIKey(ctx, id)
	if err != nil {
		return models.AdminAPIKeyCreateResult{}, err
	}
	return models.AdminAPIKeyCreateResult{APIKey: item, Secret: secret}, nil
}

func (s *Store) GetAdminAPIKey(ctx context.Context, id int64) (models.AdminAPIKey, error) {
	if id <= 0 {
		return models.AdminAPIKey{}, sql.ErrNoRows
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT ID_, NAME_, KEY_PREFIX_, CREATED_BY_USER_ID_, CREATED_AT_, LAST_USED_AT_, REVOKED_AT_
		FROM admin_api_key
		WHERE ID_ = ?`, id)
	return scanAdminAPIKey(row)
}

func (s *Store) RevokeAdminAPIKey(ctx context.Context, id int64) error {
	if id <= 0 {
		return sql.ErrNoRows
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE admin_api_key
		SET REVOKED_AT_ = COALESCE(REVOKED_AT_, ?)
		WHERE ID_ = ? AND REVOKED_AT_ IS NULL`,
		time.Now().UTC(),
		id,
	)
	if err != nil {
		return fmt.Errorf("revoke admin api key: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("revoke admin api key rows affected: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) UserForAdminAPIKey(ctx context.Context, secret string) (models.User, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return models.User{}, models.ErrUnauthorized
	}

	now := time.Now().UTC()
	row := s.db.QueryRowContext(ctx, userSelectList+`
		JOIN admin_api_key k ON k.CREATED_BY_USER_ID_ = auth_user.ID_
		WHERE k.KEY_HASH_ = ? AND k.REVOKED_AT_ IS NULL`,
		adminAPIKeyHash(secret),
	)
	user, err := scanUser(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return models.User{}, models.ErrUnauthorized
		}
		return models.User{}, fmt.Errorf("load admin api key user: %w", err)
	}
	if err := s.hydrateUserRoles(ctx, &user); err != nil {
		return models.User{}, err
	}
	if !hasRole(user, models.RolePlatformAdmin) {
		return models.User{}, models.ErrForbidden
	}

	if _, err := s.db.ExecContext(ctx, `
		UPDATE admin_api_key
		SET LAST_USED_AT_ = ?
		WHERE KEY_HASH_ = ? AND REVOKED_AT_ IS NULL`,
		now,
		adminAPIKeyHash(secret),
	); err != nil {
		return models.User{}, fmt.Errorf("touch admin api key: %w", err)
	}
	return user, nil
}

func adminAPIKeyHash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func adminAPIKeyPrefix(secret string) string {
	if len(secret) <= adminAPIKeyPrefixLength {
		return secret
	}
	return secret[:adminAPIKeyPrefixLength]
}

func scanAdminAPIKey(row scanner) (models.AdminAPIKey, error) {
	var (
		item       models.AdminAPIKey
		createdAt  time.Time
		lastUsedAt sql.NullTime
		revokedAt  sql.NullTime
	)
	if err := row.Scan(
		&item.ID,
		&item.Name,
		&item.KeyPrefix,
		&item.CreatedByUserID,
		&createdAt,
		&lastUsedAt,
		&revokedAt,
	); err != nil {
		return models.AdminAPIKey{}, err
	}
	item.CreatedAt = createdAt.UTC()
	if lastUsedAt.Valid {
		value := lastUsedAt.Time.UTC()
		item.LastUsedAt = &value
	}
	if revokedAt.Valid {
		value := revokedAt.Time.UTC()
		item.RevokedAt = &value
	}
	return item, nil
}
