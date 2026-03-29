package data

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/linlay/cligrep-server/internal/models"
)

func adminEmailSet(emails []string) map[string]struct{} {
	set := make(map[string]struct{}, len(emails))
	for _, email := range emails {
		normalized := strings.ToLower(strings.TrimSpace(email))
		if normalized == "" {
			continue
		}
		set[normalized] = struct{}{}
	}
	return set
}

func (s *Store) ensureRolesSeeded(ctx context.Context, execer sqlExecer) error {
	now := time.Now().UTC()
	roles := []struct {
		key   models.Role
		label string
	}{
		{key: models.RoleMember, label: "Member"},
		{key: models.RolePlatformAdmin, label: "Platform Admin"},
	}

	for _, role := range roles {
		if _, err := execer.ExecContext(ctx, `
			INSERT INTO auth_role (ROLE_KEY_, DISPLAY_NAME_, CREATED_AT_, UPDATED_AT_)
			VALUES (?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				DISPLAY_NAME_ = VALUES(DISPLAY_NAME_),
				UPDATED_AT_ = VALUES(UPDATED_AT_)`,
			string(role.key),
			role.label,
			now,
			now,
		); err != nil {
			return fmt.Errorf("seed auth role %s: %w", role.key, err)
		}
	}

	return nil
}

func (s *Store) syncUserRoles(ctx context.Context, userID int64, email string) error {
	return s.syncUserRolesWithExecer(ctx, s.db, userID, email)
}

func (s *Store) syncUserRolesWithExecer(ctx context.Context, execer sqlExecer, userID int64, email string) error {
	if userID <= 0 {
		return nil
	}
	if err := s.ensureRolesSeeded(ctx, execer); err != nil {
		return err
	}

	now := time.Now().UTC()
	if err := insertUserRoleWithExecer(ctx, execer, userID, models.RoleMember, now); err != nil {
		return err
	}

	if _, ok := s.adminEmails[strings.ToLower(strings.TrimSpace(email))]; ok {
		if err := insertUserRoleWithExecer(ctx, execer, userID, models.RolePlatformAdmin, now); err != nil {
			return err
		}
	}

	return nil
}

func insertUserRoleWithExecer(ctx context.Context, execer sqlExecer, userID int64, role models.Role, createdAt time.Time) error {
	if _, err := execer.ExecContext(ctx, `
		INSERT IGNORE INTO auth_user_role (USER_ID_, ROLE_ID_, CREATED_AT_)
		SELECT ?, r.ID_, ?
		FROM auth_role r
		WHERE r.ROLE_KEY_ = ?`,
		userID,
		createdAt,
		string(role),
	); err != nil {
		return fmt.Errorf("assign role %s to user %d: %w", role, userID, err)
	}
	return nil
}

func (s *Store) loadUserRoles(ctx context.Context, userID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.ROLE_KEY_
		FROM auth_user_role ur
		JOIN auth_role r ON r.ID_ = ur.ROLE_ID_
		WHERE ur.USER_ID_ = ?
		ORDER BY r.ROLE_KEY_ ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("load user roles: %w", err)
	}
	defer rows.Close()

	roles := make([]string, 0, 2)
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, fmt.Errorf("scan user role: %w", err)
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user roles: %w", err)
	}
	return roles, nil
}

func (s *Store) hydrateUserRoles(ctx context.Context, user *models.User) error {
	if user == nil || user.ID <= 0 {
		return nil
	}
	if err := s.syncUserRoles(ctx, user.ID, user.Email); err != nil {
		return err
	}
	roles, err := s.loadUserRoles(ctx, user.ID)
	if err != nil {
		return err
	}
	user.Roles = roles
	return nil
}
