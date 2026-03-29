package app

import (
	"testing"

	"github.com/linlay/cligrep-server/internal/config"
	"github.com/linlay/cligrep-server/internal/models"
)

func TestExecutionTemplatesExposeExpectedOptions(t *testing.T) {
	app := &App{cfg: config.Config{BusyBoxImage: "busybox:1.36.1"}}

	templates := app.ExecutionTemplates()
	if len(templates) != 2 {
		t.Fatalf("expected 2 execution templates, got %d", len(templates))
	}
	if templates[0].ID != "download-only" {
		t.Fatalf("expected first template download-only, got %q", templates[0].ID)
	}
	if templates[1].ID != "busybox-applet" {
		t.Fatalf("expected second template busybox-applet, got %q", templates[1].ID)
	}
}

func TestBuildAdminCLIAppliesBusyBoxTemplate(t *testing.T) {
	app := &App{cfg: config.Config{BusyBoxImage: "busybox:1.36.1"}}
	user := models.User{ID: 1, Username: "linlay", DisplayName: "Linlay", Roles: []string{string(models.RoleMember)}}

	cli, err := app.buildAdminCLI(user, models.CLI{}, models.AdminCLIUpsertRequest{
		Slug:              "grep",
		DisplayName:       "Grep",
		ExecutionTemplate: "busybox-applet",
	}, true)
	if err != nil {
		t.Fatalf("buildAdminCLI returned error: %v", err)
	}
	if !cli.Executable {
		t.Fatal("expected busybox-applet template to produce executable cli")
	}
	if cli.EnvironmentKind != models.EnvironmentKindSandbox {
		t.Fatalf("expected SANDBOX environment, got %q", cli.EnvironmentKind)
	}
	if cli.RuntimeImage != "busybox:1.36.1" {
		t.Fatalf("expected busybox runtime image, got %q", cli.RuntimeImage)
	}
}

func TestEnsureCLIManageAccessAllowsOwnerAndAdmin(t *testing.T) {
	ownedUserID := int64(9)
	cli := models.CLI{Slug: "my-cli", OwnerUserID: &ownedUserID}

	if err := ensureCLIManageAccess(models.User{ID: 9, Roles: []string{string(models.RoleMember)}}, cli); err != nil {
		t.Fatalf("owner should have access: %v", err)
	}

	if err := ensureCLIManageAccess(models.User{ID: 2, Roles: []string{string(models.RolePlatformAdmin)}}, cli); err != nil {
		t.Fatalf("platform admin should have access: %v", err)
	}

	if err := ensureCLIManageAccess(models.User{ID: 4, Roles: []string{string(models.RoleMember)}}, cli); err == nil {
		t.Fatal("non-owner member should be forbidden")
	}
}
