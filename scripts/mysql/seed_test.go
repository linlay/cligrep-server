package mysqlschema

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestSeedCLIsOnlyIncludePlatformOwnedEntries(t *testing.T) {
	body, err := os.ReadFile("seed-clis.sql")
	if err != nil {
		t.Fatalf("read seed-clis.sql: %v", err)
	}

	seed := string(body)
	required := []string{
		"('grep', 'grep'",
		"('find', 'find'",
		"('sort', 'sort'",
		"('builtin-grep', 'builtin grep'",
		"('builtin-create', 'builtin create'",
		"('builtin-make', 'builtin make'",
		"OFFICIAL_URL_",
	}

	for _, fragment := range required {
		if !strings.Contains(seed, fragment) {
			t.Fatalf("expected seed to contain %q", fragment)
		}
	}
	if strings.Contains(seed, "GITHUB_URL_") {
		t.Fatal("expected seed to stop using GITHUB_URL_")
	}

	slugs := extractSeedValues(t, seed)
	want := []string{
		"awk",
		"builtin-create",
		"builtin-grep",
		"builtin-make",
		"find",
		"grep",
		"ls",
		"sed",
		"sort",
	}
	if !slices.Equal(slugs, want) {
		t.Fatalf("expected platform seed slugs %v, got %v", want, slugs)
	}
}

func TestSeedCLILocalesOnlyIncludePlatformOwnedEntries(t *testing.T) {
	body, err := os.ReadFile("seed-cli-locales.sql")
	if err != nil {
		t.Fatalf("read seed-cli-locales.sql: %v", err)
	}

	seed := string(body)
	required := []string{
		"('grep', 'zh'",
		"('builtin-grep', 'zh'",
		"('builtin-create', 'zh'",
		"('builtin-make', 'zh'",
	}

	for _, fragment := range required {
		if !strings.Contains(seed, fragment) {
			t.Fatalf("expected localized seed to contain %q", fragment)
		}
	}

	slugs := extractSeedValues(t, seed)
	want := []string{
		"awk",
		"builtin-create",
		"builtin-grep",
		"builtin-make",
		"find",
		"grep",
		"ls",
		"sed",
		"sort",
	}
	if !slices.Equal(slugs, want) {
		t.Fatalf("expected localized platform seed slugs %v, got %v", want, slugs)
	}
}

func extractSeedValues(t *testing.T, seed string) []string {
	t.Helper()
	re := regexp.MustCompile(`\('([^']+)', '`)
	matches := re.FindAllStringSubmatch(seed, -1)
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		values = append(values, match[1])
	}
	slices.Sort(values)
	return values
}
