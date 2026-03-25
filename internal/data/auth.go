package data

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"

	"github.com/linlay/cligrep-server/internal/models"
)

const userSelectList = `SELECT auth_user.ID_, auth_user.USERNAME_, auth_user.DISPLAY_NAME_, auth_user.EMAIL_, auth_user.AVATAR_URL_, auth_user.AUTH_PROVIDER_, auth_user.IP_, auth_user.CREATED_AT_ FROM auth_user`

var localUsernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]{3,32}$`)

type sqlExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (s *Store) LoginMock(ctx context.Context, username string) (models.User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		username = "operator"
	}

	ip := generateMockIP(username)
	now := time.Now().UTC()

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO auth_user (USERNAME_, DISPLAY_NAME_, EMAIL_, AVATAR_URL_, AUTH_PROVIDER_, AUTH_SUB_, IP_, CREATED_AT_, UPDATED_AT_, LAST_LOGIN_AT_)
		VALUES (?, ?, '', '', 'mock', NULL, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			DISPLAY_NAME_ = VALUES(DISPLAY_NAME_),
			AUTH_PROVIDER_ = 'mock',
			AUTH_SUB_ = NULL,
			IP_ = VALUES(IP_),
			UPDATED_AT_ = VALUES(UPDATED_AT_),
			LAST_LOGIN_AT_ = VALUES(LAST_LOGIN_AT_)`,
		username,
		username,
		ip,
		now,
		now,
		now,
	); err != nil {
		return models.User{}, fmt.Errorf("upsert mock user: %w", err)
	}

	row := s.db.QueryRowContext(ctx, userSelectList+` WHERE USERNAME_ = ?`, username)
	return scanUser(row)
}

