package plugins

import (
	"errors"
	"time"
)

var (
	ErrConversationBindingInvalid       = errors.New("invalid conversation binding")
	ErrConversationGenerationSuperseded = errors.New("conversation generation superseded")
)

// SessionEvents returns the service-owned durable stream store used by the
// production WebSocket gateway.
func (s *Service) SessionEvents() *SessionEventLog {
	return s.sessionEvents
}

// SessionDelivery returns the service-owned poison delivery dispatcher.
func (s *Service) SessionDelivery() *SessionDeliveryDispatcher {
	return s.sessionDelivery
}

// AuthorizeConversationConsumer validates an active capability declaration and
// the loader-issued user/plugin/generation binding without exposing token data.
func (s *Service) AuthorizeConversationConsumer(
	pluginID string,
	userID string,
	generation int64,
	bindingToken string,
) error {
	record, err := s.Get(pluginID)
	if err != nil || record.Status != StatusActive || !record.Capabilities.CanRead("messages") {
		return ErrConversationBindingInvalid
	}
	if conversationGeneration(record.InstalledAt) != generation {
		return ErrConversationGenerationSuperseded
	}
	claims, err := s.conversationTokens.parse(bindingToken)
	if err != nil ||
		claims.Kind != tokenKindBinding ||
		claims.PluginID != pluginID ||
		claims.UserID != userID ||
		claims.Generation != generation {
		return ErrConversationBindingInvalid
	}
	return nil
}

func (s *Service) MintSessionStreamGrant(
	pluginID string,
	userID string,
	generation int64,
	sessionID string,
	consumerID string,
	wireID string,
	cutoff uint64,
) (snapshotToken string, resumeToken string, expiresAt time.Time, err error) {
	fingerprint := consumerID
	if fingerprint == "" {
		fingerprint = wireID
	}
	snapshotToken, err = s.conversationTokens.mintSnapshot(
		pluginID,
		userID,
		generation,
		sessionID,
		nil,
		"",
		nil,
		int64(cutoff),
		fingerprint,
	)
	if err != nil {
		return "", "", time.Time{}, err
	}
	resumeToken, err = s.conversationTokens.mintResume(
		pluginID,
		userID,
		generation,
		sessionID,
		consumerID,
		wireID,
		cutoff,
	)
	if err != nil {
		return "", "", time.Time{}, err
	}
	return snapshotToken, resumeToken, time.Now().UTC().Add(conversationTokenTTL), nil
}

func (s *Service) ValidateSessionResume(
	token string,
	key SessionDeliveryCursorKey,
	sequence uint64,
) error {
	if token == "" {
		return ErrConversationBindingInvalid
	}
	claims, err := s.conversationTokens.parse(token)
	if err != nil ||
		claims.Kind != tokenKindResume ||
		claims.PluginID != key.PluginID ||
		claims.UserID != key.UserID ||
		claims.Generation != key.Generation ||
		claims.SessionID != key.SessionID ||
		claims.ConsumerID != key.ConsumerID ||
		claims.WireID != key.WireID ||
		claims.Sequence > sequence {
		return ErrConversationBindingInvalid
	}
	return nil
}
