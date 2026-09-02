package process

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
)

func TestManagerBackgroundWorkReferenceCounting(t *testing.T) {
	manager := NewManager(&config.InstanceConfig{WorkDir: t.TempDir()}, newTestLogger(t))
	first := manager.BeginBackgroundWork()
	second := manager.BeginBackgroundWork()

	if !manager.HasBackgroundWork() {
		t.Fatal("background work was not reported")
	}
	first()
	first()
	if !manager.HasBackgroundWork() {
		t.Fatal("idempotent release removed another owner's background work")
	}
	second()
	if manager.HasBackgroundWork() {
		t.Fatal("background work remained after all owners released")
	}
}
