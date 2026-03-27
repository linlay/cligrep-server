package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/linlay/cligrep-server/internal/config"
	"github.com/linlay/cligrep-server/internal/models"
)

type stubApp struct {
	health                   map[string]any
	sessionUser              models.User
	sessionErr               error
	googleAuthURL            string
	googleLoginErr           error
	localRegisterUser        models.User
	localRegisterToken       string
	localRegisterErr         error
	localLoginUser           models.User
	localLoginToken          string
	localLoginErr            error
	updatedProfileUser       models.User
	updateProfileErr         error
	createSessionToken       string
	favoritesRequestedUserID int64
	recordedAuthAttempts     []models.AuthLoginLog
}

func (s stubApp) Health(ctx context.Context) map[string]any {
	return s.health
}

func (stubApp) Homepage(ctx context.Context, sort string) (map[string]any, error) {
	return nil, nil
}

func (stubApp) GetCLI(ctx context.Context, slug string) (map[string]any, error) {
	return nil, nil
}

func (stubApp) ExecuteCLI(ctx context.Context, request models.ExecRequest) (models.ExecutionResult, error) {
	return models.ExecutionResult{}, nil
}

func (stubApp) ExecuteBuiltin(ctx context.Context, request models.BuiltinExecRequest) (models.BuiltinExecResponse, error) {
	return models.BuiltinExecResponse{}, nil
}

func (s stubApp) RegisterLocal(ctx context.Context, request models.LocalRegisterRequest, metadata models.SessionMetadata) (models.User, string, error) {
	if s.localRegisterErr != nil {
		return models.User{}, "", s.localRegisterErr
	}
	if s.localRegisterUser.ID != 0 {
		return s.localRegisterUser, s.localRegisterToken, nil
	}
	return models.User{ID: 11, Username: request.Username, DisplayName: request.DisplayName, AuthProvider: "local", IP: metadata.IP}, "local-register-token", nil
}

func (s stubApp) LoginLocal(ctx context.Context, request models.LocalLoginRequest, metadata models.SessionMetadata) (models.User, string, error) {
	if s.localLoginErr != nil {
		return models.User{}, "", s.localLoginErr
	}
	if s.localLoginUser.ID != 0 {
		return s.localLoginUser, s.localLoginToken, nil
	}
	return models.User{ID: 12, Username: request.Username, DisplayName: request.Username, AuthProvider: "local", IP: metadata.IP}, "local-login-token", nil
}

func (s stubApp) CreateSession(ctx context.Context, userID int64, metadata models.SessionMetadata) (string, error) {
	if s.createSessionToken != "" {
		return s.createSessionToken, nil
	}
	return "session-token", nil
}

func (s stubApp) SessionUser(ctx context.Context, sessionToken string) (models.User, error) {
	if s.sessionErr != nil {
		return models.User{}, s.sessionErr
	}
	return s.sessionUser, nil
}

func (stubApp) DeleteSession(ctx context.Context, sessionToken string) error {
	return nil
}

func (s stubApp) UpdateProfile(ctx context.Context, userID int64, request models.UpdateProfileRequest) (models.User, error) {
	if s.updateProfileErr != nil {
		return models.User{}, s.updateProfileErr
	}
	if s.updatedProfileUser.ID != 0 {
		return s.updatedProfileUser, nil
	}
	return models.User{ID: userID, Username: "alice", DisplayName: request.DisplayName, AuthProvider: "local", IP: "127.0.0.1"}, nil
}

func (s *stubApp) RecordAuthAttempt(ctx context.Context, entry models.AuthLoginLog) error {
	s.recordedAuthAttempts = append(s.recordedAuthAttempts, entry)
	return nil
}

func (s stubApp) GoogleAuthURL(state string) (string, error) {
	if s.googleAuthURL == "" {
		return "", models.ErrAuthNotConfigured
	}
	return s.googleAuthURL, nil
}

func (s stubApp) LoginWithGoogleCode(ctx context.Context, code string, metadata models.SessionMetadata) (models.User, string, error) {
	if s.googleLoginErr != nil {
		return models.User{}, "", s.googleLoginErr
	}
	return models.User{ID: 7, Username: "google:sub"}, "google-session", nil
}

func (s *stubApp) ListFavorites(ctx context.Context, userID int64) ([]models.CLI, error) {
	s.favoritesRequestedUserID = userID
	return nil, nil
}

func (stubApp) SetFavorite(ctx context.Context, request models.FavoriteMutation) error {
	return nil
}

func (stubApp) ListComments(ctx context.Context, cliSlug string) ([]models.Comment, error) {
	return nil, nil
}

func (stubApp) AddComment(ctx context.Context, request models.CommentMutation) (models.Comment, error) {
	return models.Comment{}, nil
}

func testConfig() config.Config {
	return config.Config{
		CORSOrigin:         "http://localhost:5173",
		AuthCookieName:     "cligrep_session",
		AuthSuccessURL:     "http://localhost:5173/",
		AuthFailureURL:     "http://localhost:5173/login",
		AuthCookieSameSite: http.SameSiteLaxMode,
		SessionTTL:         24 * time.Hour,
	}
}

