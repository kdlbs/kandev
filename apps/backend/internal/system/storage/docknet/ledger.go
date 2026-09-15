package docknet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/system/storage"
)

// LedgerStore is the persisted first-seen/quarantine substrate.
type LedgerStore interface {
	GetNetworkLedgerEntry(ctx context.Context, networkID string) (storage.NetworkLedgerEntry, error)
	UpsertNetworkLedgerObserved(
		ctx context.Context, networkID, networkName string, at time.Time,
	) (storage.NetworkLedgerEntry, error)
	TransitionNetworkLedgerEntry(
		ctx context.Context, networkID string, next storage.NetworkLedgerState,
		markedAt, deleteAfter time.Time, metadata json.RawMessage, lastError string,
	) (storage.NetworkLedgerEntry, error)
	RecordNetworkLedgerRemoved(
		ctx context.Context, networkID, networkName string, at time.Time, runID string,
	) (storage.NetworkLedgerEntry, error)
	ListNetworkLedgerEntries(ctx context.Context) ([]storage.NetworkLedgerEntry, error)
}

// Ledger is the two-phase quarantine ledger: observed -> marked (grace) ->
// removed. Rollback semantics: cancelling a pending mark never un-deletes,
// because removals are terminal rows.
type Ledger struct {
	store LedgerStore
	now   func() time.Time
}

// LedgerConfig wires the ledger; Now defaults to time.Now.
type LedgerConfig struct {
	Store LedgerStore
	Now   func() time.Time
}

func NewLedger(config LedgerConfig) *Ledger {
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &Ledger{store: config.Store, now: now}
}

// ObserveSighting records a census sighting and returns the effective
// first-seen time for classification. The very first sighting fixes
// FirstSeenAt; later sightings only advance LastSeenAt.
func (l *Ledger) ObserveSighting(
	ctx context.Context, networkID, networkName string,
) (storage.NetworkLedgerEntry, error) {
	return l.store.UpsertNetworkLedgerObserved(ctx, networkID, networkName, l.now())
}

// Mark quarantine-marks an eligible network: the removal intent plus its
// delete-after deadline are persisted before any removal attempt. A network
// already carrying an unexpired mark keeps that mark's original timestamps:
// re-classification never resets the quarantine window (AC8), so a due network
// cannot be trapped in an endless re-mark loop.
func (l *Ledger) Mark(
	ctx context.Context, networkID, networkName string, quarantineWindow time.Duration, metadata json.RawMessage,
) (storage.NetworkLedgerEntry, error) {
	current, err := l.store.GetNetworkLedgerEntry(ctx, networkID)
	if err == nil && current.State == storage.NetworkLedgerStateMarked && current.DeleteAfter != nil {
		if !l.now().Before(*current.DeleteAfter) {
			return current, nil // mark matured: keep the original due deadline
		}
		return current, nil // window still running: original mark stands
	}
	if err != nil && !isNotFound(err) {
		return storage.NetworkLedgerEntry{}, err
	}
	at := l.now().UTC()
	deleteAfter := at.Add(quarantineWindow)
	return l.store.TransitionNetworkLedgerEntry(
		ctx, networkID, storage.NetworkLedgerStateMarked, at, deleteAfter, metadata, "",
	)
}

func isNotFound(err error) bool {
	return errors.Is(err, storage.ErrNotFound)
}

// RecordRemoved records a successful per-network SDK removal. Terminal: a
// rollback of the feature never resurrects the row to observed.
func (l *Ledger) RecordRemoved(
	ctx context.Context, networkID, networkName, runID string,
) (storage.NetworkLedgerEntry, error) {
	return l.store.RecordNetworkLedgerRemoved(ctx, networkID, networkName, l.now().UTC(), runID)
}

// RecordSkipped aborts a removal before the SDK call (revalidation found an
// attachment, lease lost, or the census no longer lists the network) and
// returns the network to observation so the next cycle re-classifies it.
func (l *Ledger) RecordSkipped(
	ctx context.Context, networkID, lastError string,
) (storage.NetworkLedgerEntry, error) {
	return l.store.TransitionNetworkLedgerEntry(
		ctx, networkID, storage.NetworkLedgerStateSkipped, time.Time{}, time.Time{}, nil, lastError,
	)
}

// Entries lists the active ledger rows.
func (l *Ledger) Entries(ctx context.Context) ([]storage.NetworkLedgerEntry, error) {
	return l.store.ListNetworkLedgerEntries(ctx)
}

// QuarantineDeadline reports the delete-after deadline of a marked row.
func QuarantineDeadline(entry storage.NetworkLedgerEntry) (time.Time, error) {
	if entry.State != storage.NetworkLedgerStateMarked || entry.DeleteAfter == nil {
		return time.Time{}, fmt.Errorf("network %s is not quarantine-marked", entry.NetworkID)
	}
	return *entry.DeleteAfter, nil
}
