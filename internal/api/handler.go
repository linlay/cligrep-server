package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/linlay/cligrep-server/internal/config"
	appi18n "github.com/linlay/cligrep-server/internal/i18n"
	"github.com/linlay/cligrep-server/internal/models"
)

type Handler struct {
	app            application
	mux            *http.ServeMux
	corsOrigins    []string
	cookieName     string
	cookieSecure   bool
	cookieDomain   string
	cookieSameSite http.SameSite
	sessionTTL     time.Duration
	authSuccessURL string
	authFailureURL string
	googleRedirect string
}

type application interface {
	Health(ctx context.Context) map[string]any
	Homepage(ctx context.Context, sort string) (map[string]any, error)
	Search(ctx context.Context, query string) (map[string]any, error)
	GetCLI(ctx context.Context, slug string) (map[string]any, error)
	ExecuteCLI(ctx context.Context, request models.ExecRequest) (models.ExecutionResult, error)
	ExecuteBuiltin(ctx context.Context, request models.BuiltinExecRequest) (models.BuiltinExecResponse, error)
	RegisterLocal(ctx context.Context, request models.LocalRegisterRequest, metadata models.SessionMetadata) (models.User, string, error)
	LoginLocal(ctx context.Context, request models.LocalLoginRequest, metadata models.SessionMetadata) (models.User, string, error)
	CreateSession(ctx context.Context, userID int64, metadata models.SessionMetadata) (string, error)
	SessionUser(ctx context.Context, sessionToken string) (models.User, error)
	DeleteSession(ctx context.Context, sessionToken string) error
	UpdateProfile(ctx context.Context, userID int64, request models.UpdateProfileRequest) (models.User, error)
	RecordAuthAttempt(ctx context.Context, entry models.AuthLoginLog) error
	GoogleAuthURL(state string) (string, error)
	LoginWithGoogleCode(ctx context.Context, code string, metadata models.SessionMetadata) (models.User, string, error)
	ListFavorites(ctx context.Context, userID int64) ([]models.CLI, error)
	SetFavorite(ctx context.Context, request models.FavoriteMutation) error
	ListComments(ctx context.Context, cliSlug string) ([]models.Comment, error)
	AddComment(ctx context.Context, request models.CommentMutation) (models.Comment, error)
	AdminMe(ctx context.Context, user models.User) models.AdminMe
	ListAdminCLIs(ctx context.Context, user models.User) ([]models.CLI, error)
	GetAdminCLI(ctx context.Context, user models.User, slug string) (map[string]any, error)
	CreateAdminCLI(ctx context.Context, user models.User, request models.AdminCLIUpsertRequest) (models.CLI, error)
	UpdateAdminCLI(ctx context.Context, user models.User, slug string, request models.AdminCLIUpsertRequest) (models.CLI, error)
	PublishAdminCLI(ctx context.Context, user models.User, slug string) (models.CLI, error)
	UnpublishAdminCLI(ctx context.Context, user models.User, slug string) (models.CLI, error)
	DeleteAdminCLI(ctx context.Context, user models.User, slug string) error
	CreateAdminRelease(ctx context.Context, user models.User, slug string, request models.AdminReleaseUpsertRequest) (models.CLIRelease, error)
	UpdateAdminRelease(ctx context.Context, user models.User, slug, version string, request models.AdminReleaseUpsertRequest) (models.CLIRelease, error)
	DeleteAdminRelease(ctx context.Context, user models.User, slug, version string) error
	UploadAdminReleaseAsset(ctx context.Context, user models.User, slug, version string, metadata models.CLIReleaseAsset, reader io.Reader) (models.CLIReleaseAsset, error)
	DeleteAdminReleaseAsset(ctx context.Context, user models.User, slug, version string, assetID int64) error
}

func NewHandler(application application, cfg config.Config) http.Handler {
	handler := &Handler{
		app:            application,
		mux:            http.NewServeMux(),
		corsOrigins:    parseCORSOrigins(cfg.CORSOrigin),
		cookieName:     cfg.AuthCookieName,
		cookieSecure:   cfg.AuthCookieSecure,
		cookieDomain:   cfg.AuthCookieDomain,
		cookieSameSite: cfg.AuthCookieSameSite,
		sessionTTL:     cfg.SessionTTL,
		authSuccessURL: cfg.AuthSuccessURL,
		authFailureURL: cfg.AuthFailureURL,
		googleRedirect: cfg.GoogleRedirect,
	}
	handler.routes()
	return handler.withMiddleware(handler.mux)
}

