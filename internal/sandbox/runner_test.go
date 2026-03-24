package sandbox

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/linlay/cligrep-server/internal/config"
)

func TestRunnerProbeMissingDockerCLI(t *testing.T) {
	runner := &Runner{
		cfg: config.Config{
			BusyBoxImage: "busybox:1.36.1",
			PythonImage:  "python:3.12-slim",
		},
		lookPath: func(file string) (string, error) {
			if file != "docker" {
				t.Fatalf("unexpected lookup: %s", file)
			}
			return "", errors.New("not found")
		},
		checkCommand: func(ctx context.Context, name string, args ...string) error {
			t.Fatalf("checkCommand should not be called when docker is missing")
			return nil
		},
	}

	probe := runner.Probe(context.Background())

	if probe.DockerCLI {
		t.Fatal("expected dockerCli to be false")
	}
	if probe.DockerDaemon || probe.BusyBoxImage || probe.PythonImage || probe.Ready {
		t.Fatal("expected remaining probe flags to be false")
	}
	if len(probe.Issues) != 1 || !strings.Contains(probe.Issues[0], "docker CLI") {
		t.Fatalf("expected docker CLI issue, got %#v", probe.Issues)
	}
}

func TestRunnerProbeDockerDaemonUnavailable(t *testing.T) {
	runner := probeTestRunner(func(cmd string) error {
		if cmd == "docker info" {
			return errors.New("daemon unavailable")
		}
		return nil
	})

	probe := runner.Probe(context.Background())

	if !probe.DockerCLI {
		t.Fatal("expected dockerCli to be true")
	}
	if probe.DockerDaemon || probe.BusyBoxImage || probe.PythonImage || probe.Ready {
		t.Fatal("expected daemon, image, and ready flags to be false")
	}
	if len(probe.Issues) != 1 || !strings.Contains(probe.Issues[0], "docker daemon") {
		t.Fatalf("expected docker daemon issue, got %#v", probe.Issues)
	}
}

func TestRunnerProbeBusyBoxImageMissing(t *testing.T) {
	runner := probeTestRunner(func(cmd string) error {
		if cmd == "docker image inspect busybox:1.36.1" {
			return errors.New("missing image")
		}
		return nil
	})

	probe := runner.Probe(context.Background())

	if !probe.DockerCLI || !probe.DockerDaemon {
		t.Fatal("expected docker CLI and daemon to be ready")
	}
	if probe.BusyBoxImage {
		t.Fatal("expected busyboxImage to be false")
	}
	if !probe.PythonImage {
		t.Fatal("expected pythonImage to be true")
	}
	if probe.Ready {
		t.Fatal("expected ready to be false")
	}
	if len(probe.Issues) != 1 || !strings.Contains(probe.Issues[0], "busybox:1.36.1") {
		t.Fatalf("expected busybox image issue, got %#v", probe.Issues)
	}
}

func TestRunnerProbePythonImageMissing(t *testing.T) {
	runner := probeTestRunner(func(cmd string) error {
		if cmd == "docker image inspect python:3.12-slim" {
			return errors.New("missing image")
		}
		return nil
	})

	probe := runner.Probe(context.Background())

	if !probe.DockerCLI || !probe.DockerDaemon || !probe.BusyBoxImage {
		t.Fatal("expected docker CLI, daemon, and busybox image to be ready")
	}
	if probe.PythonImage {
		t.Fatal("expected pythonImage to be false")
	}
	if probe.Ready {
		t.Fatal("expected ready to be false")
	}
	if len(probe.Issues) != 1 || !strings.Contains(probe.Issues[0], "python:3.12-slim") {
		t.Fatalf("expected python image issue, got %#v", probe.Issues)
	}
}

func TestRunnerProbeReady(t *testing.T) {
	runner := probeTestRunner(func(string) error { return nil })

	probe := runner.Probe(context.Background())

	if !probe.DockerCLI || !probe.DockerDaemon || !probe.BusyBoxImage || !probe.PythonImage {
		t.Fatalf("expected all probe flags to be true, got %+v", probe)
	}
	if !probe.Ready {
		t.Fatal("expected ready to be true")
	}
	if len(probe.Issues) != 0 {
		t.Fatalf("expected no issues, got %#v", probe.Issues)
	}
}

func TestRunnerProbeAggregatesImageIssuesInOrder(t *testing.T) {
	var calls []string

	runner := &Runner{
		cfg: config.Config{
			BusyBoxImage: "busybox:1.36.1",
			PythonImage:  "python:3.12-slim",
		},
		lookPath: func(file string) (string, error) {
			return "/usr/bin/docker", nil
		},
		checkCommand: func(ctx context.Context, name string, args ...string) error {
			cmd := strings.Join(append([]string{name}, args...), " ")
			calls = append(calls, cmd)
			if cmd == "docker image inspect busybox:1.36.1" || cmd == "docker image inspect python:3.12-slim" {
				return errors.New("missing image")
			}
			return nil
		},
	}

	probe := runner.Probe(context.Background())

	expectedCalls := []string{
		"docker info",
		"docker image inspect busybox:1.36.1",
		"docker image inspect python:3.12-slim",
	}
	if strings.Join(calls, "|") != strings.Join(expectedCalls, "|") {
		t.Fatalf("unexpected probe order: got %v want %v", calls, expectedCalls)
	}
	if probe.Ready {
		t.Fatal("expected ready to be false")
	}
	if len(probe.Issues) != 2 {
		t.Fatalf("expected two issues, got %#v", probe.Issues)
	}
	if !strings.Contains(probe.Issues[0], "busybox:1.36.1") || !strings.Contains(probe.Issues[1], "python:3.12-slim") {
		t.Fatalf("unexpected issues: %#v", probe.Issues)
	}
}

func probeTestRunner(check func(cmd string) error) *Runner {
	return &Runner{
		cfg: config.Config{
			BusyBoxImage: "busybox:1.36.1",
			PythonImage:  "python:3.12-slim",
		},
		lookPath: func(file string) (string, error) {
			if file != "docker" {
				return "", errors.New("unexpected binary")
			}
			return "/usr/bin/docker", nil
		},
		checkCommand: func(ctx context.Context, name string, args ...string) error {
			return check(strings.Join(append([]string{name}, args...), " "))
		},
	}
}
