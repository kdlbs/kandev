# Standalone retained-journal reproduction

## Baseline and scope

- Source: [GitHub issue #5165](https://github.com/kdlbs/kandev/issues/5165), read on 2026-11-07.
- Current main: `52309e613fc9a76d38a9f891b74b4d8b32bff265`, verified with `git ls-remote origin refs/heads/main`.
- Reported release: v0.99.1, `813a741b21108ea8ed10ab58954d85c2e5205999`.
- Host: Linux amd64, Go 1.24.4.
- Issue assignment: `carlosflorencio`, verified after the mutation.

The release and current main contain the same unconditional standalone POST
at this boundary. The reproduction ran on current main without production edits.
There are no issue attachments or comments.

## Smallest state and failure path

1. Hold a real journal handle at `agentctl-journals/session-5165/delivery.bbolt`.
2. Expose an old standalone instance for that session on the control inventory.
3. Request a different execution ID with the same session journal owner.
4. Call the real `StandaloneExecutor.CreateInstance` through its HTTP control client.
5. Observe CREATE without prior inventory, owner adoption, or exact disposal.
6. Observe a second journal open fail with `lock_timeout`.

The two cases model stopped and live agent status responses.
Both managers retain a real exclusive bbolt handle.
The test does not launch a real native provider process.
It does not prove which child state applied in the reporter's deployment.
Its live-status fixture lacks a full initialized native association.
Permanent adoption coverage must add that association and SQL delivery evidence.

The existing `journalReconnectControl` fixture opens the same real journal for
CREATE. It uses a 100 ms acquisition timeout to keep the diagnostic short.
The production two-second timeout remains unchanged.
The fixture's DELETE operation closes only the selected handle.
Its status endpoint is available if an ownership resolver requests it.

## Command and observed result

The temporary file was
`apps/backend/internal/agent/runtime/lifecycle/issue5165_repro_test.go`.
From the repository root:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestIssue5165ReproStandaloneRetainedJournal$' -count=1 -v)
```

Result: exit 1, two expected behavioral failures, approximately 0.15 seconds
of package test execution after compilation.

```text
stopped_owner:
  running=false control_calls=[CREATE requested-execution]
  failed to create standalone instance: failed to create instance:
  failed to create new instance: open delivery journal: lock_timeout (status 500)
live_owner:
  running=true control_calls=[CREATE requested-execution]
  failed to create standalone instance: failed to create instance:
  failed to create new instance: open delivery journal: lock_timeout (status 500)
```

The assertions describe the intended disposition for a fully verified owner.
The fixture proves the current duplicate open before ownership inspection.
The live case also requires the retained instance ID and no CREATE.
The stopped case requires DELETE before replacement creation.

## Temporary test source

This is diagnostic source, not a permanent test added by this package.
Copy it to the path above only for a temporary reproduction or during explicit
implementation. It uses the existing lifecycle test fixtures from this baseline.
Remove the temporary file after the diagnostic run.

```go
package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
)

// Temporary diagnostic; remove after recording the fix-package evidence.
func TestIssue5165ReproStandaloneRetainedJournal(t *testing.T) {
	for _, running := range []bool{false, true} {
		name := "stopped_owner"
		if running {
			name = "live_owner"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			location, err := journal.ResolveLocation(root, "session-5165")
			require.NoError(t, err)
			control := newJournalReconnectControl(func(string) string { return location.Path })
			statusServer := newReconnectStatusServer(t, control)
			_, statusPort := parseHTTPURL(t, statusServer.URL)
			control.addInstance("retained-execution", "task-5165", "session-5165", statusPort, running)
			control.holdJournal(t, "retained-execution", location.Path)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/health":
					w.WriteHeader(http.StatusOK)
				case r.URL.Path == "/api/v1/instances" && r.Method == http.MethodGet:
					instances, listErr := control.ListInstances(r.Context())
					if listErr != nil {
						http.Error(w, listErr.Error(), http.StatusInternalServerError)
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]interface{}{"instances": instances})
				case r.URL.Path == "/api/v1/instances" && r.Method == http.MethodPost:
					var request agentctl.CreateInstanceRequest
					if decodeErr := json.NewDecoder(r.Body).Decode(&request); decodeErr != nil {
						http.Error(w, decodeErr.Error(), http.StatusBadRequest)
						return
					}
					response, createErr := control.CreateInstance(r.Context(), &request)
					if createErr != nil {
						w.WriteHeader(http.StatusInternalServerError)
						_ = json.NewEncoder(w).Encode(map[string]string{"error": createErr.Error()})
						return
					}
					_ = json.NewEncoder(w).Encode(response)
				case strings.HasPrefix(r.URL.Path, "/api/v1/instances/") && r.Method == http.MethodDelete:
					if deleteErr := control.DeleteInstance(r.Context(), strings.TrimPrefix(r.URL.Path, "/api/v1/instances/")); deleteErr != nil {
						http.Error(w, deleteErr.Error(), http.StatusInternalServerError)
					}
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)
			host, port := parseHTTPURL(t, server.URL)
			log := newTestDockerLogger()
			executor := NewStandaloneExecutor(agentctl.NewControlClient(host, port, log), host, port, log)
			instance, createErr := executor.CreateInstance(context.Background(), &ExecutorCreateRequest{
				InstanceID: "requested-execution", PreviousExecutionID: "retained-execution",
				TaskID: "task-5165", SessionID: "session-5165", WorkspacePath: root,
				DurableJournalHostRoot: root, DurableJournalOwnerID: "session-5165",
			})
			t.Logf("running=%v control_calls=%v error=%v", running, control.recordedMethods(), createErr)
			if createErr != nil {
				t.Fatalf("retained owner must be reconciled before journal acquisition: %v", createErr)
			}
			if running {
				require.Equal(t, "retained-execution", instance.StandaloneInstanceID)
				require.False(t, control.hasMethod("CREATE "), "a live owner must not be replaced")
			} else {
				require.True(t, control.hasMethod("DELETE retained-execution"), "release the stopped manager before opening its journal")
			}
		})
	}
}
```

## Evidence limits and cleanup

This diagnostic proves the duplicate-open mechanism and its actual typed reason.
It does not reproduce the reporter's whole Docker deployment, original signal,
control credential rotation, or startup inventory history.
The reporter's operating-system evidence remains reported evidence.
A standalone runtime inside a Docker-deployed backend still enters this path.
That deployment shape does not make it a Docker executor.

Test cleanup closes fixture HTTP servers and journal handles and removes the
isolated temporary roots. The planning turn removes the throwaway Go file.
Only the design and delivery records remain in the workspace.
No live process, reporter data, journal, SQL record, or repository setting changed.

Task 01 converts the defect into permanent regression coverage.
Task 02 proves real instance teardown, SQL reconciliation, and repeated restart.
Neither implementation result is claimed by this reproduction.

## Existing control tests

These commands passed against unchanged production code during investigation:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run '^(TestDockerReconnectReconcilesPreviousJournalOwner|TestDockerReconnectAdoptsLivePreviousJournalOwner|TestReuseExistingAdoptsWithoutInitialize|TestReuseExistingIdentityMismatchPreservesPeer)$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/instance -run '^TestCreateInstance(RejectsUnavailableJournal|TeardownReleasesJournal)$' -count=1 -v)
```

The lifecycle command passed four tests. The instance command passed two tests.
They establish the existing disposal and adoption mechanisms used by this design.
They do not establish corrected standalone recovery.
