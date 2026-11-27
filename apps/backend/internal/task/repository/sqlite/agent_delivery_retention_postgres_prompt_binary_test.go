package sqlite

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jmoiron/sqlx"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func (h *retentionPostgresBinaryHarness) requireRealPromptAndEventWrites(t *testing.T, database *sqlx.DB, workspaceID string) {
	t.Helper()
	taskID, sessionID := h.liveTaskID, h.liveSessionID
	if taskID == "" {
		taskID, sessionID = h.createRetentionLiveTask(t, workspaceID)
		h.liveTaskID, h.liveSessionID = taskID, sessionID
	}
	var previousCompleted, previousEvents int
	if err := database.Get(&previousCompleted, `SELECT COUNT(*) FROM agent_delivery_submissions WHERE session_id = $1 AND state = 'completed' AND LENGTH(payload) > 0`, sessionID); err != nil {
		t.Fatal(err)
	}
	if err := database.Get(&previousEvents, `SELECT COUNT(*) FROM agent_delivery_inbox WHERE session_id = $1 AND LENGTH(payload) > 0`, sessionID); err != nil {
		t.Fatal(err)
	}
	h.livePromptCount++
	h.retentionMessageAdd(t, taskID, sessionID)
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		var completed, events int
		if err := database.Get(&completed, `SELECT COUNT(*) FROM agent_delivery_submissions WHERE session_id = $1 AND state = 'completed' AND harness_generation > 0 AND owner_generation > 0 AND LENGTH(payload) > 0`, sessionID); err != nil {
			t.Fatal(err)
		}
		if err := database.Get(&events, `SELECT COUNT(*) FROM agent_delivery_inbox WHERE session_id = $1 AND harness_generation > 0 AND LENGTH(payload) > 0`, sessionID); err != nil {
			t.Fatal(err)
		}
		if completed > previousCompleted && events > previousEvents {
			var outputs int
			if err := database.Get(&outputs, `SELECT COUNT(*) FROM task_session_messages WHERE task_session_id = $1 AND content LIKE '%Slow response complete after 1s.%'`, sessionID); err != nil {
				t.Fatal(err)
			}
			if outputs != h.livePromptCount {
				t.Fatalf("mock provider completion count: %d, want exactly %d; possible resend", outputs, h.livePromptCount)
			}
			t.Logf("PostgreSQL live mock transport: %d completed prompt, %d authenticated event rows", completed, events)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	h.logLiveOwnerEvidence(t, database)
	h.preserveFailedJournalFrame(t)
	h.retentionWSRequest(t, "session.stop", map[string]any{"session_id": sessionID})
	t.Fatal("real backend mock prompt did not reach completed delivery and event rows")
}

