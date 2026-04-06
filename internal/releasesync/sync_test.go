package releasesync

import (
	"context"
	"os"
	"path/filepath"
	"slices"
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

	writeReleaseFile(t, root, "sampledb", "v0.1.0", "sampledb_v0.1.0_darwin_arm64.tar.gz", 128, time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC))
	writeReleaseFile(t, root, "sampledb", "v0.1.0", "sampledb_v0.1.0_linux_amd64.tar.gz", 256, time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC))
	writeReleaseFile(t, root, "sampledb", "v0.1.0", "sampledb_v0.1.0_checksums.txt", 32, time.Date(2026, 3, 25, 12, 5, 0, 0, time.UTC))
	writeLatestSymlink(t, root, "sampledb", "sampledb_darwin_arm64.tar.gz", "../v0.1.0/sampledb_v0.1.0_darwin_arm64.tar.gz")
	writeLatestSymlink(t, root, "sampledb", "sampledb_linux_amd64.tar.gz", "../v0.1.0/sampledb_v0.1.0_linux_amd64.tar.gz")
	writeLatestSymlink(t, root, "sampledb", "checksums.txt", "../v0.1.0/sampledb_v0.1.0_checksums.txt")

	writeReleaseFile(t, root, "mailtool", "v1.1.0", "mailtool.x86_64-linux.tgz", 512, time.Date(2026, 2, 10, 9, 0, 0, 0, time.UTC))
	writeReleaseFile(t, root, "mailtool", "v1.1.0", "mailtool_v1.1.0_checksums.txt", 64, time.Date(2026, 2, 10, 9, 5, 0, 0, time.UTC))
	writeReleaseFile(t, root, "mailtool", "v1.2.0", "mailtool.x86_64-linux.tgz", 768, time.Date(2026, 2, 19, 10, 14, 0, 0, time.UTC))
	writeReleaseFile(t, root, "mailtool", "v1.2.0", "mailtool.aarch64-linux.tgz", 769, time.Date(2026, 2, 19, 10, 14, 0, 0, time.UTC))
	writeReleaseFile(t, root, "mailtool", "v1.2.0", "mailtool_v1.2.0_checksums.txt", 65, time.Date(2026, 2, 19, 10, 20, 0, 0, time.UTC))
	writeLatestSymlink(t, root, "mailtool", "mailtool_linux_amd64.tgz", "../v1.2.0/mailtool.x86_64-linux.tgz")
	writeLatestSymlink(t, root, "mailtool", "mailtool_linux_arm64.tgz", "../v1.2.0/mailtool.aarch64-linux.tgz")
	writeLatestSymlink(t, root, "mailtool", "checksums.txt", "../v1.2.0/mailtool_v1.2.0_checksums.txt")

	store := &fakeStore{}
	syncer := New(root, baseURL, store)

	results, err := syncer.Sync(context.Background(), []string{"sampledb", "mailtool"})
	if err != nil {
		t.Fatalf("sync releases: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	sampledbReleases := store.bySlug["sampledb"]
	if len(sampledbReleases) != 1 {
		t.Fatalf("expected 1 sampledb release, got %d", len(sampledbReleases))
	}
	if !sampledbReleases[0].IsCurrent || sampledbReleases[0].Version != "v0.1.0" {
		t.Fatalf("unexpected current sampledb release: %+v", sampledbReleases[0])
	}
	if len(sampledbReleases[0].Assets) != 2 {
		t.Fatalf("expected 2 sampledb assets, got %d", len(sampledbReleases[0].Assets))
	}
	if sampledbReleases[0].Assets[0].ChecksumURL != baseURL+"/sampledb/v0.1.0/sampledb_v0.1.0_checksums.txt" {
		t.Fatalf("unexpected sampledb checksum url %q", sampledbReleases[0].Assets[0].ChecksumURL)
	}

	mailtoolReleases := store.bySlug["mailtool"]
	if len(mailtoolReleases) != 2 {
		t.Fatalf("expected 2 mailtool releases, got %d", len(mailtoolReleases))
	}
	if mailtoolReleases[0].Version != "v1.2.0" || !mailtoolReleases[0].IsCurrent {
		t.Fatalf("expected v1.2.0 current release first, got %+v", mailtoolReleases[0])
	}
	if mailtoolReleases[0].Assets[0].OS != "linux" || mailtoolReleases[0].Assets[0].Arch == "unknown" {
		t.Fatalf("expected classified mailtool asset, got %+v", mailtoolReleases[0].Assets[0])
	}
}

func TestSyncRejectsMixedLatestTargets(t *testing.T) {
	root := t.TempDir()
	writeReleaseFile(t, root, "requester", "v0.1.0", "requester_v0.1.0_linux_amd64.tar.gz", 128, time.Now().UTC())
	writeReleaseFile(t, root, "requester", "v0.1.0", "requester_v0.1.0_checksums.txt", 32, time.Now().UTC())
	writeReleaseFile(t, root, "requester", "v0.2.0", "requester_v0.2.0_linux_amd64.tar.gz", 128, time.Now().UTC())
	writeReleaseFile(t, root, "requester", "v0.2.0", "requester_v0.2.0_checksums.txt", 32, time.Now().UTC())
	writeLatestSymlink(t, root, "requester", "requester_linux_amd64.tar.gz", "../v0.1.0/requester_v0.1.0_linux_amd64.tar.gz")
	writeLatestSymlink(t, root, "requester", "checksums.txt", "../v0.2.0/requester_v0.2.0_checksums.txt")

	syncer := New(root, "https://cligrep.com/cli-releases", &fakeStore{})
	if _, err := syncer.Sync(context.Background(), []string{"requester"}); err == nil {
		t.Fatal("expected mixed latest targets to fail")
	}
}

func TestSyncSupportsTarXZAndUnknownPlatformAssets(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 3, 27, 13, 0, 0, 0, time.UTC)

	writeReleaseFile(t, root, "mediakit", "v7.1.0", "mediakit-7.1.0.tar.xz", 1024, now)
	writeReleaseFile(t, root, "mediakit", "v7.1.0", "mediakit_v7.1.0_checksums.txt", 64, now)
	writeLatestSymlink(t, root, "mediakit", "mediakit.tar.xz", "../v7.1.0/mediakit-7.1.0.tar.xz")
	writeLatestSymlink(t, root, "mediakit", "checksums.txt", "../v7.1.0/mediakit_v7.1.0_checksums.txt")

	writeReleaseFile(t, root, "noteskit", "v0.3.0", "noteskit_py-0.3.0-py3-none-any.whl", 2048, now)
	writeReleaseFile(t, root, "noteskit", "v0.3.0", "noteskit-0.3.0.tar.gz", 4096, now)
	writeReleaseFile(t, root, "noteskit", "v0.3.0", "noteskit_v0.3.0_checksums.txt", 64, now)
	writeLatestSymlink(t, root, "noteskit", "noteskit.whl", "../v0.3.0/noteskit_py-0.3.0-py3-none-any.whl")
	writeLatestSymlink(t, root, "noteskit", "noteskit.tar.gz", "../v0.3.0/noteskit-0.3.0.tar.gz")
	writeLatestSymlink(t, root, "noteskit", "checksums.txt", "../v0.3.0/noteskit_v0.3.0_checksums.txt")

	store := &fakeStore{}
	syncer := New(root, "https://cligrep.com/cli-releases", store)

	if _, err := syncer.Sync(context.Background(), []string{"mediakit", "noteskit"}); err != nil {
		t.Fatalf("sync releases: %v", err)
	}

	mediakitReleases := store.bySlug["mediakit"]
	if len(mediakitReleases) != 1 {
		t.Fatalf("expected 1 mediakit release, got %d", len(mediakitReleases))
	}
	if got := mediakitReleases[0].Assets[0].PackageKind; got != "tar.xz" {
		t.Fatalf("expected tar.xz package kind, got %q", got)
	}

	noteskitReleases := store.bySlug["noteskit"]
	if len(noteskitReleases) != 1 {
		t.Fatalf("expected 1 noteskit release, got %d", len(noteskitReleases))
	}
	if len(noteskitReleases[0].Assets) != 2 {
		t.Fatalf("expected 2 noteskit assets, got %d", len(noteskitReleases[0].Assets))
	}
	for _, asset := range noteskitReleases[0].Assets {
		if asset.OS != "unknown" || asset.Arch != "unknown" {
			t.Fatalf("expected unknown platform for %s, got os=%q arch=%q", asset.FileName, asset.OS, asset.Arch)
		}
	}
}

func TestDiscoverSlugsIgnoresFilesAndSortsDirectories(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"noteskit", "sampledb", "mailtool"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	slugs, err := DiscoverSlugs(root)
	if err != nil {
		t.Fatalf("discover slugs: %v", err)
	}
	want := []string{"mailtool", "noteskit", "sampledb"}
	if !slices.Equal(slugs, want) {
		t.Fatalf("expected slugs %v, got %v", want, slugs)
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
