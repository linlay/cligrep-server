package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/linlay/cligrep-server/internal/builtin"
	"github.com/linlay/cligrep-server/internal/config"
	"github.com/linlay/cligrep-server/internal/data"
	"github.com/linlay/cligrep-server/internal/models"
	"github.com/linlay/cligrep-server/internal/sandbox"
	"github.com/linlay/cligrep-server/internal/seed"
	"github.com/linlay/cligrep-server/internal/util"
)

type App struct {
	cfg      config.Config
	store    *data.Store
	runner   *sandbox.Runner
	builtins *builtin.Service
}

func New(ctx context.Context, cfg config.Config) (*App, error) {
	store, err := data.Open(cfg.DatabasePath)
	if err != nil {
		return nil, err
	}

	runner := sandbox.NewRunner(cfg)
	clis := seed.ExtractSeededCLIs(ctx, runner)
	if err := store.SeedCLIs(ctx, clis); err != nil {
		store.Close()
		return nil, fmt.Errorf("seed database: %w", err)
	}
	if err := store.SeedMockUsers(ctx, seed.MockUsers()); err != nil {
		store.Close()
		return nil, fmt.Errorf("seed mock users: %w", err)
	}
	for _, favorite := range seed.FavoriteSeeds() {
		if err := store.SeedFavoritesByUsername(ctx, favorite.Username, favorite.CLISlug); err != nil {
			store.Close()
			return nil, fmt.Errorf("seed favorites: %w", err)
		}
	}
	for _, execution := range seed.ExecutionSeeds() {
		if err := store.SeedExecutionLog(ctx, execution.SeedKey, execution.CLISlug, execution.Line, execution.Mode, execution.DurationMS, execution.CreatedAt); err != nil {
			store.Close()
			return nil, fmt.Errorf("seed execution logs: %w", err)
		}
	}

	return &App{
		cfg:      cfg,
		store:    store,
		runner:   runner,
		builtins: builtin.NewService(store, runner),
	}, nil
}

func (a *App) Close() error {
	return a.store.Close()
}

func (a *App) SandboxStatus(ctx context.Context) sandbox.ProbeResult {
	return a.runner.Probe(ctx)
}

func (a *App) Health(ctx context.Context) map[string]any {
	sandboxStatus := a.SandboxStatus(ctx)

	return map[string]any{
		"status":            "ok",
		"busyboxImage":      a.cfg.BusyBoxImage,
		"pythonImage":       a.cfg.PythonImage,
		"databasePath":      a.cfg.DatabasePath,
		"timestamp":         time.Now().UTC().Format(time.RFC3339),
		"commandTimeoutMs":  a.cfg.CommandTimeout.Milliseconds(),
		"singleLineOnly":    true,
		"runtimeKinds":      []string{"SANDBOX", "WEBSITE", "TEXT"},
		"homepageSortModes": []string{"favorites", "newest", "runs"},
		"sandboxReady":      sandboxStatus.Ready,
		"sandbox":           sandboxStatus,
	}
}

func (a *App) Homepage(ctx context.Context, sort string) (map[string]any, error) {
	items, total, err := a.store.ListHomepageCLIs(ctx, sort, 12)
	if err != nil {
		return nil, err
	}
	if sort == "" {
		sort = "favorites"
	}
	return map[string]any{
		"items": items,
		"total": total,
		"sort":  sort,
	}, nil
}

func (a *App) Search(ctx context.Context, query string) ([]models.CLI, error) {
	return a.store.SearchCLIs(ctx, query, 20)
}

func (a *App) GetCLI(ctx context.Context, slug string) (map[string]any, error) {
	cli, err := a.store.GetCLI(ctx, slug)
	if err != nil {
		return nil, err
	}

	comments, err := a.store.ListComments(ctx, slug)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"cli":      cli,
		"comments": comments,
		"examples": uniqueStrings([]string{
			exampleTail(cli.ExampleLine, cli.Slug),
			"--help",
			"--version",
		}),
	}, nil
}

