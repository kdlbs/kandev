package lifecycle

import (
	"errors"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

func TestStartupAttemptGenerationRejectsStaleEventsAfterRecovery(t *testing.T) {
	execution := &AgentExecution{}
	first := execution.beginStartupAttempt()
	if !execution.acceptsStartupAttempt(first) {
		t.Fatal("initial startup generation should be current")
	}

	second, ok := execution.beginStartupRecovery()
	if !ok {
		t.Fatal("expected startup recovery generation")
	}
	if second == first || execution.acceptsStartupAttempt(first) {
		t.Fatal("first child generation remained current after recovery")
	}
	if !execution.acceptsStartupAttempt(second) {
		t.Fatal("retry startup generation should be current")
	}
	if _, ok := execution.beginStartupRecovery(); ok {
		t.Fatal("startup recovery should be attempted at most once")
	}
	execution.finishStartupRecovery()
}

func TestOnlineManagedRuntimeArgsPreserveTrustedLaunchIdentity(t *testing.T) {
	spec := agents.ManagedNPMRuntimeSpec{
		Package: "@scope/managed-acp",
		ACPArgs: []string{"--acp", "--model", "fast"},
	}
	initial := []string{"greywall", "--", "npx", "--yes", "--prefer-offline", "@scope/managed-acp@1.2.3", "--acp", "--model", "fast"}

	got, packageSpec, ok := onlineManagedRuntimeArgs(initial, spec)
	if !ok {
		t.Fatal("expected managed runtime recovery command")
	}
	want := []string{"greywall", "--", "npx", "--yes", "--prefer-online", "@scope/managed-acp@1.2.3", "--acp", "--model", "fast"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("online args = %#v, want %#v", got, want)
	}
	if packageSpec != "@scope/managed-acp@1.2.3" {
		t.Fatalf("package spec = %q, want exact selected spec", packageSpec)
	}
}

func TestOnlineManagedRuntimeArgsRejectsNonManagedCommands(t *testing.T) {
	spec := agents.ManagedNPMRuntimeSpec{Package: "managed-acp"}
	for _, args := range [][]string{
		{"native-agent", "--acp"},
		{"npx", "--yes", "--prefer-offline", "other-agent@1.2.3"},
		{"npx", "--yes", "--prefer-online", "managed-acp@1.2.3"},
	} {
		if _, _, ok := onlineManagedRuntimeArgs(args, spec); ok {
			t.Fatalf("command %#v should not be eligible for managed runtime recovery", args)
		}
	}
}

func TestOnlineManagedRuntimeArgsAcceptsTrustedUnversionedPackage(t *testing.T) {
	spec := agents.ManagedNPMRuntimeSpec{Package: "managed-acp"}
	args := []string{"npx", "--yes", "--prefer-offline", "managed-acp", "--acp"}

	got, packageSpec, ok := onlineManagedRuntimeArgs(args, spec)
	if !ok {
		t.Fatal("expected the trusted unversioned command to be eligible")
	}
	if packageSpec != "managed-acp" {
		t.Fatalf("package spec = %q, want managed-acp", packageSpec)
	}
	want := []string{"npx", "--yes", "--prefer-online", "managed-acp", "--acp"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("online args = %#v, want %#v", got, want)
	}
}

func TestManagedRuntimeRetryFailureClassificationPreservesSecondResult(t *testing.T) {
	retryErr := errors.New("failed to initialize ACP: secret=abcdefghijklmnopqrstuvwxyz123456")
	second := &routingerr.Error{
		Code:       routingerr.CodeAuthRequired,
		RawExcerpt: "authentication required",
	}

	code, details := managedRuntimeRetryFailureClassification(
		routingerr.CodeManagedRuntimeNpmResolution,
		"initial npm details",
		retryErr,
		true,
		second,
	)
	if code != routingerr.CodeAuthRequired || details != "authentication required" {
		t.Fatalf("retry classification = (%q, %q), want auth_required and sanitized second excerpt", code, details)
	}

	code, details = managedRuntimeRetryFailureClassification(
		routingerr.CodeManagedRuntimeNpmResolution,
		"initial npm details",
		retryErr,
		false,
		nil,
	)
	if code != routingerr.CodeAgentRuntime {
		t.Fatalf("configure failure code = %q, want %q", code, routingerr.CodeAgentRuntime)
	}
	if details == retryErr.Error() {
		t.Fatal("configure failure details must be sanitized")
	}
}
