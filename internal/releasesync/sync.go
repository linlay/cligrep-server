package releasesync

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/linlay/cligrep-server/internal/models"
)

const sourceKindWebsiteReleaseDir = "website_release_dir"

var defaultSlugs = []string{"dbx", "httpx", "mock", "himalaya"}

type store interface {
	ReplaceCLIReleases(ctx context.Context, slug string, releases []models.CLIRelease) error
}

type Syncer struct {
	root    string
	baseURL string
	store   store
}

type Result struct {
	Slug           string
	ReleaseCount   int
	AssetCount     int
	CurrentVersion string
}

func New(root, baseURL string, store store) *Syncer {
	return &Syncer{
		root:    strings.TrimSpace(root),
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		store:   store,
	}
}

func SupportedSlugs() []string {
	return slices.Clone(defaultSlugs)
}

func (s *Syncer) Sync(ctx context.Context, slugs []string) ([]Result, error) {
	if s.store == nil {
		return nil, fmt.Errorf("release sync store is required")
	}
	if strings.TrimSpace(s.root) == "" {
		return nil, fmt.Errorf("release sync root is required")
	}
	if strings.TrimSpace(s.baseURL) == "" {
		return nil, fmt.Errorf("release sync base url is required")
	}
	if len(slugs) == 0 {
		slugs = SupportedSlugs()
	}

	results := make([]Result, 0, len(slugs))
	for _, slug := range slugs {
		releases, currentVersion, err := s.scanSlug(slug)
		if err != nil {
			return nil, err
		}
		if err := s.store.ReplaceCLIReleases(ctx, slug, releases); err != nil {
			return nil, fmt.Errorf("replace releases for %s: %w", slug, err)
		}
		assetCount := 0
		for _, release := range releases {
			assetCount += len(release.Assets)
		}
		results = append(results, Result{
			Slug:           slug,
			ReleaseCount:   len(releases),
			AssetCount:     assetCount,
			CurrentVersion: currentVersion,
		})
	}

	return results, nil
}

func (s *Syncer) scanSlug(slug string) ([]models.CLIRelease, string, error) {
	slugDir := filepath.Join(s.root, slug)
	entries, err := os.ReadDir(slugDir)
	if err != nil {
		return nil, "", fmt.Errorf("read release directory %s: %w", slugDir, err)
	}

	currentVersion, err := s.currentVersion(slug)
	if err != nil {
		return nil, "", err
	}

	var releases []models.CLIRelease
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		version := entry.Name()
		if version == "latest" || !strings.HasPrefix(version, "v") {
			continue
		}

		release, err := s.scanVersionDir(slug, version, currentVersion == version)
		if err != nil {
			return nil, "", err
		}
		releases = append(releases, release)
	}

	if len(releases) == 0 {
		return nil, "", fmt.Errorf("no release versions found for %s under %s", slug, slugDir)
	}

	slices.SortFunc(releases, func(a, b models.CLIRelease) int {
		if a.PublishedAt.Equal(b.PublishedAt) {
			return strings.Compare(b.Version, a.Version)
		}
		if a.PublishedAt.After(b.PublishedAt) {
			return -1
		}
		return 1
	})

	return releases, currentVersion, nil
}

func (s *Syncer) currentVersion(slug string) (string, error) {
	latestDir := filepath.Join(s.root, slug, "latest")
	entries, err := os.ReadDir(latestDir)
	if err != nil {
		return "", fmt.Errorf("read latest directory %s: %w", latestDir, err)
	}

	versions := map[string]struct{}{}
	for _, entry := range entries {
		if entry.Type()&fs.ModeSymlink == 0 {
			continue
		}
		resolvedPath, err := filepath.EvalSymlinks(filepath.Join(latestDir, entry.Name()))
		if err != nil {
			return "", fmt.Errorf("resolve latest symlink %s/%s: %w", latestDir, entry.Name(), err)
		}
		version := filepath.Base(filepath.Dir(resolvedPath))
		if version == "" || version == "." || version == "latest" {
			continue
		}
		versions[version] = struct{}{}
	}

	if len(versions) == 0 {
		return "", fmt.Errorf("latest directory %s does not point to any versioned release", latestDir)
	}
	if len(versions) > 1 {
		keys := make([]string, 0, len(versions))
		for version := range versions {
			keys = append(keys, version)
		}
		slices.Sort(keys)
		return "", fmt.Errorf("latest directory %s points to multiple versions: %s", latestDir, strings.Join(keys, ", "))
	}
	for version := range versions {
		return version, nil
	}
	return "", fmt.Errorf("failed to resolve current version for %s", slug)
}

