package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/secrets"
)

// This one-shot test is removed after the exact pre-release fixture is safely
// drained. It reads credentials only into memory, never logs them, and has no
// production API surface.
func TestFixtureTeardownOnce(t *testing.T) {
	if os.Getenv("FF_POST_DEREGISTER_INSPECT") != "yes" && os.Getenv("FF_DEREGISTER_FIXTURE_JOB") != "yes" {
		t.Skip("fixture-only read or whole-job cleanup requires an explicit mode")
	}
	root := "/private/tmp/nomad-wo04-rjr6dc8s"
	const taskID = "104ce439-3805-4307-be5a-7c4e8354ba20"
	const allocationID = "526ad79c-228f-5647-83c8-c051bb6849d1"
	if os.Getenv("FF_FIXTURE_NODE_ID") == "" {
		t.Fatal("exact fixture teardown requires FF_FIXTURE_NODE_ID")
	}
	if got := os.Getenv("FF_ALLOW_EXACT_FIXTURE_TEARDOWN"); got != "yes" {
		t.Fatal("exact fixture teardown requires explicit test opt-in")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	fixtureDB := filepath.Join(root, "kandev/home/data/kandev.db")
	db, err := sqlx.Open("sqlite3", "file:"+fixtureDB+"?mode=ro&_busy_timeout=5000")
	if err != nil {
		t.Fatal("open fixture database")
	}
	defer func() { _ = db.Close() }()
	var rawMetadata string
	var runtimeID string
	if err := db.Get(&rawMetadata, "SELECT metadata FROM executors_running WHERE task_id = ? AND runtime = ?", taskID, "nomad"); err != nil {
		t.Fatal("load exact fixture runtime row")
	}
	if err := db.Get(&runtimeID, "SELECT agent_execution_id FROM executors_running WHERE task_id = ? AND runtime = ?", taskID, "nomad"); err != nil {
		t.Fatal("load exact fixture execution identity")
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(rawMetadata), &metadata); err != nil {
		t.Fatal("decode fixture metadata")
	}
	stateData, err := json.Marshal(metadata[nomad.EnvironmentStateMetadataKey])
	if err != nil {
		t.Fatal("decode fixture Nomad state")
	}
	var state nomad.EnvironmentState
	if err := json.Unmarshal(stateData, &state); err != nil {
		t.Fatal("decode fixture Nomad state")
	}
	if runtimeID != state.ExecutionID || state.AllocationID != allocationID || state.NodeID != os.Getenv("FF_FIXTURE_NODE_ID") || state.Phase != "ready" {
		t.Fatal("fixture runtime, allocation, node, or phase identity changed")
	}
	profile, err := nomadProfileFromMetadata(metadata)
	if err != nil {
		t.Fatal("decode fixture profile")
	}
	key, err := secrets.NewMasterKeyProvider(filepath.Join(root, "kandev/home/data"))
	if err != nil {
		t.Fatal("open fixture secret key")
	}
	authRef := getMetadataString(metadata, MetadataKeyAuthTokenSecret)
	if authRef == "" || profile.NomadTokenSecretID == "" {
		t.Fatal("fixture credential references are absent")
	}
	authToken, nomadToken := fixtureSecretForTeardown(t, db, key, authRef), fixtureSecretForTeardown(t, db, key, profile.NomadTokenSecretID)
	if authToken == "" || nomadToken == "" {
		t.Fatal("fixture credentials are empty")
	}
	client, err := nomad.NewClient(profile.Address, nomadToken)
	if err != nil {
		t.Fatal("configure fixture Nomad client")
	}
	defer client.CloseIdleConnections()
	if os.Getenv("FF_POST_DEREGISTER_INSPECT") == "yes" {
		fixtureInspectPostDeregister(t, ctx, root, profile, state, nomadToken, client)
		return
	}
	if os.Getenv("FF_INSPECT_ONLY") == "yes" {
		job, err := client.GetJob(ctx, state.Namespace, state.JobID)
		if err != nil || !jobMatchesState(job, state) {
			t.Fatal("immutable fixture job ownership did not match during read-only inspection")
		}
		allocations, err := client.ListAllocations(ctx, state.Namespace, state.JobID)
		if err != nil {
			t.Fatal("read fixture allocation history")
		}
		t.Logf("fixture allocation count=%d", len(allocations))
		for _, allocation := range allocations {
			t.Logf("allocation id=%s create_index=%d node_id=%s desired=%s client=%s task=%s", allocation.ID, allocation.CreateIndex, allocation.NodeID, allocation.DesiredStatus, allocation.ClientStatus, allocation.TaskStates["agentctl"].State)
		}
		return
	}
	if os.Getenv("FF_DEREGISTER_FIXTURE_JOB") == "yes" {
		teardownFixtureJobAndReleaseFence(t, ctx, root, profile, state, nomadToken)
		return
	}

	t.Fatal("teardown mode must be read-only inspection or exact whole-job deregistration")
}

func fixtureInspectPostDeregister(t *testing.T, ctx context.Context, root string, profile nomad.ProfileConfig, state nomad.EnvironmentState, nomadToken string, client *nomad.Client) {
	t.Helper()
	var jobStatus struct {
		ID          string `json:"ID"`
		Namespace   string `json:"Namespace"`
		Status      string `json:"Status"`
		Stop        bool   `json:"Stop"`
		ModifyIndex uint64 `json:"ModifyIndex"`
	}
	if err := fixtureNomadGet(ctx, profile.Address, nomadToken, "/v1/job/"+url.PathEscape(state.JobID)+"?namespace="+url.QueryEscape(state.Namespace), &jobStatus); err != nil {
		t.Fatal("read deregistered fixture job status")
	}
	if jobStatus.ID != state.JobID || jobStatus.Namespace != state.Namespace {
		t.Fatal("deregistered job identity changed")
	}
	t.Logf("job id=%s namespace=%s status=%s stop=%t modify_index=%d", jobStatus.ID, jobStatus.Namespace, jobStatus.Status, jobStatus.Stop, jobStatus.ModifyIndex)
	allocations, err := client.ListAllocations(ctx, state.Namespace, state.JobID)
	if err != nil {
		t.Fatal("read retained fixture allocation history")
	}
	t.Logf("retained fixture allocation count=%d", len(allocations))
	for _, summary := range allocations {
		full, getErr := client.GetAllocation(ctx, state.Namespace, summary.ID)
		if getErr != nil {
			t.Fatal("read retained full allocation")
		}
		t.Logf("allocation id=%s create_index=%d node_id=%s desired=%s client=%s task=%s", full.ID, full.CreateIndex, full.NodeID, full.DesiredStatus, full.ClientStatus, full.TaskStates["agentctl"].State)
		var details struct {
			TaskStates map[string]struct {
				StartedAt  time.Time `json:"StartedAt"`
				FinishedAt time.Time `json:"FinishedAt"`
				Events     []struct {
					Type     string `json:"Type"`
					Message  string `json:"Message"`
					ExitCode int    `json:"ExitCode"`
				} `json:"Events"`
			} `json:"TaskStates"`
		}
		if err := fixtureNomadGet(ctx, profile.Address, nomadToken, "/v1/allocation/"+url.PathEscape(summary.ID)+"?namespace="+url.QueryEscape(state.Namespace), &details); err != nil {
			t.Fatal("read allocation event summary")
		}
		for taskName, task := range details.TaskStates {
			for _, event := range task.Events {
				message := strings.ToLower(event.Message)
				t.Logf("task_event allocation=%s task=%s type=%s started=%t finished=%t exit_code=%d mentions_fence=%t mentions_owner_conflict=%t mentions_agentctl=%t mentions_bootstrap=%t mentions_workspace=%t mentions_permission=%t", full.ID, taskName, event.Type, !task.StartedAt.IsZero(), !task.FinishedAt.IsZero(), event.ExitCode, strings.Contains(message, "fence"), strings.Contains(message, "already exists"), strings.Contains(message, "agentctl"), strings.Contains(message, "bootstrap"), strings.Contains(message, "workspace"), strings.Contains(message, "permission"))
			}
		}
	}
	processes := fixtureScopedAgentProcessIdentities(t, root)
	for _, process := range processes {
		t.Logf("fixture_agent_process class=%s pid=%d start=%s", process.Class, process.PID, process.Start)
	}
	t.Logf("fixture_agent_process_count=%d", len(processes))
}

type fixtureScopedProcess struct {
	Class string
	PID   int
	Start string
}

func fixtureScopedAgentProcessIdentities(t *testing.T, root string) []fixtureScopedProcess {
	t.Helper()
	output, err := exec.Command("/bin/ps", "-axo", "pid=,lstart=,command=").Output()
	if err != nil {
		t.Fatal("cannot inspect fixture-scoped process identities")
	}
	var result []fixtureScopedProcess
	for _, line := range strings.Split(string(output), "\n") {
		if !strings.Contains(line, root) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}
		command := strings.ToLower(strings.Join(fields[6:], " "))
		class := ""
		switch {
		case strings.Contains(command, "agentctl"):
			class = "agentctl"
		case strings.Contains(command, "fixture-acp") || strings.Contains(command, "inert-acp"):
			class = "acp-fixture"
		default:
			continue
		}
		pid, parseErr := strconv.Atoi(fields[0])
		if parseErr != nil {
			continue
		}
		result = append(result, fixtureScopedProcess{Class: class, PID: pid, Start: strings.Join(fields[1:6], " ")})
	}
	return result
}

