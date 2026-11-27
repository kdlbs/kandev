package agents

import (
	"slices"
	"testing"
)

// AC-AGENTS-RUNTIME-UPDATES-003.9, AC-AGENTS-RUNTIME-UPDATES-003.10,
// AC-AGENTS-RUNTIME-NATIVE-CHECK-001.2, AC-AGENTS-RUNTIME-NATIVE-CHECK-001.8.
func TestOmpHarnessUpdateContract(t *testing.T) {
	agent := NewOmpACP()
	spec := agent.HarnessUpdate()
	if spec.Package != "@oh-my-pi/pi-coding-agent" {
		t.Errorf("metadata package = %q", spec.Package)
	}
	if !slices.Equal(spec.UpdateCommand.Args(), []string{"omp", "update"}) {
		t.Errorf("update argv = %v", spec.UpdateCommand.Args())
	}
	if _, managed := any(agent).(ManagedNPMRuntimeAgent); managed {
		t.Fatal("OMP must not be a managed npm runtime")
	}
	for name, got := range map[string][]string{
		"BuildCommand":            agent.BuildCommand(CommandOptions{}).Args(),
		"Runtime.Cmd":             agent.Runtime().Cmd.Args(),
		"InferenceConfig.Command": agent.InferenceConfig().Command.Args(),
	} {
		if !slices.Equal(got, []string{"omp", "acp"}) {
			t.Errorf("%s = %v, want omp acp", name, got)
		}
	}
	if got := agent.PassthroughConfig().PassthroughCmd.Args(); !slices.Equal(got, []string{"omp"}) {
		t.Errorf("passthrough command = %v", got)
	}
	if got := agent.InstallScript(); got != "bun install -g @oh-my-pi/pi-coding-agent" {
		t.Errorf("install script = %q", got)
	}
	check := agent.NativeUpdateCheck()
	if got := check.Command.Args(); !slices.Equal(got, []string{"omp", "update", "--check"}) {
		t.Errorf("native check argv = %v", got)
	}
	if check.Parse == nil || check.Context.Sources == nil || len(check.Context.EnvKeys) == 0 {
		t.Errorf("native check spec is incomplete: %+v", check)
	}
	capability := RuntimeUpdateCapabilities(agent)
	want := RuntimeUpdateCapability{
		RuntimeID: "omp-acp", Owner: "external", Mechanism: "native", Management: "manual",
		Source: RuntimeReleaseSource{NPM: "@oh-my-pi/pi-coding-agent", GuidanceURL: "https://www.npmjs.com/package/@oh-my-pi/pi-coding-agent"},
	}
	if capability.RuntimeID != want.RuntimeID || capability.Owner != want.Owner || capability.Mechanism != want.Mechanism ||
		capability.Management != want.Management || capability.Source != want.Source || capability.Managed != nil || capability.ManagedFallback != nil {
		t.Errorf("runtime update capability = %+v, want %+v", capability, want)
	}
}
