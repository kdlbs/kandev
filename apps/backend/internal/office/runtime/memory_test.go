package runtime

import "testing"

func TestParseMemoryNamespace(t *testing.T) {
	ns, err := ParseMemoryNamespace("/workspaces/ws-1/memory/agents/agent-1/knowledge/notes.md")
	if err != nil {
		t.Fatalf("ParseMemoryNamespace: %v", err)
	}
	if ns.WorkspaceID != "ws-1" || ns.Kind != MemoryKindAgent || ns.ID != "agent-1" || ns.Key != "knowledge/notes.md" {
		t.Fatalf("namespace = %#v", ns)
	}
}

func TestCanAccessMemoryScopesAgentWritesToSelf(t *testing.T) {
	runCtx := RunContext{
		WorkspaceID: "ws-1",
		AgentID:     "agent-1",
		Capabilities: Capabilities{
			CanReadMemory:  true,
			CanWriteMemory: true,
		},
	}
	own := MemoryNamespace{WorkspaceID: "ws-1", Kind: MemoryKindAgent, ID: "agent-1", Key: "knowledge/a"}
	if !CanAccessMemory(runCtx, own, true) {
		t.Fatal("expected own agent memory write to be allowed")
	}
	other := MemoryNamespace{WorkspaceID: "ws-1", Kind: MemoryKindAgent, ID: "agent-2", Key: "knowledge/a"}
	if CanAccessMemory(runCtx, other, true) {
		t.Fatal("expected other agent memory write to be denied")
	}
}

func TestCanAccessMemoryRequiresCapabilityAndWorkspace(t *testing.T) {
	runCtx := RunContext{WorkspaceID: "ws-1", AgentID: "agent-1", Capabilities: Capabilities{CanReadMemory: true}}
	ns := MemoryNamespace{WorkspaceID: "ws-2", Kind: MemoryKindTask, ID: "task-1", Key: "notes"}
	if CanAccessMemory(runCtx, ns, false) {
		t.Fatal("expected cross-workspace read to be denied")
	}
	ns.WorkspaceID = "ws-1"
	if CanAccessMemory(runCtx, ns, true) {
		t.Fatal("expected write without write_memory to be denied")
	}
}