func fixtureNomadGet(ctx context.Context, address, token, path string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(address, "/")+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Nomad-Token", token)
	resp, err := (&http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Nomad read returned HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(target)
}

func fixtureSecretForTeardown(t *testing.T, db *sqlx.DB, key *secrets.MasterKeyProvider, id string) string {
	t.Helper()
	var row struct {
		Encrypted []byte `db:"encrypted_value"`
		Nonce     []byte `db:"nonce"`
	}
	if err := db.Get(&row, "SELECT encrypted_value, nonce FROM secrets WHERE id = ?", id); err != nil {
		t.Fatal("read fixture secret")
	}
	plain, err := secrets.Decrypt(row.Encrypted, row.Nonce, key.Key())
	if err != nil {
		t.Fatal("decrypt fixture secret")
	}
	return string(plain)
}

func teardownFixtureJobAndReleaseFence(t *testing.T, ctx context.Context, root string, profile nomad.ProfileConfig, state nomad.EnvironmentState, nomadToken string) {
	t.Helper()
	client, err := nomad.NewClient(profile.Address, nomadToken)
	if err != nil {
		t.Fatal("configure fixture Nomad client")
	}
	defer client.CloseIdleConnections()
	job, err := client.GetJob(ctx, state.Namespace, state.JobID)
	if err != nil || !jobMatchesState(job, state) {
		t.Fatal("exact job ownership did not match before deregistration")
	}
	var jobState struct {
		ID        string `json:"ID"`
		Namespace string `json:"Namespace"`
		Status    string `json:"Status"`
		Stop      bool   `json:"Stop"`
	}
	if err := fixtureNomadGet(ctx, profile.Address, nomadToken, "/v1/job/"+url.PathEscape(state.JobID)+"?namespace="+url.QueryEscape(state.Namespace), &jobState); err != nil {
		t.Fatal("read exact job state before deregistration")
	}
	if jobState.ID != state.JobID || jobState.Namespace != state.Namespace {
		t.Fatal("job state identity did not match the exact fixture")
	}
	allocations, err := client.ListAllocations(ctx, state.Namespace, state.JobID)
	if err != nil || len(allocations) != 2 {
		t.Fatal("expected the original and observed failed successor allocations")
	}
	seen := map[string]bool{}
	for _, summary := range allocations {
		full, getErr := client.GetAllocation(ctx, state.Namespace, summary.ID)
		if getErr != nil || full.ID != summary.ID || full.JobID != state.JobID || full.Namespace != state.Namespace || full.NodeID != state.NodeID || full.CreateIndex != summary.CreateIndex {
			t.Fatal("full allocation identity did not match the exact job, namespace, node, and history row")
		}
		if summary.ID != state.AllocationID && summary.ID != "dabd02c5-d45b-c1ec-39d3-91c81fe91e2a" {
			t.Fatal("job contains an allocation outside the two previously observed fixture records")
		}
		if summary.ID == state.AllocationID && (full.CreateIndex != state.AllocationCreateIndex || full.ClientStatus != "complete" || full.DesiredStatus != "stop") {
			t.Fatal("original allocation no longer matches the observed terminal record")
		}
		if summary.ID == "dabd02c5-d45b-c1ec-39d3-91c81fe91e2a" && (full.CreateIndex != 171 || full.ClientStatus != "failed" || full.DesiredStatus != "run") {
			t.Fatal("unexpected successor allocation no longer matches the observed failed record")
		}
		if full.ClientStatus != "complete" && full.ClientStatus != "failed" {
			t.Fatal("job has a nonterminal allocation; preserve fence and stop")
		}
		if full.DesiredStatus != "stop" && full.DesiredStatus != "run" {
			t.Fatal("job allocation has an unexpected desired state")
		}
		if full.TaskStates["agentctl"].State != "dead" {
			t.Fatal("job allocation still has a nonterminal agentctl task")
		}
		if summary.ID == "dabd02c5-d45b-c1ec-39d3-91c81fe91e2a" {
			var events struct {
				TaskStates map[string]struct {
					Events []struct {
						Type     string `json:"Type"`
						ExitCode int    `json:"ExitCode"`
					} `json:"Events"`
				} `json:"TaskStates"`
			}
			if err := fixtureNomadGet(ctx, profile.Address, nomadToken, "/v1/allocation/"+url.PathEscape(summary.ID)+"?namespace="+url.QueryEscape(state.Namespace), &events); err != nil {
				t.Fatal("read successor allocation task events")
			}
			fenceRejected := false
			for _, event := range events.TaskStates["agentctl"].Events {
				if event.Type == "Terminated" && event.ExitCode == 73 {
					fenceRejected = true
				}
			}
			if !fenceRejected {
				t.Fatal("successor did not show the bootstrap's existing-fence refusal")
			}
		}
		seen[full.ID] = true
	}
	if !seen[state.AllocationID] || !seen["dabd02c5-d45b-c1ec-39d3-91c81fe91e2a"] {
		t.Fatal("one of the exact allocation records is missing")
	}
	if jobState.Status != "dead" || !jobState.Stop {
		deleteURL := strings.TrimRight(profile.Address, "/") + "/v1/job/" + url.PathEscape(state.JobID) + "?namespace=" + url.QueryEscape(state.Namespace) + "&purge=false"
		deleteReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, deleteURL, nil)
		if err != nil {
			t.Fatal("build exact non-purge job deregistration")
		}
		deleteReq.Header.Set("X-Nomad-Token", nomadToken)
		deleteClient := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		deleteResp, err := deleteClient.Do(deleteReq)
		if err != nil {
			t.Fatal("deregister exact immutable job")
		}
		_ = deleteResp.Body.Close()
		if deleteResp.StatusCode < 200 || deleteResp.StatusCode >= 300 {
			t.Fatalf("job deregistration returned HTTP %d", deleteResp.StatusCode)
		}
	}
	deadline := time.Now().Add(60 * time.Second)
	var terminal []nomad.AllocationRecord
	for time.Now().Before(deadline) {
		var currentJob struct {
			ID        string `json:"ID"`
			Namespace string `json:"Namespace"`
			Status    string `json:"Status"`
			Stop      bool   `json:"Stop"`
		}
		jobErr := fixtureNomadGet(ctx, profile.Address, nomadToken, "/v1/job/"+url.PathEscape(state.JobID)+"?namespace="+url.QueryEscape(state.Namespace), &currentJob)
		current, getErr := client.ListAllocations(ctx, state.Namespace, state.JobID)
		if jobErr == nil && currentJob.ID == state.JobID && currentJob.Namespace == state.Namespace && currentJob.Status == "dead" && currentJob.Stop && getErr == nil && len(current) == 2 {
			allTerminal := true
			for _, allocation := range current {
				wantDesired := ""
				switch allocation.ID {
				case state.AllocationID:
					wantDesired = "stop"
				case "dabd02c5-d45b-c1ec-39d3-91c81fe91e2a":
					wantDesired = "run" // failed historical allocation; the immutable job is now stopped
				}
				if wantDesired == "" || (allocation.ClientStatus != "complete" && allocation.ClientStatus != "failed") || allocation.DesiredStatus != wantDesired || allocation.TaskStates["agentctl"].State != "dead" {
					allTerminal = false
				}
			}
			if allTerminal {
				terminal = current
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	if len(terminal) != 2 {
		t.Fatal("exact immutable job did not stop while retaining both known terminal allocation records")
	}
	terminalByID := map[string]nomad.AllocationRecord{}
	for _, allocation := range terminal {
		terminalByID[allocation.ID] = allocation
	}
	if terminalByID[state.AllocationID].ID == "" || terminalByID["dabd02c5-d45b-c1ec-39d3-91c81fe91e2a"].ID == "" {
		t.Fatal("job history changed after deregistration; an expected allocation is missing")
	}
	node, err := getNomadNodeStatus(ctx, profile.Address, nomadToken, state.NodeID)
	if err != nil || node != "ready" {
		t.Fatal("known worker node is not ready after exact job deregistration")
	}
	assertPIDsGoneWithSameStartTimes(t, []fixturePIDIdentity{
		{PID: 95182, Start: "Sat Oct 24 23:53:40 2026"}, {PID: 95295, Start: "Sat Oct 24 23:53:41 2026"},
		{PID: 95307, Start: "Sat Oct 24 23:53:41 2026"}, {PID: 95314, Start: "Sat Oct 24 23:53:41 2026"},
	})
	if processes := fixtureScopedAgentProcessIdentities(t, root); len(processes) != 0 {
		t.Fatal("a fixture-scoped agentctl or ACP process remains after job-wide deregistration")
	}
	for _, port := range []int{state.ControlPort, state.InstancePort} {
		conn, dialErr := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 300*time.Millisecond)
		if dialErr == nil {
			_ = conn.Close()
			t.Fatalf("reserved agentctl port %d remains open", port)
		}
	}
	sshTarget, err := ResolveSSHTarget(SSHConnConfig{Host: state.SSHHost, Port: state.SSHPort, User: state.SSHUser,
		IdentitySource: SSHIdentitySourceFile, IdentityFile: filepath.Join(root, "ssh/client_ed25519"), PinnedFingerprint: state.SSHHostFingerprint})
	if err != nil {
		t.Fatal("resolve pinned fixture worker")
	}
	sshClient, err := DialSSH(ctx, sshTarget)
	if err != nil {
		t.Fatal("connect to pinned fixture worker for exact fence release")
	}
	defer func() { _ = sshClient.Close() }()
	if err := verifyNomadWorkerFence(ctx, sshClient, profile, state); err != nil {
		t.Fatal("exact worker fence owner changed before release")
	}
	workerStateRoot := filepath.Join(filepath.Dir(filepath.Dir(profile.AllocationRoot)), "worker-state", state.NodeID)
	fenceDir := filepath.Join(workerStateRoot, "active-environment")
	fencePath := filepath.Join(fenceDir, "owner")
	owner := strings.Join([]string{state.InstallationID, state.NodeID, state.EnvironmentID, fmt.Sprint(state.Generation), state.OperationID, state.InputDigest, state.JobID}, ":") + "\n" + state.AllocationID
	cmd := "f=" + shellQuote(fencePath) + "; d=" + shellQuote(fenceDir) +
		`; [ ! -L "$d" ] && [ -d "$d" ] && [ ! -L "$f" ] && [ -f "$f" ] && [ "$(stat -f %Lp "$f")" = 600 ] && [ "$(cat "$f")" = ` + shellQuote(owner) +
		` ] || exit 78; rm "$f"; rmdir "$d"; printf fixture-fence-released`
	output, _, err := runSSHCommand(ctx, sshClient, cmd)
	if err != nil || strings.TrimSpace(output) != "fixture-fence-released" {
		t.Fatal("exact owner compare-and-delete of the fence failed")
	}
	evidence := map[string]any{"task_id": "104ce439-3805-4307-be5a-7c4e8354ba20", "session_id": state.ExecutionID,
		"environment_id": state.EnvironmentID, "generation": state.Generation, "operation_id": state.OperationID,
		"job_id": state.JobID, "node_id": state.NodeID, "runtime": "nomad", "legacy_workspace_root": state.WorkspaceRoot,
		"drain_receipt_observed_validated": true, "drain_request_id": "operator-drain-" + state.AllocationID,
		"allocations": []map[string]any{{"id": state.AllocationID, "create_index": terminalByID[state.AllocationID].CreateIndex, "desired": terminalByID[state.AllocationID].DesiredStatus, "client": terminalByID[state.AllocationID].ClientStatus},
			{"id": "dabd02c5-d45b-c1ec-39d3-91c81fe91e2a", "create_index": terminalByID["dabd02c5-d45b-c1ec-39d3-91c81fe91e2a"].CreateIndex, "desired": terminalByID["dabd02c5-d45b-c1ec-39d3-91c81fe91e2a"].DesiredStatus, "client": terminalByID["dabd02c5-d45b-c1ec-39d3-91c81fe91e2a"].ClientStatus}},
		"agentctl_and_acp_pids_same_start_absent": true, "reserved_ports_closed": true, "exact_fence_owner_compare_and_delete": true,
		"completed_at": time.Now().UTC().Format(time.RFC3339Nano)}
	evidenceBytes, _ := json.MarshalIndent(evidence, "", "  ")
	if err := os.WriteFile(filepath.Join(root, "manual-teardown-evidence.json"), append(evidenceBytes, '\n'), 0o600); err != nil {
		t.Fatal("write sanitized manual teardown evidence")
	}
	t.Log("exact immutable job deregistered without purge; both allocations terminal, processes and ports absent, exact fence released")
}

type fixturePIDIdentity struct {
	PID   int
	Start string
}

func assertPIDsGoneWithSameStartTimes(t *testing.T, identities []fixturePIDIdentity) {
	t.Helper()
	for _, identity := range identities {
		output, err := exec.Command("/bin/ps", "-o", "lstart=", "-p", strconv.Itoa(identity.PID)).Output()
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(output)) != "" && strings.HasPrefix(strings.TrimSpace(string(output)), identity.Start) {
			t.Fatalf("fixture PID %d with the original start identity remains alive", identity.PID)
		}
	}
}

func getNomadNodeStatus(ctx context.Context, address, token, nodeID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(address, "/")+"/v1/node/"+url.PathEscape(nodeID), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Nomad-Token", token)
	resp, err := (&http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("node query returned HTTP %d", resp.StatusCode)
	}
	var row struct {
		ID     string `json:"ID"`
		Status string `json:"Status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&row); err != nil {
		return "", err
	}
	if row.ID != nodeID {
		return "", fmt.Errorf("node identity mismatch")
	}
	return row.Status, nil
}
