package builtin

import (
	"context"
	"strings"
	"testing"

	"github.com/linlay/cligrep-server/internal/models"
)

func TestHelpTextOmitsLegacyLoginCommands(t *testing.T) {
	text := helpText(context.Background())
	if strings.Contains(text, "login") || strings.Contains(text, "logout") {
		t.Fatalf("expected help text to omit login/logout, got %q", text)
	}
}

func TestUnknownCommandHintListsCurrentBuiltins(t *testing.T) {
	service := &Service{}

	response, err := service.Execute(context.Background(), models.BuiltinExecRequest{Line: "unknown"})
	if err != nil {
		t.Fatalf("execute unknown command: %v", err)
	}
	if len(response.Hints) == 0 {
		t.Fatal("expected hints for unknown command")
	}
	if strings.Contains(response.Hints[0], "login") || strings.Contains(response.Hints[0], "logout") {
		t.Fatalf("expected hints to omit login/logout, got %q", response.Hints[0])
	}
}
