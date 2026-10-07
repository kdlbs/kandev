package managedruntime

import (
	"context"
	"errors"
	"testing"
)

// @covers AC-AGENTS-RUNTIME-NOTIFY-001.8, AC-AGENTS-RUNTIME-UPDATES-001.6
func TestValidatedVersionIsIndependentOfSelection(t *testing.T) {
	settings := &memorySettings{}
	store := NewStore(settings)
	ctx := context.Background()

	if _, found, err := store.GetValidated(ctx, "opencode-acp", "opencode-ai"); err != nil || found {
		t.Fatalf("empty validated record: found=%v err=%v", found, err)
	}
	if err := store.SaveValidated(ctx, "opencode-acp", "opencode-ai", "1.18.32"); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.Get(ctx, "opencode-acp", "opencode-ai"); err != nil || found {
		t.Fatalf("validated record became an operator selection: found=%v err=%v", found, err)
	}
	if err := store.Save(ctx, "opencode-acp", "opencode-ai", "1.18.34"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, "opencode-acp", "opencode-ai"); err != nil {
		t.Fatal(err)
	}
	validated, found, err := store.GetValidated(ctx, "opencode-acp", "opencode-ai")
	if err != nil || !found || validated != (Selection{Package: "opencode-ai", Version: "1.18.32"}) {
		t.Fatalf("selection delete changed validated record: %+v found=%v err=%v", validated, found, err)
	}
	if _, found, err := store.GetValidated(ctx, "opencode-acp", "other-package"); err != nil || found {
		t.Fatalf("validated record leaked across packages: found=%v err=%v", found, err)
	}
	if _, found, err := store.GetValidated(ctx, "claude-acp", "opencode-ai"); err != nil || found {
		t.Fatalf("validated record leaked across agents: found=%v err=%v", found, err)
	}
}

func TestValidatedVersionRejectsInvalidInput(t *testing.T) {
	settings := &memorySettings{}
	store := NewStore(settings)
	ctx := context.Background()
	for _, version := range []string{"", "latest", "1.2.3-beta.1"} {
		if err := store.SaveValidated(ctx, "opencode-acp", "opencode-ai", version); !errors.Is(err, ErrInvalidSelection) {
			t.Fatalf("SaveValidated(%q) error = %v", version, err)
		}
	}
	if err := store.SaveValidated(ctx, "", "opencode-ai", "1.0.0"); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("missing agent error = %v", err)
	}
	settings.values = map[string][]byte{validatedKey("opencode-acp"): []byte(`{"package":"opencode-ai","version":"next"}`)}
	if _, found, err := store.GetValidated(ctx, "opencode-acp", "opencode-ai"); !errors.Is(err, ErrInvalidSelection) || found {
		t.Fatalf("corrupt validated record: found=%v err=%v", found, err)
	}
}
