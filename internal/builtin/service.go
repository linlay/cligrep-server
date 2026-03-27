package builtin

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/linlay/cligrep-server/internal/data"
	"github.com/linlay/cligrep-server/internal/models"
	"github.com/linlay/cligrep-server/internal/sandbox"
	"github.com/linlay/cligrep-server/internal/util"
)

type Service struct {
	store  *data.Store
	runner *sandbox.Runner
}

func NewService(store *data.Store, runner *sandbox.Runner) *Service {
	return &Service{store: store, runner: runner}
}

func (s *Service) Execute(ctx context.Context, request models.BuiltinExecRequest) (models.BuiltinExecResponse, error) {
	line := strings.TrimSpace(request.Line)
	if strings.Contains(line, "\n") {
		return models.BuiltinExecResponse{}, fmt.Errorf("multiline input is not allowed")
	}
	if line == "" {
		return s.helpResponse(), nil
	}

	tokens, err := util.SplitLine(line)
	if err != nil {
		return models.BuiltinExecResponse{}, err
	}
	if len(tokens) == 0 {
		return s.helpResponse(), nil
	}

	switch tokens[0] {
	case "grep":
		return s.handleGrep(ctx, tokens, request)
	case "help":
		return s.helpResponse(), nil
	case "clear":
		return models.BuiltinExecResponse{
			Action:       "clear",
			Message:      "Terminal cleared. Back to the registry homepage.",
			SessionState: "home",
			ResolvedCLI:  "builtin:clear",
			Hints:        []string{"Use grep <query> to search.", "Press Enter on a highlighted result to open it."},
		}, nil
	case "create":
		return s.handleCreate(ctx, tokens, request)
	case "make":
		return s.handleMake(ctx, tokens, request)
	default:
		return models.BuiltinExecResponse{
			Action:       "help",
			Message:      fmt.Sprintf("Unknown built-in command %q.", tokens[0]),
			SessionState: "execution",
			ResolvedCLI:  "builtin:error",
			Execution: &models.ExecutionResult{
				Stdout:       helpText(),
				Stderr:       fmt.Sprintf("unknown built-in command: %s", tokens[0]),
				ExitCode:     1,
				DurationMS:   0,
				Mode:         "builtin",
				ResolvedCLI:  "builtin:error",
				SessionState: "execution",
			},
			Hints: []string{"Available built-ins: grep, create, make, help, clear."},
		}, nil
	}
}

func (s *Service) handleGrep(ctx context.Context, tokens []string, request models.BuiltinExecRequest) (models.BuiltinExecResponse, error) {
	query := strings.TrimSpace(strings.Join(tokens[1:], " "))
	if query == "" {
		return models.BuiltinExecResponse{
			Action:       "search",
			Message:      "grep needs a query. Example: grep ripgrep",
			SessionState: "search-results",
			ResolvedCLI:  "builtin:grep",
			Hints:        []string{"Search name, tags, summaries, and stored help text."},
		}, nil
	}

	results, err := s.store.SearchCLIs(ctx, query, 20)
	if err != nil {
		return models.BuiltinExecResponse{}, err
	}

	return models.BuiltinExecResponse{
		Action:        "search",
		Message:       fmt.Sprintf("Found %d CLI matches for %q.", len(results), query),
		SessionState:  "search-results",
		ResolvedCLI:   "builtin:grep",
		SearchResults: results,
		Hints:         []string{"Enter on a highlighted result opens its run mode.", "Esc returns to the homepage grid."},
	}, nil
}

func (s *Service) handleCreate(ctx context.Context, tokens []string, request models.BuiltinExecRequest) (models.BuiltinExecResponse, error) {
	if len(tokens) < 3 || tokens[1] != "python" {
		return models.BuiltinExecResponse{
			Action:       "create",
			Message:      "Usage: create python \"build a CLI that ...\"",
			SessionState: "execution",
			ResolvedCLI:  "builtin:create",
			Execution: &models.ExecutionResult{
				Stdout:       "Use create python \"your spec\" to generate and preview a Python CLI.",
				ExitCode:     0,
				Mode:         "builtin",
				ResolvedCLI:  "builtin:create",
				SessionState: "execution",
			},
		}, nil
	}

	spec := strings.TrimSpace(strings.Join(tokens[2:], " "))
	scriptName, scriptContent := generatePythonCLI(spec)

	result, err := s.runner.RunPythonScript(ctx, scriptName, scriptContent, []string{"--help"})
	if err != nil {
		return models.BuiltinExecResponse{}, err
	}
	result.Mode = "builtin"
	result.SessionState = "execution"

	var userID *int64
	if request.UserID != nil {
		userID = request.UserID
	}

	asset, err := s.store.SaveAsset(ctx, models.GeneratedAsset{
		Kind:    "python-cli",
		Name:    scriptName,
		UserID:  userID,
		Content: scriptContent,
	})
	if err != nil {
		return models.BuiltinExecResponse{}, err
	}

	return models.BuiltinExecResponse{
		Action:       "create",
		Message:      fmt.Sprintf("Generated %s from spec %q and previewed it in the Python sandbox.", scriptName, spec),
		SessionState: "execution",
		ResolvedCLI:  "builtin:create",
		Execution:    &result,
		Asset:        &asset,
		Hints:        []string{"The generated file is saved to the database as an asset.", "Use make dockerfile <cli> for a packaging draft next."},
	}, nil
}