func (h *Handler) routes() {
	h.mux.HandleFunc("/healthz", h.handleHealth)
	h.mux.HandleFunc("/api/v1/clis/trending", h.handleTrending)
	h.mux.HandleFunc("/api/v1/clis/search", h.handleSearch)
	h.mux.HandleFunc("/api/v1/clis/", h.handleCLIBySlug)
	h.mux.HandleFunc("/api/v1/exec", h.handleExec)
	h.mux.HandleFunc("/api/v1/builtin/exec", h.handleBuiltinExec)
	h.mux.HandleFunc("/api/v1/auth/google/start", h.handleGoogleStart)
	h.mux.HandleFunc("/api/v1/auth/google/callback", h.handleGoogleCallback)
	h.mux.HandleFunc("/api/v1/auth/local/register", h.handleLocalRegister)
	h.mux.HandleFunc("/api/v1/auth/local/login", h.handleLocalLogin)
	h.mux.HandleFunc("/api/v1/auth/me", h.handleMe)
	h.mux.HandleFunc("/api/v1/auth/logout", h.handleLogout)
	h.mux.HandleFunc("/api/v1/favorites", h.handleFavorites)
	h.mux.HandleFunc("/api/v1/comments", h.handleComments)
	h.mux.HandleFunc("/api/v1/admin/me", h.handleAdminMe)
	h.mux.HandleFunc("/api/v1/admin/clis", h.handleAdminCLIs)
	h.mux.HandleFunc("/api/v1/admin/clis/", h.handleAdminCLIBySlug)
}

func (h *Handler) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := h.allowedOrigin(r.Header.Get("Origin")); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin, Accept-Language, X-CLIGREP-Locale, X-CLIGREP-Timezone")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-CLIGREP-Locale, X-CLIGREP-Timezone")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,DELETE,OPTIONS")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		r = r.WithContext(requestContext(r))
		if user, err := h.lookupSessionUser(r); err == nil {
			r = r.WithContext(withCurrentUser(r.Context(), user))
		}

		next.ServeHTTP(w, r)
	})
}

func parseCORSOrigins(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{"*"}
	}

	parts := strings.Split(value, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		origin := strings.TrimSpace(part)
		if origin == "" {
			continue
		}
		origins = append(origins, origin)
	}
	if len(origins) == 0 {
		return []string{"*"}
	}
	return origins
}

