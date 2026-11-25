package lifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/antigravityruntime"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentruntime"
)

func TestAntigravityLocalLaunchPolicy(t *testing.T) {
	m := &Manager{}
	base := []string{"agy_acp_server.par", "--uid="}
	policy, _, resolved, _, err := m.applyLocalRuntimeCommand(context.Background(), nil, agents.NewAntigravityACP(),
		agentruntime.RuntimeDocker, base, nil, "", false)
	if err != nil {
		t.Fatalf("non-local resolution error: %v", err)
	}
	if policy != nil {
		t.Fatal("docker launch received a personal auth policy")
	}
	if resolved[0] != "agy_acp_server.par" {
		t.Fatalf("non-local command changed to %q", resolved[0])
	}
}

func TestAntigravityCommandLocality(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KANDEV_HOME_DIR", t.TempDir())
	dir := t.TempDir()
	serverName, harnessName := "agy_acp_server.par", "localharness_external"
	if runtime.GOOS == "windows" {
		serverName, harnessName = "agy_acp_server.exe", "localharness_external.exe"
	}
	server := filepath.Join(dir, serverName)
	if err := os.WriteFile(server, []byte("server"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, harnessName), []byte("harness"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	args := []string{serverName, "--uid="}
	m := &Manager{}
	policy, _, args, _, err := m.applyLocalRuntimeCommand(context.Background(), nil, agents.NewAntigravityACP(),
		agentruntime.RuntimeStandalone, args, nil, "", false)
	if err != nil {
		t.Fatalf("local resolution error: %v", err)
	}
	if policy == nil || policy.MethodID != "oauth-personal" || policy.AllowInteractive {
		t.Fatalf("local policy = %+v", policy)
	}
	if args[0] != server {
		t.Fatalf("local command = %q, want %q", args[0], server)
	}
}

func TestAntigravityLocalLaunchUsesCLIAndPreservesProfileArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake CLI uses a POSIX script")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KANDEV_HOME_DIR", t.TempDir())
	dir := t.TempDir()
	cliPath := filepath.Join(dir, "agy")
	if err := os.WriteFile(cliPath, []byte(lifecycleAntigravityCLIScript("1.5.15")), 0o755); err != nil {
		t.Fatal(err)
	}
	writeLifecycleAntigravityStandalone(t, dir)
	t.Setenv("PATH", dir)
	base := agents.NewAntigravityACP().Runtime().Cmd.Args()
	args := append(append([]string(nil), base...), "--profile-flag", "value with spaces", "--process-per-session=false", "--", "after-delimiter")
	policy, _, resolved, _, err := (&Manager{}).applyLocalRuntimeCommand(context.Background(), nil,
		agents.NewAntigravityACP(), agentruntime.RuntimeStandalone, args, nil, "", false)
	if err != nil {
		t.Fatalf("applyLocalRuntimeCommand: %v", err)
	}
	want := []string{cliPath, "acp", "--process-per-session", "--profile-flag", "value with spaces", "--process-per-session=false", "--process-per-session", "--", "after-delimiter"}
	if len(resolved) != len(want) {
		t.Fatalf("command = %v, want %v", resolved, want)
	}
	for index := range want {
		if resolved[index] != want[index] {
			t.Fatalf("command = %v, want %v", resolved, want)
		}
	}
	if policy == nil || policy.RuntimeVariant != acpcompat.AntigravityRuntimeCLI || policy.MethodID != "" {
		t.Fatalf("CLI policy = %+v", policy)
	}
}

func TestAntigravityTaskCommandSelectsCLIWithoutStandaloneBundle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake CLI uses a POSIX script")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KANDEV_HOME_DIR", t.TempDir())
	binDir := t.TempDir()
	cliPath := filepath.Join(binDir, "agy")
	if err := os.WriteFile(cliPath, []byte(lifecycleAntigravityCLIScript("1.5.15")), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	manager := &Manager{commandBuilder: NewCommandBuilder(), logger: newTestLogger()}
	request := &LaunchRequest{ExecutorType: "local"}
	commands, err := manager.buildAgentCommandWithContext(
		context.Background(), request, nil, agents.NewAntigravityACP(), agentruntime.RuntimeStandalone, false,
	)
	if err != nil {
		t.Fatalf("build CLI task command without standalone bundle: %v", err)
	}
	if len(commands.args) < 2 || commands.args[0] != cliPath || commands.args[1] != "acp" || request.AuthPolicy == nil ||
		request.AuthPolicy.RuntimeVariant != acpcompat.AntigravityRuntimeCLI {
		t.Fatalf("CLI task command/policy = %#v / %+v", commands.args, request.AuthPolicy)
	}

	// The request keeps the selected CLI through a later command build even if
	// the candidate's qualification changes after instance creation.
	if err := os.WriteFile(cliPath, []byte(lifecycleAntigravityCLIScript("1.4.9")), 0o755); err != nil {
		t.Fatal(err)
	}
	promoted, err := manager.buildAgentCommandWithContext(
		context.Background(), request, nil, agents.NewAntigravityACP(), agentruntime.RuntimeStandalone, false,
	)
	if err != nil {
		t.Fatalf("rebuild command with carried runtime selection: %v", err)
	}
	if len(promoted.args) < 2 || promoted.args[0] != cliPath || promoted.args[1] != "acp" || request.AuthPolicy.RuntimeVariant != acpcompat.AntigravityRuntimeCLI {
		t.Fatalf("carried CLI selection changed during promotion: %#v / %+v", promoted.args, request.AuthPolicy)
	}
}

