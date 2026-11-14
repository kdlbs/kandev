package client

import (
	"encoding/json"
	"errors"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// StorageDiagnosticError carries validated host storage evidence returned by
// an agentctl admission or prompt rejection. Older peers never produce it.
type StorageDiagnosticError struct {
	Message    string
	Diagnostic *streams.DeliveryStorageDiagnostic
}

func (e *StorageDiagnosticError) Error() string {
	if e == nil {
		return "prompt failed: storage unavailable"
	}
	return "prompt failed: " + e.Message
}

// StorageDiagnosticFromError returns typed storage evidence from an agentctl
// rejection without inspecting provider text.
func StorageDiagnosticFromError(err error) (*streams.DeliveryStorageDiagnostic, bool) {
	var storageErr *StorageDiagnosticError
	if err == nil || !errors.As(err, &storageErr) || storageErr.Diagnostic == nil {
		return nil, false
	}
	return storageErr.Diagnostic, true
}

// promptStorageError accepts only the closed wire type from an authenticated
// prompt rejection. Malformed or foreign details stay absent.
func promptStorageError(payload ws.ErrorPayload) error {
	raw, ok := payload.Details["storage"]
	if !ok || raw == nil {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var diagnostic streams.DeliveryStorageDiagnostic
	if err := json.Unmarshal(encoded, &diagnostic); err != nil {
		return nil
	}
	normalized := diagnostic.Normalize()
	if normalized == nil {
		return nil
	}
	return &StorageDiagnosticError{Message: payload.Message, Diagnostic: normalized}
}
