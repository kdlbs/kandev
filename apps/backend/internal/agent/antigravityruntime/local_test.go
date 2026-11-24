package antigravityruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/acpcompat"
)

func TestResolveLocalRuntimeSelection(t *testing.T) {
	tests := []struct {
		name        string
		cliVersion  string
		cliError    error
		wantVariant string
		wantCLI     bool
	}{
		{name: "compatible CLI wins", cliVersion: "1.5.15", wantVariant: acpcompat.AntigravityRuntimeCLI, wantCLI: true},
		{name: "old CLI falls back", cliVersion: "1.4.9", wantVariant: acpcompat.AntigravityRuntimeStandalone},
		{name: "failed version probe falls back", cliError: errors.New("version command failed"), wantVariant: acpcompat.AntigravityRuntimeStandalone},
		{name: "missing CLI uses standalone", wantVariant: acpcompat.AntigravityRuntimeStandalone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, cliPath, serverPath := localResolverFixture(t, tt.cliVersion != "" || tt.cliError != nil)
			opts.VersionProbe = func(context.Context, string) (string, error) {
				return tt.cliVersion, tt.cliError
			}
			selected, err := ResolveLocal(context.Background(), opts)
			if err != nil {
				t.Fatalf("ResolveLocal: %v", err)
			}
			if selected.Variant != tt.wantVariant {
				t.Fatalf("variant = %q, want %q", selected.Variant, tt.wantVariant)
			}
			if tt.wantCLI {
				if len(selected.Args) != 2 || selected.Args[0] != cliPath || selected.Args[1] != "acp" {
					t.Fatalf("CLI argv = %v, want [%s acp]", selected.Args, cliPath)
				}
				return
			}
			if selected.Args[0] != serverPath || selected.Bundle == nil {
				t.Fatalf("standalone selection = %+v, want server %s", selected, serverPath)
			}
		})
	}
}

func TestResolveSavedRuntimeKeepsRecordedVariant(t *testing.T) {
	opts, cliPath, serverPath := localResolverFixture(t, true)
	opts.VersionProbe = func(context.Context, string) (string, error) { return "1.5.15", nil }

	cli, err := ResolveSaved(context.Background(), opts, "antigravity-cli/1.5.15")
	if err != nil {
		t.Fatalf("ResolveSaved CLI: %v", err)
	}
	if cli.Variant != acpcompat.AntigravityRuntimeCLI || cli.Args[0] != cliPath {
		t.Fatalf("saved CLI selection = %+v", cli)
	}

	standalone, err := ResolveSaved(context.Background(), opts, "antigravity-standalone/1.4.5")
	if err != nil {
		t.Fatalf("ResolveSaved standalone: %v", err)
	}
	if standalone.Variant != acpcompat.AntigravityRuntimeStandalone || standalone.Args[0] != serverPath {
		t.Fatalf("saved standalone selection = %+v", standalone)
	}

	legacy, err := ResolveSaved(context.Background(), opts, "")
	if err != nil {
		t.Fatalf("ResolveSaved legacy: %v", err)
	}
	if legacy.Variant != acpcompat.AntigravityRuntimeStandalone || legacy.Args[0] != serverPath {
		t.Fatalf("legacy selection = %+v", legacy)
	}
}

func TestResolveSavedRuntimeReturnsTypedErrorWhenCLIIsMissing(t *testing.T) {
	opts, _, _ := localResolverFixture(t, false)
	if _, err := ResolveSaved(context.Background(), opts, "antigravity-cli/1.5.15"); err == nil {
		t.Fatal("ResolveSaved error = nil, want missing CLI")
	} else {
		var savedErr *SavedRuntimeError
		if !errors.As(err, &savedErr) || savedErr.Variant != acpcompat.AntigravityRuntimeCLI {
			t.Fatalf("ResolveSaved error = %T %v, want typed CLI runtime error", err, err)
		}
	}
}

func TestResolveLocalHonorsCancellation(t *testing.T) {
	opts, _, _ := localResolverFixture(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ResolveLocal(ctx, opts); !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveLocal error = %v, want canceled", err)
	}
}

func localResolverFixture(t *testing.T, withCLI bool) (ResolveOptions, string, string) {
	t.Helper()
	binDir := t.TempDir()
	goos := runtime.GOOS
	cliName := "agy"
	serverName := "agy_acp_server.par"
	harnessName := "localharness_external"
	if goos == windowsOS {
		cliName += ".exe"
		serverName = "agy_acp_server.exe"
		harnessName = "localharness_external.exe"
	}
	cliPath := filepath.Join(binDir, cliName)
	if withCLI {
		if err := os.WriteFile(cliPath, []byte("fixture"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	serverPath := filepath.Join(binDir, serverName)
	for _, path := range []string{serverPath, filepath.Join(binDir, harnessName)} {
		if err := os.WriteFile(path, []byte("fixture"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return ResolveOptions{
		GOOS:        goos,
		GOARCH:      runtime.GOARCH,
		HomeDir:     filepath.Join(t.TempDir(), "kandev"),
		UserHomeDir: filepath.Join(t.TempDir(), "user"),
		LookPath: func(name string) (string, error) {
			path := filepath.Join(binDir, name)
			if _, err := os.Stat(path); err != nil {
				return "", err
			}
			return path, nil
		},
	}, cliPath, serverPath
}
