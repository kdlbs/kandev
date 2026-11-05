package quickchatidentity

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/google/uuid"
)

const ExternalIDPrefix = "quick-chat-create:"

var ErrInvalidCreationID = errors.New("invalid quick chat creation_id")

func NewExternalID(principalID, creationID string) (string, error) {
	if strings.TrimSpace(principalID) == "" {
		return "", ErrInvalidCreationID
	}
	parsed, err := uuid.Parse(creationID)
	if err != nil || parsed.String() != creationID {
		return "", ErrInvalidCreationID
	}
	principalHash := sha256.Sum256([]byte(principalID))
	return ExternalIDPrefix + "v1:" + hex.EncodeToString(principalHash[:]) + ":" + creationID, nil
}

func CreationIDForPrincipal(principalID, externalID string) (string, bool) {
	if strings.TrimSpace(principalID) == "" {
		return "", false
	}
	principalHash := sha256.Sum256([]byte(principalID))
	prefix := ExternalIDPrefix + "v1:" + hex.EncodeToString(principalHash[:]) + ":"
	if !strings.HasPrefix(externalID, prefix) {
		return "", false
	}
	return parseCreationID(strings.TrimPrefix(externalID, prefix))
}

func CreationIDFromExternalID(externalID string) (string, bool) {
	if !strings.HasPrefix(externalID, ExternalIDPrefix+"v1:") {
		return "", false
	}
	rest := strings.TrimPrefix(externalID, ExternalIDPrefix+"v1:")
	principalHash, creationID, found := strings.Cut(rest, ":")
	if !found || len(principalHash) != sha256.Size*2 {
		return "", false
	}
	if _, err := hex.DecodeString(principalHash); err != nil {
		return "", false
	}
	return parseCreationID(creationID)
}

func parseCreationID(value string) (string, bool) {
	parsed, err := uuid.Parse(value)
	if err != nil || parsed.String() != value {
		return "", false
	}
	return value, true
}