func (s *Syncer) scanVersionDir(slug, version string, isCurrent bool) (models.CLIRelease, error) {
	versionDir := filepath.Join(s.root, slug, version)
	entries, err := os.ReadDir(versionDir)
	if err != nil {
		return models.CLIRelease{}, fmt.Errorf("read version directory %s: %w", versionDir, err)
	}

	release := models.CLIRelease{
		Version:     version,
		IsCurrent:   isCurrent,
		SourceKind:  sourceKindWebsiteReleaseDir,
		SourceURL:   fmt.Sprintf("%s/%s/%s/", s.baseURL, slug, version),
		Assets:      []models.CLIReleaseAsset{},
		PublishedAt: time.Time{},
	}

	checksumURL := ""
	var publishedAt time.Time
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return models.CLIRelease{}, fmt.Errorf("stat version file %s/%s: %w", versionDir, entry.Name(), err)
		}
		modTime := fileInfo.ModTime().UTC()
		if modTime.After(publishedAt) {
			publishedAt = modTime
		}
		if isChecksumFile(entry.Name()) {
			checksumURL = fmt.Sprintf("%s/%s/%s/%s", s.baseURL, slug, version, entry.Name())
		}
	}

	if publishedAt.IsZero() {
		return models.CLIRelease{}, fmt.Errorf("release %s/%s has no files", slug, version)
	}
	release.PublishedAt = publishedAt

	for _, entry := range entries {
		if entry.IsDir() || isChecksumFile(entry.Name()) {
			continue
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return models.CLIRelease{}, fmt.Errorf("stat asset %s/%s: %w", versionDir, entry.Name(), err)
		}
		osName, arch, packageKind := classifyAsset(entry.Name())
		release.Assets = append(release.Assets, models.CLIReleaseAsset{
			FileName:    entry.Name(),
			DownloadURL: fmt.Sprintf("%s/%s/%s/%s", s.baseURL, slug, version, entry.Name()),
			OS:          osName,
			Arch:        arch,
			PackageKind: packageKind,
			ChecksumURL: checksumURL,
			SizeBytes:   fileInfo.Size(),
		})
	}

	slices.SortFunc(release.Assets, func(a, b models.CLIReleaseAsset) int {
		return strings.Compare(a.FileName, b.FileName)
	})

	return release, nil
}

func isChecksumFile(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	return strings.Contains(lower, "checksum") && strings.HasSuffix(lower, ".txt")
}

func classifyAsset(name string) (string, string, string) {
	lower := strings.ToLower(name)
	packageKind := packageKindForName(lower)

	patterns := []struct {
		token string
		os    string
		arch  string
	}{
		{token: "darwin_arm64", os: "darwin", arch: "arm64"},
		{token: "darwin_amd64", os: "darwin", arch: "amd64"},
		{token: "linux_arm64", os: "linux", arch: "arm64"},
		{token: "linux_amd64", os: "linux", arch: "amd64"},
		{token: "aarch64-linux", os: "linux", arch: "arm64"},
		{token: "x86_64-linux", os: "linux", arch: "amd64"},
		{token: "aarch64-darwin", os: "darwin", arch: "arm64"},
		{token: "x86_64-darwin", os: "darwin", arch: "amd64"},
		{token: "x86_64-windows", os: "windows", arch: "amd64"},
		{token: "armv7l-linux", os: "linux", arch: "armv7"},
		{token: "armv6l-linux", os: "linux", arch: "armv6"},
		{token: "i686-linux", os: "linux", arch: "386"},
	}
	for _, pattern := range patterns {
		if strings.Contains(lower, pattern.token) {
			return pattern.os, pattern.arch, packageKind
		}
	}

	return "unknown", "unknown", packageKind
}

func packageKindForName(name string) string {
	switch {
	case strings.HasSuffix(name, ".tar.xz"):
		return "tar.xz"
	case strings.HasSuffix(name, ".tar.gz"):
		return "tar.gz"
	case strings.HasSuffix(name, ".tgz"):
		return "tgz"
	case strings.HasSuffix(name, ".zip"):
		return "zip"
	default:
		return filepath.Ext(name)
	}
}
