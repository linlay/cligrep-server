package seed

import (
	"context"
	"fmt"
	"time"

	"github.com/linlay/cligrep-server/internal/models"
	"github.com/linlay/cligrep-server/internal/sandbox"
)

type CommandSeed struct {
	Slug        string
	DisplayName string
	Summary     string
	Tags        []string
	Popularity  int
	ExampleLine string
}

func Defaults() []CommandSeed {
	return []CommandSeed{
		{Slug: "grep", DisplayName: "grep", Summary: "Search text with regular expressions in a sandboxed BusyBox runtime.", Tags: []string{"text", "search", "busybox"}, Popularity: 100, ExampleLine: "grep --help"},
		{Slug: "find", DisplayName: "find", Summary: "Traverse directory trees and match files inside the sandbox.", Tags: []string{"filesystem", "search", "busybox"}, Popularity: 96, ExampleLine: "find --help"},
		{Slug: "sed", DisplayName: "sed", Summary: "Stream editing for line-by-line text transformation.", Tags: []string{"text", "transform", "busybox"}, Popularity: 94, ExampleLine: "sed --help"},
		{Slug: "awk", DisplayName: "awk", Summary: "Pattern scanning and report generation for structured text.", Tags: []string{"text", "report", "busybox"}, Popularity: 92, ExampleLine: "awk --help"},
		{Slug: "cat", DisplayName: "cat", Summary: "Concatenate and inspect files from the runtime filesystem.", Tags: []string{"text", "viewer", "busybox"}, Popularity: 90, ExampleLine: "cat --help"},
		{Slug: "ls", DisplayName: "ls", Summary: "List files and inspect directories within the isolated container.", Tags: []string{"filesystem", "list", "busybox"}, Popularity: 88, ExampleLine: "ls --help"},
		{Slug: "sort", DisplayName: "sort", Summary: "Sort textual input with standard shell-friendly switches.", Tags: []string{"text", "sort", "busybox"}, Popularity: 86, ExampleLine: "sort --help"},
		{Slug: "head", DisplayName: "head", Summary: "Preview the beginning of files or command output.", Tags: []string{"text", "preview", "busybox"}, Popularity: 84, ExampleLine: "head --help"},
		{Slug: "tail", DisplayName: "tail", Summary: "Inspect the end of files with familiar terminal semantics.", Tags: []string{"text", "preview", "busybox"}, Popularity: 82, ExampleLine: "tail --help"},
		{Slug: "xargs", DisplayName: "xargs", Summary: "Build and execute argument lists from standard input safely.", Tags: []string{"pipeline", "args", "busybox"}, Popularity: 80, ExampleLine: "xargs --help"},
	}
}

func Builtins() []models.CLI {
	return []models.CLI{
		{Slug: "builtin-grep", DisplayName: "builtin grep", Summary: "The website-native registry search command.", Type: models.CLITypeBuiltin, Tags: []string{"builtin", "search", "core"}, HelpText: "Use grep <query> to search indexed CLIs.", VersionText: "cligrep builtin v1", Popularity: 110, RuntimeImage: "builtin", Enabled: true, ExampleLine: "grep rip"},
		{Slug: "builtin-create", DisplayName: "builtin create", Summary: "Generate Python CLI scaffolds inside the website sandbox flow.", Type: models.CLITypeBuiltin, Tags: []string{"builtin", "generator", "python"}, HelpText: "Use create python \"your spec\".", VersionText: "cligrep builtin v1", Popularity: 75, RuntimeImage: "builtin", Enabled: true, ExampleLine: "create python \"make a todo cli\""},
		{Slug: "builtin-make", DisplayName: "builtin make", Summary: "Generate Dockerfile and sandbox recipe previews for CLIs.", Type: models.CLITypeBuiltin, Tags: []string{"builtin", "docker", "sandbox"}, HelpText: "Use make sandbox <cli> or make dockerfile <cli>.", VersionText: "cligrep builtin v1", Popularity: 72, RuntimeImage: "builtin", Enabled: true, ExampleLine: "make dockerfile grep"},
	}
}

func ExtractSeededCLIs(ctx context.Context, runner *sandbox.Runner) []models.CLI {
	commands := Defaults()
	seeded := make([]models.CLI, 0, len(commands))

	for _, command := range commands {
		extractCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 900*time.Millisecond)
		helpText, versionText, err := runner.ExtractBusyBoxHelp(extractCtx, command.Slug)
		cancel()
		if err != nil || helpText == "" {
			helpText = fallbackHelp(command)
		}
		if versionText == "" {
			versionText = "busybox-1.36"
		}

		seeded = append(seeded, models.CLI{
			Slug:         command.Slug,
			DisplayName:  command.DisplayName,
			Summary:      command.Summary,
			Type:         models.CLITypeSystem,
			Tags:         command.Tags,
			HelpText:     helpText,
			VersionText:  versionText,
			Popularity:   command.Popularity,
			RuntimeImage: "busybox:1.36.1",
			Enabled:      true,
			ExampleLine:  command.ExampleLine,
		})
	}

	return append(seeded, Builtins()...)
}

func fallbackHelp(command CommandSeed) string {
	return fmt.Sprintf("%s\n\nGenerated fallback help at %s.\n\nExample: %s",
		command.Summary,
		time.Now().Format(time.RFC3339),
		command.ExampleLine,
	)
}
