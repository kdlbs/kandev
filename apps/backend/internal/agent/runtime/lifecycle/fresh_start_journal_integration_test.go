package lifecycle

import (
	"context"
	"net"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/api"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/stretchr/testify/require"
)

func TestFreshStartRegisteredJournalRetirementNeverDispatchesPredecessor(t *testing.T) {
	cfg := &config.InstanceConfig{WorkDir: t.TempDir(), SessionID: "session", DeliveryIncarnationID: "incarnation",
		DeliveryHarnessGeneration: 2, DeliveryStreamID: "incarnation:g2", DurableJournalPath: filepath.Join(t.TempDir(), "delivery.bbolt")}
	peer := process.NewManager(cfg, newTestLogger())
	t.Cleanup(func() { require.NoError(t, peer.StopForTeardown(context.Background())) })
	retained, err := peer.DeliveryJournal()
	require.NoError(t, err)
	server := httptest.NewServer(api.NewServer(cfg, peer, nil, nil, newTestLogger()).Router())
	t.Cleanup(server.Close)
	host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	client := agentctl.NewClient(host, port, newTestLogger())
	execution := &AgentExecution{SessionID: "session", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 2, ReplacesNativeConversation: true}
	states := []journal.SubmissionState{journal.SubmissionPrepared, journal.SubmissionAccepted, journal.SubmissionDispatching, journal.SubmissionInterruptedUnknown}
	for i, state := range states {
		_, err = retained.PutSubmission(t.Context(), journal.Submission{ID: strconv.Itoa(i), SessionID: "session", IncarnationID: "incarnation",
			HarnessGeneration: 1, StreamID: "incarnation:g1", State: state, Hash: "hash", Payload: []byte("retained predecessor intent")})
		require.NoError(t, err)
	}
	require.NoError(t, retireUnresolvedDeliverySubmissions(t.Context(), deliveryJournalReleaser{}, client, execution))
	dispatcher := &process.SubmissionDelivery{Journal: retained}
	calls := 0
	for i := range states {
		stored, err := retained.GetSubmission(t.Context(), strconv.Itoa(i))
		require.NoError(t, err)
		require.True(t, stored.Retired)
		require.Equal(t, journal.SubmissionInterruptedUnknown, stored.State)
		require.Equal(t, []byte("retained predecessor intent"), stored.Payload)
		_, err = dispatcher.Dispatch(t.Context(), stored.ID, func(context.Context) error { calls++; return nil })
		require.Error(t, err)
	}
	require.Zero(t, calls, "retired predecessor work must never reach the provider callback")
	_, err = dispatcher.Admit(t.Context(), journal.Submission{ID: "new", SessionID: "session", IncarnationID: "incarnation",
		HarnessGeneration: 2, StreamID: "incarnation:g2", Hash: "new-hash", Payload: []byte("fresh intent")})
	require.NoError(t, err)
	_, err = dispatcher.Dispatch(t.Context(), "new", func(context.Context) error { calls++; return nil })
	require.NoError(t, err)
	require.Equal(t, 1, calls, "new-generation work is the live dispatch control")
}
