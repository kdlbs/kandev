package instance

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/pkg/agent"
)

func TestBuiltinMCPInstanceInvalidIDDoesNotAllocatePort(t *testing.T) {
	mgr := newConcurrencyTestManager(t, 1)
	_, err := mgr.admitCreate(context.Background(), &CreateRequest{ID: "exec/1", WorkspacePath: t.TempDir(), ScopedMCPRoutes: true})
	if err == nil {
		t.Fatal("admitCreate() error = nil, want invalid scoped ID error")
	}
	if got := len(mgr.provisional); got != 0 {
		t.Fatalf("provisional count = %d, want 0", got)
	}
	if got := len(mgr.portAlloc.allocated); got != 0 {
		t.Fatalf("allocated port count = %d, want 0", got)
	}
}

func TestBuiltinMCPInstanceCreateResponseAndInfoCarryEffectiveMode(t *testing.T) {
	mgr := newConcurrencyTestManager(t, 1)
	response, err := mgr.CreateInstance(context.Background(), &CreateRequest{
		ID: "exec-1", WorkspacePath: t.TempDir(), ScopedMCPRoutes: true,
		Protocol: string(agent.ProtocolACP),
	})
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}
	if !response.ScopedMCPRoutes {
		t.Fatalf("CreateResponse.ScopedMCPRoutes = false, want true")
	}
	instance, ok := mgr.GetInstance(response.ID)
	if !ok {
		t.Fatalf("GetInstance(%q) not found", response.ID)
	}
	info := instance.Info()
	if !info.ScopedMCPRoutes {
		t.Fatal("Instance.Info() did not retain scoped mode")
	}
	if err := mgr.StopInstance(context.Background(), response.ID); err != nil {
		t.Fatalf("StopInstance() error = %v", err)
	}
	encoded, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Marshal(InstanceInfo) error = %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatalf("Unmarshal(InstanceInfo) error = %v", err)
	}
	if wire["scoped_mcp_routes"] != true {
		t.Fatalf("InstanceInfo scoped_mcp_routes = %v, want true", wire["scoped_mcp_routes"])
	}
}

func TestBuiltinMCPInstanceLegacyCreateDefaultsToBareMode(t *testing.T) {
	cfg, err := (&config.Config{}).NewInstanceConfig(43210, nil)
	if err != nil {
		t.Fatalf("NewInstanceConfig() error = %v", err)
	}
	if cfg.ScopedMCPRoutes {
		t.Fatal("legacy direct instance unexpectedly enabled scoped MCP routes")
	}
}
