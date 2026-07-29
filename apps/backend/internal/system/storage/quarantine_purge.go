package storage

type QuarantinePurgeScope string

const (
	QuarantinePurgeScopeEligible QuarantinePurgeScope = "eligible"
	QuarantinePurgeScopeAll      QuarantinePurgeScope = "all"

	QuarantineConfirmationDelete   = "DELETE"
	QuarantineConfirmationEligible = "DELETE ELIGIBLE"
	QuarantineConfirmationForce    = "DELETE ALL NOW"
)

type QuarantinePurgeFailure struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

type QuarantinePurgeResult struct {
	Scope          QuarantinePurgeScope     `json:"scope"`
	Considered     int                      `json:"considered"`
	Deleted        int                      `json:"deleted"`
	DeletedBytes   int64                    `json:"deleted_bytes"`
	Protected      int                      `json:"protected"`
	ProtectedBytes int64                    `json:"protected_bytes"`
	Failed         int                      `json:"failed"`
	FailedBytes    int64                    `json:"failed_bytes"`
	Failures       []QuarantinePurgeFailure `json:"failures,omitempty"`
}
