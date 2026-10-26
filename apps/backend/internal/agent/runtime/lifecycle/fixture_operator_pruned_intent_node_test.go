package lifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/nomad"
	_ "github.com/mattn/go-sqlite3"
)

func TestPrunedIntentCleanupMetadataJSONRoundTrip(t *testing.T) {
	input := prunedIntentCleanupInput{
		TaskID: "task", SessionID: "session", ExecutionID: "operation", ExecutorID: "executor",
		ExecutorProfileID: "profile", EnvironmentID: "environment", Generation: 1,
		ExpectedJobID: "job", ExpectedAllocationID: "allocation", NodeID: "node",
	}
	claimID, claimBinding := newFixtureCleanupClaim(input)
	marker := map[string]any{"cleanup_claim_id": claimID, "cleanup_claim_binding_sha256": claimBinding, "auth_token_reference_persisted": true}
	const authRef = "$kandev-secret:operation:agentctl-auth"
	metadata := map[string]any{
		"fixture_operator_pruned_restore": marker,
		MetadataKeyAuthTokenSecret:        authRef,
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal("marshal cleanup metadata")
	}
	var roundTrip map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatal("unmarshal cleanup metadata")
	}
	var decodedMarker map[string]any
	if err := json.Unmarshal(roundTrip["fixture_operator_pruned_restore"], &decodedMarker); err != nil || verifyFixtureCleanupClaim(input, decodedMarker) != nil {
		t.Fatal("cleanup marker must round-trip as a bound JSON object, not a base64 byte string")
	}
	var decodedRef string
	if err := json.Unmarshal(roundTrip[MetadataKeyAuthTokenSecret], &decodedRef); err != nil || decodedRef != authRef {
		t.Fatal("auth reference must round-trip as its original JSON string")
	}

	// Exercise the exact repair CAS against SQLite's DATETIME affinity and the
	// real executor-row column names. This is isolated and never contacts Nomad
	// or attempts a nonce handshake.
	{
		db, err := sqlx.Open("sqlite3", filepath.Join(t.TempDir(), "repair.db"))
		if err != nil {
			t.Fatal("open isolated repair database")
		}
		defer func() { _ = db.Close() }()
		ctx := context.Background()
		if _, err := db.Exec(`CREATE TABLE executors_running (
		id TEXT, session_id TEXT, task_id TEXT, agent_execution_id TEXT,
		executor_id TEXT, runtime TEXT, status TEXT, metadata TEXT, updated_at DATETIME,
		PRIMARY KEY(id, runtime));
		CREATE TABLE task_environments (
			id TEXT PRIMARY KEY, task_id TEXT, executor_type TEXT, ownership_generation INTEGER,
			status TEXT, executor_id TEXT, executor_profile_id TEXT
		);`); err != nil {
			t.Fatal("create isolated executor schema")
		}
		input := prunedIntentCleanupInput{
			TaskID: "task-1", SessionID: "session-1", ExecutionID: "operation-1", ExecutorID: "executor-1",
			ExecutorProfileID: "profile-1", EnvironmentID: "environment-1", Generation: 3,
		}
		claimID, claimBinding := newFixtureCleanupClaim(input)
		marker := map[string]any{"cleanup_claim_id": claimID, "cleanup_claim_binding_sha256": claimBinding, "auth_token_reference_persisted": true}
		markerBytes, _ := json.Marshal(marker)
		const repairAuthRef = "kandev-runtime:operation-1:agentctl-auth"
		authRefJSON, _ := json.Marshal(repairAuthRef)
		brokenMetadata, err := json.Marshal(map[string]any{
			"fixture_operator_pruned_restore": markerBytes,
			MetadataKeyAuthTokenSecret:        authRefJSON,
			"unrelated":                       map[string]any{"preserve": true},
		})
		if err != nil {
			t.Fatal("encode pre-repair metadata")
		}
		const stamp = "2026-10-24 21:59:51.558895+00:00"
		if _, err := db.ExecContext(ctx, `INSERT INTO executors_running
		(id,session_id,task_id,agent_execution_id,executor_id,runtime,status,metadata,updated_at)
		VALUES(?,?,?,?,?,'nomad','stopped',?,?)`, input.SessionID, input.SessionID, input.TaskID, input.ExecutionID, input.ExecutorID, string(brokenMetadata), stamp); err != nil {
			t.Fatal("insert exact pre-repair row")
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO task_environments
		(id,task_id,executor_type,ownership_generation,status,executor_id,executor_profile_id)
		VALUES(?,?,'nomad',?,'creating',?,?)`, input.EnvironmentID, input.TaskID, input.Generation, input.ExecutorID, input.ExecutorProfileID); err != nil {
			t.Fatal("insert exact environment owner")
		}
		row := fixtureOperatorRow{
			ID: input.SessionID, SessionID: input.SessionID, TaskID: input.TaskID,
			ExecutionID: input.ExecutionID, ExecutorID: input.ExecutorID, Runtime: "nomad", Status: "stopped",
			Metadata: string(brokenMetadata), UpdatedAt: stamp,
		}
		var oldMetadata map[string]json.RawMessage
		if err := json.Unmarshal([]byte(row.Metadata), &oldMetadata); err != nil {
			t.Fatal("decode pre-repair metadata")
		}
		decodedMarker = map[string]any{}
		if err := decodeFixturePrunedMarker(oldMetadata["fixture_operator_pruned_restore"], &decodedMarker); err != nil || verifyFixtureCleanupClaim(input, decodedMarker) != nil {
			t.Fatal("decode exact original cleanup claim")
		}
		decodedRef, err = decodeFixtureSecretReference(oldMetadata[MetadataKeyAuthTokenSecret])
		if err != nil || decodedRef != repairAuthRef {
			t.Fatal("decode exact original auth reference")
		}
		fixedMetadata, err := json.Marshal(map[string]any{
			"fixture_operator_pruned_restore": decodedMarker,
			MetadataKeyAuthTokenSecret:        decodedRef,
			"unrelated":                       map[string]any{"preserve": true},
		})
		if err != nil {
			t.Fatal("encode corrected metadata")
		}
		if err := repairPrunedIntentMetadataCAS(ctx, db, row, input, string(fixedMetadata)); err != nil {
			t.Fatalf("repair exact SQLite row: %v", err)
		}
		repaired, err := readFixtureOperatorRow(ctx, db, input.TaskID)
		if err != nil || repaired.ExecutionID != input.ExecutionID || repaired.Status != "stopped" {
			t.Fatal("repair changed operation identity or lifecycle state")
		}
		var verified map[string]json.RawMessage
		if err := json.Unmarshal([]byte(repaired.Metadata), &verified); err != nil {
			t.Fatal("decode repaired metadata")
		}
		var verifiedMarker map[string]any
		if json.Unmarshal(verified["fixture_operator_pruned_restore"], &verifiedMarker) != nil || verifyFixtureCleanupClaim(input, verifiedMarker) != nil {
			t.Fatal("repaired marker is not a bound nested object")
		}
		var verifiedRef string
		if json.Unmarshal(verified[MetadataKeyAuthTokenSecret], &verifiedRef) != nil || verifiedRef != repairAuthRef {
			t.Fatal("repaired auth reference is not the original string")
		}
		if string(verified["unrelated"]) != `{"preserve":true}` {
			t.Fatal("repair did not preserve unrelated metadata")
		}
	}
}

func persistFixturePrunedDrainReceipt(ctx context.Context, db *sqlx.DB, row fixtureOperatorRow, input prunedIntentCleanupInput, receiptDigest string) error {
	if receiptDigest == "" || row.Metadata == "" || row.UpdatedAt == "" {
		return errors.New("cleanup receipt checkpoint is incomplete")
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal([]byte(row.Metadata), &metadata); err != nil {
		return err
	}
	var marker map[string]any
	if err := json.Unmarshal(metadata["fixture_operator_pruned_restore"], &marker); err != nil {
		return errors.New("cleanup claim metadata is missing")
	}
	if err := verifyFixtureCleanupClaim(input, marker); err != nil {
		return err
	}
	claimBinding, _ := marker["cleanup_claim_binding_sha256"].(string)
	marker["drain_receipt_digest"] = receiptDigest
	marker["cleanup_receipt_binding_sha256"] = fixtureCleanupReceiptBinding(claimBinding, receiptDigest)
	marker["drain_receipt_persisted_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	metadata["fixture_operator_pruned_restore"], _ = json.Marshal(marker)
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	result, err := db.ExecContext(ctx, `UPDATE executors_running SET metadata = ?, updated_at = ? WHERE id = ? AND session_id = ? AND task_id = ? AND agent_execution_id = ? AND executor_id = ? AND runtime = 'nomad' AND status = 'stopped' AND updated_at = ? AND metadata = ?`, string(encoded), time.Now().UTC(), row.ID, row.SessionID, row.TaskID, row.ExecutionID, row.ExecutorID, row.UpdatedAt, row.Metadata)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return errors.New("drain receipt CAS did not bind to the exact cleanup claim row")
	}
	return nil
}

func quarantinePrunedIntentNode(ctx context.Context, address, token, nodeID, jobID string) error {
	base := strings.TrimRight(address, "/")
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	get := func() (struct{ ID, Status, SchedulingEligibility string }, error) {
		var node struct {
			ID                    string `json:"ID"`
			Status                string `json:"Status"`
			SchedulingEligibility string `json:"SchedulingEligibility"`
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/node/"+url.PathEscape(nodeID), nil)
		if err != nil {
			return struct{ ID, Status, SchedulingEligibility string }{}, err
		}
		req.Header.Set("X-Nomad-Token", token)
		resp, err := client.Do(req)
		if err != nil {
			return struct{ ID, Status, SchedulingEligibility string }{}, err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return struct{ ID, Status, SchedulingEligibility string }{}, fmt.Errorf("node read returned HTTP %d", resp.StatusCode)
		}
		if err := json.NewDecoder(resp.Body).Decode(&node); err != nil {
			return struct{ ID, Status, SchedulingEligibility string }{}, err
		}
		if node.SchedulingEligibility == "" {
			node.SchedulingEligibility = "eligible"
		}
		return struct{ ID, Status, SchedulingEligibility string }{node.ID, node.Status, node.SchedulingEligibility}, nil
	}
	node, err := get()
	if err != nil || node.ID != nodeID || node.Status != "ready" || (node.SchedulingEligibility != "eligible" && node.SchedulingEligibility != "ineligible") {
		return errors.New("dedicated fixture node identity/status/eligibility did not verify")
	}
	// Do not quarantine a host that currently carries another workload.
	var allocations []struct {
		ID            string `json:"ID"`
		JobID         string `json:"JobID"`
		ClientStatus  string `json:"ClientStatus"`
		DesiredStatus string `json:"DesiredStatus"`
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/node/"+url.PathEscape(nodeID)+"/allocations?namespace=*", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Nomad-Token", token)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return fmt.Errorf("node allocation read returned HTTP %d", resp.StatusCode)
	}
	err = json.NewDecoder(resp.Body).Decode(&allocations)
	_ = resp.Body.Close()
	if err != nil {
		return err
	}
	for _, allocation := range allocations {
		terminal := allocation.ClientStatus == "complete" || allocation.ClientStatus == "failed"
		if allocation.JobID != jobID && !terminal {
			return errors.New("fixture node has a foreign nonterminal allocation")
		}
	}
	if node.SchedulingEligibility == "eligible" {
		payload, _ := json.Marshal(map[string]string{"NodeID": nodeID, "Eligibility": "ineligible"})
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, base+"/v1/node/"+url.PathEscape(nodeID)+"/eligibility", bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Nomad-Token", token)
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("node quarantine returned HTTP %d", resp.StatusCode)
		}
	}
	verified, err := get()
	if err != nil || verified.ID != nodeID || verified.Status != "ready" || verified.SchedulingEligibility != "ineligible" {
		return errors.New("fixture node did not remain ready and ineligible after quarantine")
	}
	return nil
}

func readPrunedIntentNodeStatus(ctx context.Context, address, token, nodeID string) (string, string, error) {
	base := strings.TrimRight(address, "/")
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/node/"+url.PathEscape(nodeID), nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("X-Nomad-Token", token)
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("node read returned HTTP %d", resp.StatusCode)
	}
	var node struct {
		ID                    string `json:"ID"`
		Status                string `json:"Status"`
		SchedulingEligibility string `json:"SchedulingEligibility"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&node); err != nil {
		return "", "", err
	}
	if node.ID != nodeID {
		return "", "", errors.New("node identity does not match captured worker")
	}
	if node.SchedulingEligibility == "" {
		node.SchedulingEligibility = "eligible"
	}
	return node.Status, node.SchedulingEligibility, nil
}

func nomadStateFromFixtureRow(t *testing.T, row fixtureOperatorRow) nomad.EnvironmentState {
	t.Helper()
	var metadata map[string]any
	if err := json.Unmarshal([]byte(row.Metadata), &metadata); err != nil {
		t.Fatal("decode restored fixture metadata")
	}
	state, err := nomad.EnvironmentStateFromMetadata(metadata)
	if err != nil {
		t.Fatal("decode restored cleanup state")
	}
	return state
}

func writeFixtureJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	if _, err := os.Lstat(path); err == nil {
		t.Fatalf("refuse to overwrite fixture evidence %q", filepath.Base(path))
	} else if !os.IsNotExist(err) {
		t.Fatal("inspect fixture evidence path")
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal("encode sanitized fixture evidence")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal("create private fixture evidence")
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		_ = file.Close()
		t.Fatal("write private fixture evidence")
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		t.Fatal("sync private fixture evidence")
	}
	if err := file.Close(); err != nil {
		t.Fatal("close private fixture evidence")
	}
}