func (h *retentionPostgresBinaryHarness) createRetentionLiveTask(t *testing.T, workspaceID string) (string, string) {
	t.Helper()
	profileID, executorID := h.retentionExecutionProfiles(t)
	repoPath := filepath.Join(h.home, "repos", "prompt-writer")
	if err := os.MkdirAll(repoPath, 0700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"-c", "user.name=Retention", "-c", "user.email=retention@example.invalid", "commit", "--allow-empty", "-m", "fixture"}} {
		command := exec.Command("git", args...)
		command.Dir = repoPath
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git fixture: %v %s", err, out)
		}
	}
	repositoryID := retentionResponseID(t, h.request(t, http.MethodPost, "/api/v1/workspaces/"+workspaceID+"/repositories", map[string]any{"name": "Retention prompt writer", "source_type": "local", "local_path": repoPath, "default_branch": "main", "pull_before_worktree": false}, true))
	workflowID := retentionResponseID(t, h.request(t, http.MethodPost, "/api/v1/workflows", map[string]any{"workspace_id": workspaceID, "name": "Retention prompt writer", "workflow_template_id": "simple"}, true))
	var steps struct {
		Steps []struct {
			ID    string `json:"id"`
			Start bool   `json:"is_start_step"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(h.request(t, http.MethodGet, "/api/v1/workflows/"+workflowID+"/workflow/steps", nil, true), &steps); err != nil {
		t.Fatal(err)
	}
	stepID := ""
	for _, step := range steps.Steps {
		if step.Start {
			stepID = step.ID
			break
		}
	}
	if stepID == "" && len(steps.Steps) > 0 {
		stepID = steps.Steps[0].ID
	}
	if stepID == "" {
		t.Fatal("no workflow start step")
	}
	taskID := retentionResponseID(t, h.request(t, http.MethodPost, "/api/v1/tasks", map[string]any{"workspace_id": workspaceID, "title": "Release N actual PostgreSQL prompt", "start_agent": false, "agent_profile_id": profileID, "workflow_id": workflowID, "workflow_step_id": stepID, "repository_ids": []string{repositoryID}, "executor_profile_id": executorID}, true))
	var prepared struct {
		SessionID string `json:"session_id"`
		Success   bool   `json:"success"`
	}
	payload := h.retentionWSRequest(t, "session.launch", map[string]any{"task_id": taskID, "intent": "prepare", "agent_profile_id": profileID, "executor_profile_id": executorID, "launch_workspace": true})
	if err := json.Unmarshal(payload, &prepared); err != nil {
		t.Fatal(err)
	}
	if !prepared.Success || prepared.SessionID == "" {
		t.Fatalf("session prepare did not succeed: %s", payload)
	}
	return taskID, prepared.SessionID

}

func (h *retentionPostgresBinaryHarness) retentionExecutionProfiles(t *testing.T) (string, string) {
	t.Helper()
	var agents struct {
		Agents []struct {
			Name     string `json:"name"`
			Profiles []struct {
				ID string `json:"id"`
			} `json:"profiles"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(h.request(t, http.MethodGet, "/api/v1/agents", nil, true), &agents); err != nil {
		t.Fatal(err)
	}
	profileID := ""
	for _, agent := range agents.Agents {
		if agent.Name == "mock-agent" && len(agent.Profiles) > 0 {
			profileID = agent.Profiles[0].ID
			break
		}
	}
	if profileID == "" {
		t.Fatal("mock agent profile missing from real N")
	}
	var executors struct {
		Executors []struct {
			Type     string `json:"type"`
			Profiles []struct {
				ID string `json:"id"`
			} `json:"profiles"`
		} `json:"executors"`
	}
	if err := json.Unmarshal(h.request(t, http.MethodGet, "/api/v1/executors", nil, true), &executors); err != nil {
		t.Fatal(err)
	}
	for _, executor := range executors.Executors {
		if executor.Type == "worktree" && len(executor.Profiles) > 0 {
			return profileID, executor.Profiles[0].ID
		}
	}
	t.Fatal("worktree executor profile missing from real N")
	return "", ""
}

func (h *retentionPostgresBinaryHarness) retentionMessageAdd(t *testing.T, taskID, sessionID string) {
	t.Helper()
	h.retentionWSRequest(t, "message.add", map[string]any{"task_id": taskID, "session_id": sessionID, "client_message_id": uuid.NewString(), "content": "/slow 1s"})
}

func (h *retentionPostgresBinaryHarness) retentionWSRequest(t *testing.T, action string, payload any) json.RawMessage {
	t.Helper()
	header := http.Header{"Origin": []string{h.baseURL}}
	connection, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(h.baseURL, "http")+"/ws", header)
	if err != nil {
		if response != nil {
			_ = response.Body.Close()
		}
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()
	if err := connection.SetReadDeadline(time.Now().Add(120 * time.Second)); err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	request, err := ws.NewRequest(id, action, payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.WriteJSON(request); err != nil {
		t.Fatal(err)
	}
	for {
		var response ws.Message
		if err := connection.ReadJSON(&response); err != nil {
			t.Fatal(err)
		}
		if response.ID != id {
			continue
		}
		if response.Type == ws.MessageTypeError {
			t.Fatalf("real binary %s error: %s", action, response.Payload)
		}
		if response.Type != ws.MessageTypeResponse {
			t.Fatal(fmt.Errorf("unexpected response type %s", response.Type))
		}
		return response.Payload
	}
}

func retentionResponseID(t *testing.T, data []byte) string {
	t.Helper()
	var response struct {
		ID   string `json:"id"`
		Task struct {
			ID string `json:"id"`
		} `json:"task"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "" {
		return response.ID
	}
	if response.Task.ID != "" {
		return response.Task.ID
	}
	t.Fatalf("response has no ID: %s", data)
	return ""
}

func (h *retentionPostgresBinaryHarness) ageSettledLivePrompt(t *testing.T, database *sqlx.DB) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		err := database.Get(&h.liveSubmissionID, `SELECT id FROM agent_delivery_submissions WHERE session_id = $1 AND fully_settled_at IS NOT NULL ORDER BY created_at LIMIT 1`, h.liveSessionID)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if h.liveSubmissionID == "" {
		t.Fatal("N+1 live prompt has no normal terminal/workflow settlement proof")
	}
	if _, err := database.Exec(`UPDATE agent_delivery_inbox SET projected_at = CURRENT_TIMESTAMP - INTERVAL '48 hours' WHERE session_id = $1 AND projected_at IS NOT NULL`, h.liveSessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE agent_delivery_submissions SET fully_settled_at = CURRENT_TIMESTAMP - INTERVAL '48 hours' WHERE id = $1`, h.liveSubmissionID); err != nil {
		t.Fatal(err)
	}
}

func (h *retentionPostgresBinaryHarness) capturePrunedLiveEvidence(t *testing.T, database *sqlx.DB) {
	t.Helper()
	var payloadLength int
	if err := database.QueryRow(`SELECT LENGTH(payload), row_to_json(s)::text FROM agent_delivery_submissions s WHERE id = $1 AND payload_pruned_at IS NOT NULL`, h.liveSubmissionID).Scan(&payloadLength, &h.liveSummary); err != nil || payloadLength != 0 {
		t.Fatalf("real live prompt not pruned: %d %v", payloadLength, err)
	}
	if err := database.QueryRow(`SELECT COALESCE(MAX(projected_sequence),0),COALESCE(MAX(retired_sequence),0) FROM agent_delivery_cursors WHERE session_id = $1`, h.liveSessionID).Scan(&h.liveProjectedSequence, &h.liveRetiredSequence); err != nil || h.liveProjectedSequence == 0 {
		t.Fatalf("real live projected checkpoint missing: %d %v", h.liveProjectedSequence, err)
	}
	h.assertProtectedNativePrefix(t, database)
	t.Logf("N+1 real mock prompt/canonical payloads pruned; native projected=%d retired=%d with protected bootstrap prefix; manual stream has actual terminal retirement; compact submission=%s", h.liveProjectedSequence, h.liveRetiredSequence, h.liveSubmissionID)
}

func (h *retentionPostgresBinaryHarness) assertPrunedLiveEvidence(t *testing.T, database *sqlx.DB) {
	t.Helper()
	var summary string
	if err := database.Get(&summary, `SELECT row_to_json(s)::text FROM agent_delivery_submissions s WHERE id = $1`, h.liveSubmissionID); err != nil {
		t.Fatal(err)
	}
	if summary != h.liveSummary {
		t.Fatalf("supported rollback/forward mutated compact live prompt summary\nbefore: %s\nafter: %s", h.liveSummary, summary)
	}
	var projected, retired int64
	if err := database.QueryRow(`SELECT COALESCE(MAX(projected_sequence),0),COALESCE(MAX(retired_sequence),0) FROM agent_delivery_cursors WHERE session_id = $1`, h.liveSessionID).Scan(&projected, &retired); err != nil || projected < h.liveProjectedSequence || retired != h.liveRetiredSequence {
		t.Fatalf("native checkpoint changed inconsistently: projected=%d retired=%d %v", projected, retired, err)
	}
	h.assertProtectedNativePrefix(t, database)
	history := h.request(t, http.MethodGet, "/api/v1/task-sessions/"+h.liveSessionID+"/messages", nil, true)
	if !strings.Contains(string(history), "Slow response complete after 1s.") {
		t.Fatalf("real live canonical output missing: %s", history)
	}
}

func (h *retentionPostgresBinaryHarness) captureProtectedNativePrefix(t *testing.T, database *sqlx.DB) {
	t.Helper()
	if err := database.Get(&h.liveProtectedThrough, `SELECT MIN(sequence)-1 FROM agent_delivery_inbox WHERE session_id = $1 AND event_type IN ('message_chunk','reasoning')`, h.liveSessionID); err != nil || h.liveProtectedThrough <= 0 {
		t.Fatalf("native bootstrap prefix missing: %d %v", h.liveProtectedThrough, err)
	}
	h.liveProtectedPrefix = h.protectedNativePrefix(t, database)
}

func (h *retentionPostgresBinaryHarness) protectedNativePrefix(t *testing.T, database *sqlx.DB) string {
	t.Helper()
	var rows string
	if err := database.Get(&rows, `SELECT COALESCE(json_agg(row_to_json(record) ORDER BY sequence)::text, '[]') FROM (SELECT * FROM agent_delivery_inbox WHERE session_id = $1 AND sequence <= $2) record`, h.liveSessionID, h.liveProtectedThrough); err != nil {
		t.Fatal(err)
	}
	return rows
}

func (h *retentionPostgresBinaryHarness) assertProtectedNativePrefix(t *testing.T, database *sqlx.DB) {
	t.Helper()
	if after := h.protectedNativePrefix(t, database); after != h.liveProtectedPrefix {
		t.Fatalf("native protected bootstrap prefix changed\nbefore: %s\nafter: %s", h.liveProtectedPrefix, after)
	}
}

func (h *retentionPostgresBinaryHarness) logLiveOwnerEvidence(t *testing.T, database *sqlx.DB) {
	t.Helper()
	queries := []string{
		`SELECT id,session_id,task_id,agent_execution_id,status,resumable FROM executors_running WHERE session_id = $1`,
		`SELECT session_id,incarnation_id,generation,predecessor_generation,native_session_id,created_at,committed_at FROM harness_session_generations WHERE session_id = $1`,
		`SELECT stream_id,session_id,incarnation_id,harness_generation,received_sequence,projected_sequence,retired_sequence FROM agent_delivery_cursors WHERE session_id = $1`,
		`SELECT id,session_id,incarnation_id,harness_generation,owner_generation,state,terminal_stream_id,terminal_sequence,terminal_turn_id FROM agent_delivery_submissions WHERE session_id = $1`,
	}
	for _, query := range queries {
		var rows string
		if err := database.Get(&rows, `SELECT COALESCE(json_agg(row_to_json(record))::text,'[]') FROM (`+query+`) record`, h.liveSessionID); err != nil {
			t.Logf("live owner diagnostic failed: %v", err)
		} else {
			t.Logf("live owner evidence: %s", rows)
		}
	}
}
