package journal

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	p "github.com/kandev/kandev/internal/mcp/profile"
	bolt "go.etcd.io/bbolt"
)

type ProviderReplacementPhase string

const (
	ProviderReplacementPrepared ProviderReplacementPhase = "prepared"
	ProviderReplacementRetiring ProviderReplacementPhase = "retiring"
	ProviderReplacementRetired  ProviderReplacementPhase = "retired"
	ProviderReplacementStarting ProviderReplacementPhase = "starting"
	ProviderReplacementStarted  ProviderReplacementPhase = "started"
	ProviderReplacementAccepted ProviderReplacementPhase = "accepted"
	ProviderReplacementFailed   ProviderReplacementPhase = "failed"
	ProviderReplacementReleased ProviderReplacementPhase = "released"
)

var bucketProviderReplacements = []byte("provider_replacements")
var keyActiveProviderReplacement = []byte("active")
var ErrProviderReplacementConflict = errors.New("provider replacement operation conflicts with retained state")

// ProviderReplacementReceipt records operation ownership before provider effects.
// Status responses must use a separate projection if private configuration is added.
type ProviderReplacementReceipt struct {
	PreviousOperationID     string                   `json:"previous_operation_id,omitempty"`
	OperationID             string                   `json:"operation_id"`
	SessionID               string                   `json:"session_id"`
	ExecutionID             string                   `json:"execution_id"`
	HelperID                string                   `json:"helper_id"`
	SourceProcessGeneration uint64                   `json:"source_process_generation"`
	SourceConfigRevision    uint64                   `json:"source_config_revision"`
	SourceStopGeneration    uint64                   `json:"source_stop_generation"`
	SourceTaskID            string                   `json:"source_task_id"`
	SourceProfileDigest     string                   `json:"source_profile_digest"`
	SourceBindingID         string                   `json:"source_binding_id"`
	SourceIncarnationID     string                   `json:"source_incarnation_id"`
	SourceStreamID          string                   `json:"source_stream_id"`
	SourceHarnessGeneration uint64                   `json:"source_harness_generation"`
	RequestHash             string                   `json:"request_hash"`
	Phase                   ProviderReplacementPhase `json:"phase"`
	Version                 uint64                   `json:"version"`
	ExpiresAt               time.Time                `json:"expires_at"`
	TargetScope             p.ProviderMCPScope       `json:"target_scope"`
	TargetProfile           string                   `json:"target_profile"`
	ActivationHash          string                   `json:"activation_hash,omitempty"`
	TargetCredential        string                   `json:"target_credential,omitempty"`
	Catalog                 map[string][]string      `json:"catalog,omitempty"`
	TargetStreamID          string                   `json:"target_stream_id"`
	NativeSessionID         string                   `json:"native_session_id"`
	SourceTerminated        bool                     `json:"source_terminated"`
	NativeResumeConfirmed   bool                     `json:"native_resume_confirmed"`
	FailureCode             string                   `json:"failure_code,omitempty"`
}

func (j *Journal) ReadProviderReplacement(ctx context.Context, id string) (ProviderReplacementReceipt, error) {
	var receipt ProviderReplacementReceipt
	if j == nil {
		return receipt, ErrJournalClosed
	}
	j.mu.RLock()
	defer j.mu.RUnlock()
	err := j.viewLocked(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var err error
		receipt, err = readProviderReceiptTx(tx, id)
		return err
	})
	return receipt, err
}

func (j *Journal) CreateProviderReplacement(ctx context.Context, receipt ProviderReplacementReceipt) (ProviderReplacementReceipt, error) {
	if err := validateProviderReceiptIntent(receipt); err != nil {
		return ProviderReplacementReceipt{}, err
	}
	if receipt.Phase != ProviderReplacementPrepared || receipt.Version != 0 || receipt.ExpiresAt.IsZero() || !validProviderReceiptEvidence(receipt) {
		return ProviderReplacementReceipt{}, ErrProviderReplacementConflict
	}
	return j.writeProviderReplacement(ctx, receipt, 0, true)
}

func (j *Journal) UpdateProviderReplacement(ctx context.Context, version uint64, receipt ProviderReplacementReceipt) (ProviderReplacementReceipt, error) {
	if version == 0 || receipt.Version != version {
		return ProviderReplacementReceipt{}, ErrProviderReplacementConflict
	}
	if err := validateProviderReceiptIntent(receipt); err != nil {
		return ProviderReplacementReceipt{}, err
	}
	return j.writeProviderReplacement(ctx, receipt, version, false)
}

func (j *Journal) writeProviderReplacement(ctx context.Context, receipt ProviderReplacementReceipt, version uint64, create bool) (ProviderReplacementReceipt, error) {
	if j == nil {
		return ProviderReplacementReceipt{}, ErrJournalClosed
	}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return ProviderReplacementReceipt{}, err
	}
	if len(encoded) > 64<<10 {
		return ProviderReplacementReceipt{}, ErrJournalFull
	}
	j.mu.RLock()
	defer j.mu.RUnlock()
	db, err := j.dbLocked()
	if err != nil {
		return ProviderReplacementReceipt{}, err
	}
	reservation, err := j.checkDiskCapacity(ctx, db, int64(len(encoded)+128), !create)
	if err != nil {
		return ProviderReplacementReceipt{}, err
	}
	var result ProviderReplacementReceipt
	err = j.updateLocked(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var err error
		result, err = j.putProviderReceiptTx(tx, receipt, version, create)
		return err
	})
	j.finishDiskCapacityReservation(reservation, err == nil)
	return result, err
}