func TestAntigravityResumeUsesSavedRuntime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake CLI uses a POSIX script")
	}
	tests := []struct {
		name           string
		adapterVersion string
		wantVariant    string
		wantCommand    string
	}{
		{name: "saved CLI", adapterVersion: "antigravity-cli/1.5.15", wantVariant: acpcompat.AntigravityRuntimeCLI, wantCommand: "agy"},
		{name: "saved standalone", adapterVersion: "antigravity-standalone/1.4.5", wantVariant: acpcompat.AntigravityRuntimeStandalone, wantCommand: "agy_acp_server.par"},
		{name: "legacy record", wantVariant: acpcompat.AntigravityRuntimeStandalone, wantCommand: "agy_acp_server.par"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("KANDEV_HOME_DIR", t.TempDir())
			dir := t.TempDir()
			cliPath := filepath.Join(dir, "agy")
			if err := os.WriteFile(cliPath, []byte(lifecycleAntigravityCLIScript("1.5.15")), 0o755); err != nil {
				t.Fatal(err)
			}
			writeLifecycleAntigravityStandalone(t, dir)
			t.Setenv("PATH", dir)
			base := agents.NewAntigravityACP().Runtime().Cmd.Args()
			args := append(append([]string(nil), base...), "--profile-flag")
			policy, _, resolved, _, err := (&Manager{}).applyLocalRuntimeCommand(context.Background(), nil,
				agents.NewAntigravityACP(), agentruntime.RuntimeStandalone, args, nil, tt.adapterVersion, true)
			if err != nil {
				t.Fatalf("applyLocalRuntimeCommand: %v", err)
			}
			if policy == nil || policy.RuntimeVariant != tt.wantVariant || filepath.Base(resolved[0]) != tt.wantCommand {
				t.Fatalf("selection = policy %+v, command %v", policy, resolved)
			}
			if !slices.Contains(resolved, "--profile-flag") {
				t.Fatalf("profile arguments lost: %v", resolved)
			}
			if tt.wantVariant == acpcompat.AntigravityRuntimeCLI && resolved[len(resolved)-1] != "--process-per-session" {
				t.Fatalf("saved CLI command does not end with worker mode: %v", resolved)
			}
		})
	}

	t.Run("missing saved CLI returns typed error", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("KANDEV_HOME_DIR", t.TempDir())
		dir := t.TempDir()
		writeLifecycleAntigravityStandalone(t, dir)
		t.Setenv("PATH", dir)
		base := agents.NewAntigravityACP().Runtime().Cmd.Args()
		_, _, _, _, err := (&Manager{}).applyLocalRuntimeCommand(context.Background(), nil,
			agents.NewAntigravityACP(), agentruntime.RuntimeStandalone, base, nil, "antigravity-cli/1.5.15", true)
		var savedErr *antigravityruntime.SavedRuntimeError
		if !errors.As(err, &savedErr) || savedErr.Variant != acpcompat.AntigravityRuntimeCLI {
			t.Fatalf("resume error = %T %v, want saved CLI runtime error", err, err)
		}
	})
}

