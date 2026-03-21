package seed

import (
	"context"
	"fmt"
	"time"

	"github.com/linlay/cligrep-server/internal/models"
	"github.com/linlay/cligrep-server/internal/sandbox"
)

type CommandSeed struct {
	Slug            string
	DisplayName     string
	Summary         string
	Tags            []string
	Popularity      int
	ExampleLine     string
	Type            models.CLIType
	EnvironmentKind models.EnvironmentKind
	SourceType      string
	Author          string
	GitHubURL       string
	GiteeURL        string
	License         string
	CreatedAt       time.Time
	OriginalCommand string
	Executable      bool
	RuntimeImage    string
}

type FavoriteSeed struct {
	Username string
	CLISlug  string
}

type ExecutionSeed struct {
	SeedKey    string
	CLISlug    string
	Line       string
	Mode       string
	DurationMS int64
	CreatedAt  time.Time
}

func sandboxSeeds() []CommandSeed {
	return []CommandSeed{
		{
			Slug: "grep", DisplayName: "grep", Summary: "Search text with regular expressions inside the sandbox.", Tags: []string{"search", "busybox", "text"},
			Popularity: 100, ExampleLine: "grep --help", Type: models.CLITypeSystem, EnvironmentKind: models.EnvironmentKindSandbox,
			SourceType: "container_binary", Author: "BusyBox", GitHubURL: "https://github.com/mirror/busybox", License: "GPL-2.0",
			CreatedAt: time.Date(2025, 12, 10, 10, 0, 0, 0, time.UTC), OriginalCommand: "grep", Executable: true, RuntimeImage: "busybox:1.36.1",
		},
		{
			Slug: "find", DisplayName: "find", Summary: "Traverse directory trees and match files in the sandbox runtime.", Tags: []string{"search", "filesystem", "busybox"},
			Popularity: 96, ExampleLine: "find --help", Type: models.CLITypeSystem, EnvironmentKind: models.EnvironmentKindSandbox,
			SourceType: "container_binary", Author: "BusyBox", GitHubURL: "https://github.com/mirror/busybox", License: "GPL-2.0",
			CreatedAt: time.Date(2025, 12, 18, 10, 0, 0, 0, time.UTC), OriginalCommand: "find", Executable: true, RuntimeImage: "busybox:1.36.1",
		},
		{
			Slug: "sed", DisplayName: "sed", Summary: "Stream editing for line-by-line text transformation.", Tags: []string{"transform", "busybox", "text"},
			Popularity: 94, ExampleLine: "sed --help", Type: models.CLITypeSystem, EnvironmentKind: models.EnvironmentKindSandbox,
			SourceType: "container_binary", Author: "BusyBox", GitHubURL: "https://github.com/mirror/busybox", License: "GPL-2.0",
			CreatedAt: time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC), OriginalCommand: "sed", Executable: true, RuntimeImage: "busybox:1.36.1",
		},
		{
			Slug: "awk", DisplayName: "awk", Summary: "Pattern scanning and report generation for structured text.", Tags: []string{"report", "busybox", "text"},
			Popularity: 92, ExampleLine: "awk --help", Type: models.CLITypeSystem, EnvironmentKind: models.EnvironmentKindSandbox,
			SourceType: "container_binary", Author: "BusyBox", GitHubURL: "https://github.com/mirror/busybox", License: "GPL-2.0",
			CreatedAt: time.Date(2026, 1, 14, 10, 0, 0, 0, time.UTC), OriginalCommand: "awk", Executable: true, RuntimeImage: "busybox:1.36.1",
		},
		{
			Slug: "ls", DisplayName: "ls", Summary: "List files and inspect directories within the isolated container.", Tags: []string{"list", "filesystem", "busybox"},
			Popularity: 88, ExampleLine: "ls --help", Type: models.CLITypeSystem, EnvironmentKind: models.EnvironmentKindSandbox,
			SourceType: "container_binary", Author: "BusyBox", GitHubURL: "https://github.com/mirror/busybox", License: "GPL-2.0",
			CreatedAt: time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC), OriginalCommand: "ls", Executable: true, RuntimeImage: "busybox:1.36.1",
		},
		{
			Slug: "sort", DisplayName: "sort", Summary: "Sort textual input with standard shell-friendly switches.", Tags: []string{"sort", "busybox", "text"},
			Popularity: 86, ExampleLine: "sort --help", Type: models.CLITypeSystem, EnvironmentKind: models.EnvironmentKindSandbox,
			SourceType: "container_binary", Author: "BusyBox", GitHubURL: "https://github.com/mirror/busybox", License: "GPL-2.0",
			CreatedAt: time.Date(2026, 2, 12, 10, 0, 0, 0, time.UTC), OriginalCommand: "sort", Executable: true, RuntimeImage: "busybox:1.36.1",
		},
	}
}

