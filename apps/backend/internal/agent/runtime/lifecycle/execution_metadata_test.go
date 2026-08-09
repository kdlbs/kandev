package lifecycle

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

// TestExecutionMetadataConcurrentAccessIsSafe pins the invariant behind the
// "fatal error: concurrent map writes" crash in RemoveExecution: the passthrough
// cleanup deletes metadata keys while launch/prompt-path writers are still
// storing on the same execution. Run with -race, this fails on any access that
// bypasses metadataMu.
func TestExecutionMetadataConcurrentAccessIsSafe(t *testing.T) {
	mgr := newTestManager(t)
	execution := &AgentExecution{ID: "exec-1", SessionID: "session-1"}
	setPassthroughMCPFiles(execution, []string{filepath.Join(t.TempDir(), "mcp.json")})
	setPassthroughMCPEnv(execution, map[string]string{"KANDEV_MCP": "1"})

	workers := []func(int){
		func(i int) { execution.setMetadataValue("task_description", fmt.Sprintf("desc-%d", i)) },
		func(i int) { setPassthroughMCPEnv(execution, map[string]string{"KANDEV_MCP": fmt.Sprint(i)}) },
		func(i int) { setPassthroughMCPFiles(execution, []string{fmt.Sprintf("file-%d.json", i)}) },
		func(int) { mgr.cleanupPassthroughMCPConfig(execution) },
		func(int) { _ = execution.MetadataSnapshot() },
		func(int) { _ = execution.metadataString(MetadataKeyModelOverride) },
		func(int) { _ = execution.metadataBool(MetadataKeyIsRemote) },
		func(int) { _ = execution.metadataInt(MetadataKeyLocalPort) },
		func(int) { _ = getPassthroughMCPFiles(execution) },
		func(int) { _ = getPassthroughMCPEnv(execution) },
	}

	const iterations = 200
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, worker := range workers {
		wg.Add(1)
		go func(work func(int)) {
			defer wg.Done()
			<-start
			for i := range iterations {
				work(i)
			}
		}(worker)
	}
	close(start)
	wg.Wait()
}

// TestRemoveExecutionRacesMetadataWrites reproduces the reported stack directly:
// a scheduled stop calls RemoveExecution -> cleanupPassthroughMCPConfig while
// another goroutine still writes metadata on the same execution.
func TestRemoveExecutionRacesMetadataWrites(t *testing.T) {
	mgr, execution, profile := newClaudePassthroughMCPTestManager(t)
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	if _, _, _, _, err := mgr.passthroughAgentCommand(t.Context(), execution, profile); err != nil {
		t.Fatalf("passthroughAgentCommand returned error: %v", err)
	}
	if len(getPassthroughMCPFiles(execution)) == 0 {
		t.Fatal("passthrough MCP config file was not stored")
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		mgr.RemoveExecution(execution.ID)
	}()
	go func() {
		defer wg.Done()
		<-start
		for i := range 500 {
			execution.setMetadataValue("task_description", fmt.Sprintf("desc-%d", i))
		}
	}()
	close(start)
	wg.Wait()

	if files := getPassthroughMCPFiles(execution); len(files) != 0 {
		t.Fatalf("passthrough MCP config files metadata = %v, want empty", files)
	}
}

func TestExecutionMetadataAccessors(t *testing.T) {
	execution := &AgentExecution{ID: "exec-1"}

	// Reads on empty metadata return zero values rather than panicking.
	if got := execution.metadataString("missing"); got != "" {
		t.Fatalf("metadataString on empty metadata = %q, want empty", got)
	}
	if execution.metadataBool("missing") {
		t.Fatal("metadataBool on empty metadata = true, want false")
	}
	if got := execution.metadataInt("missing"); got != 0 {
		t.Fatalf("metadataInt on empty metadata = %d, want 0", got)
	}
	if got := execution.MetadataSnapshot(); got != nil {
		t.Fatalf("MetadataSnapshot on empty metadata = %v, want nil", got)
	}

	// setMetadataValue lazily allocates.
	execution.setMetadataValue("name", "kandev")
	execution.setMetadataValue("remote", true)
	execution.setMetadataValue("port", float64(45678)) // the shape metadata has after a JSON round-trip

	if got := execution.metadataString("name"); got != "kandev" {
		t.Fatalf("metadataString = %q, want kandev", got)
	}
	if !execution.metadataBool("remote") {
		t.Fatal("metadataBool = false, want true")
	}
	if got := execution.metadataInt("port"); got != 45678 {
		t.Fatalf("metadataInt = %d, want 45678", got)
	}
	if got := execution.metadataString("remote"); got != "" {
		t.Fatalf("metadataString on a bool = %q, want empty", got)
	}

	// The snapshot is a copy: mutating it must not reach the execution.
	snapshot := execution.MetadataSnapshot()
	snapshot["name"] = "tampered"
	delete(snapshot, "remote")
	if got := execution.metadataString("name"); got != "kandev" {
		t.Fatalf("snapshot mutation leaked into execution: name = %q", got)
	}
	if !execution.metadataBool("remote") {
		t.Fatal("snapshot deletion leaked into execution")
	}

	execution.deleteMetadataValues("name", "absent")
	if _, ok := execution.metadataValue("name"); ok {
		t.Fatal("deleteMetadataValues did not remove the key")
	}
	if !execution.metadataBool("remote") {
		t.Fatal("deleteMetadataValues removed an unrelated key")
	}

	// Every helper tolerates a nil receiver, which several call sites rely on.
	var nilExecution *AgentExecution
	nilExecution.setMetadataValue("name", "kandev")
	nilExecution.deleteMetadataValues("name")
	if _, ok := nilExecution.metadataValue("name"); ok {
		t.Fatal("metadataValue on nil execution reported a value")
	}
	if got := nilExecution.MetadataSnapshot(); got != nil {
		t.Fatalf("MetadataSnapshot on nil execution = %v, want nil", got)
	}
}
