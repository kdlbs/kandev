package process

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

func TestIncrementalShellSlowStorageBound(t *testing.T) {
	manager := newIncrementalShellManager(t)
	output := strings.Repeat("x", streams.MaxShellOutputFieldBytes)
	var state *streams.ShellOutputState
	var bytes int
	for index := range 7200 {
		text := fmt.Sprintf("%016d", index)
		output = output[len(text):] + text
		normalized := streams.NewShellExec("test-command", "/tmp", "", 0, false)
		normalized.ShellExec().Output = &streams.ShellExecOutput{Stdout: output, Truncated: true, StdoutTruncated: true}
		update := adapter.AgentEvent{Type: streams.EventTypeToolUpdate, SessionID: "session", ToolCallID: "shell", ToolStatus: "in_progress", NormalizedPayload: normalized, ShellOutputEligible: true}
		committed, err := manager.persistDeliveryEvent(update)
		require.NoError(t, err)
		encoded, err := json.Marshal(committed)
		require.NoError(t, err)
		bytes += len(encoded)
		if index == 1 {
			require.Less(t, len(encoded), 2048, "slow output must not admit the complete saturated tail again")
		}
		require.NotNil(t, committed.ShellOutput)
		state, err = streams.ApplyShellOutputPatch(state, committed.ShellOutput)
		require.NoError(t, err)
	}
	require.True(t, output == state.Output.Stdout)
	require.Less(t, bytes, 32<<20)
	t.Logf("new_output_bytes=%d serialized_bytes=%d", 7200*16, bytes)
}

func newIncrementalShellManager(t *testing.T) *Manager {
	t.Helper()
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	store, err := journal.Open(journal.Config{Path: path})
	require.NoError(t, err)
	manager := &Manager{cfg: &config.InstanceConfig{DurableJournalPath: path, SessionID: "session", DeliveryStreamID: "stream", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 1}, logger: newTestLogger(t), deliveryJournal: store}
	t.Cleanup(func() { require.NoError(t, manager.closeDeliveryJournal()) })
	return manager
}

func TestIncrementalShellNoopAndTerminal(t *testing.T) {
	manager := newIncrementalShellManager(t)
	normalized := streams.NewShellExec("cmd", "", "", 0, false)
	normalized.ShellExec().Output = &streams.ShellExecOutput{Stdout: "retained"}
	update := adapter.AgentEvent{Type: streams.EventTypeToolUpdate, SessionID: "session", ToolCallID: "tool", ToolStatus: "in_progress", NormalizedPayload: normalized, ShellOutputEligible: true}
	first, err := manager.persistDeliveryEvent(update)
	require.NoError(t, err)
	noop, err := manager.persistDeliveryEvent(update)
	require.NoError(t, err)
	require.Empty(t, noop.Type)
	update.ToolStatus = "complete"
	code := 4
	normalized.ShellExec().Output = &streams.ShellExecOutput{Stdout: "authoritative", ExitCode: &code}
	last, err := manager.persistDeliveryEvent(update)
	require.NoError(t, err)
	require.Equal(t, first.DeliverySequence+1, last.DeliverySequence)
	require.True(t, last.ShellOutput.Checkpoint)
	state, err := streams.ApplyShellOutputPatch(nil, last.ShellOutput)
	require.NoError(t, err)
	require.Equal(t, "authoritative", state.Output.Stdout)
	require.Equal(t, &code, state.Output.ExitCode)
}

func TestIncrementalShellNoopAtCheckpointBoundary(t *testing.T) {
	manager := newIncrementalShellManager(t)
	normalized := streams.NewShellExec("cmd", "", "", 0, false)
	update := adapter.AgentEvent{Type: streams.EventTypeToolUpdate, SessionID: "session", ToolCallID: "tool", ToolStatus: "in_progress", NormalizedPayload: normalized, ShellOutputEligible: true}
	for index := range 512 {
		normalized.ShellExec().Output = &streams.ShellExecOutput{Stdout: fmt.Sprint(index)}
		_, err := manager.persistDeliveryEvent(update)
		require.NoError(t, err)
	}
	noop, err := manager.persistDeliveryEvent(update)
	require.NoError(t, err)
	require.Empty(t, noop.Type, "checkpoint progress cannot turn unchanged output into a full-tail write")
}

func TestIncrementalShellInterleavedStorageBound(t *testing.T) {
	manager := newIncrementalShellManager(t)
	outputs := map[string]string{"first": strings.Repeat("a", streams.MaxShellOutputFieldBytes), "second": strings.Repeat("b", streams.MaxShellOutputFieldBytes)}
	states := map[string]*streams.ShellOutputState{}
	checkpoints, serialized := 0, 0
	for index := range 800 {
		id := "first"
		if index%2 != 0 {
			id = "second"
		}
		delta := fmt.Sprintf("%016d", index)
		outputs[id] = outputs[id][len(delta):] + delta
		normalized := streams.NewShellExec(id, "", "", 0, false)
		normalized.ShellExec().Output = &streams.ShellExecOutput{Stdout: outputs[id], Truncated: true, StdoutTruncated: true}
		event, err := manager.persistDeliveryEvent(adapter.AgentEvent{Type: streams.EventTypeToolUpdate, SessionID: "session", ToolCallID: id, ToolStatus: "in_progress", NormalizedPayload: normalized, ShellOutputEligible: true})
		require.NoError(t, err)
		if event.ShellOutput.Checkpoint {
			checkpoints++
		}
		states[id], err = streams.ApplyShellOutputPatch(states[id], event.ShellOutput)
		require.NoError(t, err)
		payload, err := json.Marshal(event)
		require.NoError(t, err)
		serialized += len(payload)
		barrier, err := manager.persistDeliveryEvent(adapter.AgentEvent{Type: streams.EventTypeReasoning, ReasoningText: "ordered barrier"})
		require.NoError(t, err)
		require.Equal(t, event.DeliverySequence+1, barrier.DeliverySequence)
	}
	require.Equal(t, 2, checkpoints, "tool switches and ordered barriers must preserve accepted baselines")
	require.Less(t, serialized, 8<<20)
	for id, output := range outputs {
		require.True(t, output == states[id].Output.Stdout)
	}
}

