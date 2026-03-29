package builtin

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/linlay/cligrep-server/internal/data"
	appi18n "github.com/linlay/cligrep-server/internal/i18n"
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
		return s.helpResponse(ctx), nil
	}

	tokens, err := util.SplitLine(line)
	if err != nil {
		return models.BuiltinExecResponse{}, err
	}
	if len(tokens) == 0 {
		return s.helpResponse(ctx), nil
	}

	switch tokens[0] {
	case "grep":
		return s.handleGrep(ctx, tokens, request)
	case "help":
		return s.helpResponse(ctx), nil
	case "clear":
		return models.BuiltinExecResponse{
			Action:       "clear",
			Message:      appi18n.Text(ctx, "builtin_clear_done"),
			SessionState: "home",
			ResolvedCLI:  "builtin:clear",
			Hints: []string{
				appi18n.Text(ctx, "builtin_clear_hint_search"),
				appi18n.Text(ctx, "builtin_clear_hint_open"),
			},
		}, nil
	case "create":
		return s.handleCreate(ctx, tokens, request)
	case "make":
		return s.handleMake(ctx, tokens, request)
	default:
		return models.BuiltinExecResponse{
			Action:       "help",
			Message:      appi18n.Text(ctx, "builtin_unknown_command", tokens[0]),
			SessionState: "execution",
			ResolvedCLI:  "builtin:error",
			Execution: &models.ExecutionResult{
				Stdout:       helpText(ctx),
				Stderr:       appi18n.Text(ctx, "builtin_unknown_command_stderr", tokens[0]),
				ExitCode:     1,
				DurationMS:   0,
				Mode:         "builtin",
				ResolvedCLI:  "builtin:error",
				SessionState: "execution",
			},
			Hints: []string{appi18n.Text(ctx, "builtin_available_hint")},
		}, nil
	}
}

func (s *Service) handleGrep(ctx context.Context, tokens []string, request models.BuiltinExecRequest) (models.BuiltinExecResponse, error) {
	query := strings.TrimSpace(strings.Join(tokens[1:], " "))
	if query == "" {
		return models.BuiltinExecResponse{
			Action:       "search",
			Message:      appi18n.Text(ctx, "builtin_grep_query_required"),
			SessionState: "search-results",
			ResolvedCLI:  "builtin:grep",
			Hints:        []string{appi18n.Text(ctx, "builtin_grep_query_hint")},
		}, nil
	}

	results, err := s.store.SearchCLIs(ctx, query, 20)
	if err != nil {
		return models.BuiltinExecResponse{}, err
	}

	return models.BuiltinExecResponse{
		Action:        "search",
		Message:       appi18n.Text(ctx, "builtin_grep_found", len(results), query),
		SessionState:  "search-results",
		ResolvedCLI:   "builtin:grep",
		SearchResults: results,
		Hints: []string{
			appi18n.Text(ctx, "builtin_grep_hint_open"),
			appi18n.Text(ctx, "builtin_grep_hint_escape"),
		},
	}, nil
}

func (s *Service) handleCreate(ctx context.Context, tokens []string, request models.BuiltinExecRequest) (models.BuiltinExecResponse, error) {
	if len(tokens) < 3 || tokens[1] != "python" {
		return models.BuiltinExecResponse{
			Action:       "create",
			Message:      appi18n.Text(ctx, "builtin_create_usage"),
			SessionState: "execution",
			ResolvedCLI:  "builtin:create",
			Execution: &models.ExecutionResult{
				Stdout:       appi18n.Text(ctx, "builtin_create_usage_stdout"),
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
		Message:      appi18n.Text(ctx, "builtin_create_done", scriptName, spec),
		SessionState: "execution",
		ResolvedCLI:  "builtin:create",
		Execution:    &result,
		Asset:        &asset,
		Hints: []string{
			appi18n.Text(ctx, "builtin_create_hint_saved"),
			appi18n.Text(ctx, "builtin_create_hint_next"),
		},
	}, nil
}

func (s *Service) handleMake(ctx context.Context, tokens []string, request models.BuiltinExecRequest) (models.BuiltinExecResponse, error) {
	if len(tokens) < 3 {
		return models.BuiltinExecResponse{
			Action:       "make",
			Message:      appi18n.Text(ctx, "builtin_make_usage"),
			SessionState: "execution",
			ResolvedCLI:  "builtin:make",
			Execution: &models.ExecutionResult{
				Stdout:       appi18n.Text(ctx, "builtin_make_usage_stdout"),
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
			Message:      appi18n.Text(ctx, "builtin_make_unknown_target", targetType),
			SessionState: "execution",
			ResolvedCLI:  "builtin:make",
			Execution: &models.ExecutionResult{
				Stderr:       appi18n.Text(ctx, "builtin_make_unknown_target_stderr", targetType),
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
		Message:      appi18n.Text(ctx, "builtin_make_done", targetType, cli.DisplayName),
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
		Hints: []string{
			appi18n.Text(ctx, "builtin_make_hint_preview"),
			appi18n.Text(ctx, "builtin_make_hint_escape"),
		},
	}, nil
}

func (s *Service) helpResponse(ctx context.Context) models.BuiltinExecResponse {
	return models.BuiltinExecResponse{
		Action:       "help",
		Message:      appi18n.Text(ctx, "builtin_help_loaded"),
		SessionState: "execution",
		ResolvedCLI:  "builtin:help",
		Execution: &models.ExecutionResult{
			Stdout:       helpText(ctx),
			ExitCode:     0,
			Mode:         "builtin",
			ResolvedCLI:  "builtin:help",
			SessionState: "execution",
		},
		Hints: []string{
			appi18n.Text(ctx, "builtin_generic_hint_search_wrap"),
			appi18n.Text(ctx, "builtin_generic_hint_runtime"),
		},
	}
}

func helpText(ctx context.Context) string {
	return strings.Join([]string{
		appi18n.Text(ctx, "builtin_help_title"),
		"",
		"grep <query>",
		"  " + appi18n.Text(ctx, "builtin_help_grep_desc"),
		"",
		"create python \"<spec>\"",
		"  " + appi18n.Text(ctx, "builtin_help_create_desc"),
		"",
		"make sandbox <cli>",
		"  " + appi18n.Text(ctx, "builtin_help_make_sandbox_desc"),
		"",
		"make dockerfile <cli>",
		"  " + appi18n.Text(ctx, "builtin_help_make_dockerfile_desc"),
		"",
		appi18n.Text(ctx, "builtin_help_footer"),
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
