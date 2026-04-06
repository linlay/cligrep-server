package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/linlay/cligrep-server/internal/models"
)

var cliSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,127}$`)

type executionTemplateSpec struct {
	definition   models.ExecutionTemplate
	runtimeImage string
}

func (a *App) executionTemplateSpecs() map[string]executionTemplateSpec {
	return map[string]executionTemplateSpec{
		"download-only": {
			definition: models.ExecutionTemplate{
				ID:              "download-only",
				Label:           "Download only",
				Description:     "Reference and download metadata only. This CLI is not executable in the sandbox.",
				EnvironmentKind: models.EnvironmentKindText,
				Executable:      false,
			},
			runtimeImage: "",
		},
		"busybox-applet": {
			definition: models.ExecutionTemplate{
				ID:              "busybox-applet",
				Label:           "BusyBox applet",
				Description:     "Executable inside the platform BusyBox sandbox. Best for commands already available in BusyBox.",
				EnvironmentKind: models.EnvironmentKindSandbox,
				Executable:      true,
			},
			runtimeImage: a.cfg.BusyBoxImage,
		},
	}
}

func (a *App) ExecutionTemplates() []models.ExecutionTemplate {
	specs := a.executionTemplateSpecs()
	return []models.ExecutionTemplate{
		specs["download-only"].definition,
		specs["busybox-applet"].definition,
	}
}

func (a *App) AdminMe(ctx context.Context, user models.User) models.AdminMe {
	return models.AdminMe{
		User:               user,
		CanAccessAdmin:     canAccessAdmin(user),
		IsPlatformAdmin:    hasRole(user, models.RolePlatformAdmin),
		ExecutionTemplates: a.ExecutionTemplates(),
	}
}

func (a *App) ListAdminCLIs(ctx context.Context, user models.User) ([]models.CLI, error) {
	if !canAccessAdmin(user) {
		return nil, models.ErrForbidden
	}
	return a.store.ListAdminCLIs(ctx, user)
}

func (a *App) GetAdminCLI(ctx context.Context, user models.User, slug string) (map[string]any, error) {
	if !canAccessAdmin(user) {
		return nil, models.ErrForbidden
	}

	cli, err := a.store.GetAdminCLI(ctx, slug)
	if err != nil {
		return nil, err
	}
	if err := ensureCLIManageAccess(user, cli); err != nil {
		return nil, err
	}

	releases, err := a.store.GetCLIReleases(ctx, slug)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"cli":                cli,
		"releases":           releases,
		"executionTemplates": a.ExecutionTemplates(),
	}, nil
}

func (a *App) CreateAdminCLI(ctx context.Context, user models.User, request models.AdminCLIUpsertRequest) (models.CLI, error) {
	if !canAccessAdmin(user) {
		return models.CLI{}, models.ErrForbidden
	}

	cli, err := a.buildAdminCLI(user, models.CLI{}, request, true)
	if err != nil {
		return models.CLI{}, err
	}
	return a.store.CreateOwnedCLI(ctx, user.ID, cli)
}

func (a *App) UpdateAdminCLI(ctx context.Context, user models.User, slug string, request models.AdminCLIUpsertRequest) (models.CLI, error) {
	if !canAccessAdmin(user) {
		return models.CLI{}, models.ErrForbidden
	}

	current, err := a.store.GetAdminCLI(ctx, slug)
	if err != nil {
		return models.CLI{}, err
	}
	if err := ensureCLIManageAccess(user, current); err != nil {
		return models.CLI{}, err
	}

	next, err := a.buildAdminCLI(user, current, request, false)
	if err != nil {
		return models.CLI{}, err
	}
	return a.store.UpdateAdminCLI(ctx, next)
}

func (a *App) PublishAdminCLI(ctx context.Context, user models.User, slug string) (models.CLI, error) {
	return a.setAdminCLIStatus(ctx, user, slug, models.CLIStatusPublished)
}

func (a *App) UnpublishAdminCLI(ctx context.Context, user models.User, slug string) (models.CLI, error) {
	return a.setAdminCLIStatus(ctx, user, slug, models.CLIStatusDraft)
}

func (a *App) DeleteAdminCLI(ctx context.Context, user models.User, slug string) error {
	if !canAccessAdmin(user) {
		return models.ErrForbidden
	}

	cli, err := a.store.GetAdminCLI(ctx, slug)
	if err != nil {
		return err
	}
	if err := ensureCLIManageAccess(user, cli); err != nil {
		return err
	}

	paths, err := a.store.DeleteCLI(ctx, slug)
	if err != nil {
		return err
	}
	return a.deleteStoragePaths(paths)
}

func (a *App) CreateAdminRelease(ctx context.Context, user models.User, slug string, request models.AdminReleaseUpsertRequest) (models.CLIRelease, error) {
	if !canAccessAdmin(user) {
		return models.CLIRelease{}, models.ErrForbidden
	}
	cli, err := a.store.GetAdminCLI(ctx, slug)
	if err != nil {
		return models.CLIRelease{}, err
	}
	if err := ensureCLIManageAccess(user, cli); err != nil {
		return models.CLIRelease{}, err
	}

	release, err := normalizeAdminReleaseRequest(request, true)
	if err != nil {
		return models.CLIRelease{}, err
	}
	return a.store.CreateCLIRelease(ctx, slug, release)
}

func (a *App) UpdateAdminRelease(ctx context.Context, user models.User, slug, version string, request models.AdminReleaseUpsertRequest) (models.CLIRelease, error) {
	if !canAccessAdmin(user) {
		return models.CLIRelease{}, models.ErrForbidden
	}
	cli, err := a.store.GetAdminCLI(ctx, slug)
	if err != nil {
		return models.CLIRelease{}, err
	}
	if err := ensureCLIManageAccess(user, cli); err != nil {
		return models.CLIRelease{}, err
	}
	if strings.TrimSpace(request.Version) != "" && strings.TrimSpace(request.Version) != strings.TrimSpace(version) {
		return models.CLIRelease{}, models.ErrVersionImmutable
	}

	release, err := normalizeAdminReleaseRequest(request, false)
	if err != nil {
		return models.CLIRelease{}, err
	}
	release.Version = version
	return a.store.UpdateCLIRelease(ctx, slug, version, release)
}

func (a *App) DeleteAdminRelease(ctx context.Context, user models.User, slug, version string) error {
	if !canAccessAdmin(user) {
		return models.ErrForbidden
	}
	cli, err := a.store.GetAdminCLI(ctx, slug)
	if err != nil {
		return err
	}
	if err := ensureCLIManageAccess(user, cli); err != nil {
		return err
	}

	paths, err := a.store.DeleteCLIRelease(ctx, slug, version)
	if err != nil {
		return err
	}
	return a.deleteStoragePaths(paths)
}

func (a *App) UploadAdminReleaseAsset(ctx context.Context, user models.User, slug, version string, metadata models.CLIReleaseAsset, reader io.Reader) (models.CLIReleaseAsset, error) {
	if !canAccessAdmin(user) {
		return models.CLIReleaseAsset{}, models.ErrForbidden
	}
	cli, err := a.store.GetAdminCLI(ctx, slug)
	if err != nil {
		return models.CLIReleaseAsset{}, err
	}
	if err := ensureCLIManageAccess(user, cli); err != nil {
		return models.CLIReleaseAsset{}, err
	}
	if strings.TrimSpace(metadata.FileName) == "" {
		return models.CLIReleaseAsset{}, models.ErrInvalidAssetFile
	}

	relativePath, err := a.writeReleaseAssetFile(cli, version, metadata.FileName, reader)
	if err != nil {
		return models.CLIReleaseAsset{}, err
	}
	metadata.StorageKind = "local"
	metadata.StoragePath = relativePath
	metadata.DownloadURL = a.releaseDownloadURL(relativePath)

	asset, err := a.store.CreateCLIReleaseAsset(ctx, slug, version, metadata)
	if err != nil {
		_ = a.deleteStoragePaths([]string{relativePath})
		return models.CLIReleaseAsset{}, err
	}
	return asset, nil
}

func (a *App) DeleteAdminReleaseAsset(ctx context.Context, user models.User, slug, version string, assetID int64) error {
	if !canAccessAdmin(user) {
		return models.ErrForbidden
	}
	cli, err := a.store.GetAdminCLI(ctx, slug)
	if err != nil {
		return err
	}
	if err := ensureCLIManageAccess(user, cli); err != nil {
		return err
	}

	asset, err := a.store.DeleteCLIReleaseAsset(ctx, slug, version, assetID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(asset.StoragePath) == "" {
		return nil
	}
	return a.deleteStoragePaths([]string{asset.StoragePath})
}

func (a *App) setAdminCLIStatus(ctx context.Context, user models.User, slug string, status models.CLIStatus) (models.CLI, error) {
	if !canAccessAdmin(user) {
		return models.CLI{}, models.ErrForbidden
	}

	cli, err := a.store.GetAdminCLI(ctx, slug)
	if err != nil {
		return models.CLI{}, err
	}
	if err := ensureCLIManageAccess(user, cli); err != nil {
		return models.CLI{}, err
	}

	var publishedAt *time.Time
	if status == models.CLIStatusPublished {
		now := time.Now().UTC()
		publishedAt = &now
	}
	if err := a.store.SetCLIStatus(ctx, slug, status, publishedAt); err != nil {
		return models.CLI{}, err
	}
	return a.store.GetAdminCLI(ctx, slug)
}

func (a *App) buildAdminCLI(user models.User, current models.CLI, request models.AdminCLIUpsertRequest, create bool) (models.CLI, error) {
	next := current

	slug := strings.TrimSpace(current.Slug)
	if create {
		slug = strings.ToLower(strings.TrimSpace(request.Slug))
		if !cliSlugPattern.MatchString(slug) {
			return models.CLI{}, models.ErrInvalidCLISlug
		}
		next.Slug = slug
		next.Type = models.CLITypeNative
		next.SourceType = "user"
		next.Enabled = true
		next.Status = string(models.CLIStatusDraft)
		next.CreatedAt = time.Now().UTC()
	}

	next.DisplayName = firstNonEmptyString(request.DisplayName, current.DisplayName, slug)
	next.Summary = strings.TrimSpace(request.Summary)
	next.HelpText = strings.TrimSpace(request.HelpText)
	next.Tags = normalizeTags(request.Tags)
	next.VersionText = firstNonEmptyString(request.VersionText, current.VersionText, "N/A")
	next.ExampleLine = firstNonEmptyString(request.ExampleLine, current.ExampleLine, fmt.Sprintf("%s --help", slug))
	next.Author = firstNonEmptyString(request.Author, current.Author, user.DisplayName, user.Username)
	next.OfficialURL = strings.TrimSpace(request.OfficialURL)
	next.GiteeURL = strings.TrimSpace(request.GiteeURL)
	next.License = strings.TrimSpace(request.License)
	next.OriginalCommand = firstNonEmptyString(request.OriginalCommand, current.OriginalCommand, slug)

	templateID := strings.TrimSpace(request.ExecutionTemplate)
	if current.OwnerUserID != nil || create {
		if templateID == "" {
			templateID = "download-only"
		}
	}
	if templateID == "" {
		templateID = current.ExecutionTemplate
	}
	if templateID != "" {
		spec, ok := a.executionTemplateSpecs()[templateID]
		if !ok {
			return models.CLI{}, models.ErrInvalidExecutionTemplate
		}
		next.ExecutionTemplate = templateID
		next.EnvironmentKind = spec.definition.EnvironmentKind
		next.Executable = spec.definition.Executable
		next.RuntimeImage = spec.runtimeImage
	} else if create {
		next.EnvironmentKind = models.EnvironmentKindText
		next.Executable = false
		next.RuntimeImage = ""
	}

	if !create {
		next.Status = current.Status
		next.OwnerUserID = current.OwnerUserID
		next.CreatedAt = current.CreatedAt
		next.PublishedAt = current.PublishedAt
	}

	return next, nil
}

func normalizeAdminReleaseRequest(request models.AdminReleaseUpsertRequest, create bool) (models.CLIRelease, error) {
	version := strings.TrimSpace(request.Version)
	if create && version == "" {
		return models.CLIRelease{}, models.ErrInvalidVersion
	}
	if request.PublishedAt.IsZero() {
		return models.CLIRelease{}, models.ErrInvalidReleaseTime
	}
	return models.CLIRelease{
		Version:     version,
		PublishedAt: request.PublishedAt.UTC(),
		IsCurrent:   request.IsCurrent,
		SourceKind:  strings.TrimSpace(request.SourceKind),
		SourceURL:   strings.TrimSpace(request.SourceURL),
	}, nil
}

func ensureCLIManageAccess(user models.User, cli models.CLI) error {
	if hasRole(user, models.RolePlatformAdmin) {
		return nil
	}
	if cli.OwnerUserID == nil || *cli.OwnerUserID != user.ID {
		return models.ErrForbidden
	}
	return nil
}

func canAccessAdmin(user models.User) bool {
	return user.ID > 0 && (hasRole(user, models.RoleMember) || hasRole(user, models.RolePlatformAdmin))
}

func hasRole(user models.User, role models.Role) bool {
	for _, item := range user.Roles {
		if item == string(role) {
			return true
		}
	}
	return false
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizeTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func (a *App) writeReleaseAssetFile(cli models.CLI, version, filename string, reader io.Reader) (string, error) {
	relativeDir := a.releaseStorageDir(cli, version)
	absoluteDir := filepath.Join(a.cfg.ReleasesRoot, filepath.FromSlash(relativeDir))
	if err := os.MkdirAll(absoluteDir, 0o755); err != nil {
		return "", fmt.Errorf("create release asset dir: %w", err)
	}

	safeName := sanitizeFileName(filename)
	targetPath := filepath.Join(absoluteDir, safeName)
	if _, err := os.Stat(targetPath); err == nil {
		targetPath = filepath.Join(absoluteDir, uniqueFileName(safeName))
	}

	file, err := os.Create(targetPath)
	if err != nil {
		return "", fmt.Errorf("create release asset file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, reader); err != nil {
		return "", fmt.Errorf("write release asset file: %w", err)
	}

	relativePath, err := filepath.Rel(a.cfg.ReleasesRoot, targetPath)
	if err != nil {
		return "", fmt.Errorf("compute release asset path: %w", err)
	}
	return filepath.ToSlash(relativePath), nil
}

func (a *App) releaseStorageDir(cli models.CLI, version string) string {
	cleanVersion := sanitizePathSegment(version)
	if cli.OwnerUserID != nil && *cli.OwnerUserID > 0 {
		return filepath.ToSlash(filepath.Join("users", fmt.Sprintf("%d", *cli.OwnerUserID), sanitizePathSegment(cli.Slug), cleanVersion))
	}
	return filepath.ToSlash(filepath.Join("platform", sanitizePathSegment(cli.Slug), cleanVersion))
}

func sanitizeFileName(name string) string {
	base := filepath.Base(strings.TrimSpace(name))
	base = strings.ReplaceAll(base, "..", "")
	base = strings.ReplaceAll(base, "/", "-")
	base = strings.ReplaceAll(base, "\\", "-")
	base = strings.TrimSpace(base)
	if base == "" || base == "." {
		return fmt.Sprintf("asset-%d.bin", time.Now().UTC().Unix())
	}
	return base
}

func uniqueFileName(name string) string {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	return fmt.Sprintf("%s-%d%s", stem, time.Now().UTC().UnixNano(), ext)
}

func sanitizePathSegment(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.ReplaceAll(trimmed, "..", "")
	trimmed = strings.ReplaceAll(trimmed, "/", "-")
	trimmed = strings.ReplaceAll(trimmed, "\\", "-")
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}

func (a *App) releaseDownloadURL(relativePath string) string {
	base := strings.TrimRight(strings.TrimSpace(a.cfg.ReleasesBaseURL), "/")
	if base == "" {
		return "/" + strings.TrimLeft(relativePath, "/")
	}
	return base + "/" + strings.TrimLeft(relativePath, "/")
}

func (a *App) deleteStoragePaths(paths []string) error {
	for _, relativePath := range paths {
		trimmed := strings.TrimSpace(relativePath)
		if trimmed == "" {
			continue
		}
		absolutePath := filepath.Join(a.cfg.ReleasesRoot, filepath.FromSlash(trimmed))
		if err := os.Remove(absolutePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("delete release asset file %s: %w", trimmed, err)
		}
		a.removeEmptyParents(filepath.Dir(absolutePath))
	}
	return nil
}

func (a *App) removeEmptyParents(dir string) {
	root := filepath.Clean(a.cfg.ReleasesRoot)
	current := filepath.Clean(dir)
	for strings.HasPrefix(current, root) && current != root {
		if err := os.Remove(current); err != nil {
			return
		}
		current = filepath.Dir(current)
	}
}
