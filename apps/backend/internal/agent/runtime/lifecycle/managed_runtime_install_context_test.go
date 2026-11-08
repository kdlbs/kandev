package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/managedruntime"
	runtimeenv "github.com/kandev/kandev/internal/agent/runtime/environment"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/auth/authn"
)

func TestManagedRuntimeInstallerEnvironmentDigest(t *testing.T) {
	definitions := []runtimeenv.Definition{
		{Key: "KANDEV_TASK_ID", Literal: "task-a", Origin: runtimeenv.OriginManagedRuntime},
		{Key: "KANDEV_SESSION_ID", Literal: "session-a", Origin: runtimeenv.OriginManagedRuntime},
		{Key: "PROFILE_VAR", Literal: "baseline", Origin: runtimeenv.OriginAgentProfile},
		{Key: "NPM_TOKEN", Literal: "credential-a", Origin: runtimeenv.OriginManagedCredentials},
		{Key: "EXECUTOR_VAR", Literal: "executor", Origin: runtimeenv.OriginExecutorProfile},
	}
	resolved := map[string]string{
		"KANDEV_TASK_ID": "task-a", "KANDEV_SESSION_ID": "session-a",
		"PROFILE_VAR": "baseline", "NPM_TOKEN": "credential-a", "EXECUTOR_VAR": "executor",
	}
	digest := managedRuntimeInstallerEnvironmentDigest(definitions, resolved)
	if digest == "" {
		t.Fatal("installer environment digest is empty")
	}
	generatedChanged := make([]runtimeenv.Definition, len(definitions))
	copy(generatedChanged, definitions)
	generatedResolved := make(map[string]string, len(resolved))
	for key, value := range resolved {
		generatedResolved[key] = value
	}
	generatedResolved["KANDEV_TASK_ID"] = "task-b"
	generatedResolved["KANDEV_SESSION_ID"] = "session-b"
	generatedChanged[0] = runtimeenv.Definition{Key: "KANDEV_TASK_ID", Literal: "task-b", Origin: runtimeenv.OriginManagedRuntime}
	generatedChanged[1] = runtimeenv.Definition{Key: "KANDEV_SESSION_ID", Literal: "session-b", Origin: runtimeenv.OriginManagedRuntime}
	if changed := managedRuntimeInstallerEnvironmentDigest(generatedChanged, generatedResolved); changed != digest {
		t.Fatal("generated task identity changed the installer installation context")
	}
	for name, mutate := range map[string]func(map[string]string){
		"profile":     func(values map[string]string) { values["PROFILE_VAR"] = "changed" },
		"credentials": func(values map[string]string) { values["NPM_TOKEN"] = "credential-b" },
		"executor":    func(values map[string]string) { values["EXECUTOR_VAR"] = "changed" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := make(map[string]string, len(resolved))
			for key, value := range resolved {
				changed[key] = value
			}
			mutate(changed)
			if managedRuntimeInstallerEnvironmentDigest(definitions, changed) == digest {
				t.Fatalf("changed %s input reused the installer installation context", name)
			}
		})
	}
	if digest := managedRuntimeInstallerEnvironmentDigest(nil, nil); digest == "" {
		t.Fatal("a known empty installer environment must still produce a stable digest")
	} else if again := managedRuntimeInstallerEnvironmentDigest(nil, nil); again != digest {
		t.Fatalf("empty installer environment digest changed: %q != %q", digest, again)
	}
}

func TestManagedRuntimeInstallContextRequiresProvenance(t *testing.T) {
	if context := managedRuntimeInstallContext(nil); context != nil {
		t.Fatalf("nil execution context = %#v, want nil", context)
	}
	if context := managedRuntimeInstallContext(&AgentExecution{}); context != nil {
		t.Fatalf("execution without provenance context = %#v, want nil", context)
	}
	execution := &AgentExecution{
		ManagedRuntimeInstallerEnvDigest: managedruntime.InstallerEnvironmentDigest(map[string]string{"PROFILE": "baseline"}),
		ManagedRuntimeOwnerUserID:        "owner-a",
	}
	context := managedRuntimeInstallContext(execution)
	if context == nil || context.InstallerEnvDigest != execution.ManagedRuntimeInstallerEnvDigest ||
		context.OwnerScope != managedruntime.RepairOwnerScopeTask(execution.ManagedRuntimeOwnerUserID) || !context.Valid() {
		t.Fatalf("installer context = %#v, want the task scope and exact digest", context)
	}
}

func TestManagedRuntimePreparationRemainingBudget(t *testing.T) {
	preparation := agentctltypes.ManagedRuntimePreparationDirective{PackageSpec: "managed-acp@1.2.3"}
	deadlineCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	bounded, err := managedRuntimePreparationWithRemainingBudget(deadlineCtx, preparation)
	if err != nil {
		t.Fatalf("bounded preparation: %v", err)
	}
	if bounded.RemainingBudgetSeconds < 28 || bounded.RemainingBudgetSeconds > 30 {
		t.Fatalf("remaining budget = %d, want the caller's deadline", bounded.RemainingBudgetSeconds)
	}
	if preparation.RemainingBudgetSeconds != 0 {
		t.Fatal("budget calculation mutated the caller's directive")
	}
	expiredCtx, expiredCancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer expiredCancel()
	if _, err := managedRuntimePreparationWithRemainingBudget(expiredCtx, preparation); err == nil {
		t.Fatal("accepted an expired recovery deadline")
	}
	unbounded, err := managedRuntimePreparationWithRemainingBudget(context.Background(), preparation)
	if err != nil || unbounded.RemainingBudgetSeconds != 0 {
		t.Fatalf("unbounded preparation = (%#v, %v), want no explicit budget", unbounded, err)
	}
}

func TestManagedRuntimeWorkspaceOwnerResolution(t *testing.T) {
	info := &WorkspaceInfo{OwnerUserID: "workspace-owner"}
	if got := managedRuntimeWorkspaceOwnerUserID(context.Background(), info); got != "workspace-owner" {
		t.Fatalf("workspace owner = %q, want the recorded owner", got)
	}
	identity := authn.WithIdentity(context.Background(), authn.Identity{UserID: "actor-a"})
	if got := managedRuntimeWorkspaceOwnerUserID(identity, &WorkspaceInfo{}); got != "actor-a" {
		t.Fatalf("actor owner = %q, want the authenticated actor", got)
	}
	if got := managedRuntimeWorkspaceOwnerUserID(context.Background(), &WorkspaceInfo{}); got != "" {
		t.Fatalf("missing owner = %q, want empty", got)
	}
}
