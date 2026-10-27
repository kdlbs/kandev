package nomad

import (
	"errors"
	"strings"
)

const readmitOperatorMaxLen = 128

var (
	// ErrNomadReadmitInvalid reports an incomplete request or an attestation
	// that does not affirm every required fact.
	ErrNomadReadmitInvalid = errors.New("nomad readmission request is invalid")
	// ErrNomadReadmitNotFound reports that no retained row matches the exact identity.
	ErrNomadReadmitNotFound = errors.New("nomad readmission target was not found")
	// ErrNomadReadmitBusy reports that another operation holds the environment claim; retry later.
	ErrNomadReadmitBusy = errors.New("nomad readmission target is busy")
	// ErrNomadReadmitPhase reports that the matching row is not cleanup_pending or quarantined.
	ErrNomadReadmitPhase = errors.New("nomad readmission target is not in a releasable phase")
)

// NomadReadmitRequest names one exact retained inventory row and the
// operator's manual attestation that its runtime and fence are gone. NodeID is
// empty only for a pool row that was never placed on a node; the exact-row
// match rejects an empty NodeID against any placed row.
type NomadReadmitRequest struct {
	NodeID        string
	EnvironmentID string
	Generation    int64
	JobID         string
	Attestation   OperatorAttestation
}

// Validate rejects an incomplete identity or attestation.
func (r NomadReadmitRequest) Validate() error {
	operator := strings.TrimSpace(r.Attestation.Operator)
	if r.EnvironmentID == "" || r.Generation < 1 || r.JobID == "" ||
		!r.Attestation.Complete() || len(operator) > readmitOperatorMaxLen || strings.ContainsFunc(operator, isControlRune) {
		return ErrNomadReadmitInvalid
	}
	return nil
}

func isControlRune(r rune) bool { return r < 0x20 || r == 0x7f }
