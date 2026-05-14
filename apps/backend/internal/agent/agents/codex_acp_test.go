package agents

import (
	"strings"
	"testing"
)

func TestCodexACPRuntimeNoLongerBindMountsHostHome(t *testing.T) {
	a := NewCodexACP()
	rt := a.Runtime()

	for _, m := range rt.Mounts {
		if strings.Contains(m.Source, "{home}") {
			t.Fatalf("codex Mounts unexpectedly references {home}: %+v", m)
		}
	}
}

func TestCodexACPSessionDirTemplate(t *testing.T) {
	a := NewCodexACP()
	cfg := a.Runtime().SessionConfig

	if cfg.SessionDirTemplate != "{home}/.codex" {
		t.Fatalf("SessionDirTemplate = %q, want %q", cfg.SessionDirTemplate, "{home}/.codex")
	}
	if cfg.SessionDirTarget != "/root/.codex" {
		t.Fatalf("SessionDirTarget = %q, want %q", cfg.SessionDirTarget, "/root/.codex")
	}
}