func TestHandleHealthIncludesSandboxStatus(t *testing.T) {
	handler := NewHandler(&stubApp{
		health: map[string]any{
			"status":            "ok",
			"busyboxImage":      "busybox:1.36.1",
			"pythonImage":       "python:3.12-slim",
			"databaseHost":      "db.example.internal",
			"databasePort":      3306,
			"databaseName":      "app_database",
			"timestamp":         "2026-03-24T00:00:00Z",
			"commandTimeoutMs":  int64(4000),
			"singleLineOnly":    true,
			"runtimeKinds":      []string{"SANDBOX", "WEBSITE", "TEXT"},
			"homepageSortModes": []string{"favorites", "newest", "runs"},
			"sandboxReady":      true,
			"sandbox": map[string]any{
				"dockerCli":    true,
				"dockerDaemon": true,
				"busyboxImage": true,
				"pythonImage":  true,
				"ready":        true,
				"issues":       []string{},
			},
			"auth": map[string]any{
				"google": map[string]any{
					"configured":         true,
					"clientIdConfigured": true,
					"redirectUrl":        "https://api.example.com/api/v1/auth/google/callback",
					"successUrl":         "https://app.example.com/",
					"failureUrl":         "https://app.example.com/login?error=google_oauth",
				},
			},
		},
	}, testConfig())

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	for _, key := range []string{
		"status",
		"busyboxImage",
		"pythonImage",
		"databaseHost",
		"databasePort",
		"databaseName",
		"timestamp",
		"commandTimeoutMs",
		"singleLineOnly",
		"runtimeKinds",
		"homepageSortModes",
		"sandboxReady",
		"sandbox",
		"auth",
	} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("expected key %q in payload", key)
		}
	}
}

func TestHandleGoogleStartRedirectsAndSetsStateCookie(t *testing.T) {
	handler := NewHandler(&stubApp{googleAuthURL: "https://accounts.google.com/o/oauth2/v2/auth?client_id=test"}, testConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/start", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected redirect, got %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); location == "" {
		t.Fatal("expected redirect location")
	}

	found := false
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == "cligrep_session_oauth_state" && cookie.Value != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected oauth state cookie to be set")
	}
}

func TestHandleMeReturnsSessionUser(t *testing.T) {
	handler := NewHandler(&stubApp{
		sessionUser: models.User{ID: 42, Username: "google:sub", AuthProvider: "google"},
	}, testConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "cligrep_session", Value: "token"})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleMeReturnsUnauthorizedWithoutSession(t *testing.T) {
	handler := NewHandler(&stubApp{sessionErr: errors.New("missing")}, testConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandleLocalRegisterSetsSessionCookie(t *testing.T) {
	handler := NewHandler(&stubApp{
		localRegisterUser:  models.User{ID: 21, Username: "alice", DisplayName: "Alice", AuthProvider: "local", IP: "127.0.0.1"},
		localRegisterToken: "local-register-token",
	}, testConfig())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/local/register", strings.NewReader(`{"username":"alice","password":"password123","displayName":"Alice"}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("expected session cookie")
	}
}

func TestHandleLocalLoginReturnsUnauthorizedOnInvalidCredentials(t *testing.T) {
	handler := NewHandler(&stubApp{
		localLoginErr: models.ErrInvalidCredentials,
	}, testConfig())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/local/login", strings.NewReader(`{"username":"alice","password":"wrong"}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandleMePatchUpdatesDisplayName(t *testing.T) {
	handler := NewHandler(&stubApp{
		sessionUser:        models.User{ID: 5, Username: "alice", DisplayName: "Alice", AuthProvider: "local"},
		updatedProfileUser: models.User{ID: 5, Username: "alice", DisplayName: "Alice Chen", AuthProvider: "local", IP: "127.0.0.1"},
	}, testConfig())

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/auth/me", strings.NewReader(`{"displayName":"Alice Chen"}`))
	req.AddCookie(&http.Cookie{Name: "cligrep_session", Value: "token"})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandleGoogleCallbackRedirectsWithAuthReason(t *testing.T) {
	handler := NewHandler(&stubApp{
		googleLoginErr: models.NewAuthFailureError(
			models.AuthFailureReasonGoogleJWKSFetchFailed,
			errors.New("dial tcp timeout"),
		),
	}, testConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/callback?state=expected-state&code=test-code", nil)
	req.AddCookie(&http.Cookie{Name: "cligrep_session_oauth_state", Value: "expected-state"})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected redirect, got %d", rec.Code)
	}

	location := rec.Header().Get("Location")
	if location == "" {
		t.Fatal("expected redirect location")
	}
	if !strings.Contains(location, "authError=google_callback_failed") {
		t.Fatalf("expected authError in redirect, got %q", location)
	}
	if !strings.Contains(location, "authReason=google_jwks_fetch_failed") {
		t.Fatalf("expected authReason in redirect, got %q", location)
	}
}

func TestFavoritesPreferSessionUser(t *testing.T) {
	app := &stubApp{
		sessionUser: models.User{ID: 15, Username: "google:sub", AuthProvider: "google"},
	}
	handler := NewHandler(app, testConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/favorites?userId=99", nil)
	req.AddCookie(&http.Cookie{Name: "cligrep_session", Value: "token"})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if app.favoritesRequestedUserID != 15 {
		t.Fatalf("expected session user id 15, got %d", app.favoritesRequestedUserID)
	}
}

func TestRemovedMockAuthRoutesReturnNotFound(t *testing.T) {
	handler := NewHandler(&stubApp{}, testConfig())

	for _, path := range []string{
		"/api/v1/clis/search",
		"/api/v1/auth/mock/anonymous",
		"/api/v1/auth/mock/login",
		"/api/v1/auth/mock/logout",
	} {
		method := http.MethodPost
		if path == "/api/v1/clis/search" {
			method = http.MethodGet
		}
		req := httptest.NewRequest(method, path, nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("path %s expected 404, got %d", path, rec.Code)
		}
	}
}