func (s *Store) UpsertGoogleUser(ctx context.Context, subject, email, name, picture, ip string) (models.User, error) {
	subject = strings.TrimSpace(subject)
	email = strings.TrimSpace(email)
	name = strings.TrimSpace(name)
	picture = strings.TrimSpace(picture)
	ip = strings.TrimSpace(ip)
	if subject == "" {
		return models.User{}, fmt.Errorf("google subject is required")
	}

	displayName := name
	if displayName == "" {
		displayName = email
	}
	if displayName == "" {
		displayName = "google-user"
	}

	now := time.Now().UTC()
	username := "google:" + subject

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO auth_user (USERNAME_, DISPLAY_NAME_, EMAIL_, AVATAR_URL_, AUTH_PROVIDER_, AUTH_SUB_, IP_, CREATED_AT_, UPDATED_AT_, LAST_LOGIN_AT_)
		VALUES (?, ?, ?, ?, 'google', ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			USERNAME_ = VALUES(USERNAME_),
			DISPLAY_NAME_ = VALUES(DISPLAY_NAME_),
			EMAIL_ = VALUES(EMAIL_),
			AVATAR_URL_ = VALUES(AVATAR_URL_),
			IP_ = VALUES(IP_),
			UPDATED_AT_ = VALUES(UPDATED_AT_),
			LAST_LOGIN_AT_ = VALUES(LAST_LOGIN_AT_)`,
		username,
		displayName,
		email,
		picture,
		subject,
		ip,
		now,
		now,
		now,
	); err != nil {
		return models.User{}, fmt.Errorf("upsert google user: %w", err)
	}

	row := s.db.QueryRowContext(ctx, userSelectList+` WHERE AUTH_PROVIDER_ = 'google' AND AUTH_SUB_ = ?`, subject)
	return scanUser(row)
}

func (s *Store) RegisterLocal(ctx context.Context, request models.LocalRegisterRequest, metadata models.SessionMetadata, ttl time.Duration) (models.User, string, error) {
	username := strings.TrimSpace(request.Username)
	if err := validateLocalUsername(username); err != nil {
		_ = s.RecordAuthAttempt(ctx, models.AuthLoginLog{
			Username:      username,
			DisplayName:   strings.TrimSpace(request.DisplayName),
			AuthMethod:    models.AuthMethodLocalPassword,
			LoginResult:   models.AuthResultFailure,
			FailureReason: "invalid_username",
			IP:            strings.TrimSpace(metadata.IP),
			UserAgent:     strings.TrimSpace(metadata.UserAgent),
			LoginAt:       time.Now().UTC(),
		})
		return models.User{}, "", err
	}

	if err := validateLocalPassword(request.Password); err != nil {
		_ = s.RecordAuthAttempt(ctx, models.AuthLoginLog{
			Username:      username,
			DisplayName:   strings.TrimSpace(request.DisplayName),
			AuthMethod:    models.AuthMethodLocalPassword,
			LoginResult:   models.AuthResultFailure,
			FailureReason: "weak_password",
			IP:            strings.TrimSpace(metadata.IP),
			UserAgent:     strings.TrimSpace(metadata.UserAgent),
			LoginAt:       time.Now().UTC(),
		})
		return models.User{}, "", err
	}

	displayName := firstNonEmpty(strings.TrimSpace(request.DisplayName), username)
	passwordHash, err := hashPassword(request.Password)
	if err != nil {
		return models.User{}, "", fmt.Errorf("hash local password: %w", err)
	}

	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.User{}, "", fmt.Errorf("begin local register tx: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		INSERT INTO auth_user (USERNAME_, DISPLAY_NAME_, EMAIL_, AVATAR_URL_, AUTH_PROVIDER_, AUTH_SUB_, IP_, CREATED_AT_, UPDATED_AT_, LAST_LOGIN_AT_)
		VALUES (?, ?, '', '', 'local', NULL, ?, ?, ?, ?)`,
		username,
		displayName,
		strings.TrimSpace(metadata.IP),
		now,
		now,
		now,
	)
	if err != nil {
		if isDuplicateEntryError(err) {
			_ = s.RecordAuthAttempt(ctx, models.AuthLoginLog{
				Username:      username,
				DisplayName:   displayName,
				AuthMethod:    models.AuthMethodLocalPassword,
				LoginResult:   models.AuthResultFailure,
				FailureReason: "duplicate_username",
				IP:            strings.TrimSpace(metadata.IP),
				UserAgent:     strings.TrimSpace(metadata.UserAgent),
				LoginAt:       now,
			})
			return models.User{}, "", models.ErrUsernameTaken
		}
		return models.User{}, "", fmt.Errorf("insert local user: %w", err)
	}

	userID, err := result.LastInsertId()
	if err != nil {
		return models.User{}, "", fmt.Errorf("local user id: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO auth_local_credential (USER_ID_, PASSWORD_HASH_, PASSWORD_UPDATED_AT_, CREATED_AT_)
		VALUES (?, ?, ?, ?)`,
		userID,
		passwordHash,
		now,
		now,
	); err != nil {
		return models.User{}, "", fmt.Errorf("insert local credential: %w", err)
	}

	sessionToken, err := createSessionWithExecer(ctx, tx, userID, metadata, ttl)
	if err != nil {
		return models.User{}, "", err
	}

	userIDCopy := userID
	if err := insertAuthAttemptWithExecer(ctx, tx, models.AuthLoginLog{
		UserID:      &userIDCopy,
		Username:    username,
		DisplayName: displayName,
		AuthMethod:  models.AuthMethodLocalPassword,
		LoginResult: models.AuthResultSuccess,
		IP:          strings.TrimSpace(metadata.IP),
		UserAgent:   strings.TrimSpace(metadata.UserAgent),
		LoginAt:     now,
	}); err != nil {
		return models.User{}, "", err
	}

	if err := tx.Commit(); err != nil {
		return models.User{}, "", fmt.Errorf("commit local register tx: %w", err)
	}

	user, err := s.GetUser(ctx, userID)
	if err != nil {
		return models.User{}, "", err
	}
	return user, sessionToken, nil
}

func (s *Store) LoginLocal(ctx context.Context, request models.LocalLoginRequest, metadata models.SessionMetadata, ttl time.Duration) (models.User, string, error) {
	username := strings.TrimSpace(request.Username)
	now := time.Now().UTC()
	if username == "" || strings.TrimSpace(request.Password) == "" {
		_ = s.RecordAuthAttempt(ctx, models.AuthLoginLog{
			Username:      username,
			AuthMethod:    models.AuthMethodLocalPassword,
			LoginResult:   models.AuthResultFailure,
			FailureReason: "invalid_credentials",
			IP:            strings.TrimSpace(metadata.IP),
			UserAgent:     strings.TrimSpace(metadata.UserAgent),
			LoginAt:       now,
		})
		return models.User{}, "", models.ErrInvalidCredentials
	}

	var (
		user         models.User
		passwordHash string
		createdAt    time.Time
	)
	row := s.db.QueryRowContext(ctx, `
		SELECT auth_user.ID_, auth_user.USERNAME_, auth_user.DISPLAY_NAME_, auth_user.EMAIL_, auth_user.AVATAR_URL_, auth_user.AUTH_PROVIDER_, auth_user.IP_, auth_user.CREATED_AT_, cred.PASSWORD_HASH_
		FROM auth_user
		JOIN auth_local_credential cred ON cred.USER_ID_ = auth_user.ID_
		WHERE auth_user.USERNAME_ = ? AND auth_user.AUTH_PROVIDER_ = 'local'`,
		username,
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
		&passwordHash,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_ = s.RecordAuthAttempt(ctx, models.AuthLoginLog{
				Username:      username,
				AuthMethod:    models.AuthMethodLocalPassword,
				LoginResult:   models.AuthResultFailure,
				FailureReason: "invalid_credentials",
				IP:            strings.TrimSpace(metadata.IP),
				UserAgent:     strings.TrimSpace(metadata.UserAgent),
				LoginAt:       now,
			})
			return models.User{}, "", models.ErrInvalidCredentials
		}
		return models.User{}, "", fmt.Errorf("load local credential: %w", err)
	}
	user.CreatedAt = createdAt.UTC()

	ok, err := verifyPasswordHash(passwordHash, request.Password)
	if err != nil {
		return models.User{}, "", fmt.Errorf("verify local password: %w", err)
	}
	if !ok {
		_ = s.RecordAuthAttempt(ctx, models.AuthLoginLog{
			UserID:        &user.ID,
			Username:      user.Username,
			DisplayName:   user.DisplayName,
			AuthMethod:    models.AuthMethodLocalPassword,
			LoginResult:   models.AuthResultFailure,
			FailureReason: "invalid_credentials",
			IP:            strings.TrimSpace(metadata.IP),
			UserAgent:     strings.TrimSpace(metadata.UserAgent),
			LoginAt:       now,
		})
		return models.User{}, "", models.ErrInvalidCredentials
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.User{}, "", fmt.Errorf("begin local login tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		UPDATE auth_user
		SET IP_ = ?, LAST_LOGIN_AT_ = ?
		WHERE ID_ = ?`,
		strings.TrimSpace(metadata.IP),
		now,
		user.ID,
	); err != nil {
		return models.User{}, "", fmt.Errorf("update local login metadata: %w", err)
	}

	sessionToken, err := createSessionWithExecer(ctx, tx, user.ID, metadata, ttl)
	if err != nil {
		return models.User{}, "", err
	}

	userIDCopy := user.ID
	if err := insertAuthAttemptWithExecer(ctx, tx, models.AuthLoginLog{
		UserID:      &userIDCopy,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		AuthMethod:  models.AuthMethodLocalPassword,
		LoginResult: models.AuthResultSuccess,
		IP:          strings.TrimSpace(metadata.IP),
		UserAgent:   strings.TrimSpace(metadata.UserAgent),
		LoginAt:     now,
	}); err != nil {
		return models.User{}, "", err
	}

	if err := tx.Commit(); err != nil {
		return models.User{}, "", fmt.Errorf("commit local login tx: %w", err)
	}

	user.IP = strings.TrimSpace(metadata.IP)
	return user, sessionToken, nil
}

func (s *Store) UpdateUserDisplayName(ctx context.Context, userID int64, displayName string) (models.User, error) {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return models.User{}, models.ErrInvalidDisplayName
	}

	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		UPDATE auth_user
		SET DISPLAY_NAME_ = ?, UPDATED_AT_ = ?
		WHERE ID_ = ?`,
		displayName,
		now,
		userID,
	)
	if err != nil {
		return models.User{}, fmt.Errorf("update display name: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return models.User{}, fmt.Errorf("display name rows affected: %w", err)
	}
	if rows == 0 {
		return models.User{}, sql.ErrNoRows
	}

	return s.GetUser(ctx, userID)
}

func (s *Store) RecordAuthAttempt(ctx context.Context, logEntry models.AuthLoginLog) error {
	return insertAuthAttemptWithExecer(ctx, s.db, logEntry)
}

func insertAuthAttemptWithExecer(ctx context.Context, execer sqlExecer, logEntry models.AuthLoginLog) error {
	loginAt := logEntry.LoginAt.UTC()
	if logEntry.LoginAt.IsZero() {
		loginAt = time.Now().UTC()
	}
	_, err := execer.ExecContext(ctx, `
		INSERT INTO auth_login_log (USER_ID_, USERNAME_, DISPLAY_NAME_, AUTH_METHOD_, LOGIN_RESULT_, FAILURE_REASON_, IP_, USER_AGENT_, LOGIN_AT_)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		logEntry.UserID,
		strings.TrimSpace(logEntry.Username),
		strings.TrimSpace(logEntry.DisplayName),
		string(logEntry.AuthMethod),
		string(logEntry.LoginResult),
		strings.TrimSpace(logEntry.FailureReason),
		strings.TrimSpace(logEntry.IP),
		strings.TrimSpace(logEntry.UserAgent),
		loginAt,
	)
	if err != nil {
		return fmt.Errorf("insert auth login log: %w", err)
	}
	return nil
}

