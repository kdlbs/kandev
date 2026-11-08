package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
)

// standaloneJournalControlServer wraps the real-journal reconnect fixture with
// the standalone control routes the executor uses.
type standaloneJournalControlServer struct {
	control          *journalReconnectControl
	deleteFailures   map[string]bool
	requestListError bool
}

func newStandaloneJournalControlServer(t *testing.T, control *journalReconnectControl) (*standaloneJournalControlServer, *httptest.Server) {
	t.Helper()
	fixture := &standaloneJournalControlServer{
		control:        control,
		deleteFailures: map[string]bool{},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/health":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/v1/instances" && r.Method == http.MethodGet:
			if fixture.requestListError {
				http.Error(w, "inventory unavailable", http.StatusInternalServerError)
				return
			}
			instances, err := control.ListInstances(r.Context())
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"instances": instances})
		case r.URL.Path == "/api/v1/instances" && r.Method == http.MethodPost:
			var request agentctl.CreateInstanceRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			response, err := control.CreateInstance(r.Context(), &request)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			_ = json.NewEncoder(w).Encode(response)
		case strings.HasPrefix(r.URL.Path, "/api/v1/instances/"):
			id := strings.TrimPrefix(r.URL.Path, "/api/v1/instances/")
			switch r.Method {
			case http.MethodDelete:
				if err := control.DeleteInstance(r.Context(), id); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				if fixture.deleteFailures[id] {
					http.Error(w, "delete response lost", http.StatusInternalServerError)
				}
			case http.MethodGet:
				info, err := control.GetInstance(r.Context(), id)
				if errors.Is(err, agentctl.ErrInstanceNotFound) {
					http.NotFound(w, r)
					return
				}
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				_ = json.NewEncoder(w).Encode(info)
			default:
				http.NotFound(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return fixture, server
}

func newStandaloneReconnectExecutor(t *testing.T, server *httptest.Server) *StandaloneExecutor {
	t.Helper()
	host, port := parseHTTPURL(t, server.URL)
	log := newTestDockerLogger()
	return NewStandaloneExecutor(agentctl.NewControlClient(host, port, log), host, port, log)
}

func standaloneReconnectRequest(root, requestedID, previousID string) *ExecutorCreateRequest {
	return &ExecutorCreateRequest{
		InstanceID:             requestedID,
		PreviousExecutionID:    previousID,
		TaskID:                 "task-5165",
		SessionID:              "session-5165",
		WorkspacePath:          root,
		DurableJournalHostRoot: root,
		DurableJournalOwnerID:  "session-5165",
	}
}

func seedRetainedJournal(t *testing.T, path string) (journal.Event, journal.Submission) {
	t.Helper()
	return seedRetainedJournalForSession(t, path, "session-5165")
}

func seedRetainedJournalForSession(t *testing.T, path, sessionID string) (journal.Event, journal.Submission) {
	t.Helper()
	opened, err := journal.Open(journal.Config{Path: path})
	require.NoError(t, err)
	defer func() { require.NoError(t, opened.Close()) }()
	event, err := opened.Append(context.Background(), journal.Event{
		SessionID: sessionID, IncarnationID: "incarnation-1", HarnessGeneration: 1,
		StreamID: "stream-1", Sequence: 1, Type: "message_chunk", Payload: []byte(`{"text":"retained"}`),
	})
	require.NoError(t, err)
	submission, err := opened.PutSubmission(context.Background(), journal.Submission{
		ID: "submission-1", StreamID: "stream-1", SessionID: sessionID,
		IncarnationID: "incarnation-1", HarnessGeneration: 1,
		Hash: "hash-1", Payload: []byte(`{"prompt":"uncertain"}`), State: journal.SubmissionDispatching,
	})
	require.NoError(t, err)
	return event, submission
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.2
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.3
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.1
func TestStandaloneReconnectDisposesStoppedJournalOwner(t *testing.T) {
	root := t.TempDir()
	location, err := journal.ResolveLocation(root, "session-5165")
	require.NoError(t, err)
	event, submission := seedRetainedJournal(t, location.Path)

	control := newJournalReconnectControl(func(string) string { return location.Path })
	statusServer := newReconnectStatusServer(t, control)
	_, statusPort := parseHTTPURL(t, statusServer.URL)
	control.addInstance("retained-execution", "task-5165", "session-5165", statusPort, false)
	control.holdJournal(t, "retained-execution", location.Path)
	fixture, server := newStandaloneJournalControlServer(t, control)
	executor := newStandaloneReconnectExecutor(t, server)

	instance, err := executor.CreateInstance(context.Background(), standaloneReconnectRequest(root, "requested-execution", "retained-execution"))
	require.NoError(t, err)
	require.Equal(t, "requested-execution", instance.InstanceID)
	require.Equal(t, "requested-execution", instance.StandaloneInstanceID)
	require.NotNil(t, instance.Client)
	require.True(t, fixture.control.hasMethod("DELETE retained-execution"))
	require.True(t, fixture.control.hasMethod("CREATE requested-execution"))

	methods := fixture.control.recordedMethods()
	deleteIndex := indexOfMethod(methods, "DELETE retained-execution")
	createIndex := indexOfMethod(methods, "CREATE requested-execution")
	require.GreaterOrEqual(t, deleteIndex, 0)
	require.Greater(t, createIndex, deleteIndex, "the stopped owner must release its journal before the replacement opens it")

	fixture.control.mu.Lock()
	if replacement := fixture.control.held["requested-execution"]; replacement != nil {
		require.NoError(t, replacement.Close())
		delete(fixture.control.held, "requested-execution")
	}
	fixture.control.mu.Unlock()

	reopened, err := journal.Open(journal.Config{Path: location.Path, ReadOnly: true})
	require.NoError(t, err)
	defer func() { require.NoError(t, reopened.Close()) }()
	events, stream, err := reopened.Replay(context.Background(), event.StreamID, 0, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, event.Sequence, events[0].Sequence)
	require.Equal(t, event.Payload, events[0].Payload)
	require.Equal(t, event.StreamID, stream.StreamID)
	retainedSubmission, err := reopened.GetSubmission(context.Background(), submission.ID)
	require.NoError(t, err)
	require.Equal(t, submission.Hash, retainedSubmission.Hash)
	require.Equal(t, submission.State, retainedSubmission.State)
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.3
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.6
func TestStandaloneReconnectAdoptsLiveJournalOwner(t *testing.T) {
	root := t.TempDir()
	location, err := journal.ResolveLocation(root, "session-5165")
	require.NoError(t, err)
	control := newJournalReconnectControl(func(string) string { return location.Path })
	statusServer := newReconnectStatusServer(t, control)
	_, statusPort := parseHTTPURL(t, statusServer.URL)
	control.addInstance("retained-execution", "task-5165", "session-5165", statusPort, true)
	control.holdJournal(t, "retained-execution", location.Path)
	fixture, server := newStandaloneJournalControlServer(t, control)
	executor := newStandaloneReconnectExecutor(t, server)

	instance, err := executor.CreateInstance(context.Background(), standaloneReconnectRequest(root, "requested-execution", "retained-execution"))
	require.NoError(t, err)
	require.Equal(t, "retained-execution", instance.InstanceID)
	require.Equal(t, "retained-execution", instance.StandaloneInstanceID)
	require.Equal(t, statusPort, instance.StandalonePort)
	require.Equal(t, true, instance.Metadata[MetadataKeyReuseExistingProcess])
	require.True(t, instance.ReusedWorkspace)
	require.False(t, fixture.control.hasMethod("DELETE"), "a live owner must not be replaced")
	require.False(t, fixture.control.hasMethod("CREATE"), "a live owner must not be replaced")
	fixture.control.mu.Lock()
	defer fixture.control.mu.Unlock()
	require.Contains(t, fixture.control.held, "retained-execution")
	require.NotContains(t, fixture.control.held, "requested-execution")
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-003.2
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.1
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.2
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.3
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.4
func TestStandaloneReconnectBlocksUnresolvedJournalOwners(t *testing.T) {
	t.Run("unknown candidate blocks a verified owner", func(t *testing.T) {
		root := t.TempDir()
		location, err := journal.ResolveLocation(root, "session-5165")
		require.NoError(t, err)
		control := newJournalReconnectControl(func(string) string { return location.Path })
		statusServer := newReconnectStatusServer(t, control)
		_, statusPort := parseHTTPURL(t, statusServer.URL)
		control.addInstance("retained-execution", "task-5165", "session-5165", statusPort, false)
		control.addInstance("unknown-execution", "task-5165", "session-5165", statusPort, true)
		control.holdJournal(t, "retained-execution", location.Path)
		control.mu.Lock()
		control.statusFailures["unknown-execution"] = true
		control.mu.Unlock()
		fixture, server := newStandaloneJournalControlServer(t, control)

		_, err = newStandaloneReconnectExecutor(t, server).CreateInstance(context.Background(), standaloneReconnectRequest(root, "requested-execution", "retained-execution"))
		require.ErrorIs(t, err, errStandaloneJournalOwnerBlocked)
		require.False(t, fixture.control.hasMethod("DELETE"))
		require.False(t, fixture.control.hasMethod("CREATE"))
	})

	t.Run("conflicting task blocks", func(t *testing.T) {
		root := t.TempDir()
		location, err := journal.ResolveLocation(root, "session-5165")
		require.NoError(t, err)
		control := newJournalReconnectControl(func(string) string { return location.Path })
		statusServer := newReconnectStatusServer(t, control)
		_, statusPort := parseHTTPURL(t, statusServer.URL)
		control.addInstance("retained-execution", "other-task", "session-5165", statusPort, false)
		fixture, server := newStandaloneJournalControlServer(t, control)

		_, err = newStandaloneReconnectExecutor(t, server).CreateInstance(context.Background(), standaloneReconnectRequest(root, "requested-execution", "retained-execution"))
		require.ErrorIs(t, err, errStandaloneJournalOwnerBlocked)
		require.False(t, fixture.control.hasMethod("DELETE"))
		require.False(t, fixture.control.hasMethod("CREATE"))
	})

	t.Run("requested identity conflict blocks", func(t *testing.T) {
		root := t.TempDir()
		location, err := journal.ResolveLocation(root, "session-5165")
		require.NoError(t, err)
		control := newJournalReconnectControl(func(string) string { return location.Path })
		statusServer := newReconnectStatusServer(t, control)
		_, statusPort := parseHTTPURL(t, statusServer.URL)
		control.addInstance("requested-execution", "task-5165", "other-session", statusPort, false)
		fixture, server := newStandaloneJournalControlServer(t, control)

		_, err = newStandaloneReconnectExecutor(t, server).CreateInstance(context.Background(), standaloneReconnectRequest(root, "requested-execution", ""))
		require.ErrorIs(t, err, errStandaloneJournalOwnerBlocked)
		require.False(t, fixture.control.hasMethod("DELETE"))
		require.False(t, fixture.control.hasMethod("CREATE"))
	})

	t.Run("workspace conflict blocks", func(t *testing.T) {
		root := t.TempDir()
		location, err := journal.ResolveLocation(root, "session-5165")
		require.NoError(t, err)
		control := newJournalReconnectControl(func(string) string { return location.Path })
		statusServer := newReconnectStatusServer(t, control)
		_, statusPort := parseHTTPURL(t, statusServer.URL)
		control.addInstance("retained-execution", "task-5165", "session-5165", statusPort, false)
		control.mu.Lock()
		control.instances["retained-execution"].WorkspacePath = t.TempDir()
		control.mu.Unlock()
		fixture, server := newStandaloneJournalControlServer(t, control)

		_, err = newStandaloneReconnectExecutor(t, server).CreateInstance(context.Background(), standaloneReconnectRequest(root, "requested-execution", "retained-execution"))
		require.ErrorIs(t, err, errStandaloneJournalOwnerBlocked)
		require.False(t, fixture.control.hasMethod("DELETE"))
		require.False(t, fixture.control.hasMethod("CREATE"))
	})

	t.Run("delivery conflict blocks", func(t *testing.T) {
		root := t.TempDir()
		location, err := journal.ResolveLocation(root, "session-5165")
		require.NoError(t, err)
		control := newJournalReconnectControl(func(string) string { return location.Path })
		statusServer := newReconnectStatusServer(t, control)
		_, statusPort := parseHTTPURL(t, statusServer.URL)
		control.addInstance("retained-execution", "task-5165", "session-5165", statusPort, true)
		control.mu.Lock()
		control.deliverySession["retained-execution"] = "other-session"
		control.mu.Unlock()
		fixture, server := newStandaloneJournalControlServer(t, control)

		_, err = newStandaloneReconnectExecutor(t, server).CreateInstance(context.Background(), standaloneReconnectRequest(root, "requested-execution", "retained-execution"))
		require.ErrorIs(t, err, errStandaloneJournalOwnerBlocked)
		require.False(t, fixture.control.hasMethod("DELETE"))
		require.False(t, fixture.control.hasMethod("CREATE"))
	})

	t.Run("multiple live owners block", func(t *testing.T) {
		root := t.TempDir()
		location, err := journal.ResolveLocation(root, "session-5165")
		require.NoError(t, err)
		control := newJournalReconnectControl(func(string) string { return location.Path })
		statusServer := newReconnectStatusServer(t, control)
		_, statusPort := parseHTTPURL(t, statusServer.URL)
		control.addInstance("retained-execution-a", "task-5165", "session-5165", statusPort, true)
		control.addInstance("retained-execution-b", "task-5165", "session-5165", statusPort, true)
		control.holdJournal(t, "retained-execution-a", location.Path)
		fixture, server := newStandaloneJournalControlServer(t, control)

		_, err = newStandaloneReconnectExecutor(t, server).CreateInstance(context.Background(), standaloneReconnectRequest(root, "requested-execution", "retained-execution-a"))
		require.ErrorIs(t, err, errStandaloneJournalOwnerBlocked)
		require.False(t, fixture.control.hasMethod("DELETE"))
		require.False(t, fixture.control.hasMethod("CREATE"))
	})

	t.Run("unavailable retained source roots block", func(t *testing.T) {
		root := t.TempDir()
		location, err := journal.ResolveLocation(root, "session-5165")
		require.NoError(t, err)
		control := newJournalReconnectControl(func(string) string { return location.Path })
		statusServer := newReconnectStatusServer(t, control)
		_, statusPort := parseHTTPURL(t, statusServer.URL)
		control.addInstance("retained-execution", "task-5165", "session-5165", statusPort, true)
		fixture, server := newStandaloneJournalControlServer(t, control)
		request := standaloneReconnectRequest(root, "requested-execution", "retained-execution")
		request.WorkspaceSourceRoots = []string{"/replacement/root"}

		_, err = newStandaloneReconnectExecutor(t, server).CreateInstance(context.Background(), request)
		require.ErrorIs(t, err, errStandaloneJournalOwnerBlocked)
		require.False(t, fixture.control.hasMethod("DELETE"))
		require.False(t, fixture.control.hasMethod("CREATE"))
	})

	t.Run("empty retained evidence stays authoritative", func(t *testing.T) {
		root := t.TempDir()
		location, err := journal.ResolveLocation(root, "session-5165")
		require.NoError(t, err)
		control := newJournalReconnectControl(func(string) string { return location.Path })
		statusServer := newReconnectStatusServer(t, control)
		_, statusPort := parseHTTPURL(t, statusServer.URL)
		control.addInstance("retained-execution", "task-5165", "session-5165", statusPort, true)
		control.mu.Lock()
		control.instances["retained-execution"].WorkspaceSourceRoots = []string{}
		control.instances["retained-execution"].Env = map[string]string{}
		control.mu.Unlock()
		fixture, server := newStandaloneJournalControlServer(t, control)
		request := standaloneReconnectRequest(root, "requested-execution", "retained-execution")
		request.WorkspaceSourceRoots = []string{"/replacement/root"}
		request.Env = map[string]string{"REQUEST_ONLY": "value"}

		instance, err := newStandaloneReconnectExecutor(t, server).CreateInstance(context.Background(), request)
		require.NoError(t, err)
		require.Empty(t, instance.WorkspaceSourceRoots)
		require.Empty(t, instance.Env)
		require.False(t, fixture.control.hasMethod("DELETE"))
		require.False(t, fixture.control.hasMethod("CREATE"))
	})

	t.Run("duplicate identifiers block", func(t *testing.T) {
		executor := &StandaloneExecutor{logger: newTestDockerLogger()}
		_, err := executor.classifyStandaloneJournalOwners(
			context.Background(), nil, standaloneReconnectRequest(t.TempDir(), "requested-execution", ""),
			[]*agentctl.InstanceInfo{
				{ID: "duplicate", TaskID: "task-5165", SessionID: "session-5165", Port: 41001},
				{ID: "duplicate", TaskID: "task-5165", SessionID: "session-5165", Port: 41002},
			},
			map[string]struct{}{},
		)
		require.ErrorIs(t, err, errStandaloneJournalOwnerBlocked)
	})

	t.Run("incomplete inventory blocks", func(t *testing.T) {
		root := t.TempDir()
		control := newJournalReconnectControl(func(string) string { return locationFor(t, root).Path })
		fixture, server := newStandaloneJournalControlServer(t, control)
		fixture.requestListError = true

		_, err := newStandaloneReconnectExecutor(t, server).CreateInstance(context.Background(), standaloneReconnectRequest(root, "requested-execution", "retained-execution"))
		require.ErrorIs(t, err, errStandaloneJournalOwnerBlocked)
	})

	t.Run("excess candidates block", func(t *testing.T) {
		root := t.TempDir()
		location, err := journal.ResolveLocation(root, "session-5165")
		require.NoError(t, err)
		control := newJournalReconnectControl(func(string) string { return location.Path })
		statusServer := newReconnectStatusServer(t, control)
		_, statusPort := parseHTTPURL(t, statusServer.URL)
		for index := 0; index <= maxReconnectOwnerCandidates; index++ {
			control.addInstance("retained-execution-"+string(rune('a'+index)), "task-5165", "session-5165", statusPort, false)
		}
		fixture, server := newStandaloneJournalControlServer(t, control)

		_, err = newStandaloneReconnectExecutor(t, server).CreateInstance(context.Background(), standaloneReconnectRequest(root, "requested-execution", "retained-execution-a"))
		require.ErrorIs(t, err, errStandaloneJournalOwnerBlocked)
		require.False(t, fixture.control.hasMethod("DELETE"))
		require.False(t, fixture.control.hasMethod("CREATE"))
	})

	t.Run("failed disposal blocks", func(t *testing.T) {
		root := t.TempDir()
		location, err := journal.ResolveLocation(root, "session-5165")
		require.NoError(t, err)
		control := newJournalReconnectControl(func(string) string { return location.Path })
		statusServer := newReconnectStatusServer(t, control)
		_, statusPort := parseHTTPURL(t, statusServer.URL)
		control.addInstance("retained-execution", "task-5165", "session-5165", statusPort, false)
		control.holdJournal(t, "retained-execution", location.Path)
		control.deleteErr = errors.New("teardown failed")
		fixture, server := newStandaloneJournalControlServer(t, control)

		_, err = newStandaloneReconnectExecutor(t, server).CreateInstance(context.Background(), standaloneReconnectRequest(root, "requested-execution", "retained-execution"))
		require.ErrorIs(t, err, errStandaloneJournalOwnerBlocked)
		require.False(t, fixture.control.hasMethod("CREATE"))
	})

	t.Run("lost delete response reconciles exact absence", func(t *testing.T) {
		root := t.TempDir()
		location, err := journal.ResolveLocation(root, "session-5165")
		require.NoError(t, err)
		control := newJournalReconnectControl(func(string) string { return location.Path })
		statusServer := newReconnectStatusServer(t, control)
		_, statusPort := parseHTTPURL(t, statusServer.URL)
		control.addInstance("retained-execution", "task-5165", "session-5165", statusPort, false)
		control.holdJournal(t, "retained-execution", location.Path)
		fixture, server := newStandaloneJournalControlServer(t, control)
		fixture.deleteFailures["retained-execution"] = true

		instance, err := newStandaloneReconnectExecutor(t, server).CreateInstance(context.Background(), standaloneReconnectRequest(root, "requested-execution", "retained-execution"))
		require.NoError(t, err)
		require.Equal(t, "requested-execution", instance.StandaloneInstanceID)
		require.True(t, fixture.control.hasMethod("CREATE requested-execution"))
	})

	t.Run("different session keeps its instance", func(t *testing.T) {
		root := t.TempDir()
		location, err := journal.ResolveLocation(root, "session-5165")
		require.NoError(t, err)
		otherLocation := locationFor(t, root)
		control := newJournalReconnectControl(func(sessionID string) string {
			if sessionID == "other-session" {
				return otherLocation.Path
			}
			return location.Path
		})
		statusServer := newReconnectStatusServer(t, control)
		_, statusPort := parseHTTPURL(t, statusServer.URL)
		control.addInstance("other-session-execution", "task-5165", "other-session", statusPort, false)
		control.holdJournal(t, "other-session-execution", otherLocation.Path)
		fixture, server := newStandaloneJournalControlServer(t, control)

		instance, err := newStandaloneReconnectExecutor(t, server).CreateInstance(context.Background(), standaloneReconnectRequest(root, "requested-execution", ""))
		require.NoError(t, err)
		require.Equal(t, "requested-execution", instance.StandaloneInstanceID)
		require.False(t, fixture.control.hasMethod("DELETE other-session-execution"))
		fixture.control.mu.Lock()
		defer fixture.control.mu.Unlock()
		require.Contains(t, fixture.control.instances, "other-session-execution")
	})
}

func locationFor(t *testing.T, root string) journal.Location {
	t.Helper()
	location, err := journal.ResolveLocation(root, "other-session")
	require.NoError(t, err)
	return location
}

func indexOfMethod(methods []string, want string) int {
	for index, method := range methods {
		if method == want {
			return index
		}
	}
	return -1
}
