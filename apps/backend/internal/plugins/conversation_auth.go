package plugins

import "errors"

var (
	ErrConversationBindingInvalid       = errors.New("invalid conversation binding")
	ErrConversationGenerationSuperseded = errors.New("conversation generation superseded")
)

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
