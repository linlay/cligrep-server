package releasesync

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/linlay/cligrep-server/internal/models"
)

type fakeStore struct {
	bySlug map[string][]models.CLIRelease
}

func (f *fakeStore) ReplaceCLIReleases(ctx context.Context, slug string, releases []models.CLIRelease) error {
	if f.bySlug == nil {
		f.bySlug = make(map[string][]models.CLIRelease)
	}
	f.bySlug[slug] = releases
	return nil
}

func TestSyncImportsWebsiteReleaseDirectories(t *testing.T) {
	root := t.TempDir()
	baseURL := "https://cligrep.com/cli-releases"

	writeReleaseFile(t, root, "dbx", "v0.1.0", "dbx_v0.1.0_darwin_arm64.tar.gz", 128, time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC))
	writeReleaseFile(t, root, "dbx", "v0.1.0", "dbx_v0.1.0_linux_amd64.tar.gz", 256, time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC))
	writeReleaseFile(t, root, "dbx", "v0.1.0", "dbx_v0.1.0_checksums.txt", 32, time.Date(2026, 3, 25, 12, 5, 0, 0, time.UTC))
	writeLatestSymlink(t, root, "dbx", "dbx_darwin_arm64.tar.gz", "../v0.1.0/dbx_v0.1.0_darwin_arm64.tar.gz")
	writeLatestSymlink(t, root, "dbx", "dbx_linux_amd64.tar.gz", "../v0.1.0/dbx_v0.1.0_linux_amd64.tar.gz")
	writeLatestSymlink(t, root, "dbx", "checksums.txt", "../v0.1.0/dbx_v0.1.0_checksums.txt")

	writeReleaseFile(t, root, "himalaya", "v1.1.0", "himalaya.x86_64-linux.tgz", 512, time.Date(2026, 2, 10, 9, 0, 0, 0, time.UTC))
	writeReleaseFile(t, root, "himalaya", "v1.1.0", "himalaya_v1.1.0_checksums.txt", 64, time.Date(2026, 2, 10, 9, 5, 0, 0, time.UTC))
	writeReleaseFile(t, root, "himalaya", "v1.2.0", "himalaya.x86_64-linux.tgz", 768, time.Date(2026, 2, 19, 10, 14, 0, 0, time.UTC))
	writeReleaseFile(t, root, "himalaya", "v1.2.0", "himalaya.aarch64-linux.tgz", 769, time.Date(2026, 2, 19, 10, 14, 0, 0, time.UTC))
	writeReleaseFile(t, root, "himalaya", "v1.2.0", "himalaya_v1.2.0_checksums.txt", 65, time.Date(2026, 2, 19, 10, 20, 0, 0, time.UTC))
	writeLatestSymlink(t, root, "himalaya", "himalaya_linux_amd64.tgz", "../v1.2.0/himalaya.x86_64-linux.tgz")
	writeLatestSymlink(t, root, "himalaya", "himalaya_linux_arm64.tgz", "../v1.2.0/himalaya.aarch64-linux.tgz")
	writeLatestSymlink(t, root, "himalaya", "checksums.txt", "../v1.2.0/himalaya_v1.2.0_checksums.txt")

	store := &fakeStore{}
	syncer := New(root, baseURL, store)

	results, err := syncer.Sync(context.Background(), []string{"dbx", "himalaya"})
	if err != nil {
		t.Fatalf("sync releases: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	dbxReleases := store.bySlug["dbx"]
	if len(dbxReleases) != 1 {
		t.Fatalf("expected 1 dbx release, got %d", len(dbxReleases))
	}
	if !dbxReleases[0].IsCurrent || dbxReleases[0].Version != "v0.1.0" {
		t.Fatalf("unexpected current dbx release: %+v", dbxReleases[0])
	}
	if len(dbxReleases[0].Assets) != 2 {
		t.Fatalf("expected 2 dbx assets, got %d", len(dbxReleases[0].Assets))
	}
	if dbxReleases[0].Assets[0].ChecksumURL != baseURL+"/dbx/v0.1.0/dbx_v0.1.0_checksums.txt" {
		t.Fatalf("unexpected dbx checksum url %q", dbxReleases[0].Assets[0].ChecksumURL)
	}

	himalayaReleases := store.bySlug["himalaya"]
	if len(himalayaReleases) != 2 {
		t.Fatalf("expected 2 himalaya releases, got %d", len(himalayaReleases))
	}
	if himalayaReleases[0].Version != "v1.2.0" || !himalayaReleases[0].IsCurrent {
		t.Fatalf("expected v1.2.0 current release first, got %+v", himalayaReleases[0])
	}
	if himalayaReleases[0].Assets[0].OS != "linux" || himalayaReleases[0].Assets[0].Arch == "unknown" {
		t.Fatalf("expected classified himalaya asset, got %+v", himalayaReleases[0].Assets[0])
	}
}

func TestSyncRejectsMixedLatestTargets(t *testing.T) {
	root := t.TempDir()
	writeReleaseFile(t, root, "httpx", "v0.1.0", "httpx_v0.1.0_linux_amd64.tar.gz", 128, time.Now().UTC())
	writeReleaseFile(t, root, "httpx", "v0.1.0", "httpx_v0.1.0_checksums.txt", 32, time.Now().UTC())
	writeReleaseFile(t, root, "httpx", "v0.2.0", "httpx_v0.2.0_linux_amd64.tar.gz", 128, time.Now().UTC())
	writeReleaseFile(t, root, "httpx", "v0.2.0", "httpx_v0.2.0_checksums.txt", 32, time.Now().UTC())
	writeLatestSymlink(t, root, "httpx", "httpx_linux_amd64.tar.gz", "../v0.1.0/httpx_v0.1.0_linux_amd64.tar.gz")
	writeLatestSymlink(t, root, "httpx", "checksums.txt", "../v0.2.0/httpx_v0.2.0_checksums.txt")

	syncer := New(root, "https://cligrep.com/cli-releases", &fakeStore{})
	if _, err := syncer.Sync(context.Background(), []string{"httpx"}); err == nil {
		t.Fatal("expected mixed latest targets to fail")
	}
}

func TestSyncSupportsTarXZAndUnknownPlatformAssets(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 3, 27, 13, 0, 0, 0, time.UTC)

	writeReleaseFile(t, root, "ffmpeg", "v7.1.0", "ffmpeg-7.1.0.tar.xz", 1024, now)
	writeReleaseFile(t, root, "ffmpeg", "v7.1.0", "ffmpeg_v7.1.0_checksums.txt", 64, now)
	writeLatestSymlink(t, root, "ffmpeg", "ffmpeg.tar.xz", "../v7.1.0/ffmpeg-7.1.0.tar.xz")
	writeLatestSymlink(t, root, "ffmpeg", "checksums.txt", "../v7.1.0/ffmpeg_v7.1.0_checksums.txt")

	writeReleaseFile(t, root, "notebooklm", "v0.3.0", "notebooklm_py-0.3.0-py3-none-any.whl", 2048, now)
	writeReleaseFile(t, root, "notebooklm", "v0.3.0", "notebooklm-py-0.3.0.tar.gz", 4096, now)
	writeReleaseFile(t, root, "notebooklm", "v0.3.0", "notebooklm_v0.3.0_checksums.txt", 64, now)
	writeLatestSymlink(t, root, "notebooklm", "notebooklm.whl", "../v0.3.0/notebooklm_py-0.3.0-py3-none-any.whl")
	writeLatestSymlink(t, root, "notebooklm", "notebooklm.tar.gz", "../v0.3.0/notebooklm-py-0.3.0.tar.gz")
	writeLatestSymlink(t, root, "notebooklm", "checksums.txt", "../v0.3.0/notebooklm_v0.3.0_checksums.txt")

	store := &fakeStore{}
	syncer := New(root, "https://cligrep.com/cli-releases", store)

	if _, err := syncer.Sync(context.Background(), []string{"ffmpeg", "notebooklm"}); err != nil {
		t.Fatalf("sync releases: %v", err)
	}

	ffmpegReleases := store.bySlug["ffmpeg"]
	if len(ffmpegReleases) != 1 {
		t.Fatalf("expected 1 ffmpeg release, got %d", len(ffmpegReleases))
	}
	if got := ffmpegReleases[0].Assets[0].PackageKind; got != "tar.xz" {
		t.Fatalf("expected tar.xz package kind, got %q", got)
	}

	notebooklmReleases := store.bySlug["notebooklm"]
	if len(notebooklmReleases) != 1 {
		t.Fatalf("expected 1 notebooklm release, got %d", len(notebooklmReleases))
	}
	if len(notebooklmReleases[0].Assets) != 2 {
		t.Fatalf("expected 2 notebooklm assets, got %d", len(notebooklmReleases[0].Assets))
	}
	for _, asset := range notebooklmReleases[0].Assets {
		if asset.OS != "unknown" || asset.Arch != "unknown" {
			t.Fatalf("expected unknown platform for %s, got os=%q arch=%q", asset.FileName, asset.OS, asset.Arch)
		}
	}
}

func writeReleaseFile(t *testing.T, root, slug, version, name string, size int, modTime time.Time) {
	t.Helper()
	dir := filepath.Join(root, slug, version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir release dir: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatalf("write release file: %v", err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("set release file times: %v", err)
	}
}

func writeLatestSymlink(t *testing.T, root, slug, name, target string) {
	t.Helper()
	dir := filepath.Join(root, slug, "latest")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir latest dir: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
		t.Fatalf("create latest symlink: %v", err)
	}
}