func (j *Journal) putProviderReceiptTx(tx *bolt.Tx, next ProviderReplacementReceipt, version uint64, create bool) (ProviderReplacementReceipt, error) {
	bucket, err := tx.CreateBucketIfNotExists(bucketProviderReplacements)
	if err != nil {
		return next, err
	}
	current, err := readProviderReceiptTx(tx, next.OperationID)
	if err != nil {
		return next, err
	}
	replay, err := validateProviderReceiptWrite(bucket, current, next, version, create)
	if err != nil {
		return next, err
	}
	if replay {
		return current, nil
	}
	next.Version = current.Version + 1
	return next, j.storeProviderReceiptTx(tx, bucket, next)
}

func validateProviderReceiptWrite(bucket *bolt.Bucket, current, next ProviderReplacementReceipt, version uint64, create bool) (bool, error) {
	if create && current.OperationID != "" {
		return true, providerReceiptReplayError(current, next)
	}
	if create {
		if len(bucket.Get(keyActiveProviderReplacement)) != 0 || bucket.Stats().KeyN >= 2048 {
			return false, ErrProviderReplacementConflict
		}
		return false, nil
	}
	if current.OperationID == "" || current.Version != version || !validProviderReceiptUpdate(current, next) || string(bucket.Get(keyActiveProviderReplacement)) != next.OperationID {
		return false, ErrProviderReplacementConflict
	}
	return false, nil
}

func providerReceiptReplayError(current, next ProviderReplacementReceipt) error {
	if !sameProviderReceiptIntent(current, next) {
		return ErrProviderReplacementConflict
	}
	return nil
}

func (j *Journal) storeProviderReceiptTx(tx *bolt.Tx, bucket *bolt.Bucket, next ProviderReplacementReceipt) error {
	encoded, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if len(encoded) > 64<<10 {
		return ErrJournalFull
	}
	oldBytes := bucket.Get(providerReceiptKey(next.OperationID))
	total, err := decodeInt64(tx.Bucket(bucketMeta).Get(keyJournalBytes))
	if err != nil {
		return err
	}
	delta := int64(len(encoded) - len(oldBytes))
	if exceedsLimit(total, delta, j.config.MaxJournalBytes) {
		return ErrJournalFull
	}
	if err := bucket.Put(providerReceiptKey(next.OperationID), encoded); err != nil {
		return err
	}
	if err := tx.Bucket(bucketMeta).Put(keyJournalBytes, encodeInt64(total+delta)); err != nil {
		return err
	}
	if err := recordProviderReaderRequirement(tx, bucket, next); err != nil {
		return err
	}
	if providerReceiptTerminal(next.Phase) {
		err = bucket.Delete(keyActiveProviderReplacement)
	} else {
		err = bucket.Put(keyActiveProviderReplacement, []byte(next.OperationID))
	}
	return err
}

func providerReceiptKey(id string) []byte { return []byte("operation/" + id) }
func readProviderReceiptTx(tx *bolt.Tx, id string) (ProviderReplacementReceipt, error) {
	var receipt ProviderReplacementReceipt
	bucket := tx.Bucket(bucketProviderReplacements)
	if bucket == nil {
		return receipt, nil
	}
	raw := bucket.Get(providerReceiptKey(id))
	if raw == nil {
		return receipt, nil
	}
	if json.Unmarshal(raw, &receipt) != nil {
		return receipt, ErrJournalCorrupt
	}
	if receipt.OperationID != id || receipt.Version == 0 || validateProviderReceiptIntent(receipt) != nil || !validProviderReceiptEvidence(receipt) {
		return receipt, ErrJournalCorrupt
	}
	return receipt, nil
}

func validateProviderReceiptIntent(r ProviderReplacementReceipt) error {
	if !validProviderReceiptPhase(r.Phase) || r.OperationID == "" || len(r.OperationID) > 128 || strings.TrimSpace(r.OperationID) != r.OperationID || r.SessionID == "" || r.ExecutionID == "" || r.HelperID == "" || r.SourceProcessGeneration == 0 || len(r.RequestHash) != 64 {
		return ErrProviderReplacementConflict
	}
	if _, err := hex.DecodeString(r.RequestHash); err != nil {
		return ErrProviderReplacementConflict
	}
	return nil
}

