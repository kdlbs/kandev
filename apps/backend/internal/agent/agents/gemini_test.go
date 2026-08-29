package agents

import (
	"slices"
	"testing"
)

func TestGeminiUsesManagedPackageOnEveryNPMCommandSurface(t *testing.T) {
	ag := NewGemini()
	wantACP := ag.ManagedNPMRuntime().CachedACPCommand().Args()
	wantPassthrough := []string{"npx", "--yes", "--prefer-offline", "@google/gemini-cli"}

	if got := ag.BuildCommand(CommandOptions{}).Args(); !slices.Equal(got, wantACP) {
		t.Fatalf("BuildCommand = %#v, want %#v", got, wantACP)
	}
	if got := ag.Runtime().Cmd.Args(); !slices.Equal(got, wantACP) {
		t.Fatalf("Runtime Cmd = %#v, want %#v", got, wantACP)
	}
	if got := ag.InferenceConfig().Command.Args(); !slices.Equal(got, wantACP) {
		t.Fatalf("Inference Command = %#v, want %#v", got, wantACP)
	}
	if got := ag.PassthroughConfig().PassthroughCmd.Args(); !slices.Equal(got, wantPassthrough) {
		t.Fatalf("Passthrough Cmd = %#v, want %#v", got, wantPassthrough)
	}
	if got, want := ag.InstallScript(), "npm install -g @google/gemini-cli"; got != want {
		t.Fatalf("InstallScript = %q, want %q", got, want)
	}
}
