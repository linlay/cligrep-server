package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linlay/cligrep-server/internal/models"
)

type stubApp struct {
	health map[string]any
}

func (s stubApp) Health(ctx context.Context) map[string]any {
	return s.health
}

func (stubApp) Homepage(ctx context.Context, sort string) (map[string]any, error) {
	return nil, nil
}

func (stubApp) Search(ctx context.Context, query string) ([]models.CLI, error) {
	return nil, nil
}

func (stubApp) GetCLI(ctx context.Context, slug string) (map[string]any, error) {
	return nil, nil
}

func (stubApp) ExecuteCLI(ctx context.Context, request models.ExecRequest) (models.ExecutionResult, error) {
	return models.ExecutionResult{}, nil
}

func (stubApp) ExecuteBuiltin(ctx context.Context, request models.BuiltinExecRequest) (models.BuiltinExecResponse, error) {
	return models.BuiltinExecResponse{}, nil
}

func (stubApp) Login(ctx context.Context, request models.LoginRequest) (models.User, error) {
	return models.User{}, nil
}

func (stubApp) AnonymousSession(ctx context.Context) (models.User, error) {
	return models.User{}, nil
}

func (stubApp) ListFavorites(ctx context.Context, userID int64) ([]models.CLI, error) {
	return nil, nil
}

func (stubApp) SetFavorite(ctx context.Context, request models.FavoriteMutation) error {
	return nil
}

func (stubApp) ListComments(ctx context.Context, cliSlug string) ([]models.Comment, error) {
	return nil, nil
}

func (stubApp) AddComment(ctx context.Context, request models.CommentMutation) (models.Comment, error) {
	return models.Comment{}, nil
}

func TestHandleHealthIncludesSandboxStatus(t *testing.T) {
	handler := NewHandler(stubApp{
		health: map[string]any{
			"status":            "ok",
			"busyboxImage":      "busybox:1.36.1",
			"pythonImage":       "python:3.12-slim",
			"databaseHost":      "13.212.113.109",
			"databasePort":      3306,
			"databaseName":      "cligrep",
			"timestamp":         "2026-03-24T00:00:00Z",
			"commandTimeoutMs":  int64(4000),
			"singleLineOnly":    true,
			"runtimeKinds":      []string{"SANDBOX", "WEBSITE", "TEXT"},
			"homepageSortModes": []string{"favorites", "newest", "runs"},
			"sandboxReady":      true,
			"sandbox": map[string]any{
				"dockerCli":    true,
				"dockerDaemon": true,
				"busyboxImage": true,
				"pythonImage":  true,
				"ready":        true,
				"issues":       []string{},
			},
		},
	}, "*")

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	for _, key := range []string{
		"status",
		"busyboxImage",
		"pythonImage",
		"databaseHost",
		"databasePort",
		"databaseName",
		"timestamp",
		"commandTimeoutMs",
		"singleLineOnly",
		"runtimeKinds",
		"homepageSortModes",
		"sandboxReady",
		"sandbox",
	} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("expected key %q in payload", key)
		}
	}

	if ready, ok := payload["sandboxReady"].(bool); !ok || !ready {
		t.Fatalf("expected sandboxReady=true, got %#v", payload["sandboxReady"])
	}

	sandboxPayload, ok := payload["sandbox"].(map[string]any)
	if !ok {
		t.Fatalf("expected sandbox object, got %#v", payload["sandbox"])
	}

	for _, key := range []string{"dockerCli", "dockerDaemon", "busyboxImage", "pythonImage", "ready", "issues"} {
		if _, ok := sandboxPayload[key]; !ok {
			t.Fatalf("expected sandbox key %q", key)
		}
	}

	if ready, ok := sandboxPayload["ready"].(bool); !ok || !ready {
		t.Fatalf("expected sandbox.ready=true, got %#v", sandboxPayload["ready"])
	}
}
