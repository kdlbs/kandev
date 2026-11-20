package lifecycle

import "fmt"

const (
	DeliveryRecoveryRefusalLiveWork               = "live_work"
	DeliveryRecoveryRefusalPeerRecordMissing      = "peer_record_missing"
	DeliveryRecoveryRefusalPeerRecordMismatch     = "peer_record_mismatch"
	DeliveryRecoveryRefusalGenerationSuperseded   = "generation_superseded"
	DeliveryRecoveryRefusalNativeIdentityMismatch = "native_identity_mismatch"
	DeliveryRecoveryRefusalJournalUnresolved      = "journal_unresolved"
	DeliveryRecoveryRefusalProjectionPending      = "projection_pending"
	DeliveryRecoveryRefusalExecutionUnavailable   = "execution_unavailable"
	DeliveryRecoveryRefusalEvidenceUnavailable    = "evidence_unavailable"
	DeliveryRecoveryRefusalOfficeOwned            = "office_owned"
	DeliveryRecoveryRefusalSQLResolutionRefused   = "sql_resolution_refused"
	DeliveryRecoveryRefusalUnclassified           = "unclassified"
)

var deliveryRecoveryRefusalReasons = map[string]struct{}{
	DeliveryRecoveryRefusalLiveWork:               {},
	DeliveryRecoveryRefusalPeerRecordMissing:      {},
	DeliveryRecoveryRefusalPeerRecordMismatch:     {},
	DeliveryRecoveryRefusalGenerationSuperseded:   {},
	DeliveryRecoveryRefusalNativeIdentityMismatch: {},
	DeliveryRecoveryRefusalJournalUnresolved:      {},
	DeliveryRecoveryRefusalProjectionPending:      {},
	DeliveryRecoveryRefusalExecutionUnavailable:   {},
	DeliveryRecoveryRefusalEvidenceUnavailable:    {},
	DeliveryRecoveryRefusalOfficeOwned:            {},
	DeliveryRecoveryRefusalSQLResolutionRefused:   {},
	DeliveryRecoveryRefusalUnclassified:           {},
}

// IsDeliveryRecoveryRefusalReason reports whether reason belongs to the closed log set.
func IsDeliveryRecoveryRefusalReason(reason string) bool {
	_, ok := deliveryRecoveryRefusalReasons[reason]
	return ok
}

type deliveryRecoveryRefusalError struct {
	reason string
	cause  error
}

func (e *deliveryRecoveryRefusalError) Error() string {
	return fmt.Sprintf("delivery recovery refused (%s): %v", e.reason, e.cause)
}

func (e *deliveryRecoveryRefusalError) Unwrap() error { return e.cause }

func (e *deliveryRecoveryRefusalError) DeliveryRecoveryRefusalReason() string { return e.reason }

func withDeliveryRecoveryRefusal(reason string, cause error) error {
	if cause == nil {
		return nil
	}
	if !IsDeliveryRecoveryRefusalReason(reason) {
		reason = DeliveryRecoveryRefusalUnclassified
	}
	return &deliveryRecoveryRefusalError{reason: reason, cause: cause}
}
