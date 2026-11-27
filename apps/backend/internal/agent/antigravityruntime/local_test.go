package antigravityruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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
			opts.ACPHelpProbe = func(context.Context, string) (string, error) {
				return antigravityWorkerHelpFixture, nil
			}
			selected, err := ResolveLocal(context.Background(), opts)
			if err != nil {
				t.Fatalf("ResolveLocal: %v", err)
			}
			if selected.Variant != tt.wantVariant {
				t.Fatalf("variant = %q, want %q", selected.Variant, tt.wantVariant)
			}
			if tt.wantCLI {
				if len(selected.Args) != 3 || selected.Args[0] != cliPath || selected.Args[1] != "acp" || selected.Args[2] != "--process-per-session" {
					t.Fatalf("CLI argv = %v, want [%s acp --process-per-session]", selected.Args, cliPath)
				}
				return
			}
			if selected.Args[0] != serverPath || selected.Bundle == nil {
				t.Fatalf("standalone selection = %+v, want server %s", selected, serverPath)
			}
		})
	}
}

func TestResolveLocalReportsDangerousPermissionFlagSupport(t *testing.T) {
	for _, tt := range []struct {
		name string
		help string
		want bool
	}{
		{
			name: "exact declaration",
			help: antigravityWorkerHelpFixture + "  --dangerously-skip-permissions  Approve all tool requests\n",
			want: true,
		},
		{
			name: "similar option does not qualify",
			help: antigravityWorkerHelpFixture + "  --dangerously-skip-permissions-extra  Similar option\n",
		},
		{
			name: "usage text does not qualify",
			help: antigravityWorkerHelpFixture + "\nExamples:\n  agy acp --dangerously-skip-permissions\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, _ := localResolverFixture(t, true)
			opts.VersionProbe = func(context.Context, string) (string, error) { return "1.5.15", nil }
			opts.ACPHelpProbe = func(context.Context, string) (string, error) { return tt.help, nil }
			selected, err := ResolveLocal(context.Background(), opts)
			if err != nil {
				t.Fatalf("ResolveLocal: %v", err)
			}
			if selected.SupportsDangerouslySkipPermissions != tt.want {
				t.Fatalf("SupportsDangerouslySkipPermissions = %t, want %t", selected.SupportsDangerouslySkipPermissions, tt.want)
			}
		})
	}
}

func TestFilterDangerouslySkipPermissions(t *testing.T) {
	for _, tt := range []struct {
		name      string
		args      []string
		permitted bool
		want      []string
	}{
		{
			name:      "permitted option is retained",
			args:      []string{"agy", "acp", "--dangerously-skip-permissions", "--", "prompt"},
			permitted: true,
			want:      []string{"agy", "acp", "--dangerously-skip-permissions", "--", "prompt"},
		},
		{
			name: "unsupported option is removed before delimiter",
			args: []string{"agy", "acp", "--dangerously-skip-permissions", "--", "prompt"},
			want: []string{"agy", "acp", "--", "prompt"},
		},
		{
			name: "similarly named option remains",
			args: []string{"agy", "acp", "--dangerously-skip-permissions-extra"},
			want: []string{"agy", "acp", "--dangerously-skip-permissions-extra"},
		},
		{
			name: "skip-looking token consumed as an option value remains",
			args: []string{"agy", "acp", "--log-file", "--dangerously-skip-permissions", "--dangerously-skip-permissions"},
			want: []string{"agy", "acp", "--log-file", "--dangerously-skip-permissions"},
		},
		{
			name: "delimiter consumed as an option value does not stop filtering",
			args: []string{"agy", "acp", "--log-file", "--", "--dangerously-skip-permissions"},
			want: []string{"agy", "acp", "--log-file", "--"},
		},
		{
			name: "text after delimiter remains",
			args: []string{"agy", "acp", "--", "--dangerously-skip-permissions"},
			want: []string{"agy", "acp", "--", "--dangerously-skip-permissions"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := FilterDangerouslySkipPermissions(tt.args, tt.permitted); !slices.Equal(got, tt.want) {
				t.Fatalf("FilterDangerouslySkipPermissions(%q, %t) = %q, want %q", tt.args, tt.permitted, got, tt.want)
			}
		})
	}
}

