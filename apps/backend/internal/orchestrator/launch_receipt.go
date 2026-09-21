package orchestrator

import "encoding/json"

// LaunchTriState avoids inferring lifecycle stages from provider error prose.
type LaunchTriState string

const (
	LaunchTriStateUnknown LaunchTriState = "unknown"
	LaunchTriStateFalse   LaunchTriState = "false"
	LaunchTriStateTrue    LaunchTriState = "true"
)

type LaunchAttemptIdentity struct {
	SessionID   string `json:"session_id"`
	Incarnation string `json:"incarnation,omitempty"`
	Generation  uint64 `json:"generation,omitempty"`
}

func (i LaunchAttemptIdentity) equal(other LaunchAttemptIdentity) bool {
	return i.SessionID == other.SessionID && i.Incarnation == other.Incarnation && i.Generation == other.Generation
}

type LaunchFactKind string

const (
	LaunchFactProcessStarted                   LaunchFactKind = "process_started"
	LaunchFactInferenceStarted                 LaunchFactKind = "inference_started"
	LaunchFactTerminalPreflightFailure         LaunchFactKind = "terminal_preflight_failure"
	LaunchFactTerminalACPInitializationFailure LaunchFactKind = "terminal_acp_initialization_failure"
)

type LaunchReceiptFact struct {
	Identity LaunchAttemptIdentity `json:"identity"`
	Kind     LaunchFactKind        `json:"kind"`
}

type LaunchReceipt struct {
	Identity                   LaunchAttemptIdentity `json:"identity"`
	CatalogAttachmentAttemptID string                `json:"catalog_attachment_attempt_id,omitempty"`
	ProcessCreated             LaunchTriState        `json:"process_created"`
	InferenceStarted           LaunchTriState        `json:"inference_started"`
	Facts                      []LaunchReceiptFact   `json:"facts,omitempty"`
}

type LaunchReceiptHistory struct {
	Current  LaunchReceipt   `json:"current"`
	Previous []LaunchReceipt `json:"previous,omitempty"`
}

func loadLaunchReceiptHistory(raw interface{}) (LaunchReceiptHistory, bool) {
	if history, ok := raw.(LaunchReceiptHistory); ok {
		return history, history.Current.Identity.SessionID != ""
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return LaunchReceiptHistory{}, false
	}
	var history LaunchReceiptHistory
	if err := json.Unmarshal(encoded, &history); err != nil || history.Current.Identity.SessionID == "" {
		return LaunchReceiptHistory{}, false
	}
	return history, true
}

func (h *LaunchReceiptHistory) Start(identity LaunchAttemptIdentity) {
	if identity.SessionID == "" || h.Current.Identity.equal(identity) {
		return
	}
	if h.Current.Identity.SessionID != "" {
		h.Previous = append([]LaunchReceipt{h.Current}, h.Previous...)
		if len(h.Previous) > 2 {
			h.Previous = h.Previous[:2]
		}
	}
	h.Current = LaunchReceipt{Identity: identity, ProcessCreated: LaunchTriStateUnknown, InferenceStarted: LaunchTriStateUnknown}
}

func (h *LaunchReceiptHistory) Apply(fact LaunchReceiptFact) bool {
	if !h.Current.Identity.equal(fact.Identity) {
		return false
	}
	for _, existing := range h.Current.Facts {
		if existing.Kind == fact.Kind {
			return true
		}
	}
	h.Current.Facts = append(h.Current.Facts, fact)
	switch fact.Kind {
	case LaunchFactProcessStarted:
		h.Current.ProcessCreated = LaunchTriStateTrue
	case LaunchFactInferenceStarted:
		h.Current.InferenceStarted = LaunchTriStateTrue
	case LaunchFactTerminalPreflightFailure:
		h.Current.ProcessCreated = LaunchTriStateFalse
		h.Current.InferenceStarted = LaunchTriStateFalse
	case LaunchFactTerminalACPInitializationFailure:
		h.Current.InferenceStarted = LaunchTriStateFalse
	}
	return true
}
