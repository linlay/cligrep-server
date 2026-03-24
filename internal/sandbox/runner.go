package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/linlay/cligrep-server/internal/config"
	"github.com/linlay/cligrep-server/internal/models"
)

type Runner struct {
	cfg          config.Config
	lookPath     func(file string) (string, error)
	checkCommand func(ctx context.Context, name string, args ...string) error
}

type ProbeResult struct {
	DockerCLI    bool     `json:"dockerCli"`
	DockerDaemon bool     `json:"dockerDaemon"`
	BusyBoxImage bool     `json:"busyboxImage"`
	PythonImage  bool     `json:"pythonImage"`
	Ready        bool     `json:"ready"`
	Issues       []string `json:"issues"`
}

func NewRunner(cfg config.Config) *Runner {
	return &Runner{
		cfg:      cfg,
		lookPath: exec.LookPath,
		checkCommand: func(ctx context.Context, name string, args ...string) error {
			return exec.CommandContext(ctx, name, args...).Run()
		},
	}
}

func (r *Runner) Probe(ctx context.Context) ProbeResult {
	probe := ProbeResult{
		Issues: make([]string, 0, 2),
	}

	if _, err := r.lookPath("docker"); err != nil {
		probe.Issues = append(probe.Issues, "docker CLI is not installed or not in PATH")
		return probe
	}
	probe.DockerCLI = true

	if err := r.checkCommand(ctx, "docker", "info"); err != nil {
		probe.Issues = append(probe.Issues, "docker daemon is not reachable")
		return probe
	}
	probe.DockerDaemon = true

	if r.imageAvailable(ctx, r.cfg.BusyBoxImage) {
		probe.BusyBoxImage = true
	} else {
		probe.Issues = append(probe.Issues, fmt.Sprintf("docker image %s is not available locally", r.cfg.BusyBoxImage))
	}

	if r.imageAvailable(ctx, r.cfg.PythonImage) {
		probe.PythonImage = true
	} else {
		probe.Issues = append(probe.Issues, fmt.Sprintf("docker image %s is not available locally", r.cfg.PythonImage))
	}

	probe.Ready = probe.DockerCLI && probe.DockerDaemon && probe.BusyBoxImage && probe.PythonImage
	return probe
}

func (r *Runner) ExtractBusyBoxHelp(ctx context.Context, cli string) (string, string, error) {
	if !r.imageAvailable(ctx, r.cfg.BusyBoxImage) {
		return "", "", fmt.Errorf("busybox image %s not available locally", r.cfg.BusyBoxImage)
	}

	result, err := r.runDocker(ctx, r.cfg.BusyBoxImage, nil, []string{"busybox", cli, "--help"}, "")
	if err != nil {
		return "", "", err
	}

	helpText := strings.TrimSpace(strings.Join([]string{result.Stdout, result.Stderr}, "\n"))
	version := "busybox-1.36"
	if versionResult, versionErr := r.runDocker(ctx, r.cfg.BusyBoxImage, nil, []string{"busybox"}, ""); versionErr == nil {
		output := strings.TrimSpace(versionResult.Stdout)
		if output != "" {
			firstLine := strings.Split(output, "\n")[0]
			version = firstLine
		}
	}
	return helpText, version, nil
}

func (r *Runner) RunBusyBox(ctx context.Context, cli models.CLI, args []string) (models.ExecutionResult, error) {
	return r.runDocker(ctx, cli.RuntimeImage, nil, append([]string{"busybox", cli.Slug}, args...), cli.Slug)
}

func (r *Runner) RunPythonScript(ctx context.Context, scriptName string, scriptContent string, args []string) (models.ExecutionResult, error) {
	tempDir, err := os.MkdirTemp("", "cligrep-python-*")
	if err != nil {
		return models.ExecutionResult{}, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	scriptPath := filepath.Join(tempDir, scriptName)
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0o644); err != nil {
		return models.ExecutionResult{}, fmt.Errorf("write python script: %w", err)
	}

	mount := fmt.Sprintf("%s:/workspace:ro", tempDir)
	command := append([]string{"python", filepath.Join("/workspace", scriptName)}, args...)
	return r.runDocker(ctx, r.cfg.PythonImage, []string{
		"-e", "PYTHONDONTWRITEBYTECODE=1",
		"-v", mount,
		"--workdir", "/workspace",
	}, command, "python-generated")
}

func (r *Runner) runDocker(ctx context.Context, image string, extraArgs []string, command []string, resolvedCLI string) (models.ExecutionResult, error) {
	startedAt := time.Now()

	runCtx, cancel := context.WithTimeout(ctx, r.cfg.CommandTimeout)
	defer cancel()

	if !r.imageAvailable(runCtx, image) {
		return models.ExecutionResult{
			Stderr:      fmt.Sprintf("docker image %s is not available locally. Pull it before running sandbox commands.", image),
			ExitCode:    1,
			DurationMS:  time.Since(startedAt).Milliseconds(),
			Mode:        "execution",
			ResolvedCLI: resolvedCLI,
		}, nil
	}

	args := []string{
		"run",
		"--rm",
		"--network", "none",
		"--cpus", r.cfg.ContainerCPUs,
		"--memory", r.cfg.ContainerMemory,
		"--pids-limit", "64",
		"--read-only",
		"--tmpfs", "/tmp:rw,size=64m",
	}
	args = append(args, extraArgs...)
	args = append(args, image)
	args = append(args, command...)

	cmd := exec.CommandContext(runCtx, "docker", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(startedAt).Milliseconds()

	result := models.ExecutionResult{
		Stdout:      strings.TrimSpace(stdout.String()),
		Stderr:      strings.TrimSpace(stderr.String()),
		DurationMS:  duration,
		Mode:        "execution",
		ResolvedCLI: resolvedCLI,
	}

	if err == nil {
		result.ExitCode = 0
		return result, nil
	}

	result.ExitCode = 1
	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
	}
	if runCtx.Err() == context.DeadlineExceeded {
		result.Stderr = strings.TrimSpace(strings.Join([]string{result.Stderr, "execution timed out"}, "\n"))
	}

	return result, nil
}

func (r *Runner) imageAvailable(ctx context.Context, image string) bool {
	checkCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()

	return r.checkCommand(checkCtx, "docker", "image", "inspect", image) == nil
}
