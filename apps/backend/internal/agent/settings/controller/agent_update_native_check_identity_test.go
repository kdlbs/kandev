package controller

import (
	"github.com/kandev/kandev/internal/agent/agents"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeCheckSymlinkRetargetChangesIdentity(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	dir := filepath.Join(h.home, "install", "versions")
	first, second := filepath.Join(dir, "18.13.3"), filepath.Join(dir, "18.13.4")
	writeNativeCheckFile(t, first, "first binary")
	writeNativeCheckFile(t, second, "second binary")
	link := filepath.Join(h.home, "omp")
	if err := os.Symlink(first, link); err != nil {
		t.Fatal(err)
	}
	h.runner.setPath(link)
	spec := agents.NewOmpACP().NativeUpdateCheck()
	before := h.ctrl.prepareNativeUpdateCheck(ompAgentID, "18.13.3", spec)
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(second, link); err != nil {
		t.Fatal(err)
	}
	after := h.ctrl.prepareNativeUpdateCheck(ompAgentID, "18.13.3", spec)
	if before.key == after.key {
		t.Fatal("retargeted executable to a different real path but cache identity did not change")
	}
}

func TestNativeCheckFoldedXDGBunfigChangesIdentity(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	xdg := filepath.Join(h.home, "xdg")
	h.runner.setEnv(append(h.baseEnv(), "xdg_config_home="+xdg)...)
	config := filepath.Join(xdg, ".bunfig.toml")
	writeNativeCheckFile(t, config, "[install]\nregistry = 'https://old.invalid'\n")
	spec := agents.NewOmpACP().NativeUpdateCheck()
	before := h.ctrl.prepareNativeUpdateCheck(ompAgentID, "18.13.3", spec)
	writeNativeCheckFile(t, config, "[install]\nregistry = 'https://new-and-longer.invalid'\n")
	after := h.ctrl.prepareNativeUpdateCheck(ompAgentID, "18.13.3", spec)
	if before.key == after.key {
		t.Fatal("changed upstream-selected XDG bunfig size but cache identity did not change")
	}
}
