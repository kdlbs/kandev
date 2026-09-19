package plugins

import (
	"context"
	"errors"
)

var (
	ErrConversationBindingInvalid       = errors.New("invalid conversation binding")
	ErrConversationGenerationSuperseded = errors.New("conversation generation superseded")
)

// ManagedConversationIdentity is the non-secret identity retained by a live
// subscription. It is re-resolved before every delivery.
type ManagedConversationIdentity struct {
	PluginID    string
	UserID      string
	Generation  int64
	WorkspaceID string
	TaskID      string
	SessionID   string
}

// AuthorizeConversationConsumer validates the short-lived plugin binding used
// by the live source subscription.
func (s *Service) AuthorizeConversationConsumer(
	pluginID, userID string, generation int64, bindingToken string,
) error {
	record, err := s.Get(pluginID)
	if err != nil || record.Status != StatusActive || !record.Capabilities.CanRead("messages") {
		return ErrConversationBindingInvalid
	}
	if conversationGeneration(record.InstalledAt) != generation {
		return ErrConversationGenerationSuperseded
	}
	claims, err := s.conversationTokens.parse(bindingToken)
	if err != nil || claims.Kind != tokenKindBinding || claims.PluginID != pluginID ||
		claims.UserID != userID || claims.Generation != generation {
		return ErrConversationBindingInvalid
	}
	return nil
}

func (s *Service) AuthorizeManagedConversationConsumer(
	ctx context.Context, pluginID, userID string, generation int64, bindingToken, managedToken, taskID, sessionID string,
) (ManagedConversationIdentity, error) {
	identity := ManagedConversationIdentity{}
	record, err := s.Get(pluginID)
	if err != nil || record.Status != StatusActive || !record.Capabilities.AgentConversation {
		return identity, ErrConversationBindingInvalid
	}
	if conversationGeneration(record.InstalledAt) != generation {
		return identity, ErrConversationGenerationSuperseded
	}
	if !s.validConversationBindingToken(pluginID, userID, generation, bindingToken) {
		return identity, ErrConversationBindingInvalid
	}
	identity, ok := s.managedConversationIdentityFromToken(pluginID, userID, generation, managedToken, taskID, sessionID)
	if !ok {
		return identity, ErrConversationBindingInvalid
	}
	if !s.ValidateManagedConversationIdentity(ctx, identity) {
		return ManagedConversationIdentity{}, ErrConversationBindingInvalid
	}
	return identity, nil
}

func (s *Service) validConversationBindingToken(pluginID, userID string, generation int64, bindingToken string) bool {
	claims, err := s.conversationTokens.parse(bindingToken)
	return err == nil && claims.Kind == tokenKindBinding && claims.PluginID == pluginID &&
		claims.UserID == userID && claims.Generation == generation
}

func (s *Service) managedConversationIdentityFromToken(pluginID, userID string, generation int64, managedToken, taskID, sessionID string) (ManagedConversationIdentity, bool) {
	claims, err := s.conversationTokens.parse(managedToken)
	if err != nil || claims.Kind != tokenKindManaged || claims.PluginID != pluginID || claims.UserID != userID ||
		claims.Generation != generation || claims.TaskID == nil || claims.WorkspaceID == "" ||
		claims.SessionID != sessionID || *claims.TaskID != taskID {
		return ManagedConversationIdentity{}, false
	}
	return ManagedConversationIdentity{
		PluginID: pluginID, UserID: userID, Generation: generation,
		WorkspaceID: claims.WorkspaceID, TaskID: taskID, SessionID: sessionID,
	}, true
}

func (s *Service) ValidateManagedConversationIdentity(ctx context.Context, identity ManagedConversationIdentity) bool {
	record, err := s.Get(identity.PluginID)
	if err != nil || record.Status != StatusActive || !record.Capabilities.AgentConversation ||
		conversationGeneration(record.InstalledAt) != identity.Generation {
		return false
	}
	bridge, ok := s.agentConversationDeps().(managedConversationResolver)
	if !ok || bridge == nil {
		return false
	}
	descriptor, err := bridge.ResolveManagedConversation(ctx, identity.PluginID, identity.WorkspaceID, identity.SessionID)
	return err == nil && descriptor.TaskID == identity.TaskID && descriptor.SessionID == identity.SessionID &&
		descriptor.WorkspaceID == identity.WorkspaceID
}
