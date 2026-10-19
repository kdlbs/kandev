package process

import (
	"context"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

const shellDeliveryComplete = "complete"

type shellDeliveryKey struct {
	Session    string
	Stream     string
	Tool       string
	Parent     string
	Submission string
	Generation uint64
	Shell      streams.ShellExecPayload
}

type shellDeliveryBaseline struct {
	State  *streams.ShellOutputState
	Status string
	Count  int
	Bytes  int
}

func (m *Manager) persistDeliveryBatch(ctx context.Context, updates []adapter.AgentEvent) ([]adapter.AgentEvent, error) {
	if m.cfg == nil || m.cfg.DurableJournalPath == "" {
		return m.persistDeliveryBatchRaw(ctx, updates)
	}
	m.shellDeliveryMu.Lock()
	defer m.shellDeliveryMu.Unlock()
	next := make(map[shellDeliveryKey]shellDeliveryBaseline, len(m.shellDelivery))
	for key, value := range m.shellDelivery {
		next[key] = value
	}
	encoded := make([]adapter.AgentEvent, 0, len(updates))
	positions := make([]int, 0, len(updates))
	for index, update := range updates {
		value, err := m.prepareShellDelivery(update, next)
		if err != nil {
			return updates, err
		}
		if value.Type != "" {
			encoded = append(encoded, value)
			positions = append(positions, index)
		}
	}
	results := make([]adapter.AgentEvent, len(updates))
	if len(encoded) > 0 {
		committed, err := m.persistDeliveryBatchRaw(ctx, encoded)
		if err != nil {
			return updates, err
		}
		for index, position := range positions {
			results[position] = committed[index]
		}
	}
	m.shellDelivery = next
	return results, nil
}

func (m *Manager) prepareShellDelivery(update adapter.AgentEvent, baselines map[shellDeliveryKey]shellDeliveryBaseline) (adapter.AgentEvent, error) {
	clearShellDeliveryGeneration(update, baselines)
	shell := compactShellPayload(update)
	if shell == nil {
		clearOrdinaryShellDelivery(update, baselines)
		return update, nil
	}
	identity := *shell
	identity.Output = nil
	key := shellDeliveryKey{Session: update.SessionID, Stream: m.DeliveryStreamID(), Tool: update.ToolCallID, Parent: update.ParentToolCallID, Submission: m.deliverySubmissionIDForEvent(update), Generation: update.PromptGeneration, Shell: identity}
	baseline, exists := baselines[key]
	if !update.ShellOutputEligible && !exists {
		return update, nil
	}
	if !exists {
		clearSupersededShellDelivery(baselines, key)
		baseline.State = &streams.ShellOutputState{Epoch: uuid.NewString()}
	}
	patch, state, err := prepareShellOutputPatch(update, baseline, exists)
	if err != nil {
		return update, err
	}
	if patch == nil {
		return adapter.AgentEvent{}, nil
	}
	update = compactShellDeliveryEvent(update, patch)
	baseline = advanceShellDeliveryBaseline(baseline, state, patch, update.ToolStatus)
	baselines[key] = baseline
	if shellDeliveryTerminal(update.ToolStatus) {
		delete(baselines, key)
	}
	return update, nil
}

func clearOrdinaryShellDelivery(update adapter.AgentEvent, baselines map[shellDeliveryKey]shellDeliveryBaseline) {
	if update.Type != streams.EventTypeToolUpdate || update.NormalizedPayload == nil {
		return
	}
	for key := range baselines {
		if key.Session == update.SessionID && key.Tool == update.ToolCallID && key.Generation == update.PromptGeneration {
			delete(baselines, key)
		}
	}
}

func compactShellPayload(update adapter.AgentEvent) *streams.ShellExecPayload {
	if update.Type != streams.EventTypeToolUpdate || update.NormalizedPayload == nil {
		return nil
	}
	shell := update.NormalizedPayload.ShellExec()
	if shell == nil || shell.Background || update.NormalizedPayload.BackgroundWork() != nil || shell.Output == nil {
		return nil
	}
	return shell
}

func prepareShellOutputPatch(update adapter.AgentEvent, baseline shellDeliveryBaseline, exists bool) (*streams.ShellOutputPatch, *streams.ShellOutputState, error) {
	base := baseline.State
	if !exists {
		base = nil
	}
	output := *update.NormalizedPayload.ShellExec().Output
	terminal := shellDeliveryTerminal(update.ToolStatus)
	patch, err := streams.EncodeShellOutputPatch(base, output, baseline.State.Epoch, false)
	if err != nil {
		return nil, nil, err
	}
	if unchangedShellDelivery(update, baseline, patch, terminal) {
		return nil, nil, nil
	}
	checkpoint := !exists || terminal || (patch != nil && (baseline.Count >= 512 || baseline.Bytes >= 2*streams.MaxShellOutputFieldBytes))
	patch, err = ensureShellOutputCheckpoint(base, output, baseline.State.Epoch, patch, checkpoint)
	if err != nil {
		return nil, nil, err
	}
	state, err := streams.ApplyShellOutputPatch(base, patch)
	return patch, state, err
}

func compactShellDeliveryEvent(update adapter.AgentEvent, patch *streams.ShellOutputPatch) adapter.AgentEvent {
	update.Type = streams.EventTypeShellOutputUpdate
	update.ShellOutput = patch
	update.ShellOutputHasContent = len(update.ToolCallContents) > 0
	update.NormalizedPayload = update.NormalizedPayload.Snapshot()
	update.NormalizedPayload.ShellExec().Output = nil
	if update.ShellOutputEligible {
		update.ToolCallContents = nil
	}
	return update
}

func advanceShellDeliveryBaseline(baseline shellDeliveryBaseline, state *streams.ShellOutputState, patch *streams.ShellOutputPatch, status string) shellDeliveryBaseline {
	if patch.Checkpoint {
		baseline.Count = 0
		baseline.Bytes = 0
	} else {
		if patch.Stdout != nil {
			baseline.Bytes += len(patch.Stdout.Data)
		}
		if patch.Stderr != nil {
			baseline.Bytes += len(patch.Stderr.Data)
		}
	}
	if patch.BaseRevision == 0 || patch.BaseDigest != patch.ResultDigest {
		baseline.Count++
	}
	baseline.State = state
	baseline.Status = status
	return baseline
}

func shellDeliveryTerminal(status string) bool {
	switch status {
	case shellDeliveryComplete, "completed", "success", "error", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func clearShellDeliveryGeneration(update adapter.AgentEvent, baselines map[shellDeliveryKey]shellDeliveryBaseline) {
	if update.Type != streams.EventTypeComplete && update.Type != streams.EventTypeError {
		return
	}
	for key := range baselines {
		if key.Generation == update.PromptGeneration {
			delete(baselines, key)
		}
	}
}

func clearSupersededShellDelivery(baselines map[shellDeliveryKey]shellDeliveryBaseline, key shellDeliveryKey) {
	for old := range baselines {
		if old.Session == key.Session && old.Stream == key.Stream && old.Tool == key.Tool && old.Generation == key.Generation {
			delete(baselines, old)
		}
	}
}

func unchangedShellDelivery(update adapter.AgentEvent, baseline shellDeliveryBaseline, patch *streams.ShellOutputPatch, terminal bool) bool {
	return patch == nil && update.ShellOutputEligible && baseline.Status == update.ToolStatus && !terminal
}

func ensureShellOutputCheckpoint(base *streams.ShellOutputState, output streams.ShellExecOutput, epoch string, patch *streams.ShellOutputPatch, checkpoint bool) (*streams.ShellOutputPatch, error) {
	if patch != nil && (!checkpoint || patch.Checkpoint) {
		return patch, nil
	}
	full, err := streams.EncodeShellOutputPatch(base, output, epoch, true)
	if err != nil {
		return nil, err
	}
	if !checkpoint {
		full.Checkpoint = false
		full.Stdout = nil
		full.Stderr = nil
	}
	return full, nil
}
