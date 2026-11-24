package controller

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/antigravityruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

func TestAntigravityCommandPreviewUsesSelectedLocalRuntime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses POSIX CLI scripts")
	}
	tests := []struct {
		name           string
		cliVersion     string
		wantVariant    string
		prepareRuntime func(*testing.T, string) string
	}{
		{
			name:        "compatible CLI in a path with spaces",
			cliVersion:  "1.5.15",
			wantVariant: "cli",
			prepareRuntime: func(t *testing.T, home string) string {
				bin := filepath.Join(t.TempDir(), "Antigravity CLI")
				if err := os.MkdirAll(bin, 0o755); err != nil {
					t.Fatal(err)
				}
				cli := filepath.Join(bin, "agy")
				writeAntigravityCLI(t, cli, "1.5.15")
				t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
				return cli
			},
		},
		{
			name:        "standalone bundle in a path with spaces",
			cliVersion:  "1.4.9",
			wantVariant: "standalone",
			prepareRuntime: func(t *testing.T, home string) string {
				bin := t.TempDir()
				writeAntigravityCLI(t, filepath.Join(bin, "agy"), "1.4.9")
				t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
				artifact, err := antigravityruntime.ArtifactFor(runtime.GOOS, runtime.GOARCH)
				if err != nil {
					t.Fatal(err)
				}
				bundleDir := antigravityruntime.ManagedDestination(home, artifact)
				if err := os.MkdirAll(bundleDir, 0o755); err != nil {
					t.Fatal(err)
				}
				for _, name := range []string{artifact.ServerName, artifact.HarnessNames[0]} {
					if err := os.WriteFile(filepath.Join(bundleDir, name), []byte("fixture"), 0o755); err != nil {
						t.Fatal(err)
					}
				}
				manifest, err := json.Marshal(antigravityruntime.Manifest{
					Version: artifact.Version, OS: artifact.OS, Arch: artifact.Arch,
					Digest: artifact.SHA256, Server: artifact.ServerName, Harness: artifact.HarnessNames[0],
				})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(bundleDir, antigravityruntime.ManifestName), manifest, 0o644); err != nil {
					t.Fatal(err)
				}
				return filepath.Join(bundleDir, artifact.ServerName)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "kandev home with spaces")
			if err := os.MkdirAll(home, 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("HOME", t.TempDir())
			t.Setenv("KANDEV_HOME_DIR", home)
			selected := tt.prepareRuntime(t, home)
			controller := newTestController(map[string]agents.Agent{
				agents.AntigravityACPAgentID: agents.NewAntigravityACP(),
			})
			response, err := controller.PreviewAgentCommand(context.Background(), agents.AntigravityACPAgentID, CommandPreviewRequest{
				CLIFlags: []dto.CLIFlagDTO{
					{Flag: "--profile 'value with spaces'", Enabled: true},
					{Flag: "--process-per-session=false", Enabled: true},
				},
			})
			if err != nil {
				t.Fatalf("PreviewAgentCommand: %v", err)
			}
			if response.Command[0] != selected {
				t.Fatalf("command = %#v, want selected %s runtime", response.Command, tt.wantVariant)
			}
			if tt.wantVariant == "cli" && !slices.Equal(response.Command[:2], []string{selected, "acp"}) {
				t.Fatalf("CLI command prefix = %#v, want [%s acp]", response.Command[:2], selected)
			}
			if tt.wantVariant == "cli" {
				if !slices.Contains(response.Command, "--process-per-session=false") || response.Command[len(response.Command)-1] != "--process-per-session" {
					t.Fatalf("CLI worker flag is not effective after profile flags: %#v", response.Command)
				}
			}
			if !slices.Contains(response.Command, "value with spaces") {
				t.Fatalf("CLI flag value was not preserved as one argv token: %#v", response.Command)
			}
		})
	}
}

func writeAntigravityCLI(t *testing.T, path, version string) {
	t.Helper()
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 'agy " + version + "'; exit 0; fi\n" +
		"if [ \"$1\" = \"acp\" ] && [ \"$2\" = \"--help\" ]; then printf 'Usage: agy acp [flags]\\n\\nFlags:\\n  --process-per-session  Serve each session from an isolated worker process\\n'; exit 0; fi\nexit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}