func TestRequireWorkerModePreservesOptionValuesAndDelimiter(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "literal delimiter is an option value",
			args: []string{"agy", "acp", "--log-file", "--"},
			want: []string{"agy", "acp", "--log-file", "--", workerModeFlag},
		},
		{
			name: "worker-looking token is a value",
			args: []string{"agy", "acp", "--log-file", workerModeFlag},
			want: []string{"agy", "acp", "--log-file", workerModeFlag, workerModeFlag},
		},
		{
			name: "worker flag follows profile options before delimiter",
			args: []string{"agy", "acp", "--profile", "value", "--", "tail"},
			want: []string{"agy", "acp", "--profile", "value", workerModeFlag, "--", "tail"},
		},
		{
			name: "false worker value is overridden after profile options",
			args: []string{"agy", "acp", workerModeFlag, workerModeFlag + "=false", "--", "tail"},
			want: []string{"agy", "acp", workerModeFlag, workerModeFlag + "=false", workerModeFlag, "--", "tail"},
		},
		{
			name: "base worker option is reasserted after profile options",
			args: []string{"agy", "acp", workerModeFlag, "--profile-flag"},
			want: []string{"agy", "acp", workerModeFlag, "--profile-flag", workerModeFlag},
		},
		{
			name: "explicit true worker option is preserved",
			args: []string{"agy", "acp", workerModeFlag + "=true"},
			want: []string{"agy", "acp", workerModeFlag + "=true"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := RequireWorkerMode(tt.args); !slices.Equal(got, tt.want) {
				t.Fatalf("RequireWorkerMode(%q) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestHasWorkerModeFlagRequiresExactOption(t *testing.T) {
	for _, tt := range []struct {
		name   string
		output string
		want   bool
	}{
		{name: "declared option", output: "Flags:\n  --process-per-session  Serve each session in a worker", want: true},
		{name: "declared option with value", output: "Options:\n  --process-per-session=<boolean>  Enable worker mode", want: true},
		{name: "usage only", output: "Usage: agy acp [--process-per-session]", want: false},
		{name: "prose warning in flags section", output: "Flags:\n  Warning: use --process-per-session on this host", want: false},
		{name: "examples after options do not qualify", output: "Flags:\n  --help  Show help\n\nExamples:\n  --process-per-session is unavailable in this build", want: false},
		{name: "other option suffix", output: "Flags:\n  --process-per-session-extra  Similar option", want: false},
		{name: "disabling option", output: "Flags:\n  --no-process-per-session  Disable worker mode", want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasWorkerModeFlag(tt.output); got != tt.want {
				t.Fatalf("hasWorkerModeFlag(%q) = %t, want %t", tt.output, got, tt.want)
			}
		})
	}
}

func TestResolveLocalFallsBackWhenWorkerHelpIsUnqualified(t *testing.T) {
	for _, tt := range []struct {
		name   string
		output string
		err    error
	}{
		{
			name:   "failed help with declaration",
			output: antigravityWorkerHelpFixture,
			err:    errors.New("exit status 1"),
		},
		{
			name:   "successful prose-only help",
			output: "Flags:\n  Warning: --process-per-session may be used by another command",
		},
		{
			name:   "successful examples after options",
			output: "Flags:\n  --help  Show help\n\nExamples:\n  --process-per-session is unavailable in this build",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			opts, _, serverPath := localResolverFixture(t, true)
			opts.VersionProbe = func(context.Context, string) (string, error) { return "1.5.15", nil }
			opts.ACPHelpProbe = func(context.Context, string) (string, error) { return tt.output, tt.err }

			selected, err := ResolveLocal(context.Background(), opts)
			if err != nil {
				t.Fatalf("ResolveLocal: %v", err)
			}
			if selected.Variant != acpcompat.AntigravityRuntimeStandalone || selected.Args[0] != serverPath {
				t.Fatalf("selected runtime = %+v, want standalone fallback", selected)
			}

			_, err = ResolveSaved(context.Background(), opts, "antigravity-cli/1.5.15")
			var savedErr *SavedRuntimeError
			if !errors.As(err, &savedErr) || savedErr.Variant != acpcompat.AntigravityRuntimeCLI || savedErr.Cause == nil {
				t.Fatalf("ResolveSaved error = %T %v, want typed CLI runtime error", err, err)
			}
			if tt.err == nil && !errors.Is(err, errCLIWorkerModeUnsupported) {
				t.Fatalf("ResolveSaved error = %v, want unsupported worker option", err)
			}
			if tt.err != nil && !errors.Is(err, tt.err) {
				t.Fatalf("ResolveSaved error = %v, want failed help cause %v", err, tt.err)
			}
		})
	}
}

func TestResolveLocalHonorsCancellationDuringHelpCheck(t *testing.T) {
	opts, _, _ := localResolverFixture(t, true)
	opts.VersionProbe = func(context.Context, string) (string, error) { return "1.5.15", nil }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts.ACPHelpProbe = func(helpCtx context.Context, _ string) (string, error) {
		cancel()
		return "", helpCtx.Err()
	}
	if _, err := ResolveLocal(ctx, opts); !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveLocal error = %v, want caller cancellation", err)
	}
}

func TestResolveSavedRuntimeKeepsRecordedVariant(t *testing.T) {
	opts, cliPath, serverPath := localResolverFixture(t, true)
	opts.VersionProbe = func(context.Context, string) (string, error) { return "1.5.15", nil }
	opts.ACPHelpProbe = func(context.Context, string) (string, error) {
		return antigravityWorkerHelpFixture, nil
	}

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

func TestResolveSavedRuntimeReturnsTypedErrorWhenWorkerFlagIsMissing(t *testing.T) {
	opts, _, _ := localResolverFixture(t, true)
	opts.VersionProbe = func(context.Context, string) (string, error) { return "1.5.15", nil }
	opts.ACPHelpProbe = func(context.Context, string) (string, error) {
		return "Flags:\n  --log-file  Write logs to a file", nil
	}
	_, err := ResolveSaved(context.Background(), opts, "antigravity-cli/1.5.15")
	var savedErr *SavedRuntimeError
	if !errors.As(err, &savedErr) || savedErr.Variant != acpcompat.AntigravityRuntimeCLI || !errors.Is(savedErr.Cause, errCLIWorkerModeUnsupported) {
		t.Fatalf("ResolveSaved error = %T %v, want typed CLI runtime error for missing worker flag", err, err)
	}
}

const antigravityWorkerHelpFixture = `Usage: agy acp [flags]

Flags:
  --process-per-session  Serve each session from an isolated worker process
`

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