func (h *Handler) allowedOrigin(requestOrigin string) string {
	if len(h.corsOrigins) == 0 {
		return requestOrigin
	}
	for _, allowed := range h.corsOrigins {
		if allowed == "*" {
			return requestOrigin
		}
		if requestOrigin != "" && requestOrigin == allowed {
			return allowed
		}
	}
	if requestOrigin == "" && len(h.corsOrigins) == 1 {
		return h.corsOrigins[0]
	}
	return ""
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeCatalogError(w, r.Context(), http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	writeJSON(w, http.StatusOK, h.app.Health(r.Context()))
}

func (h *Handler) handleTrending(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeCatalogError(w, r.Context(), http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}

	payload, err := h.app.Homepage(r.Context(), r.URL.Query().Get("sort"))
	if err != nil {
		writeLocalizedError(w, r.Context(), http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeCatalogError(w, r.Context(), http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}

	payload, err := h.app.Search(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		writeLocalizedError(w, r.Context(), http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func (h *Handler) handleCLIBySlug(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeCatalogError(w, r.Context(), http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}

	slug := strings.TrimPrefix(r.URL.Path, "/api/v1/clis/")
	if slug == "" {
		writeCatalogError(w, r.Context(), http.StatusBadRequest, "missing_cli_slug")
		return
	}
	if slug == "search" {
		writeCatalogError(w, r.Context(), http.StatusNotFound, "not_found")
		return
	}

	payload, err := h.app.GetCLI(r.Context(), slug)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeLocalizedError(w, r.Context(), status, err)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func (h *Handler) handleExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeCatalogError(w, r.Context(), http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}

	var request models.ExecRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeCatalogError(w, r.Context(), http.StatusBadRequest, "invalid_json_body")
		return
	}
	request.Locale = appi18n.LocaleFromContext(r.Context())
	request.Timezone = appi18n.TimezoneFromContext(r.Context())
	if user, ok := currentUserFromContext(r.Context()); ok {
		request.UserID = &user.ID
	}

	result, err := h.app.ExecuteCLI(r.Context(), request)
	if err != nil {
		writeLocalizedError(w, r.Context(), http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) handleBuiltinExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeCatalogError(w, r.Context(), http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}

	var request models.BuiltinExecRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeCatalogError(w, r.Context(), http.StatusBadRequest, "invalid_json_body")
		return
	}
	request.Locale = appi18n.LocaleFromContext(r.Context())
	request.Timezone = appi18n.TimezoneFromContext(r.Context())
	if user, ok := currentUserFromContext(r.Context()); ok {
		request.UserID = &user.ID
	}

	response, err := h.app.ExecuteBuiltin(r.Context(), request)
	if err != nil {
		writeLocalizedError(w, r.Context(), http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) handleGoogleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeCatalogError(w, r.Context(), http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}

	state, err := generateNonce()
	if err != nil {
		writeCatalogError(w, r.Context(), http.StatusInternalServerError, "failed_init_auth_state")
		return
	}

	authURL, err := h.app.GoogleAuthURL(state)
	if err != nil {
		h.redirectFailure(w, r, "google_auth_not_configured")
		return
	}

	http.SetCookie(w, h.newCookie(h.oauthStateCookieName(), state, 10*time.Minute))
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (h *Handler) handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeCatalogError(w, r.Context(), http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}

	stateCookie, err := r.Cookie(h.oauthStateCookieName())
	if err != nil || stateCookie.Value == "" {
		_ = h.app.RecordAuthAttempt(r.Context(), models.AuthLoginLog{
			AuthMethod:    models.AuthMethodGoogle,
			LoginResult:   models.AuthResultFailure,
			FailureReason: "missing_state",
			IP:            requestIP(r),
			UserAgent:     strings.TrimSpace(r.UserAgent()),
			LoginAt:       time.Now().UTC(),
		})
		h.redirectFailure(w, r, "missing_state")
		return
	}
	if stateCookie.Value != r.URL.Query().Get("state") {
		h.clearOAuthStateCookie(w)
		_ = h.app.RecordAuthAttempt(r.Context(), models.AuthLoginLog{
			AuthMethod:    models.AuthMethodGoogle,
			LoginResult:   models.AuthResultFailure,
			FailureReason: "invalid_state",
			IP:            requestIP(r),
			UserAgent:     strings.TrimSpace(r.UserAgent()),
			LoginAt:       time.Now().UTC(),
		})
		h.redirectFailure(w, r, "invalid_state")
		return
	}

	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		h.clearOAuthStateCookie(w)
		_ = h.app.RecordAuthAttempt(r.Context(), models.AuthLoginLog{
			AuthMethod:    models.AuthMethodGoogle,
			LoginResult:   models.AuthResultFailure,
			FailureReason: "missing_code",
			IP:            requestIP(r),
			UserAgent:     strings.TrimSpace(r.UserAgent()),
			LoginAt:       time.Now().UTC(),
		})
		h.redirectFailure(w, r, "missing_code")
		return
	}

	user, sessionToken, err := h.app.LoginWithGoogleCode(r.Context(), code, requestMetadata(r))
	if err != nil {
		h.clearOAuthStateCookie(w)
		authReason := string(models.AuthFailureReasonFromError(err))
		log.Printf(
			"google callback failed: reason=%s err=%v redirect_uri=%s host=%s path=%s has_code=%t state_valid=%t",
			authReason,
			err,
			h.googleRedirect,
			r.Host,
			r.URL.Path,
			code != "",
			true,
		)
		h.redirectFailure(w, r, "google_callback_failed", authReason)
		return
	}

	h.setSessionCookie(w, sessionToken)
	h.clearOAuthStateCookie(w)
	http.Redirect(w, r, h.authSuccessURLForUser(user), http.StatusFound)
}

func (h *Handler) handleLocalRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeCatalogError(w, r.Context(), http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}

	var request models.LocalRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeCatalogError(w, r.Context(), http.StatusBadRequest, "invalid_json_body")
		return
	}

	user, sessionToken, err := h.app.RegisterLocal(r.Context(), request, requestMetadata(r))
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, models.ErrUsernameTaken):
			status = http.StatusConflict
		case errors.Is(err, models.ErrInvalidUsername), errors.Is(err, models.ErrWeakPassword), errors.Is(err, models.ErrInvalidDisplayName):
			status = http.StatusBadRequest
		}
		writeLocalizedError(w, r.Context(), status, err)
		return
	}

	h.setSessionCookie(w, sessionToken)
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (h *Handler) handleLocalLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeCatalogError(w, r.Context(), http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}

	var request models.LocalLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeCatalogError(w, r.Context(), http.StatusBadRequest, "invalid_json_body")
		return
	}

	user, sessionToken, err := h.app.LoginLocal(r.Context(), request, requestMetadata(r))
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, models.ErrInvalidCredentials):
			status = http.StatusUnauthorized
		case errors.Is(err, models.ErrInvalidUsername), errors.Is(err, models.ErrWeakPassword), errors.Is(err, models.ErrInvalidDisplayName):
			status = http.StatusBadRequest
		}
		writeLocalizedError(w, r.Context(), status, err)
		return
	}

	h.setSessionCookie(w, sessionToken)
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (h *Handler) handleMe(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		user, ok := currentUserFromContext(r.Context())
		if !ok {
			writeLocalizedError(w, r.Context(), http.StatusUnauthorized, models.ErrUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"user": user})
	case http.MethodPatch:
		user, ok := currentUserFromContext(r.Context())
		if !ok {
			writeLocalizedError(w, r.Context(), http.StatusUnauthorized, models.ErrUnauthorized)
			return
		}
		var request models.UpdateProfileRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeCatalogError(w, r.Context(), http.StatusBadRequest, "invalid_json_body")
			return
		}
		updated, err := h.app.UpdateProfile(r.Context(), user.ID, request)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, sql.ErrNoRows) {
				status = http.StatusNotFound
			}
			writeLocalizedError(w, r.Context(), status, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"user": updated})
	default:
		writeCatalogError(w, r.Context(), http.StatusMethodNotAllowed, "method_not_allowed")
	}
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeCatalogError(w, r.Context(), http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}

	h.deleteCurrentSession(r)
	h.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *Handler) handleFavorites(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		userID, err := userIDFromRequest(r, r.URL.Query().Get("userId"))
		if err != nil {
			writeLocalizedError(w, r.Context(), http.StatusBadRequest, err)
			return
		}
		items, err := h.app.ListFavorites(r.Context(), userID)
		if err != nil {
			writeLocalizedError(w, r.Context(), http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		var request models.FavoriteMutation
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeCatalogError(w, r.Context(), http.StatusBadRequest, "invalid_json_body")
			return
		}
		if user, ok := currentUserFromContext(r.Context()); ok {
			request.UserID = user.ID
		}
		if err := h.app.SetFavorite(r.Context(), request); err != nil {
			writeLocalizedError(w, r.Context(), http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	default:
		writeCatalogError(w, r.Context(), http.StatusMethodNotAllowed, "method_not_allowed")
	}
}

func (h *Handler) handleComments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cliSlug := strings.TrimSpace(r.URL.Query().Get("cliSlug"))
		if cliSlug == "" {
			writeCatalogError(w, r.Context(), http.StatusBadRequest, "cli_slug_required")
			return
		}
		comments, err := h.app.ListComments(r.Context(), cliSlug)
		if err != nil {
			writeLocalizedError(w, r.Context(), http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": comments})
	case http.MethodPost:
		var request models.CommentMutation
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeCatalogError(w, r.Context(), http.StatusBadRequest, "invalid_json_body")
			return
		}
		if user, ok := currentUserFromContext(r.Context()); ok {
			request.UserID = user.ID
		}
		comment, err := h.app.AddComment(r.Context(), request)
		if err != nil {
			writeLocalizedError(w, r.Context(), http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"item": comment})
	default:
		writeCatalogError(w, r.Context(), http.StatusMethodNotAllowed, "method_not_allowed")
	}
}

type contextKey string

const currentUserKey contextKey = "current-user"

func withCurrentUser(ctx context.Context, user models.User) context.Context {
	return context.WithValue(ctx, currentUserKey, user)
}

func currentUserFromContext(ctx context.Context) (models.User, bool) {
	user, ok := ctx.Value(currentUserKey).(models.User)
	return user, ok
}

func userIDFromRequest(r *http.Request, raw string) (int64, error) {
	if user, ok := currentUserFromContext(r.Context()); ok {
		return user.ID, nil
	}
	return parseUserID(raw)
}

func requestMetadata(r *http.Request) models.SessionMetadata {
	return models.SessionMetadata{
		IP:        requestIP(r),
		UserAgent: strings.TrimSpace(r.UserAgent()),
	}
}

func requestContext(r *http.Request) context.Context {
	locale := appi18n.ParseLocale(
		r.Header.Get("X-CLIGREP-Locale"),
		r.Header.Get("Accept-Language"),
	)
	timezone := appi18n.NormalizeTimezone(r.Header.Get("X-CLIGREP-Timezone"))
	return appi18n.WithRequestContext(r.Context(), locale, timezone)
}

func requestIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func (h *Handler) lookupSessionUser(r *http.Request) (models.User, error) {
	cookie, err := r.Cookie(h.cookieName)
	if err != nil {
		return models.User{}, err
	}
	return h.app.SessionUser(r.Context(), cookie.Value)
}

func (h *Handler) newCookie(name, value string, maxAge time.Duration) *http.Cookie {
	cookie := &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: h.cookieSameSite,
	}
	if h.cookieDomain != "" {
		cookie.Domain = h.cookieDomain
	}
	if maxAge > 0 {
		cookie.Expires = time.Now().Add(maxAge)
		cookie.MaxAge = int(maxAge.Seconds())
	}
	return cookie
}

