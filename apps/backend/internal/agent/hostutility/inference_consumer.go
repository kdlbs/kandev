package hostutility

import (
	"context"
	"fmt"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
)

const inferenceOwnerHost = "host"

func (m *Manager) SetInferenceConsumer(consumer agentruntime.InferenceConsumer) {
	m.inferenceConsumer = consumer
}

func (m *Manager) executeOwnedInference(ctx context.Context, inst *instance, req *utility.PromptRequest, profileID string) (*utility.PromptResponse, error) {
	if m.inferenceConsumer == nil {
		return inst.client.InferencePromptOwned(ctx, req)
	}
	identity := agentruntime.InferenceIdentityFromContext(ctx)
	if identity.LogicalProfileID == "" {
		identity.LogicalProfileID = profileID
	}
	if profileID != "" {
		identity.ExecutionProfileID = profileID
	}
	identity.OwnerKind = inferenceOwnerHost
	identity.ExecutorType = "standalone"
	return m.inferenceConsumer.Execute(ctx, inst.client, req, identity)
}

// InferenceClients reads native instance inventory, including pre-policy children
// whose earlier backend worker is no longer in this manager's process map.
func (m *Manager) InferenceClients(ctx context.Context) ([]agentruntime.InferenceClientRef, error) {
	if m.runtimeOwner == nil {
		return m.cachedInferenceClients(), fmt.Errorf("inference inventory owner unavailable")
	}
	lease, err := m.runtimeOwner.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer lease.Close()
	control := lease.NewControlClient(m.log)
	defer control.Close()
	instances, err := control.ListInstances(lease.Context())
	if err != nil {
		return nil, err
	}
	var refs []agentruntime.InferenceClientRef
	for _, info := range instances {
		if info == nil || info.ID == "" || info.Port <= 0 {
			continue
		}
		client := lease.NewBoundInstanceClient(info.Port, m.log, agentruntime.WithInferenceExecutionID(info.ID))
		kind := inferenceOwnerHost
		if info.SessionID != "" {
			kind = "session"
		}
		refs = append(refs, agentruntime.InferenceClientRef{Client: client, Release: client.Close, Identity: agentruntime.InferenceIdentity{TaskID: info.TaskID, SessionID: info.SessionID, ParentExecutionID: info.ID, OwnerKind: kind, ExecutorType: "standalone"}})
	}
	if err := lease.CheckCurrent(); err != nil {
		for _, ref := range refs {
			ref.Release()
		}
		return nil, fmt.Errorf("inference runtime owner changed: %w", err)
	}
	return refs, nil
}

func (m *Manager) cachedInferenceClients() []agentruntime.InferenceClientRef {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var refs []agentruntime.InferenceClientRef
	for _, inst := range m.instances {
		if inst != nil && inst.client != nil {
			refs = append(refs, agentruntime.InferenceClientRef{Client: inst.client, Release: func() {}, Identity: agentruntime.InferenceIdentity{OwnerKind: inferenceOwnerHost, ExecutorType: "standalone"}})
		}
	}
	return refs
}
