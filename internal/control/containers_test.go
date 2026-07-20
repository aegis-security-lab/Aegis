package control

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestDockerPiCommandBuildsIsolatedRuntime(t *testing.T) {
	dataDir := t.TempDir()
	profile := ContainerProfile{
		Image: "example/pi:latest", NodePath: "node", PiPath: "/opt/pi/cli.js",
		WorkspacePath: "/workspace", NetworkMode: "bridge", MemoryMB: 1024, CPUs: 1.5,
	}
	command, args := dockerPiCommand(profile, "aegis-execution-1", "/host/project", dataDir, "/host/guard.ts", Config{Provider: "openai"}, []string{"--mode", "rpc"}, []string{
		"PATH=/host/bin", "HOME=/secret/home", "OPENAI_API_KEY=test-key",
		"AEGIS_CONTROL_URL=http://127.0.0.1:8080", "AEGIS_WORKSPACE=/workspace",
	}, "http://127.0.0.1:8080")
	if command != "docker" {
		t.Fatalf("command = %q", command)
	}
	joined := strings.Join(args, "\n")
	for _, expected := range []string{
		"aegis-execution-1", "/host/project:/workspace",
		filepath.Join(dataDir, "sessions") + ":/aegis/sessions",
		"AEGIS_CONTROL_URL=http://host.docker.internal:8080",
		"OPENAI_API_KEY=test-key", "example/pi:latest", "/opt/pi/cli.js",
	} {
		if !strings.Contains(joined, expected) {
			t.Errorf("docker arguments do not contain %q", expected)
		}
	}
	for _, forbidden := range []string{"PATH=/host/bin", "HOME=/secret/home"} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("host environment leaked into container: %q", forbidden)
		}
	}
	if !slices.Contains(args, "--rm") {
		t.Error("container must be ephemeral")
	}
}

func TestContainerHostURLOnlyRewritesLoopbackHost(t *testing.T) {
	if got := containerHostURL("http://localhost:3000/api"); got != "http://host.docker.internal:3000/api" {
		t.Fatalf("localhost rewrite = %q", got)
	}
	if got := containerHostURL("https://api.example.test/v1"); got != "https://api.example.test/v1" {
		t.Fatalf("external URL changed = %q", got)
	}
}