func (a *App) ExecuteBuiltin(ctx context.Context, request models.BuiltinExecRequest) (models.BuiltinExecResponse, error) {
	response, err := a.builtins.Execute(ctx, request)
	if err != nil {
		return models.BuiltinExecResponse{}, err
	}

	if response.Execution != nil {
		if logErr := a.store.LogExecution(ctx, request.UserID, response.ResolvedCLI, request.Line, "builtin", *response.Execution); logErr != nil {
			return models.BuiltinExecResponse{}, logErr
		}
	}
	return response, nil
}

func (a *App) ExecuteCLI(ctx context.Context, request models.ExecRequest) (models.ExecutionResult, error) {
	line := strings.TrimSpace(request.Line)
	if line == "" {
		return models.ExecutionResult{}, errors.New("command line cannot be empty")
	}
	if strings.Contains(line, "\n") {
		return models.ExecutionResult{}, errors.New("multiline input is not allowed")
	}
	if util.ContainsForbiddenOperator(line) {
		return models.ExecutionResult{}, errors.New("shell operators, pipes, and redirects are disabled in v1")
	}

	cli, err := a.store.GetCLI(ctx, request.CLISlug)
	if err != nil {
		return models.ExecutionResult{}, err
	}
	if cli.Type == models.CLITypeBuiltin {
		return models.ExecutionResult{}, errors.New("builtin commands must use /api/v1/builtin/exec")
	}
	if !cli.Executable || cli.EnvironmentKind != models.EnvironmentKindSandbox {
		return models.ExecutionResult{}, errors.New("this CLI is indexed for reference only and cannot be executed in the sandbox")
	}

	tokens, err := util.SplitLine(line)
	if err != nil {
		return models.ExecutionResult{}, err
	}
	if len(tokens) == 0 {
		return models.ExecutionResult{}, errors.New("command line cannot be empty")
	}
	if tokens[0] != cli.Slug {
		tokens = append([]string{cli.Slug}, tokens...)
		line = strings.Join(tokens, " ")
	}

	result, err := a.runner.RunBusyBox(ctx, cli, tokens[1:])
	if err != nil {
		return models.ExecutionResult{}, err
	}
	result.SessionState = "execution"

	if logErr := a.store.LogExecution(ctx, request.UserID, cli.Slug, line, "cli", result); logErr != nil {
		return models.ExecutionResult{}, logErr
	}

	return result, nil
}

func (a *App) Login(ctx context.Context, request models.LoginRequest) (models.User, error) {
	return a.store.LoginMock(ctx, request.Username)
}

func (a *App) AnonymousSession(ctx context.Context) (models.User, error) {
	return a.store.LoginMock(ctx, "anonymous")
}

func (a *App) ListFavorites(ctx context.Context, userID int64) ([]models.CLI, error) {
	return a.store.ListFavorites(ctx, userID)
}

func (a *App) SetFavorite(ctx context.Context, request models.FavoriteMutation) error {
	return a.store.SetFavorite(ctx, request)
}

func (a *App) ListComments(ctx context.Context, cliSlug string) ([]models.Comment, error) {
	return a.store.ListComments(ctx, cliSlug)
}

func (a *App) AddComment(ctx context.Context, request models.CommentMutation) (models.Comment, error) {
	body := strings.TrimSpace(request.Body)
	if body == "" {
		return models.Comment{}, errors.New("comment body cannot be empty")
	}
	request.Body = body
	return a.store.AddComment(ctx, request)
}

func exampleTail(exampleLine string, cliSlug string) string {
	if strings.HasPrefix(exampleLine, cliSlug+" ") {
		return strings.TrimSpace(strings.TrimPrefix(exampleLine, cliSlug))
	}
	return exampleLine
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	items := make([]string, 0, len(values))

	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		items = append(items, value)
	}

	return items
}
