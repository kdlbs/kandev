# Upstream reproduction for issue #4880

Date: 2026-10-29. Baseline: `1b0d0881a4108697b8c346d17c5493fd34dd3f36`.
Production code remained unchanged during this reproduction.

## Command and observed result

The temporary file was `apps/backend/internal/agent/runtime/lifecycle/zz_issue4880_repro_test.go`.
The command ran from the repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle -run '^TestIssue4880' -count=1 -v)
```

The command exited 1, with the intended regression assertions:

- Recoverable agent failure: expected backend force `[false]`, received `[true]`.
- Resumed bootstrap failure: expected backend force `[false]`, received `[true]`.
- Reconnect: opened the same journal before resolving its previous owner.
  The real bbolt open returned `open delivery journal: timeout`.

The reconnect control trace was:

```text
GET /health
GET /api/v1/instances/new-execution
POST /api/v1/instances
```

No GET or DELETE addressed `old-execution` before the duplicate open.
The failure tests finished in 0.120 seconds after compilation.
The temporary Go file was removed after the investigation.

## Smallest fixture

To repeat the reproduction, create the temporary file only if that path is absent.
Use the following Go source. Run the command, then remove only the file you created.
This fixture requires no Docker daemon, provider credentials, or live database.

```go
package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

func TestIssue4880RemoteRecoverableFailurePreservesRuntime(t *testing.T) {
	for _, reason := range []string{StopReasonRecoverableAgentFailure, StopReasonAgentBootstrapFailed} {
		t.Run(reason, func(t *testing.T) {
			log := newTestRegistryLogger()
			registry := NewExecutorRegistry(log)
			backend := &stopTrackingExecutor{MockExecutor: MockExecutor{name: executor.NameRemoteDocker}}
			registry.Register(backend)
			mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
			cleanupManagerStopCh(t, mgr)
			execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session", RuntimeName: executor.NameRemoteDocker, ContainerID: "retained-container", isResumedSession: true}
			require.NoError(t, mgr.executionStore.Add(execution))
			require.NoError(t, mgr.StopAgentWithReason(context.Background(), execution.ID, reason, true))
			require.Equal(t, []bool{false}, backend.forces, "recoverable remote failure must preserve the established workspace")
		})
	}
}

type issue4880Endpoint struct {
	host string
	port int
}

func (e issue4880Endpoint) Resolve(context.Context, string, int, string) (string, int, error) {
	return e.host, e.port, nil
}

func (e issue4880Endpoint) Close() error { return nil }

func TestIssue4880ResumeWithPreviousJournalOwner(t *testing.T) {
	journalPath := filepath.Join(t.TempDir(), "delivery.bbolt")
	prior, err := journal.Open(journal.Config{Path: journalPath})
	require.NoError(t, err)
	t.Cleanup(func() { _ = prior.Close() })
	var acquisitionErr error
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/instances/old-execution":
			_, _ = w.Write([]byte(`{"id":"old-execution","port":41001,"session_id":"session"}`))
		case r.Method == http.MethodPost:
			var request struct { DurableJournalPath string `json:"durable_journal_path"` }
			_ = json.NewDecoder(r.Body).Decode(&request)
			if request.DurableJournalPath == "" {
				http.Error(w, "missing journal path", http.StatusBadRequest)
				return
			}
			second, openErr := journal.Open(journal.Config{Path: journalPath, OpenTimeout: 25 * time.Millisecond})
			acquisitionErr = openErr
			if second != nil { _ = second.Close() }
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"new-execution","port":41002}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	host, port := parseHTTPURL(t, server.URL)
	r := &DockerExecutor{logger: newTestDockerLogger(), endpoints: issue4880Endpoint{host: host, port: port}}
	_, err = r.bringupAgentctl(context.Background(), nil, "retained-container", "", &ExecutorCreateRequest{
		InstanceID: "new-execution", PreviousExecutionID: "old-execution", SessionID: "session",
		DurableJournalHostRoot: t.TempDir(), DurableJournalOwnerID: "session",
	})
	require.NoError(t, err)
	t.Logf("control requests: %v; journal acquisition: %v", requests, acquisitionErr)
	require.ErrorIs(t, acquisitionErr, bolt.ErrTimeout, "the fixture must reproduce real bbolt lock contention")
	require.NoError(t, acquisitionErr, "resume must reconcile the previous owner before opening its journal again")
}

```

## Evidence limits

The lifecycle cases exercise the real stop policy with a recording executor.
They prove the destructive force request, not deletion on a real daemon.
Source tracing connects that request to `RemoteDockerExecutor.StopInstance`.

The reconnect case exercises real `bringupAgentctl` with a mock control endpoint.
Its create handler performs the same real journal open as `process.NewManager`.
It uses a short fixture timeout while production keeps its existing default.
It proves the competing-open mechanism and the skipped previous-owner lookup.
It does not identify the reporter's historical lock holder.

The registered-launch rollback gap comes from source inspection.
Its permanent failing regression belongs to Task 01.
The separate archived-task workflow-edit symptom has no upstream reproduction.
Real Docker/SSH recovery, unpushed commit preservation, and restart evidence remain
Task 03 gates. No deployment or live runtime was changed.

## Documentation coverage preflight

Run from the repository root. This checks the planned production scope against
all three work orders. It does not claim that production files changed.

```bash
node - <<'NODE'
const fs = require('node:fs');
const validator = require('./.github/scripts/pr-docs.cjs');
const directory = 'docs/plans/remote-docker-recoverable-resume';
const packagePaths = fs.readdirSync(directory)
  .filter(name => name.endsWith('.md'))
  .map(name => `${directory}/${name}`);
const references = [
  'docs/specs/executors/requirements/remote-docker-executor.md',
  'docs/specs/platform/requirements/durable-agent-delivery.md',
  'docs/specs/executors/system-design/remote-docker-executor.md',
  'docs/specs/platform/system-design/durable-agent-journal-ownership.md',
];
const fileContents = Object.fromEntries([...packagePaths, ...references]
  .map(file => [file, fs.readFileSync(file, 'utf8')]));
const changedFiles = packagePaths.map(filename => ({ filename, status: 'added' }));
changedFiles.push({
  filename: 'apps/backend/internal/agent/runtime/lifecycle/recovery_stop.go',
  status: 'modified',
});
const result = validator.validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({
  status: result.status,
  workOrders: result.workOrders.length,
  errors: result.errors,
}, null, 2));
if (!result.ok || result.workOrders.length !== 3) process.exit(1);
NODE
```

Investigation result: `covered`, three work orders, zero reference errors.
Repeat against the actual production diff during implementation.
