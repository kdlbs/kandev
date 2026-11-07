package process

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
)

func TestManagedRuntimePreparationDirectiveBindsExactFailureAndVersion(t *testing.T) {
	command := agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand("1.2.3").Args()
	manager := NewManager(&config.InstanceConfig{
		AgentType: "claude-acp", AgentArgs: command, AgentEnv: []string{"PATH=/usr/bin"}, WorkDir: t.TempDir(),
	}, newTestLogger(t))
	t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })
	manager.startupGeneration = 17
	manager.startupEvidence = &types.ManagedStartupEvidence{
		ProcessGeneration: 17,
		CacheEvidence: &types.ManagedRuntimeCacheEvidence{
			Condition:         types.ManagedRuntimeCacheRootManifestMissing,
			PackageSpec:       "@agentclientprotocol/claude-agent-acp@1.2.3",
			ProcessGeneration: 17,
		},
	}
	directive := types.ManagedRuntimePreparationDirective{
		Condition:               types.ManagedRuntimeCacheRootManifestMissing,
		PackageSpec:             "@agentclientprotocol/claude-agent-acp@1.2.3",
		FailedProcessGeneration: 17,
	}
	snapshot, err := manager.validateManagedRuntimePreparationDirective(directive)
	if err != nil {
		t.Fatalf("validate operator-selected exact version: %v", err)
	}
	if !sameManagedCommand(snapshot.Command, command) {
		t.Fatalf("validated command = %#v, want exact selected version command %#v", snapshot.Command, command)
	}
	if err := manager.Configure("", command, true, nil, "", nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.validateManagedRuntimePreparationDirective(directive); err != nil {
		t.Fatalf("unchanged retry configuration retired verified evidence: %v", err)
	}

	for name, mutate := range map[string]func(*types.ManagedRuntimePreparationDirective){
		"stale generation": func(value *types.ManagedRuntimePreparationDirective) { value.FailedProcessGeneration++ },
		"different exact version": func(value *types.ManagedRuntimePreparationDirective) {
			value.PackageSpec = "@agentclientprotocol/claude-agent-acp@1.2.4"
		},
		"different condition":                func(value *types.ManagedRuntimePreparationDirective) { value.Condition = "other" },
		"probe attempt on process directive": func(value *types.ManagedRuntimePreparationDirective) { value.ProbeAttemptID = "probe-1" },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := directive
			mutate(&invalid)
			if _, err := manager.validateManagedRuntimePreparationDirective(invalid); err == nil {
				t.Fatalf("accepted mismatched directive %#v", invalid)
			}
		})
	}

	manager.managedRuntimeCommand = append([]string(nil), command...)
	if err := manager.Configure("", []string{"npx", "--yes", "--prefer-offline", "operator-package"}, true, nil, "", nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.validateManagedRuntimePreparationDirective(directive); err == nil {
		t.Fatal("accepted evidence after the managed command changed")
	}
}

func TestPreparedRuntimeOwnerSurvivesStopAndClosesOnlyAtInstanceTeardown(t *testing.T) {
	packageSpec := "@agentclientprotocol/claude-agent-acp@1.2.3"
	prepared := makeProcessPreparedRuntimeFixture(t, packageSpec)
	root := filepath.Dir(prepared.ProjectPrefix())
	manager := NewManager(&config.InstanceConfig{WorkDir: t.TempDir()}, newTestLogger(t))
	manager.preparedRuntime = prepared
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("private owner root after agent Stop = %v, want still present", err)
	}
	if err := manager.StopForTeardown(context.Background()); err != nil {
		t.Fatalf("StopForTeardown: %v", err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("private owner root after instance teardown = %v, want removed", err)
	}
}

func TestManagedRuntimePreparationRejectsLateEvidenceAfterConfigure(t *testing.T) {
	command := agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand("1.2.3").Args()
	manager := NewManager(&config.InstanceConfig{AgentType: "claude-acp", AgentArgs: command, WorkDir: t.TempDir()}, newTestLogger(t))
	t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })
	generation := manager.beginManagedStartupGeneration()
	done := manager.startupEvidenceDone
	changed := agents.NewClaudeACP().ManagedNPMRuntime().RuntimeCommand("1.2.4").Args()
	if err := manager.Configure("", changed, true, nil, "", nil, false); err != nil {
		t.Fatal(err)
	}
	manager.recordManagedStartupEvidence(&types.ManagedStartupEvidence{ProcessGeneration: generation})
	if evidence := manager.ManagedStartupEvidence(context.Background(), generation); evidence != nil {
		t.Fatalf("accepted late evidence from before reconfiguration: %#v", evidence)
	}
	select {
	case <-done:
	default:
		t.Fatal("configuration did not release pending evidence readers")
	}
	if manager.ProcessGeneration() != generation {
		t.Fatal("configuration changed the launched process identity")
	}
}

func makeProcessPreparedRuntimeFixture(t *testing.T, packageSpec string) *managedruntime.PreparedRuntime {
	t.Helper()
	prepared, err := managedruntime.PreparePrivateRuntime(context.Background(), packageSpec, nil,
		func(_ context.Context, command managedruntime.ManagedRuntimeCommand) error {
			cache, ok := runtimeCommandArgument(command.Args, "--cache")
			if !ok {
				return os.ErrInvalid
			}
			separator := strings.LastIndexByte(packageSpec, '@')
			name, exactVersion := packageSpec[:separator], packageSpec[separator+1:]
			executionRoot := filepath.Join(cache, "_npx", managedruntime.NpxExecutionCacheKey(packageSpec))
			packageDir := filepath.Join(executionRoot, "node_modules", filepath.FromSlash(name))
			binName, target := "claude-agent-acp", filepath.Join("bin", "cli.js")
			manifest, marshalErr := json.Marshal(map[string]any{
				"name": name, "version": exactVersion, "bin": map[string]string{binName: filepath.ToSlash(target)},
			})
			if marshalErr != nil {
				return marshalErr
			}
			if err := os.MkdirAll(filepath.Join(packageDir, "bin"), 0o700); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(executionRoot, "package.json"), []byte(`{}`), 0o600); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(packageDir, "package.json"), manifest, 0o600); err != nil {
				return err
			}
			executable := filepath.Join(packageDir, target)
			if err := os.WriteFile(executable, []byte("#!/usr/bin/env node\n"), 0o700); err != nil {
				return err
			}
			binDirectory := filepath.Join(executionRoot, "node_modules", ".bin")
			if err := os.MkdirAll(binDirectory, 0o700); err != nil {
				return err
			}
			if runtime.GOOS == "windows" {
				relative, err := filepath.Rel(binDirectory, executable)
				if err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(binDirectory, binName+".cmd"), []byte(strings.ReplaceAll(relative, "/", "\\")), 0o600)
			}
			relative, err := filepath.Rel(binDirectory, executable)
			if err != nil {
				return err
			}
			return os.Symlink(relative, filepath.Join(binDirectory, binName))
		})
	if err != nil {
		t.Fatalf("prepare private runtime fixture: %v", err)
	}
	return prepared
}

func runtimeCommandArgument(args []string, key string) (string, bool) {
	for index, arg := range args {
		if arg == key && index+1 < len(args) {
			return args[index+1], true
		}
	}
	return "", false
}