func textSeeds() []CommandSeed {
	return []CommandSeed{
		{
			Slug: "rg", DisplayName: "ripgrep", Summary: "A Rust-native recursive search CLI with fast ignore handling.", Tags: []string{"search", "native-rust", "text"},
			Popularity: 99, ExampleLine: "rg TODO src", Type: models.CLITypeNative, EnvironmentKind: models.EnvironmentKindText,
			SourceType: "native_rust", Author: "BurntSushi", GitHubURL: "https://github.com/BurntSushi/ripgrep", License: "MIT OR Unlicense",
			CreatedAt: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC), OriginalCommand: "rg", Executable: false, RuntimeImage: "text-only",
		},
		{
			Slug: "fd", DisplayName: "fd", Summary: "A user-friendly Rust-native alternative to find.", Tags: []string{"search", "native-rust", "filesystem"},
			Popularity: 91, ExampleLine: "fd package json", Type: models.CLITypeNative, EnvironmentKind: models.EnvironmentKindText,
			SourceType: "native_rust", Author: "sharkdp", GitHubURL: "https://github.com/sharkdp/fd", License: "MIT OR Apache-2.0",
			CreatedAt: time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC), OriginalCommand: "fd", Executable: false, RuntimeImage: "text-only",
		},
		{
			Slug: "glow", DisplayName: "glow", Summary: "A native Go markdown reader for terminal-first documentation.", Tags: []string{"docs", "native-go", "markdown"},
			Popularity: 83, ExampleLine: "glow README.md", Type: models.CLITypeNative, EnvironmentKind: models.EnvironmentKindText,
			SourceType: "native_go", Author: "Charmbracelet", GitHubURL: "https://github.com/charmbracelet/glow", License: "MIT",
			CreatedAt: time.Date(2026, 1, 28, 10, 0, 0, 0, time.UTC), OriginalCommand: "glow", Executable: false, RuntimeImage: "text-only",
		},
		{
			Slug: "uv", DisplayName: "uv", Summary: "Rust-based Python tooling for environments and package management.", Tags: []string{"python", "native-rust", "package-manager"},
			Popularity: 97, ExampleLine: "uv pip install rich", Type: models.CLITypeNative, EnvironmentKind: models.EnvironmentKindText,
			SourceType: "native_rust", Author: "Astral", GitHubURL: "https://github.com/astral-sh/uv", License: "MIT OR Apache-2.0",
			CreatedAt: time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC), OriginalCommand: "uv", Executable: false, RuntimeImage: "text-only",
		},
		{
			Slug: "npm", DisplayName: "npm", Summary: "Node package manager help and usage reference.", Tags: []string{"node", "package-manager", "help-only"},
			Popularity: 95, ExampleLine: "npm install vite", Type: models.CLITypeText, EnvironmentKind: models.EnvironmentKindText,
			SourceType: "package_manager", Author: "npm", GitHubURL: "https://github.com/npm/cli", License: "Artistic-2.0",
			CreatedAt: time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC), OriginalCommand: "npm", Executable: false, RuntimeImage: "text-only",
		},
		{
			Slug: "apt", DisplayName: "apt", Summary: "APT package manager reference for Debian and Ubuntu systems.", Tags: []string{"linux", "package-manager", "help-only"},
			Popularity: 89, ExampleLine: "apt install ripgrep", Type: models.CLITypeText, EnvironmentKind: models.EnvironmentKindText,
			SourceType: "package_manager", Author: "Debian APT Team", License: "GPL-2.0",
			CreatedAt: time.Date(2026, 1, 22, 10, 0, 0, 0, time.UTC), OriginalCommand: "apt", Executable: false, RuntimeImage: "text-only",
		},
		{
			Slug: "mcp-inspect", DisplayName: "mcp inspect", Summary: "Inspect an MCP server manifest and bridge metadata through a CLI wrapper.", Tags: []string{"mcp", "bridge", "tooling"},
			Popularity: 79, ExampleLine: "mcp-inspect server.json", Type: models.CLITypeGateway, EnvironmentKind: models.EnvironmentKindText,
			SourceType: "mcp_bridge", Author: "CLI Grep Labs", License: "Apache-2.0",
			CreatedAt: time.Date(2026, 3, 14, 10, 0, 0, 0, time.UTC), OriginalCommand: "mcp-inspect", Executable: false, RuntimeImage: "text-only",
		},
		{
			Slug: "yt-skim.py", DisplayName: "yt-skim.py", Summary: "A Python script CLI for skimming transcripts and producing notes.", Tags: []string{"python", "script", "notes"},
			Popularity: 76, ExampleLine: "yt-skim.py https://youtu.be/demo", Type: models.CLITypeGateway, EnvironmentKind: models.EnvironmentKindText,
			SourceType: "python_script", Author: "CLI Grep Labs", License: "MIT",
			CreatedAt: time.Date(2026, 3, 6, 10, 0, 0, 0, time.UTC), OriginalCommand: "python yt_skim.py", Executable: false, RuntimeImage: "text-only",
		},
	}
}

