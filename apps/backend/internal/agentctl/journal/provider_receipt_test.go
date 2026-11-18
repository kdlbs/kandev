package journal

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

func providerReceiptFixture(id string) ProviderReplacementReceipt {
	return ProviderReplacementReceipt{OperationID: id, SessionID: "session", ExecutionID: "execution", HelperID: "helper", SourceProcessGeneration: 1, SourceConfigRevision: 1, RequestHash: strings.Repeat("a", 64), Phase: ProviderReplacementPrepared, ExpiresAt: time.Now().Add(30 * time.Second).UTC()}
}

func TestMCPProviderReplacementReceiptReplayAndCAS(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	j, err := Open(Config{Path: path})
	require.NoError(t, err)
	defer func() { _ = j.Close() }()
	first := providerReceiptFixture("first")
	stored, err := j.CreateProviderReplacement(ctx, first)
	require.NoError(t, err)
	require.Equal(t, uint64(1), stored.Version)
	replayedInput := first
	replayedInput.ExpiresAt = replayedInput.ExpiresAt.Add(time.Second)
	duplicate, err := j.CreateProviderReplacement(ctx, replayedInput)
	require.NoError(t, err)
	require.Equal(t, stored, duplicate)
	other := providerReceiptFixture("other")
	_, err = j.CreateProviderReplacement(ctx, other)
	require.Error(t, err, "overlapping operations cannot own the same helper")
	next := stored
	next.Phase = ProviderReplacementRetiring
	next, err = j.UpdateProviderReplacement(ctx, stored.Version, next)
	require.NoError(t, err)
	require.NoError(t, j.Close())
	j, err = Open(Config{Path: path})
	require.NoError(t, err)
	recovered, err := j.ReadProviderReplacement(ctx, "first")
	require.NoError(t, err)
	require.Equal(t, next, recovered)
	_, err = j.UpdateProviderReplacement(ctx, stored.Version, stored)
	require.Error(t, err, "stale writers cannot reopen a retired phase")
	replay, err := j.CreateProviderReplacement(ctx, first)
	require.NoError(t, err)
	require.Equal(t, next, replay)
	forged := first
	forged.RequestHash = strings.Repeat("b", 64)
	_, err = j.CreateProviderReplacement(ctx, forged)
	require.Error(t, err)
}

func TestMCPProviderReplacementReceiptTerminalDoesNotReplay(t *testing.T) {
	ctx := context.Background()
	j, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	require.NoError(t, err)
	defer func() { _ = j.Close() }()
	initial := providerReceiptFixture("first")
	stored, err := j.CreateProviderReplacement(ctx, initial)
	require.NoError(t, err)
	stored.Phase = ProviderReplacementReleased
	stored, err = j.UpdateProviderReplacement(ctx, stored.Version, stored)
	require.NoError(t, err)
	_, err = j.CreateProviderReplacement(ctx, providerReceiptFixture("second"))
	require.NoError(t, err)
	replay, err := j.CreateProviderReplacement(ctx, initial)
	require.NoError(t, err)
	require.Equal(t, ProviderReplacementReleased, replay.Phase)
	stored.Phase = ProviderReplacementPrepared
	_, err = j.UpdateProviderReplacement(ctx, stored.Version, stored)
	require.Error(t, err)
}

func TestMCPProviderReplacementReceiptCannotStartWithoutBoundTarget(t *testing.T) {
	ctx := context.Background()
	j, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	require.NoError(t, err)
	defer func() { _ = j.Close() }()
	stored, err := j.CreateProviderReplacement(ctx, providerReceiptFixture("first"))
	require.NoError(t, err)
	for _, phase := range []ProviderReplacementPhase{ProviderReplacementRetiring, ProviderReplacementRetired} {
		stored.Phase = phase
		if phase == ProviderReplacementRetired {
			stored.SourceTerminated = true
		}
		stored, err = j.UpdateProviderReplacement(ctx, stored.Version, stored)
		require.NoError(t, err)
	}
	stored.Phase = ProviderReplacementStarting
	_, err = j.UpdateProviderReplacement(ctx, stored.Version, stored)
	require.Error(t, err, "a start requires a scoped profile and exact native conversation")
}

func TestMCPProviderReplacementReceiptMissingActiveIndexFailsClosed(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	j, err := Open(Config{Path: path})
	require.NoError(t, err)
	_, err = j.CreateProviderReplacement(ctx, providerReceiptFixture("first"))
	require.NoError(t, err)
	require.NoError(t, j.updateLocked(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketProviderReplacements).Delete(keyActiveProviderReplacement)
	}))
	require.NoError(t, j.Close())
	reopened, err := Open(Config{Path: path})
	if reopened != nil {
		_ = reopened.Close()
	}
	require.ErrorIs(t, err, ErrJournalCorrupt)
}

func TestMCPProviderReplacementReceiptRejectsCorruptAdvancedEvidence(t *testing.T) {
	ctx := context.Background()
	j, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	require.NoError(t, err)
	defer func() { _ = j.Close() }()
	stored, err := j.CreateProviderReplacement(ctx, providerReceiptFixture("operation"))
	require.NoError(t, err)
	require.NoError(t, j.updateLocked(func(tx *bolt.Tx) error {
		raw := map[string]any{}
		require.NoError(t, json.Unmarshal(tx.Bucket(bucketProviderReplacements).Get(providerReceiptKey(stored.OperationID)), &raw))
		raw["phase"] = "started"
		bytes, err := json.Marshal(raw)
		require.NoError(t, err)
		return tx.Bucket(bucketProviderReplacements).Put(providerReceiptKey(stored.OperationID), bytes)
	}))
	_, err = j.ReadProviderReplacement(ctx, "operation")
	require.ErrorIs(t, err, ErrJournalCorrupt, "advanced phase requires a sealed target and durable owner evidence")
}

func TestMCPProviderReplacementReceiptRequiresTerminationProof(t *testing.T) {
	ctx := context.Background()
	j, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	require.NoError(t, err)
	defer func() { _ = j.Close() }()
	receipt, err := j.CreateProviderReplacement(ctx, providerReceiptFixture("operation"))
	require.NoError(t, err)
	receipt.Phase = ProviderReplacementRetiring
	receipt, err = j.UpdateProviderReplacement(ctx, receipt.Version, receipt)
	require.NoError(t, err)
	receipt.Phase = ProviderReplacementRetired
	_, err = j.UpdateProviderReplacement(ctx, receipt.Version, receipt)
	require.Error(t, err, "retired is a factual termination receipt, never a requested state")
	receipt.SourceTerminated = true
	receipt, err = j.UpdateProviderReplacement(ctx, receipt.Version, receipt)
	require.NoError(t, err)
	receipt.Phase = ProviderReplacementFailed
	receipt.SourceTerminated = false
	_, err = j.UpdateProviderReplacement(ctx, receipt.Version, receipt)
	require.Error(t, err, "a failure must retain proven termination")
}
