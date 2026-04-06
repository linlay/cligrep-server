package mysqlschema

import (
	"os"
	"strings"
	"testing"
)

func TestSeedCLIsIncludesImportedUpstreamEntries(t *testing.T) {
	body, err := os.ReadFile("seed-clis.sql")
	if err != nil {
		t.Fatalf("read seed-clis.sql: %v", err)
	}

	seed := string(body)
	required := []string{
		"('gh', 'GitHub CLI'",
		"https://github.com/cli/cli",
		"('wecom-cli', 'WeCom CLI'",
		"https://github.com/WecomTeam/wecom-cli",
		"('obsidian', 'Obsidian CLI'",
		"https://obsidian.md/cli",
		"('jq', 'jq'",
		"https://github.com/jqlang/jq",
		"('songsee', 'songsee'",
		"https://github.com/steipete/songsee",
		"('playwright', 'Playwright CLI'",
		"playwright-cli",
		"npx playwright",
		"('vercel', 'Vercel CLI'",
		"https://github.com/vercel/vercel",
		"('supabase', 'Supabase CLI'",
		"https://github.com/supabase/cli",
		"('ffmpeg', 'FFmpeg'",
		"https://github.com/FFmpeg/FFmpeg",
		"('notebooklm', 'notebooklm-py'",
		"notebooklm-py",
		"https://github.com/teng-lin/notebooklm-py",
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
}

func TestSeedCLILocalesIncludesChineseBuiltins(t *testing.T) {
	body, err := os.ReadFile("seed-cli-locales.sql")
	if err != nil {
		t.Fatalf("read seed-cli-locales.sql: %v", err)
	}

	seed := string(body)
	required := []string{
		"('builtin-grep', 'zh'",
		"('builtin-create', 'zh'",
		"('builtin-make', 'zh'",
		"('gh', 'zh'",
		"('wecom-cli', 'zh'",
		"('obsidian', 'zh'",
		"('jq', 'zh'",
		"('songsee', 'zh'",
	}

	for _, fragment := range required {
		if !strings.Contains(seed, fragment) {
			t.Fatalf("expected localized seed to contain %q", fragment)
		}
	}
}