func (s *Store) GetUser(ctx context.Context, userID int64) (models.User, error) {
	row := s.db.QueryRowContext(ctx, userSelectList+` WHERE ID_ = ?`, userID)
	return scanUser(row)
}

func (s *Store) CreateSession(ctx context.Context, userID int64, metadata models.SessionMetadata, ttl time.Duration) (string, error) {
	return createSessionWithExecer(ctx, s.db, userID, metadata, ttl)
}

func createSessionWithExecer(ctx context.Context, execer sqlExecer, userID int64, metadata models.SessionMetadata, ttl time.Duration) (string, error) {
	if userID <= 0 {
		return "", fmt.Errorf("valid user id is required")
	}
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}

	plainToken, err := generateSessionToken()
	if err != nil {
		return "", err
	}

	now := time.Now().UTC()
	_, err = execer.ExecContext(ctx, `
		INSERT INTO auth_session (USER_ID_, TOKEN_HASH_, EXPIRES_AT_, CREATED_AT_, LAST_SEEN_AT_, USER_AGENT_, IP_)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID,
		hashToken(plainToken),
		now.Add(ttl),
		now,
		now,
		strings.TrimSpace(metadata.UserAgent),
		strings.TrimSpace(metadata.IP),
	)
	if err != nil {
		return "", fmt.Errorf("create auth session: %w", err)
	}

	return plainToken, nil
}

func (s *Store) GetUserBySessionToken(ctx context.Context, sessionToken string) (models.User, error) {
	sessionToken = strings.TrimSpace(sessionToken)
	if sessionToken == "" {
		return models.User{}, models.ErrUnauthorized
	}

	now := time.Now().UTC()
	row := s.db.QueryRowContext(ctx, userSelectList+`
		JOIN auth_session sess ON sess.USER_ID_ = auth_user.ID_
		WHERE sess.TOKEN_HASH_ = ? AND sess.EXPIRES_AT_ > ?`,
		hashToken(sessionToken),
		now,
	)
	user, err := scanUser(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return models.User{}, models.ErrUnauthorized
		}
		return models.User{}, fmt.Errorf("load user from auth session: %w", err)
	}

	if _, err := s.db.ExecContext(ctx, `
		UPDATE auth_session
		SET LAST_SEEN_AT_ = ?
		WHERE TOKEN_HASH_ = ?`,
		now,
		hashToken(sessionToken),
	); err != nil {
		return models.User{}, fmt.Errorf("touch auth session: %w", err)
	}

	return user, nil
}

func (s *Store) DeleteSession(ctx context.Context, sessionToken string) error {
	sessionToken = strings.TrimSpace(sessionToken)
	if sessionToken == "" {
		return nil
	}

	if _, err := s.db.ExecContext(ctx, `DELETE FROM auth_session WHERE TOKEN_HASH_ = ?`, hashToken(sessionToken)); err != nil {
		return fmt.Errorf("delete auth session: %w", err)
	}
	return nil
}

func validateLocalUsername(username string) error {
	if !localUsernamePattern.MatchString(strings.TrimSpace(username)) {
		return models.ErrInvalidUsername
	}
	return nil
}

func validateLocalPassword(password string) error {
	if len(strings.TrimSpace(password)) < 8 {
		return models.ErrWeakPassword
	}
	return nil
}

func isDuplicateEntryError(err error) bool {
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "duplicate")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func generateSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashToken(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