func (h *Handler) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, h.newCookie(h.cookieName, token, h.sessionTTL))
}

func (h *Handler) clearSessionCookie(w http.ResponseWriter) {
	cookie := h.newCookie(h.cookieName, "", -time.Hour)
	cookie.MaxAge = -1
	http.SetCookie(w, cookie)
}

func (h *Handler) clearOAuthStateCookie(w http.ResponseWriter) {
	cookie := h.newCookie(h.oauthStateCookieName(), "", -time.Hour)
	cookie.MaxAge = -1
	http.SetCookie(w, cookie)
}

func (h *Handler) oauthStateCookieName() string {
	return h.cookieName + "_oauth_state"
}

func generateNonce() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (h *Handler) deleteCurrentSession(r *http.Request) {
	cookie, err := r.Cookie(h.cookieName)
	if err != nil || cookie.Value == "" {
		return
	}
	_ = h.app.DeleteSession(r.Context(), cookie.Value)
}

func (h *Handler) redirectFailure(w http.ResponseWriter, r *http.Request, reason string, authReason ...string) {
	h.clearSessionCookie(w)
	sanitizedAuthReason := ""
	if len(authReason) > 0 {
		sanitizedAuthReason = strings.TrimSpace(authReason[0])
	}
	http.Redirect(w, r, appendAuthFailure(h.authFailureURL, reason, sanitizedAuthReason), http.StatusFound)
}

func (h *Handler) authSuccessURLForUser(user models.User) string {
	return h.authSuccessURL
}

func appendAuthFailure(rawURL, reason, authReason string) string {
	if strings.TrimSpace(rawURL) == "" {
		return "/"
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	query := parsed.Query()
	if strings.TrimSpace(reason) != "" {
		query.Set("authError", reason)
	}
	if strings.TrimSpace(authReason) != "" {
		query.Set("authReason", authReason)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func parseUserID(raw string) (int64, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, errors.New("valid userId is required")
	}
	return value, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"error": message,
	})
}

func writeCatalogError(w http.ResponseWriter, ctx context.Context, status int, key string, args ...any) {
	writeError(w, status, appi18n.Text(ctx, key, args...))
}

func writeLocalizedError(w http.ResponseWriter, ctx context.Context, status int, err error) {
	writeError(w, status, appi18n.LocalizeError(ctx, err))
}
