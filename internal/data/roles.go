package data

import (
	"context"
	"database/sql"
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

func (s *Store) ListPlatformAdmins(ctx context.Context) ([]models.User, error) {
	rows, err := s.db.QueryContext(ctx, userSelectList+`
		JOIN auth_user_role ur ON ur.USER_ID_ = auth_user.ID_
		JOIN auth_role r ON r.ID_ = ur.ROLE_ID_
		WHERE r.ROLE_KEY_ = ?
		ORDER BY auth_user.UPDATED_AT_ DESC, auth_user.ID_ ASC`,
		string(models.RolePlatformAdmin),
	)
	if err != nil {
		return nil, fmt.Errorf("list platform admins: %w", err)
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		if err := s.hydrateUserRoles(ctx, &user); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate platform admins: %w", err)
	}
	return users, nil
}

func (s *Store) GrantPlatformAdmin(ctx context.Context, identifier string) (models.User, error) {
	user, err := s.findUserForAdminRole(ctx, identifier)
	if err != nil {
		return models.User{}, err
	}
	if err := s.ensureRolesSeeded(ctx, s.db); err != nil {
		return models.User{}, err
	}
	if err := insertUserRoleWithExecer(ctx, s.db, user.ID, models.RolePlatformAdmin, time.Now().UTC()); err != nil {
		return models.User{}, err
	}
	if err := s.hydrateUserRoles(ctx, &user); err != nil {
		return models.User{}, err
	}
	return user, nil
}

func (s *Store) RevokePlatformAdmin(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return sql.ErrNoRows
	}
	result, err := s.db.ExecContext(ctx, `
		DELETE ur
		FROM auth_user_role ur
		JOIN auth_role r ON r.ID_ = ur.ROLE_ID_
		WHERE ur.USER_ID_ = ? AND r.ROLE_KEY_ = ?`,
		userID,
		string(models.RolePlatformAdmin),
	)
	if err != nil {
		return fmt.Errorf("revoke platform admin: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("revoke platform admin rows affected: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) findUserForAdminRole(ctx context.Context, identifier string) (models.User, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return models.User{}, sql.ErrNoRows
	}

	row := s.db.QueryRowContext(ctx, userSelectList+`
		WHERE auth_user.USERNAME_ = ?
		   OR LOWER(auth_user.EMAIL_) = LOWER(?)`,
		identifier,
		identifier,
	)
	user, err := scanUser(row)
	if err != nil {
		return models.User{}, err
	}
	return user, nil
}