func TestIncrementalShellTwoSaturatedFieldsDoNotReseed(t *testing.T) {
	manager := newIncrementalShellManager(t)
	baselines := make(map[shellDeliveryKey]shellDeliveryBaseline)
	output := streams.ShellExecOutput{Stdout: strings.Repeat("x", streams.MaxShellOutputFieldBytes), Stderr: strings.Repeat("y", streams.MaxShellOutputFieldBytes)}
	serialized := 0
	for index := range 1100 {
		delta := fmt.Sprintf("%016d", index)
		output.Stdout = output.Stdout[16:] + delta
		output.Stderr = output.Stderr[16:] + delta
		normalized := streams.NewShellExec("cmd", "", "", 0, false)
		normalized.ShellExec().Output = &output
		event, err := manager.prepareShellDelivery(adapter.AgentEvent{Type: streams.EventTypeToolUpdate, ToolCallID: "tool", ToolStatus: "in_progress", NormalizedPayload: normalized, ShellOutputEligible: true}, baselines)
		require.NoError(t, err)
		require.Equal(t, index%512 == 0, event.ShellOutput.Checkpoint, "checkpoint text cannot count as fresh progress toward the next checkpoint")
		encoded, err := json.Marshal(event)
		require.NoError(t, err)
		serialized += len(encoded)
	}
	require.Less(t, serialized, 4<<20)
}

func TestIncrementalShellOrdinaryTransitionResetsBaseline(t *testing.T) {
	manager := newIncrementalShellManager(t)
	baselines := make(map[shellDeliveryKey]shellDeliveryBaseline)
	normalized := streams.NewShellExec("cmd", "", "", 0, false)
	normalized.ShellExec().Output = &streams.ShellExecOutput{Stdout: "foreground"}
	update := adapter.AgentEvent{Type: streams.EventTypeToolUpdate, ToolCallID: "tool", ToolStatus: "in_progress", NormalizedPayload: normalized, ShellOutputEligible: true}
	first, err := manager.prepareShellDelivery(update, baselines)
	require.NoError(t, err)
	require.NotNil(t, first.ShellOutput)
	background := normalized.Snapshot()
	background.ShellExec().Background = true
	background.ShellExec().Output = &streams.ShellExecOutput{Stdout: "background"}
	ordinary := update
	ordinary.NormalizedPayload, ordinary.ShellOutputEligible = background, false
	result, err := manager.prepareShellDelivery(ordinary, baselines)
	require.NoError(t, err)
	require.Equal(t, streams.EventTypeToolUpdate, result.Type)
	require.Empty(t, baselines)
	next, err := manager.prepareShellDelivery(update, baselines)
	require.NoError(t, err)
	require.Zero(t, next.ShellOutput.BaseRevision)
	require.NotEqual(t, first.ShellOutput.Epoch, next.ShellOutput.Epoch)
}

func TestIncrementalShellMetadataDoesNotAdvanceCheckpoint(t *testing.T) {
	manager := newIncrementalShellManager(t)
	baselines := make(map[shellDeliveryKey]shellDeliveryBaseline)
	normalized := streams.NewShellExec("cmd", "", "", 0, false)
	normalized.ShellExec().Output = &streams.ShellExecOutput{Stdout: strings.Repeat("x", streams.MaxShellOutputFieldBytes)}
	update := adapter.AgentEvent{Type: streams.EventTypeToolUpdate, ToolCallID: "tool", ToolStatus: "in_progress", NormalizedPayload: normalized, ShellOutputEligible: true}
	_, err := manager.prepareShellDelivery(update, baselines)
	require.NoError(t, err)
	update.ShellOutputEligible = false
	for index := range 620 {
		update.ToolTitle = fmt.Sprintf("metadata %d", index)
		event, prepareErr := manager.prepareShellDelivery(update, baselines)
		require.NoError(t, prepareErr)
		require.False(t, event.ShellOutput.Checkpoint)
		require.Nil(t, event.ShellOutput.Stdout)
	}
	for _, baseline := range baselines {
		require.Equal(t, 1, baseline.Count)
	}
	code := 7
	normalized.ShellExec().Output.ExitCode = &code
	_, err = manager.prepareShellDelivery(update, baselines)
	require.NoError(t, err)
	for _, baseline := range baselines {
		require.Equal(t, 2, baseline.Count, "result metadata changes count even without text edits")
	}
}
