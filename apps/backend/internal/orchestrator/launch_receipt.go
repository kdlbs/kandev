package orchestrator

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
	LaunchFactProcessStarted           LaunchFactKind = "process_started"
	LaunchFactInferenceStarted         LaunchFactKind = "inference_started"
	LaunchFactTerminalPreflightFailure LaunchFactKind = "terminal_preflight_failure"
)

type LaunchReceiptFact struct {
	Identity LaunchAttemptIdentity `json:"identity"`
	Kind     LaunchFactKind        `json:"kind"`
}

type LaunchReceipt struct {
	Identity         LaunchAttemptIdentity `json:"identity"`
	ProcessCreated   LaunchTriState        `json:"process_created"`
	InferenceStarted LaunchTriState        `json:"inference_started"`
	Facts            []LaunchReceiptFact   `json:"facts,omitempty"`
}

type LaunchReceiptHistory struct {
	Current  LaunchReceipt   `json:"current"`
	Previous []LaunchReceipt `json:"previous,omitempty"`
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
	}
	return true
}