func Builtins() []models.CLI {
	return []models.CLI{
		{
			Slug: "builtin-grep", DisplayName: "builtin grep", Summary: "The website-native registry search command.",
			Type: models.CLITypeBuiltin, Tags: []string{"builtin", "search", "core"}, HelpText: "Use grep <query> to search indexed CLIs.",
			VersionText: "cligrep builtin v2", Popularity: 110, RuntimeImage: "website", Enabled: true, ExampleLine: "grep rip",
			EnvironmentKind: models.EnvironmentKindWebsite, SourceType: "website_builtin", Author: "CLI Grep", License: "Proprietary",
			CreatedAt: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC), OriginalCommand: "grep", Executable: true,
		},
		{
			Slug: "builtin-create", DisplayName: "builtin create", Summary: "Generate Python CLI scaffolds inside the website flow.",
			Type: models.CLITypeBuiltin, Tags: []string{"builtin", "generator", "python"}, HelpText: "Use create python \"your spec\".",
			VersionText: "cligrep builtin v2", Popularity: 75, RuntimeImage: "website", Enabled: true, ExampleLine: "create python \"make a todo cli\"",
			EnvironmentKind: models.EnvironmentKindWebsite, SourceType: "website_builtin", Author: "CLI Grep", License: "Proprietary",
			CreatedAt: time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC), OriginalCommand: "create python", Executable: true,
		},
		{
			Slug: "builtin-make", DisplayName: "builtin make", Summary: "Generate Dockerfile and sandbox recipe previews for CLIs.",
			Type: models.CLITypeBuiltin, Tags: []string{"builtin", "docker", "sandbox"}, HelpText: "Use make sandbox <cli> or make dockerfile <cli>.",
			VersionText: "cligrep builtin v2", Popularity: 72, RuntimeImage: "website", Enabled: true, ExampleLine: "make dockerfile grep",
			EnvironmentKind: models.EnvironmentKindWebsite, SourceType: "website_builtin", Author: "CLI Grep", License: "Proprietary",
			CreatedAt: time.Date(2026, 1, 3, 10, 0, 0, 0, time.UTC), OriginalCommand: "make", Executable: true,
		},
	}
}

