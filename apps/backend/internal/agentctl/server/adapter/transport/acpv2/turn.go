package acpv2

type foregroundTurn struct {
	active      bool
	userMessage string
	messageID   string
	terminal    bool
	stopReason  string
}

func (t *foregroundTurn) observeState(state, reason string) {
	if t.terminal {
		return
	}
	switch state {
	case "running", "requires_action":
		t.active = true
	case "idle":
		if t.active || (reason != "" && (t.messageID != "" || t.userMessage != "")) {
			t.terminal = true
			t.stopReason = reason
		}
	}
}

func (t *foregroundTurn) acknowledge(messageID string) { t.messageID = messageID }

func (t *foregroundTurn) settled() bool { return t.terminal && t.messageID != "" }
