package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/linlay/cligrep-server/internal/models"
)

func (h *Handler) handleAdminMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeCatalogError(w, r.Context(), http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}

	user, ok := currentUserFromContext(r.Context())
	if !ok {
		writeLocalizedError(w, r.Context(), http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	writeJSON(w, http.StatusOK, h.app.AdminMe(r.Context(), user))
}

func (h *Handler) handleAdminCLIs(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUserFromContext(r.Context())
	if !ok {
		writeLocalizedError(w, r.Context(), http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		items, err := h.app.ListAdminCLIs(r.Context(), user)
		if err != nil {
			writeAdminError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		var request models.AdminCLIUpsertRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeCatalogError(w, r.Context(), http.StatusBadRequest, "invalid_json_body")
			return
		}
		cli, err := h.app.CreateAdminCLI(r.Context(), user, request)
		if err != nil {
			writeAdminError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"cli": cli})
	default:
		writeCatalogError(w, r.Context(), http.StatusMethodNotAllowed, "method_not_allowed")
	}
}

func (h *Handler) handleAdminCLIBySlug(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUserFromContext(r.Context())
	if !ok {
		writeLocalizedError(w, r.Context(), http.StatusUnauthorized, models.ErrUnauthorized)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/clis/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		writeCatalogError(w, r.Context(), http.StatusBadRequest, "missing_cli_slug")
		return
	}

	slug := parts[0]
	switch {
	case len(parts) == 1:
		h.handleAdminCLIDocument(w, r, user, slug)
	case len(parts) == 2 && parts[1] == "publish":
		h.handleAdminCLIPublish(w, r, user, slug, true)
	case len(parts) == 2 && parts[1] == "unpublish":
		h.handleAdminCLIPublish(w, r, user, slug, false)
	case len(parts) == 2 && parts[1] == "releases":
		h.handleAdminReleases(w, r, user, slug)
	case len(parts) == 3 && parts[1] == "releases":
		h.handleAdminReleaseDocument(w, r, user, slug, parts[2])
	case len(parts) == 4 && parts[1] == "releases" && parts[3] == "assets":
		h.handleAdminReleaseAssets(w, r, user, slug, parts[2])
	case len(parts) == 5 && parts[1] == "releases" && parts[3] == "assets":
		h.handleAdminReleaseAssetDelete(w, r, user, slug, parts[2], parts[4])
	default:
		writeCatalogError(w, r.Context(), http.StatusNotFound, "not_found")
	}
}

func (h *Handler) handleAdminCLIDocument(w http.ResponseWriter, r *http.Request, user models.User, slug string) {
	switch r.Method {
	case http.MethodGet:
		payload, err := h.app.GetAdminCLI(r.Context(), user, slug)
		if err != nil {
			writeAdminError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, payload)
	case http.MethodPatch:
		var request models.AdminCLIUpsertRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeCatalogError(w, r.Context(), http.StatusBadRequest, "invalid_json_body")
			return
		}
		cli, err := h.app.UpdateAdminCLI(r.Context(), user, slug, request)
		if err != nil {
			writeAdminError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"cli": cli})
	case http.MethodDelete:
		if err := h.app.DeleteAdminCLI(r.Context(), user, slug); err != nil {
			writeAdminError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	default:
		writeCatalogError(w, r.Context(), http.StatusMethodNotAllowed, "method_not_allowed")
	}
}

func (h *Handler) handleAdminCLIPublish(w http.ResponseWriter, r *http.Request, user models.User, slug string, publish bool) {
	if r.Method != http.MethodPost {
		writeCatalogError(w, r.Context(), http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}

	var (
		cli models.CLI
		err error
	)
	if publish {
		cli, err = h.app.PublishAdminCLI(r.Context(), user, slug)
	} else {
		cli, err = h.app.UnpublishAdminCLI(r.Context(), user, slug)
	}
	if err != nil {
		writeAdminError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cli": cli})
}

func (h *Handler) handleAdminReleases(w http.ResponseWriter, r *http.Request, user models.User, slug string) {
	switch r.Method {
	case http.MethodGet:
		payload, err := h.app.GetAdminCLI(r.Context(), user, slug)
		if err != nil {
			writeAdminError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": payload["releases"]})
	case http.MethodPost:
		var request models.AdminReleaseUpsertRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeCatalogError(w, r.Context(), http.StatusBadRequest, "invalid_json_body")
			return
		}
		release, err := h.app.CreateAdminRelease(r.Context(), user, slug, request)
		if err != nil {
			writeAdminError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"release": release})
	default:
		writeCatalogError(w, r.Context(), http.StatusMethodNotAllowed, "method_not_allowed")
	}
}

func (h *Handler) handleAdminReleaseDocument(w http.ResponseWriter, r *http.Request, user models.User, slug, version string) {
	switch r.Method {
	case http.MethodPatch:
		var request models.AdminReleaseUpsertRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeCatalogError(w, r.Context(), http.StatusBadRequest, "invalid_json_body")
			return
		}
		release, err := h.app.UpdateAdminRelease(r.Context(), user, slug, version, request)
		if err != nil {
			writeAdminError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"release": release})
	case http.MethodDelete:
		if err := h.app.DeleteAdminRelease(r.Context(), user, slug, version); err != nil {
			writeAdminError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	default:
		writeCatalogError(w, r.Context(), http.StatusMethodNotAllowed, "method_not_allowed")
	}
}

func (h *Handler) handleAdminReleaseAssets(w http.ResponseWriter, r *http.Request, user models.User, slug, version string) {
	if r.Method != http.MethodPost {
		writeCatalogError(w, r.Context(), http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeLocalizedError(w, r.Context(), http.StatusBadRequest, models.ErrInvalidAssetFile)
		return
	}
	defer file.Close()

	sizeBytes := header.Size
	if sizeBytes < 0 {
		sizeBytes = 0
	}
	asset, err := h.app.UploadAdminReleaseAsset(r.Context(), user, slug, version, models.CLIReleaseAsset{
		FileName:    header.Filename,
		OS:          strings.TrimSpace(r.FormValue("os")),
		Arch:        strings.TrimSpace(r.FormValue("arch")),
		PackageKind: strings.TrimSpace(r.FormValue("packageKind")),
		ChecksumURL: strings.TrimSpace(r.FormValue("checksumUrl")),
		SizeBytes:   sizeBytes,
	}, file)
	if err != nil {
		writeAdminError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, models.AdminAssetUploadResult{Asset: asset})
}

func (h *Handler) handleAdminReleaseAssetDelete(w http.ResponseWriter, r *http.Request, user models.User, slug, version, rawAssetID string) {
	if r.Method != http.MethodDelete {
		writeCatalogError(w, r.Context(), http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	assetID, err := strconv.ParseInt(strings.TrimSpace(rawAssetID), 10, 64)
	if err != nil || assetID <= 0 {
		writeError(w, http.StatusBadRequest, "valid assetId is required")
		return
	}
	if err := h.app.DeleteAdminReleaseAsset(r.Context(), user, slug, version, assetID); err != nil {
		writeAdminError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func writeAdminError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, models.ErrUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, models.ErrForbidden):
		status = http.StatusForbidden
	case errors.Is(err, sql.ErrNoRows):
		status = http.StatusNotFound
	case errors.Is(err, models.ErrCLISlugTaken):
		status = http.StatusConflict
	}
	writeLocalizedError(w, r.Context(), status, err)
}