func TestAntigravityTaskResumeCommandUsesSavedRuntimeAfterRestart(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake CLI uses a POSIX script")
	}
	tests := []struct {
		name           string
		adapterVersion string
		cliVersion     string
		nativeOnly     bool
		wantVariant    string
		wantCommand    string
	}{
		{
			name:           "ordinary resume selects saved CLI",
			adapterVersion: "antigravity-cli/1.5.15",
			cliVersion:     "1.5.15",
			wantVariant:    acpcompat.AntigravityRuntimeCLI,
			wantCommand:    "agy",
		},
		{
			name:           "required native restore selects saved CLI",
			adapterVersion: "antigravity-cli/1.5.15",
			cliVersion:     "1.5.15",
			nativeOnly:     true,
			wantVariant:    acpcompat.AntigravityRuntimeCLI,
			wantCommand:    "agy",
		},
		{
			name:           "ordinary resume selects saved standalone",
			adapterVersion: "antigravity-standalone/1.4.5",
			cliVersion:     "1.5.15",
			wantVariant:    acpcompat.AntigravityRuntimeStandalone,
			wantCommand:    "agy_acp_server.par",
		},
		{
			name:           "required native restore selects saved standalone",
			adapterVersion: "antigravity-standalone/1.4.5",
			cliVersion:     "1.5.15",
			nativeOnly:     true,
			wantVariant:    acpcompat.AntigravityRuntimeStandalone,
			wantCommand:    "agy_acp_server.par",
		},
		{
			name:        "legacy native identity selects standalone",
			cliVersion:  "1.5.15",
			nativeOnly:  true,
			wantVariant: acpcompat.AntigravityRuntimeStandalone,
			wantCommand: "agy_acp_server.par",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("KANDEV_HOME_DIR", t.TempDir())
			dir := t.TempDir()
			if tt.cliVersion != "" {
				cliPath := filepath.Join(dir, "agy")
				if err := os.WriteFile(cliPath, []byte(lifecycleAntigravityCLIScript(tt.cliVersion)), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			writeLifecycleAntigravityStandalone(t, dir)
			t.Setenv("PATH", dir)

			// A fresh manager represents a backend restart. ACPSessionID is the
			// ordinary saved token path; no generation-backed native ID is set.
			manager := &Manager{commandBuilder: NewCommandBuilder(), logger: newTestLogger()}
			request := &LaunchRequest{ExecutorType: "local", RuntimeAdapterVersion: tt.adapterVersion}
			if tt.nativeOnly {
				request.RequiredNativeConversationID = "saved-native-conversation"
			} else {
				request.ACPSessionID = "saved-native-conversation"
			}
			commands, err := manager.buildAgentCommandWithContext(
				context.Background(), request, nil, agents.NewAntigravityACP(), agentruntime.RuntimeStandalone, false,
			)
			if err != nil {
				t.Fatalf("buildAgentCommandWithContext after restart: %v", err)
			}
			if filepath.Base(commands.args[0]) != tt.wantCommand || request.AuthPolicy == nil || request.AuthPolicy.RuntimeVariant != tt.wantVariant {
				t.Fatalf("resume selection = command %#v, policy %+v; want %s", commands.args, request.AuthPolicy, tt.wantVariant)
			}
			if tt.wantVariant == acpcompat.AntigravityRuntimeCLI && !slices.Contains(commands.args, "--process-per-session") {
				t.Fatalf("saved CLI resume command lacks worker mode: %#v", commands.args)
			}
		})
	}

	for _, nativeOnly := range []bool{false, true} {
		name := "ordinary resume"
		if nativeOnly {
			name = "required native restore"
		}
		t.Run("missing saved CLI does not fall back to standalone for "+name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("KANDEV_HOME_DIR", t.TempDir())
			dir := t.TempDir()
			writeLifecycleAntigravityStandalone(t, dir)
			t.Setenv("PATH", dir)
			manager := &Manager{commandBuilder: NewCommandBuilder(), logger: newTestLogger()}
			request := &LaunchRequest{
				ExecutorType:          "local",
				RuntimeAdapterVersion: "antigravity-cli/1.5.15",
			}
			if nativeOnly {
				request.RequiredNativeConversationID = "saved-native-conversation"
			} else {
				request.ACPSessionID = "saved-native-conversation"
			}
			_, err := manager.buildAgentCommandWithContext(
				context.Background(), request, nil, agents.NewAntigravityACP(), agentruntime.RuntimeStandalone, false,
			)
			var savedErr *antigravityruntime.SavedRuntimeError
			if !errors.As(err, &savedErr) || savedErr.Variant != acpcompat.AntigravityRuntimeCLI {
				t.Fatalf("resume error = %T %v, want saved CLI runtime error", err, err)
			}
		})
	}
}

func writeLifecycleAntigravityStandalone(t *testing.T, dir string) {
	t.Helper()
	serverName, harnessName := "agy_acp_server.par", "localharness_external"
	if runtime.GOOS == "windows" {
		serverName, harnessName = "agy_acp_server.exe", "localharness_external.exe"
	}
	for _, name := range []string{serverName, harnessName} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fixture"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func lifecycleAntigravityCLIScript(version string) string {
	return "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 'agy " + version + "'; exit 0; fi\n" +
		"if [ \"$1\" = \"acp\" ] && [ \"$2\" = \"--help\" ]; then printf 'Usage: agy acp [flags]\\n\\nFlags:\\n  --process-per-session  Serve each session from an isolated worker process\\n'; exit 0; fi\nexit 0\n"
}
