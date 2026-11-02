package service

import (
	"github.com/kandev/kandev/internal/task/quickchatidentity"
)

var ErrQuickChatCreationIDInvalid = quickchatidentity.ErrInvalidCreationID

// NewQuickChatCreationExternalID binds a canonical creation UUID to the
// authenticated principal without storing the principal itself.
func NewQuickChatCreationExternalID(principalID, creationID string) (string, error) {
	return quickchatidentity.NewExternalID(principalID, creationID)
}

// QuickChatCreationIDForPrincipal returns the token only when the external
// identity belongs to principalID and ends in a canonical creation UUID.
func QuickChatCreationIDForPrincipal(principalID, externalID string) (string, bool) {
	return quickchatidentity.CreationIDForPrincipal(principalID, externalID)
}

func QuickChatCreationIDFromExternalID(externalID string) string {
	creationID, ok := quickchatidentity.CreationIDFromExternalID(externalID)
	if !ok {
		return ""
	}
	return creationID
}
