package sandbox

const (
	auditStateInvalid             = "invalid"
	auditStateUnchecked           = "unchecked"
	auditStateValid               = "valid"
	auditStateVerified            = "verified"
	auditStateUnsigned            = "unsigned"
	auditStateUntrusted           = "untrusted"
	reportClassNetwork            = "network"
	reportClassCommand            = "command"
	reportClassEndpoint           = "http_endpoint"
	reportTransportTCP            = "tcp"
	reportDecisionAllowed         = "allowed"
	reportIncompleteIdentity      = "incomplete_identity"
	reportReasonOther             = "other"
	reportStatusUnavailable       = "unavailable"
	reportFinalizationFinal       = "final"
	reportFinalizationInterrupted = "interrupted"
)