func ExtractSeededCLIs(ctx context.Context, runner *sandbox.Runner) []models.CLI {
	seeds := append(sandboxSeeds(), textSeeds()...)
	seeded := make([]models.CLI, 0, len(seeds))

	for _, command := range seeds {
		helpText := fallbackHelp(command)
		versionText := "docs-only"

		if command.EnvironmentKind == models.EnvironmentKindSandbox {
			extractCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 900*time.Millisecond)
			extractedHelp, extractedVersion, err := runner.ExtractBusyBoxHelp(extractCtx, command.Slug)
			cancel()
			if err == nil && extractedHelp != "" {
				helpText = extractedHelp
			}
			if extractedVersion != "" {
				versionText = extractedVersion
			} else {
				versionText = "busybox-1.36"
			}
		}

		seeded = append(seeded, models.CLI{
			Slug:            command.Slug,
			DisplayName:     command.DisplayName,
			Summary:         command.Summary,
			Type:            command.Type,
			Tags:            command.Tags,
			HelpText:        helpText,
			VersionText:     versionText,
			Popularity:      command.Popularity,
			RuntimeImage:    command.RuntimeImage,
			Enabled:         true,
			ExampleLine:     command.ExampleLine,
			EnvironmentKind: command.EnvironmentKind,
			SourceType:      command.SourceType,
			Author:          command.Author,
			GitHubURL:       command.GitHubURL,
			GiteeURL:        command.GiteeURL,
			License:         command.License,
			CreatedAt:       command.CreatedAt,
			OriginalCommand: command.OriginalCommand,
			Executable:      command.Executable,
		})
	}

	return append(seeded, Builtins()...)
}

func MockUsers() []string {
	return []string{
		"anonymous",
		"operator",
		"lin",
		"mei",
		"kai",
		"atlas",
		"ember",
		"shellcat",
	}
}

func FavoriteSeeds() []FavoriteSeed {
	return []FavoriteSeed{
		{Username: "operator", CLISlug: "rg"},
		{Username: "lin", CLISlug: "rg"},
		{Username: "mei", CLISlug: "rg"},
		{Username: "kai", CLISlug: "uv"},
		{Username: "atlas", CLISlug: "uv"},
		{Username: "ember", CLISlug: "grep"},
		{Username: "shellcat", CLISlug: "grep"},
		{Username: "lin", CLISlug: "fd"},
		{Username: "mei", CLISlug: "npm"},
		{Username: "atlas", CLISlug: "glow"},
		{Username: "ember", CLISlug: "mcp-inspect"},
		{Username: "operator", CLISlug: "yt-skim.py"},
	}
}

func ExecutionSeeds() []ExecutionSeed {
	nowBase := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	add := func(cliSlug, line string, count int, duration int64) []ExecutionSeed {
		items := make([]ExecutionSeed, 0, count)
		for i := 0; i < count; i++ {
			items = append(items, ExecutionSeed{
				SeedKey:    fmt.Sprintf("%s-%02d", cliSlug, i+1),
				CLISlug:    cliSlug,
				Line:       line,
				Mode:       "seed",
				DurationMS: duration + int64(i*7),
				CreatedAt:  nowBase.Add(-time.Duration(i) * 6 * time.Hour),
			})
		}
		return items
	}

	var seeds []ExecutionSeed
	seeds = append(seeds, add("rg", "rg TODO src", 24, 31)...)
	seeds = append(seeds, add("uv", "uv pip install rich", 18, 42)...)
	seeds = append(seeds, add("grep", "grep --help", 16, 11)...)
	seeds = append(seeds, add("fd", "fd package json", 12, 36)...)
	seeds = append(seeds, add("npm", "npm install vite", 10, 55)...)
	seeds = append(seeds, add("glow", "glow README.md", 8, 27)...)
	seeds = append(seeds, add("mcp-inspect", "mcp-inspect server.json", 6, 44)...)
	seeds = append(seeds, add("yt-skim.py", "python yt_skim.py https://youtu.be/demo", 4, 63)...)
	seeds = append(seeds, add("apt", "apt install ripgrep", 3, 58)...)
	return seeds
}

func fallbackHelp(command CommandSeed) string {
	if command.EnvironmentKind == models.EnvironmentKindText {
		return fmt.Sprintf("%s\n\nThis command is indexed for documentation only on CLI GREP.\nOriginal command: %s\nExample: %s",
			command.Summary,
			command.OriginalCommand,
			command.ExampleLine,
		)
	}

	return fmt.Sprintf("%s\n\nGenerated fallback help at %s.\n\nExample: %s",
		command.Summary,
		time.Date(2026, 3, 21, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		command.ExampleLine,
	)
}
