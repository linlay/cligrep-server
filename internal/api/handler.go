package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/linlay/cligrep-server/internal/app"
	"github.com/linlay/cligrep-server/internal/models"
)

type Handler struct {
	app         *app.App
	mux         *http.ServeMux
	corsOrigins []string
}

func NewHandler(application *app.App, corsOrigin string) http.Handler {
	handler := &Handler{
		app:         application,
		mux:         http.NewServeMux(),
		corsOrigins: parseCORSOrigins(corsOrigin),
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
	h.mux.HandleFunc("/api/v1/auth/mock/anonymous", h.handleAnonymous)
	h.mux.HandleFunc("/api/v1/auth/mock/login", h.handleLogin)
	h.mux.HandleFunc("/api/v1/auth/mock/logout", h.handleLogout)
	h.mux.HandleFunc("/api/v1/favorites", h.handleFavorites)
	h.mux.HandleFunc("/api/v1/comments", h.handleComments)
}

func (h *Handler) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := h.allowedOrigin(r.Header.Get("Origin")); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
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
		return "*"
	}
	for _, allowed := range h.corsOrigins {
		if allowed == "*" {
			return "*"
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
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, h.app.Health(r.Context()))
}

func (h *Handler) handleTrending(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	payload, err := h.app.Homepage(r.Context(), r.URL.Query().Get("sort"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	results, err := h.app.Search(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": results})
}

func (h *Handler) handleCLIBySlug(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	slug := strings.TrimPrefix(r.URL.Path, "/api/v1/clis/")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "missing cli slug")
		return
	}

	payload, err := h.app.GetCLI(r.Context(), slug)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func (h *Handler) handleExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var request models.ExecRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	result, err := h.app.ExecuteCLI(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) handleBuiltinExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var request models.BuiltinExecRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	response, err := h.app.ExecuteBuiltin(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var request models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	user, err := h.app.Login(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (h *Handler) handleAnonymous(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	user, err := h.app.AnonymousSession(r.Context())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *Handler) handleFavorites(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		userID, err := parseUserID(r.URL.Query().Get("userId"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		items, err := h.app.ListFavorites(r.Context(), userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		var request models.FavoriteMutation
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		if err := h.app.SetFavorite(r.Context(), request); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) handleComments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cliSlug := strings.TrimSpace(r.URL.Query().Get("cliSlug"))
		if cliSlug == "" {
			writeError(w, http.StatusBadRequest, "cliSlug is required")
			return
		}
		comments, err := h.app.ListComments(r.Context(), cliSlug)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": comments})
	case http.MethodPost:
		var request models.CommentMutation
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		comment, err := h.app.AddComment(r.Context(), request)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"item": comment})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
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
