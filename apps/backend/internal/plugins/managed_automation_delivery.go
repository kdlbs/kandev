package plugins

import (
	"context"
	"errors"
	"fmt"
	"sort"

	pluginstore "github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const managedConversationWriteCapability = "host.v2.write:managed_agent_conversations"

type managedScheduleApprovalLockContextKey struct{}

func withManagedScheduleApprovalLock(ctx context.Context, installationID string) context.Context {
	return context.WithValue(ctx, managedScheduleApprovalLockContextKey{}, installationID)
}

func managedScheduleApprovalLockHeld(ctx context.Context) bool {
	installationID, ok := ctx.Value(managedScheduleApprovalLockContextKey{}).(string)
	return ok && installationID != ""
}

// ManagedConversationDestinationOption is the workspace-safe projection used
// by the native automation editor. It deliberately excludes installation,
// task, and session identifiers.
type ManagedConversationDestinationOption struct {
	PluginID          string `json:"plugin_id"`
	PluginName        string `json:"plugin_name"`
	InstanceKey       string `json:"instance_key"`
	Revision          uint64 `json:"revision"`
	Paused            bool   `json:"paused"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
}

// ListManagedConversationDestinations returns retained conversations in one
// workspace, including unavailable entries so existing schedules can be
// repaired explicitly after a plugin is disabled or its approval is revoked.
func (s *Service) ListManagedConversationDestinations(
	ctx context.Context, workspaceID string,
) ([]ManagedConversationDestinationOption, error) {
	if workspaceID == "" {
		return nil, errors.New("plugins: workspace is required")
	}
	managed := s.managedAgentConversationDeps()
	if managed == nil {
		return []ManagedConversationDestinationOption{}, nil
	}
	var result []ManagedConversationDestinationOption
	for _, record := range s.registry.List() {
		if record.InstallationID == "" {
			continue
		}
		conversations, err := managed.ListManaged(ctx, record.InstallationID, workspaceID)
		if err != nil {
			return nil, fmt.Errorf("list managed conversations for plugin %q: %w", record.ID, err)
		}
		for _, conversation := range conversations {
			if conversation.Detached || conversation.InstanceKey == "" {
				continue
			}
			option := ManagedConversationDestinationOption{
				PluginID: record.ID, PluginName: record.DisplayName,
				InstanceKey: conversation.InstanceKey, Revision: conversation.Revision,
				Paused: conversation.DesiredPaused,
			}
			switch {
			case record.Status != StatusActive:
				option.UnavailableReason = "plugin_inactive"
			default:
				_, _, _, err := s.authorizeManagedConversationTarget(
					workspaceID, record.ID, managedConversationWriteCapability,
					"EnqueueManagedAgentInputExact",
					CanonicalApprovalDigest("automation-destination-list", workspaceID, conversation.InstanceKey),
				)
				if err != nil {
					option.UnavailableReason = status.Convert(err).Message()
				}
			}
			result = append(result, option)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].PluginID != result[j].PluginID {
			return result[i].PluginID < result[j].PluginID
		}
		return result[i].InstanceKey < result[j].InstanceKey
	})
	return result, nil
}

// ResolveManagedConversationDestination validates the current installation,
// workspace grant, and retained conversation before an automation binds it.
func (s *Service) ResolveManagedConversationDestination(
	ctx context.Context, workspaceID, pluginID, instanceKey string, expectedRevision uint64,
) (string, string, bool, error) {
	if !managedScheduleApprovalLockHeld(ctx) {
		s.approvalEffectMu.Lock()
		defer s.approvalEffectMu.Unlock()
	}
	record, _, _, err := s.authorizeManagedConversationTarget(workspaceID, pluginID,
		managedConversationWriteCapability, "EnqueueManagedAgentInputExact", CanonicalApprovalDigest("automation-destination", workspaceID, instanceKey))
	if err != nil {
		return "", "", false, err
	}
	managed := s.managedAgentConversationDeps()
	if managed == nil {
		return "", "", false, status.Error(codes.Unavailable, "managed conversation service is unavailable")
	}
	descriptor, err := managed.GetManaged(ctx, record.InstallationID, workspaceID, instanceKey)
	if err != nil {
		return "", "", false, err
	}
	if descriptor.TaskID == "" || descriptor.WorkspaceID != workspaceID || descriptor.InstanceKey != instanceKey {
		return "", "", false, status.Error(codes.PermissionDenied, "managed conversation destination is unavailable")
	}
	if expectedRevision > descriptor.Revision {
		return "", "", false, status.Error(codes.Aborted, "managed conversation destination revision changed")
	}
	return record.InstallationID, descriptor.TaskID, descriptor.DesiredPaused, nil
}

// EnqueueManagedAutomationInput is a host-owned delivery path. It repeats the
// destination authorization while holding the approval effect lease through
// durable input admission, and the occurrence key remains stable across retry.
func (s *Service) EnqueueManagedAutomationInput(
	ctx context.Context, workspaceID, pluginID, instanceKey string, destinationInstallationID,
	destinationConversationID string, occurrenceKey, payload string,
) (pluginsdk.ManagedAgentInputReceipt, bool, error) {
	s.approvalEffectMu.Lock()
	defer s.approvalEffectMu.Unlock()
	requestID := CanonicalApprovalDigest("managed-automation-request", occurrenceKey)
	idempotencyKey := CanonicalApprovalDigest("managed-automation-idempotency", occurrenceKey)
	requestDigest := CanonicalApprovalDigest("managed-automation-delivery", workspaceID, pluginID, instanceKey, occurrenceKey, payload)
	record, approval, manifestDigest, err := s.authorizeManagedConversationTarget(workspaceID, pluginID,
		managedConversationWriteCapability, "EnqueueManagedAgentInputExact", requestDigest)
	if err != nil {
		return pluginsdk.ManagedAgentInputReceipt{}, false, err
	}
	if record.InstallationID != destinationInstallationID {
		return pluginsdk.ManagedAgentInputReceipt{}, false, status.Error(codes.PermissionDenied, "managed conversation destination changed")
	}
	managed := s.managedAgentConversationDeps()
	inputs, ok := managed.(ManagedAgentInputService)
	if !ok {
		return pluginsdk.ManagedAgentInputReceipt{}, false, status.Error(codes.Unavailable, "managed conversation input service is unavailable")
	}
	descriptor, err := managed.GetManaged(ctx, record.InstallationID, workspaceID, instanceKey)
	if err != nil {
		return pluginsdk.ManagedAgentInputReceipt{}, false, err
	}
	if descriptor.TaskID == "" || (destinationConversationID != "" && descriptor.TaskID != destinationConversationID) {
		return pluginsdk.ManagedAgentInputReceipt{}, false, status.Error(codes.PermissionDenied, "managed conversation identity changed")
	}
	input := pluginsdk.ManagedAgentInputEnqueue{
		RequestID: requestID, IdempotencyKey: idempotencyKey, WorkspaceID: workspaceID,
		InstanceKey: instanceKey, ExpectedConversationRevision: descriptor.Revision,
		ApprovalRevision: approval.Revision, ManifestDigest: manifestDigest,
		OccurrenceKey: occurrenceKey, Origin: pluginsdk.ManagedAgentInputAutomation, Payload: payload,
	}
	digest, err := managedInputEnqueueDigest(input)
	if err != nil {
		return pluginsdk.ManagedAgentInputReceipt{}, false, status.Error(codes.InvalidArgument, "managed automation input is invalid")
	}
	inputID := managedInputID(record.InstallationID, workspaceID, instanceKey, occurrenceKey)
	operationID := CanonicalApprovalDigest("managed-automation-input", record.InstallationID, workspaceID, instanceKey, occurrenceKey)
	receipt, _, err := inputs.EnqueueManagedInput(ctx, record.InstallationID, inputID, input, operationID, digest)
	if err != nil {
		return pluginsdk.ManagedAgentInputReceipt{}, false, err
	}
	if receipt.HostInputID != inputID {
		return pluginsdk.ManagedAgentInputReceipt{}, false, errors.New("managed automation input identity mismatch")
	}
	return receipt, descriptor.DesiredPaused, nil
}

func (s *Service) ReadManagedAutomationInput(
	ctx context.Context, workspaceID, pluginID, instanceKey, destinationInstallationID,
	destinationConversationID, inputID string,
) (pluginsdk.ManagedAgentInputReceipt, bool, error) {
	s.approvalEffectMu.Lock()
	defer s.approvalEffectMu.Unlock()
	requestDigest := CanonicalApprovalDigest("managed-automation-read", workspaceID, pluginID, instanceKey, inputID)
	record, approval, manifestDigest, err := s.authorizeManagedConversationTarget(workspaceID, pluginID,
		"host.v2.read:managed_agent_conversations", "GetManagedAgentInputExact", requestDigest)
	if err != nil {
		return pluginsdk.ManagedAgentInputReceipt{}, false, err
	}
	if record.InstallationID != destinationInstallationID {
		return pluginsdk.ManagedAgentInputReceipt{}, false, status.Error(codes.PermissionDenied, "managed conversation destination changed")
	}
	managed := s.managedAgentConversationDeps()
	inputs, ok := managed.(ManagedAgentInputService)
	if !ok {
		return pluginsdk.ManagedAgentInputReceipt{}, false, status.Error(codes.Unavailable, "managed conversation input service is unavailable")
	}
	descriptor, err := managed.GetManaged(ctx, record.InstallationID, workspaceID, instanceKey)
	if err != nil {
		return pluginsdk.ManagedAgentInputReceipt{}, false, err
	}
	if destinationConversationID != "" && descriptor.TaskID != destinationConversationID {
		return pluginsdk.ManagedAgentInputReceipt{}, false, status.Error(codes.PermissionDenied, "managed conversation identity changed")
	}
	receipt, err := inputs.GetManagedInput(ctx, record.InstallationID, pluginsdk.ManagedAgentInputQuery{
		WorkspaceID: workspaceID, InstanceKey: instanceKey, HostInputID: inputID,
		ApprovalRevision: approval.Revision, ManifestDigest: manifestDigest,
	})
	return receipt, descriptor.DesiredPaused, err
}

func (s *Service) authorizeManagedConversationTarget(
	workspaceID, pluginID, capability, method, requestDigest string,
) (*pluginstore.Record, CapabilityApproval, string, error) {
	record, err := s.Get(pluginID)
	if err != nil || record == nil || record.InstallationID == "" || record.Status != StatusActive {
		return nil, CapabilityApproval{}, "", status.Error(codes.NotFound, "managed conversation plugin is unavailable")
	}
	manifestDigest := ManifestCapabilityDigest(record.Manifest)
	approval, found, err := s.approvalCurrent(record.InstallationID, workspaceID)
	if err != nil {
		return nil, CapabilityApproval{}, "", status.Error(codes.Unavailable, "managed conversation approval is unavailable")
	}
	if !found {
		return nil, CapabilityApproval{}, "", status.Error(codes.PermissionDenied, string(ApprovalDenyMissingApproval))
	}
	decision := s.authorizePluginCapability(record.InstallationID, workspaceID, capability, approval.Revision,
		requestDigest, CanonicalApprovalDigest("host-method", method, "v2"))
	if !decision.Allowed {
		return nil, CapabilityApproval{}, "", status.Error(codes.PermissionDenied, string(decision.Reason))
	}
	return record, approval, manifestDigest, nil
}