func sameProviderReceiptIntent(a, b ProviderReplacementReceipt) bool {
	return a.PreviousOperationID == b.PreviousOperationID && a.OperationID == b.OperationID && a.RequestHash == b.RequestHash && sameProviderReceiptSource(a, b)
}
func sameProviderReceiptSource(a, b ProviderReplacementReceipt) bool {
	return a.SourceTaskID == b.SourceTaskID && a.SourceIncarnationID == b.SourceIncarnationID && a.SourceProfileDigest == b.SourceProfileDigest && a.NativeSessionID == b.NativeSessionID && a.SessionID == b.SessionID && a.ExecutionID == b.ExecutionID && a.HelperID == b.HelperID && a.SourceProcessGeneration == b.SourceProcessGeneration && a.SourceConfigRevision == b.SourceConfigRevision && a.SourceStopGeneration == b.SourceStopGeneration && a.SourceBindingID == b.SourceBindingID && a.SourceStreamID == b.SourceStreamID && a.SourceHarnessGeneration == b.SourceHarnessGeneration
}

func providerReceiptTerminal(p ProviderReplacementPhase) bool {
	return p == ProviderReplacementAccepted || p == ProviderReplacementFailed || p == ProviderReplacementReleased
}

func validProviderReceiptTransition(from, to ProviderReplacementPhase) bool {
	if providerReceiptTerminal(from) {
		return false
	}
	if to == ProviderReplacementFailed {
		return true
	}
	switch from {
	case ProviderReplacementPrepared:
		return to == ProviderReplacementRetiring || to == ProviderReplacementReleased
	case ProviderReplacementRetiring:
		return to == ProviderReplacementRetired
	case ProviderReplacementRetired:
		return to == ProviderReplacementRetired || to == ProviderReplacementStarting
	case ProviderReplacementStarting:
		return to == ProviderReplacementStarted
	case ProviderReplacementStarted:
		return to == ProviderReplacementAccepted
	}
	return false
}

func validProviderReceiptPhase(phase ProviderReplacementPhase) bool {
	switch phase {
	case ProviderReplacementPrepared, ProviderReplacementRetiring, ProviderReplacementRetired, ProviderReplacementStarting, ProviderReplacementStarted, ProviderReplacementAccepted, ProviderReplacementFailed, ProviderReplacementReleased:
		return true
	}
	return false
}

func validProviderReceiptUpdate(current, next ProviderReplacementReceipt) bool {
	if !sameProviderReceiptIntent(current, next) || !current.ExpiresAt.Equal(next.ExpiresAt) || current.NativeSessionID != next.NativeSessionID || !validProviderReceiptTransition(current.Phase, next.Phase) {
		return false
	}
	if current.Phase != ProviderReplacementRetired && (!sameProviderReceiptTarget(current, next)) {
		return false
	}
	if current.SourceTerminated && !next.SourceTerminated || current.NativeResumeConfirmed && !next.NativeResumeConfirmed {
		return false
	}
	if current.Phase != ProviderReplacementStarting && !reflect.DeepEqual(current.Catalog, next.Catalog) {
		return false
	}
	return validProviderReceiptEvidence(next)
}

func validProviderReceiptEvidence(receipt ProviderReplacementReceipt) bool {
	switch receipt.Phase {
	case ProviderReplacementPrepared, ProviderReplacementRetiring, ProviderReplacementReleased:
		return !receipt.SourceTerminated && !receipt.NativeResumeConfirmed
	case ProviderReplacementRetired:
		return receipt.SourceTerminated && !receipt.NativeResumeConfirmed
	case ProviderReplacementStarting:
		return receipt.SourceTerminated && !receipt.NativeResumeConfirmed && validProviderReceiptTarget(receipt)
	case ProviderReplacementStarted, ProviderReplacementAccepted:
		return validProviderReceiptConfirmation(receipt)
	case ProviderReplacementFailed:
		return !receipt.NativeResumeConfirmed || validProviderReceiptConfirmation(receipt)
	}
	return false
}

func validProviderReceiptTarget(receipt ProviderReplacementReceipt) bool {
	binding, _, err := p.ParseProviderMCPBinding(p.ProviderMCPBinding{Scope: receipt.TargetScope, Profile: receipt.TargetProfile})
	if err != nil {
		return false
	}
	if len(receipt.ActivationHash) != 64 || len(receipt.TargetCredential) < 32 || len(receipt.TargetCredential) > 256 {
		return false
	}
	if _, err := hex.DecodeString(receipt.ActivationHash); err != nil {
		return false
	}
	return binding.Scope.ExecutionID == receipt.ExecutionID && binding.Scope.HelperID == receipt.HelperID && binding.Scope.ProcessGeneration > receipt.SourceProcessGeneration && binding.Scope.HarnessGeneration == receipt.SourceHarnessGeneration && receipt.NativeSessionID != "" && receipt.TargetStreamID != "" && receipt.TargetStreamID == receipt.SourceStreamID
}

func sameProviderReceiptTarget(a, b ProviderReplacementReceipt) bool {
	return a.TargetScope == b.TargetScope && a.TargetProfile == b.TargetProfile && a.TargetStreamID == b.TargetStreamID && a.TargetCredential == b.TargetCredential && a.ActivationHash == b.ActivationHash
}
func validProviderReceiptConfirmation(r ProviderReplacementReceipt) bool {
	return r.SourceTerminated && r.NativeResumeConfirmed && len(r.Catalog) > 0 && validProviderReceiptTarget(r)
}