func (s *Service) handleMake(ctx context.Context, tokens []string, request models.BuiltinExecRequest) (models.BuiltinExecResponse, error) {
	if len(tokens) < 3 {
		return models.BuiltinExecResponse{
			Action:       "make",
			Message:      "Usage: make sandbox <cli> or make dockerfile <cli>",
			SessionState: "execution",
			ResolvedCLI:  "builtin:make",
			Execution: &models.ExecutionResult{
				Stdout:       "Examples:\nmake sandbox grep\nmake dockerfile grep",
				ExitCode:     0,
				Mode:         "builtin",
				ResolvedCLI:  "builtin:make",
				SessionState: "execution",
			},
		}, nil
	}

	targetType := tokens[1]
	cliSlug := tokens[2]

	cli, err := s.store.GetCLI(ctx, cliSlug)
	if err != nil {
		return models.BuiltinExecResponse{}, fmt.Errorf("load cli %s: %w", cliSlug, err)
	}

	var (
		name    string
		content string
		kind    string
	)

	switch targetType {
	case "sandbox":
		name = fmt.Sprintf("%s-sandbox.yaml", cli.Slug)
		kind = "sandbox-config"
		content = sandboxRecipe(cli)
	case "dockerfile":
		name = fmt.Sprintf("%s.Dockerfile", cli.Slug)
		kind = "dockerfile"
		content = dockerfileTemplate(cli)
	default:
		return models.BuiltinExecResponse{
			Action:       "make",
			Message:      fmt.Sprintf("Unknown make target %q.", targetType),
			SessionState: "execution",
			ResolvedCLI:  "builtin:make",
			Execution: &models.ExecutionResult{
				Stderr:       fmt.Sprintf("unknown make target: %s", targetType),
				ExitCode:     1,
				Mode:         "builtin",
				ResolvedCLI:  "builtin:make",
				SessionState: "execution",
			},
		}, nil
	}

	var userID *int64
	if request.UserID != nil {
		userID = request.UserID
	}

	asset, err := s.store.SaveAsset(ctx, models.GeneratedAsset{
		Kind:    kind,
		Name:    name,
		CLISlug: cli.Slug,
		UserID:  userID,
		Content: content,
	})
	if err != nil {
		return models.BuiltinExecResponse{}, err
	}

	return models.BuiltinExecResponse{
		Action:       "make",
		Message:      fmt.Sprintf("Generated %s preview for %s.", targetType, cli.DisplayName),
		SessionState: "execution",
		ResolvedCLI:  "builtin:make",
		Asset:        &asset,
		Execution: &models.ExecutionResult{
			Stdout:       content,
			ExitCode:     0,
			Mode:         "builtin",
			ResolvedCLI:  "builtin:make",
			SessionState: "execution",
		},
		Hints: []string{"Generated artifacts are previews backed by the database.", "Use Esc to return to search/home."},
	}, nil
}

func (s *Service) helpResponse() models.BuiltinExecResponse {
	return models.BuiltinExecResponse{
		Action:       "help",
		Message:      "Built-in command reference loaded.",
		SessionState: "execution",
		ResolvedCLI:  "builtin:help",
		Execution: &models.ExecutionResult{
			Stdout:       helpText(),
			ExitCode:     0,
			Mode:         "builtin",
			ResolvedCLI:  "builtin:help",
			SessionState: "execution",
		},
		Hints: []string{"Search mode auto-wraps plain text as grep <query>.", "Built-ins stay website-native; ordinary CLIs run in Docker sandboxes."},
	}
}

func helpText() string {
	return strings.Join([]string{
		"CLI Grep built-ins",
		"",
		"grep <query>",
		"  Search indexed CLIs by name, summary, tags, and stored help text.",
		"",
		"create python \"<spec>\"",
		"  Generate a single-file Python CLI scaffold and preview it with --help.",
		"",
		"make sandbox <cli>",
		"  Generate a sandbox recipe preview.",
		"",
		"make dockerfile <cli>",
		"  Generate a Dockerfile preview for the selected CLI.",
		"",
		"clear / help",
	}, "\n")
}

func generatePythonCLI(spec string) (string, string) {
	name := slugify(spec)
	if name == "" {
		name = "generated_cli"
	}
	fileName := fmt.Sprintf("%s.py", name)

	script := fmt.Sprintf(`#!/usr/bin/env python3
import argparse

DESCRIPTION = %q

def build_parser():
    parser = argparse.ArgumentParser(
        prog=%q,
        description=DESCRIPTION,
    )
    parser.add_argument("value", nargs="?", default="world", help="sample input payload")
    parser.add_argument("--loud", action="store_true", help="print in uppercase")
    return parser

def main():
    parser = build_parser()
    args = parser.parse_args()
    message = f"%%s -> %%s" %% (DESCRIPTION, args.value)
    if args.loud:
        message = message.upper()
    print(message)

if __name__ == "__main__":
    main()
`, spec, name)

	return fileName, script
}

func slugify(spec string) string {
	lower := strings.ToLower(spec)
	pattern := regexp.MustCompile(`[^a-z0-9]+`)
	slug := pattern.ReplaceAllString(lower, "_")
	slug = strings.Trim(slug, "_")
	if len(slug) > 24 {
		slug = slug[:24]
	}
	return slug
}

func sandboxRecipe(cli models.CLI) string {
	return fmt.Sprintf(`name: %s
runtime: %s
commandPolicy:
  mode: single-line
  allowShellOperators: false
  allowlist:
    - %s
limits:
  timeoutMs: 4000
  cpus: "0.50"
  memory: "128m"
network: none
filesystem:
  rootfs: read-only
  tmpfs:
    - /tmp
`, cli.Slug, cli.RuntimeImage, cli.Slug)
}

func dockerfileTemplate(cli models.CLI) string {
	return fmt.Sprintf(`FROM %s

WORKDIR /workspace

# Copy your wrapper or config files into the sandbox image here.
# This is a mock Dockerfile preview generated by CLI Grep.

ENTRYPOINT ["busybox", "%s"]
CMD ["--help"]
`, cli.RuntimeImage, cli.Slug)
}
