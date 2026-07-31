package controller

import (
	"context"
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
)

func newCustomTUIController(t *testing.T, st *fakeStore) *Controller {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return &Controller{agentRegistry: registry.NewRegistry(log), repo: st, logger: log}
}

// TestCreateCustomTUIAgent_CommandArgsReachArgv is the test that distinguishes
// the fix from the workaround. Smuggling flags into the space-separated
// Command string cannot express an argument that itself contains a space —
// strings.Fields would split it. Passing it through CommandArgs must deliver
// it to argv as a single element.
func TestCreateCustomTUIAgent_CommandArgsReachArgv(t *testing.T) {
	st := newFakeStore()
	c := newCustomTUIController(t, st)

	_, err := c.CreateCustomTUIAgent(context.Background(), CreateCustomTUIAgentRequest{
		DisplayName: "Spaced Args",
		Command:     "my-cli",
		CommandArgs: []string{"--system-prompt", "you are a helpful agent"},
	})
	if err != nil {
		t.Fatalf("CreateCustomTUIAgent: %v", err)
	}

	ag, ok := c.agentRegistry.Get("spaced-args")
	if !ok {
		t.Fatal("agent not registered")
	}
	want := []string{"my-cli", "--system-prompt", "you are a helpful agent"}
	if got := ag.Runtime().Cmd.Args(); !slices.Equal(got, want) {
		t.Errorf("argv = %#v, want %#v", got, want)
	}
}

// TestCreateCustomTUIAgent_CommandArgsPersisted pins that the args survive a
// restart: they must be written to the stored TUI config, not just handed to
// the in-memory registry.
func TestCreateCustomTUIAgent_CommandArgsPersisted(t *testing.T) {
	st := newFakeStore()
	c := newCustomTUIController(t, st)

	args := []string{"--flag", "value with space"}
	if _, err := c.CreateCustomTUIAgent(context.Background(), CreateCustomTUIAgentRequest{
		DisplayName: "Persisted Args",
		Command:     "my-cli",
		CommandArgs: args,
	}); err != nil {
		t.Fatalf("CreateCustomTUIAgent: %v", err)
	}

	stored, ok := st.byName["persisted-args"]
	if !ok {
		t.Fatal("agent not persisted")
	}
	if stored.TUIConfig == nil {
		t.Fatal("stored agent has no TUI config")
	}
	if got := stored.TUIConfig.CommandArgs; !slices.Equal(got, args) {
		t.Errorf("stored CommandArgs = %#v, want %#v", got, args)
	}
}

// TestCreateCustomTUIAgent_NoCommandArgs keeps the existing behaviour intact
// when the caller omits the field.
func TestCreateCustomTUIAgent_NoCommandArgs(t *testing.T) {
	st := newFakeStore()
	c := newCustomTUIController(t, st)

	if _, err := c.CreateCustomTUIAgent(context.Background(), CreateCustomTUIAgentRequest{
		DisplayName: "Plain",
		Command:     "my-cli --verbose",
	}); err != nil {
		t.Fatalf("CreateCustomTUIAgent: %v", err)
	}

	ag, ok := c.agentRegistry.Get("plain")
	if !ok {
		t.Fatal("agent not registered")
	}
	want := []string{"my-cli", "--verbose"}
	if got := ag.Runtime().Cmd.Args(); !slices.Equal(got, want) {
		t.Errorf("argv = %#v, want %#v", got, want)
	}
	if got := st.byName["plain"].TUIConfig.CommandArgs; len(got) != 0 {
		t.Errorf("stored CommandArgs = %#v, want empty", got)
	}
}
