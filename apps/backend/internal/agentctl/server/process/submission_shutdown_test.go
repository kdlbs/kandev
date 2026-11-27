package process

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/pkg/agent"
	"github.com/stretchr/testify/require"
)

type shutdownRequestAdapter struct {
	adapter.AgentAdapter
	ctx          context.Context
	closing      chan struct{}
	dispatchDone chan struct{}
	once         sync.Once
}

func (a *shutdownRequestAdapter) Close() error {
	a.once.Do(func() { close(a.closing) })
	select {
	case <-a.dispatchDone:
	case <-a.ctx.Done():
		return fmt.Errorf("durable dispatch did not settle during adapter shutdown: %w", a.ctx.Err())
	}
	return a.AgentAdapter.Close()
}

func TestSubmissionGracefulStopPreservesUnknownRequestOutcome(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mgr := NewManager(&config.InstanceConfig{
		AgentArgs: fixtureArgs(), AgentEnv: fixtureEnvSlice("cat"), WorkDir: t.TempDir(), Protocol: agent.ProtocolACP,
		SessionID: "session", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 1,
		DeliveryStreamID: "incarnation:g1", DurableJournalPath: filepath.Join(t.TempDir(), "delivery.bbolt"),
	}, newTestLogger(t))
	require.NoError(t, mgr.Start(ctx))
	t.Cleanup(func() { _ = mgr.Stop(context.Background()); _ = mgr.closeDeliveryJournal() })
	wrapped := &shutdownRequestAdapter{ctx: ctx, AgentAdapter: mgr.adapter, closing: make(chan struct{}), dispatchDone: make(chan struct{})}
	mgr.adapter = wrapped
	original := journal.Submission{ID: "prompt:old", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "incarnation:g1", Payload: []byte("do not replay"), Hash: journal.SubmissionHash([]byte("do not replay"))}
	_, err := mgr.AdmitDeliverySubmission(ctx, original)
	require.NoError(t, err)
	began := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		_, err := mgr.DispatchDeliverySubmission(context.Background(), original.ID, func(context.Context) error {
			close(began)
			<-wrapped.closing
			return &acp.RequestError{Code: -32603, Message: "prompt aborted during shutdown"}
		})
		result <- err
		close(wrapped.dispatchDone)
	}()
	select {
	case <-began:
	case <-ctx.Done():
		t.Fatal("durable prompt did not start")
	}
	require.NoError(t, mgr.Stop(ctx))
	require.Error(t, <-result)
	retained, err := mgr.DeliveryJournal()
	require.NoError(t, err)
	stored, err := retained.GetSubmission(context.Background(), original.ID)
	require.NoError(t, err)
	require.Equal(t, journal.SubmissionInterruptedUnknown, stored.State)
	require.False(t, stored.TerminalEventRetained)
	require.Zero(t, stored.TerminalSequence)
	require.Equal(t, original.Hash, stored.Hash)
	require.Equal(t, original.Payload, stored.Payload)
	calls := 0
	_, err = mgr.DispatchDeliverySubmission(context.Background(), original.ID, func(context.Context) error { calls++; return nil })
	require.ErrorIs(t, err, ErrSubmissionUncertain)
	require.Zero(t, calls)
}

type rejectedPromptError struct{}

func (rejectedPromptError) Error() string                    { return "provider rejected prompt" }
func (rejectedPromptError) DeterministicPromptFailure() bool { return true }

func TestSubmissionCancellationSettlesJournalAndKeepsKnownFailures(t *testing.T) {
	for _, condition := range []string{"transport", "request cancelled", "provider rejected"} {
		t.Run(condition, func(t *testing.T) {
			store, err := journal.Open(journal.Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
			require.NoError(t, err)
			t.Cleanup(func() { _ = store.Close() })
			delivery := &SubmissionDelivery{Journal: store}
			_, err = delivery.Admit(context.Background(), journal.Submission{ID: "prompt", Hash: "immutable", Payload: []byte("never replay")})
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stored, err := delivery.Dispatch(ctx, "prompt", func(context.Context) error {
				cancel()
				switch condition {
				case "request cancelled":
					return &acp.RequestError{Code: -32603, Message: "aborted"}
				case "provider rejected":
					return rejectedPromptError{}
				default:
					return context.Canceled
				}
			})
			require.Error(t, err)
			want := journal.SubmissionInterruptedUnknown
			if condition == "provider rejected" {
				want = journal.SubmissionFailed
			}
			require.Equal(t, want, stored.State)
			retained, err := store.GetSubmission(context.Background(), "prompt")
			require.NoError(t, err)
			require.Equal(t, want, retained.State)
			require.False(t, retained.TerminalEventRetained)
		})
	}
}

func TestSubmissionRetainedErrorSettlesKnownFailure(t *testing.T) {
	ctx := context.Background()
	store, err := journal.Open(journal.Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	delivery := &SubmissionDelivery{Journal: store}
	_, err = delivery.Admit(ctx, journal.Submission{ID: "prompt", SessionID: "session", IncarnationID: "incarnation",
		HarnessGeneration: 1, StreamID: "stream", Hash: "immutable", Payload: []byte("old instruction")})
	require.NoError(t, err)
	stored, err := delivery.Dispatch(ctx, "prompt", func(ctx context.Context) error {
		_, appendErr := store.Append(ctx, journal.Event{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
			StreamID: "stream", SubmissionID: "prompt", Type: "error", Terminal: true, Payload: []byte("provider rejected prompt")})
		require.NoError(t, appendErr)
		return rejectedPromptError{}
	})
	require.Error(t, err)
	require.Equal(t, journal.SubmissionFailed, stored.State)
	require.True(t, stored.TerminalEventRetained)
	require.EqualValues(t, 1, stored.TerminalSequence)
}

type terminalOnCloseAdapter struct {
	adapter.AgentAdapter
	updates chan adapter.AgentEvent
}

func (a *terminalOnCloseAdapter) Updates() <-chan adapter.AgentEvent { return a.updates }
func (a *terminalOnCloseAdapter) Close() error {
	a.updates <- adapter.AgentEvent{Type: adapter.EventTypeComplete, SessionID: "session", DeliverySubmissionID: "prompt"}
	close(a.updates)
	return nil
}

func TestStopDrainsTerminalBeforeClosingJournalWriter(t *testing.T) {
	mgr := NewManager(&config.InstanceConfig{SessionID: "session", DeliveryIncarnationID: "incarnation",
		DeliveryHarnessGeneration: 1, DeliveryStreamID: "stream", DurableJournalPath: filepath.Join(t.TempDir(), "delivery.bbolt")}, newTestLogger(t))
	t.Cleanup(func() { _ = mgr.closeDeliveryJournal() })
	_, err := mgr.AdmitDeliverySubmission(context.Background(), journal.Submission{ID: "prompt", SessionID: "session",
		IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream", Hash: "hash", Payload: []byte("old")})
	require.NoError(t, err)
	mgr.TrackDeliverySubmission("prompt", 1)
	a := &terminalOnCloseAdapter{updates: make(chan adapter.AgentEvent, 1)}
	mgr.adapter = a
	mgr.stopCh = make(chan struct{})
	mgr.wg.Add(1)
	// Closing first deterministically makes both the event and stop ready.
	mgr.closeAdapterAndStdin()
	mgr.forwardUpdates(a, mgr.stopCh)
	store, err := mgr.DeliveryJournal()
	require.NoError(t, err)
	stored, err := store.GetSubmission(context.Background(), "prompt")
	require.NoError(t, err)
	require.True(t, stored.TerminalEventRetained)
	require.EqualValues(t, 1, stored.TerminalSequence)
}
