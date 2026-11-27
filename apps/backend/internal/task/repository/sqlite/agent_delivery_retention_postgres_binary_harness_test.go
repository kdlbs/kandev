package sqlite

import (
	"bytes"
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
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	bolt "go.etcd.io/bbolt"
)

type retentionPostgresBinaryHarness struct {
	source                        *url.URL
	baseURL, home, container      string
	port                          int
	liveTaskID, liveSessionID     string
	liveSubmissionID, liveSummary string
	liveRetiredSequence           int64
	liveProjectedSequence         int64
	liveProtectedThrough          int64
	liveProtectedPrefix           string
	livePromptCount               int
	preserveAgents                bool
}

func newRetentionPostgresBinaryHarness(t *testing.T, dsn string) *retentionPostgresBinaryHarness {
	t.Helper()
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Scheme != "postgres" {
		t.Fatal("rollback DSN must be a PostgreSQL URL")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	container := os.Getenv("KANDEV_RETENTION_POSTGRES_CONTAINER")
	if container == "" {
		t.Fatal("set KANDEV_RETENTION_POSTGRES_CONTAINER for verified dump/restore")
	}
	harness := &retentionPostgresBinaryHarness{source: parsed, home: t.TempDir(), port: port, baseURL: fmt.Sprintf("http://127.0.0.1:%d", port), container: container, preserveAgents: true}
	databaseName := fmt.Sprintf("retention_binary_%d_%d", os.Getpid(), time.Now().UnixNano())
	harness.dockerCommand(t, "psql", "-U", parsed.User.Username(), "-d", "postgres", "-v", "ON_ERROR_STOP=1", "-c", "CREATE DATABASE "+databaseName)
	parsed.Path = "/" + databaseName
	t.Cleanup(func() {
		harness.dockerCommand(t, "psql", "-U", parsed.User.Username(), "-d", "postgres", "-v", "ON_ERROR_STOP=1", "-c", "DROP DATABASE "+databaseName+" WITH (FORCE)")
	})
	t.Cleanup(func() {
		if !harness.preserveAgents || harness.liveSessionID == "" {
			return
		}
		harness.preserveAgents = false
		stop := harness.start(t, os.Getenv("KANDEV_RETENTION_CURRENT_BINARY"))
		stop()
	})
	return harness
}

func (h *retentionPostgresBinaryHarness) start(t *testing.T, binary string) func() {
	t.Helper()
	if binary == "" {
		t.Fatal("missing compatibility binary")
	}
	ctx, cancel := context.WithCancel(context.Background())
	command := exec.CommandContext(ctx, binary, "__backend")
	command.Dir = h.home
	password, _ := h.source.User.Password()
	overrides := map[string]string{"PATH": filepath.Dir(os.Getenv("KANDEV_RETENTION_CURRENT_BINARY")) + string(os.PathListSeparator) + os.Getenv("PATH"), "HOME": h.home, "KANDEV_HOME_DIR": h.home, "KANDEV_WORKTREE_BASEPATH": filepath.Join(h.home, "worktrees"), "KANDEV_REPOCLONE_BASEPATH": filepath.Join(h.home, "repo-clones"), "KANDEV_E2E_MOCK": "true", "KANDEV_FEATURES_AGENT_SURVIVAL": fmt.Sprint(h.preserveAgents), "KANDEV_SERVER_HOST": "127.0.0.1", "KANDEV_SERVER_PORT": fmt.Sprint(h.port), "KANDEV_DATABASE_DRIVER": "postgres", "KANDEV_DATABASE_HOST": h.source.Hostname(), "KANDEV_DATABASE_PORT": h.source.Port(), "KANDEV_DATABASE_USER": h.source.User.Username(), "KANDEV_DATABASE_PASSWORD": password, "KANDEV_DATABASE_DBNAME": strings.TrimPrefix(h.source.Path, "/"), "KANDEV_DATABASE_SSLMODE": "disable", "KANDEV_DOCKER_ENABLED": "false", "KANDEV_TASK_DELIVERY_RETENTION_INTERVAL": "1h", "KANDEV_LOG_LEVEL": "warn"}
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if _, exists := overrides[key]; !exists && !strings.HasPrefix(key, "KANDEV_DATABASE_") && !strings.HasPrefix(key, "KANDEV_FEATURES_") {
			command.Env = append(command.Env, value)
		}
	}
	for key, value := range overrides {
		command.Env = append(command.Env, key+"="+value)
	}
	logfile, err := os.CreateTemp(h.home, "backend-*.log")
	if err != nil {
		t.Fatal(err)
	}
	command.Stdout, command.Stderr = logfile, logfile
	if err := command.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		_ = command.Process.Signal(syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			cancel()
			<-done
		}
		cancel()
		_ = logfile.Close()
		h.preserveBinaryLogs(t, binary, logfile.Name())
		if t.Failed() {
			h.preserveFailedJournalFrame(t)
		}
	}
	t.Cleanup(stop)
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			stopped = true
			cancel()
			_ = logfile.Close()
			data, _ := os.ReadFile(logfile.Name())
			t.Fatalf("binary startup: %v\n%s", err, data)
		default:
		}
		client := http.Client{Timeout: time.Second}
		response, err := client.Get(h.baseURL + "/ready")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return stop
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	stop()
	data, _ := os.ReadFile(logfile.Name())
	t.Fatalf("backend readiness timeout\n%s", data)
	return stop
}

func (h *retentionPostgresBinaryHarness) request(t *testing.T, method, path string, body any, wantSuccess bool) []byte {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(method, h.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	success := response.StatusCode >= 200 && response.StatusCode < 300
	if success != wantSuccess {
		t.Fatalf("%s %s status %d: %s", method, path, response.StatusCode, data)
	}
	return data
}

func (h *retentionPostgresBinaryHarness) requireHistory(t *testing.T, sessionID string) {
	t.Helper()
	body := h.request(t, http.MethodGet, "/api/v1/task-sessions/"+sessionID+"/messages", nil, true)
	if !bytes.Contains(body, []byte("byte-exact PostgreSQL saved output")) {
		t.Fatalf("saved history missing: %s", body)
	}
}

func (h *retentionPostgresBinaryHarness) requireNewSessionAndRetiredIDFence(t *testing.T, taskID string) {
	t.Helper()
	created := h.request(t, http.MethodPost, "/api/v1/_test/task-sessions", map[string]any{"task_id": taskID, "state": "WAITING_FOR_INPUT", "session_id": "postgres-new-session"}, true)
	if !bytes.Contains(created, []byte("postgres-new-session")) {
		t.Fatalf("new session identity missing: %s", created)
	}
	rejection := h.request(t, http.MethodPost, "/api/v1/_test/task-sessions", map[string]any{"task_id": taskID, "state": "WAITING_FOR_INPUT", "session_id": "postgres-retired-session"}, false)
	if !strings.Contains(strings.ToLower(string(rejection)), "retir") {
		t.Fatalf("retired identity rejected without retirement evidence: %s", rejection)
	}
	h.request(t, http.MethodDelete, "/api/v1/_test/task-sessions/postgres-new-session", nil, true)
}

func (h *retentionPostgresBinaryHarness) backupAndRestore(t *testing.T, sourceDatabase *sqlx.DB, before string) {
	t.Helper()
	liveBefore := retentionPostgresLiveEvidence(t, sourceDatabase, h.liveSessionID)
	sourceDB := strings.TrimPrefix(h.source.Path, "/")
	restoreDB := sourceDB + "_restore"
	h.dockerCommand(t, "psql", "-U", h.source.User.Username(), "-d", "postgres", "-v", "ON_ERROR_STOP=1", "-c", "CREATE DATABASE "+restoreDB)
	t.Cleanup(func() {
		h.dockerCommand(t, "psql", "-U", h.source.User.Username(), "-d", "postgres", "-v", "ON_ERROR_STOP=1", "-c", "DROP DATABASE "+restoreDB+" WITH (FORCE)")
	})
	dumpPath := filepath.Join(h.home, "pre-pruning.dump")
	if external := os.Getenv("KANDEV_RETENTION_BACKUP_OUTPUT"); external != "" {
		dumpPath = external
	}
	command := exec.Command("docker", "exec", h.container, "pg_dump", "-U", h.source.User.Username(), "-d", sourceDB, "-Fc")
	dump, err := command.Output()
	if err != nil {
		t.Fatalf("pg_dump: %v", err)
	}
	if len(dump) < 10000 {
		t.Fatalf("populated backup unexpectedly small: %d", len(dump))
	}
	if err := os.WriteFile(dumpPath, dump, 0600); err != nil {
		t.Fatal(err)
	}
	restore := exec.Command("docker", "exec", "-i", h.container, "pg_restore", "-U", h.source.User.Username(), "-d", restoreDB, "--exit-on-error", "--single-transaction")
	restore.Stdin = bytes.NewReader(dump)
	if out, err := restore.CombinedOutput(); err != nil {
		t.Fatalf("pg_restore: %v %s", err, out)
	}
	restoredURL := *h.source
	restoredURL.Path = "/" + restoreDB
	raw, err := db.OpenPostgres(restoredURL.String(), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	database := sqlx.NewDb(raw, "pgx")
	defer func() { _ = database.Close() }()
	if restored := retentionPostgresEvidence(t, database); restored != before {
		t.Fatalf("restored populated backup differs\nbefore: %s\nafter: %s", before, restored)
	}
	if restored := retentionPostgresLiveEvidence(t, database, h.liveSessionID); restored != liveBefore {
		t.Fatalf("restored real live prompt/event/canonical evidence differs\nbefore: %s\nafter: %s", liveBefore, restored)
	}
	t.Logf("verified pre-pruning backup: %d bytes; canonical output, raw delivery payloads, cursors, effects, uncertain prompt, recovery guard and pending retirement job match restored database", len(dump))
}

func (h *retentionPostgresBinaryHarness) dockerCommand(t *testing.T, args ...string) {
	t.Helper()
	command := exec.Command("docker", append([]string{"exec", h.container}, args...)...)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("database fixture command: %v %s", err, out)
	}
}

func retentionPostgresLiveEvidence(t *testing.T, database *sqlx.DB, sessionID string) string {
	t.Helper()
	var result string
	for _, table := range []string{"task_session_messages", "agent_delivery_inbox", "agent_delivery_cursors", "agent_delivery_effects", "agent_delivery_submissions", "harness_session_generations"} {
		column := "session_id"
		if table == "task_session_messages" {
			column = "task_session_id"
		}
		var rows string
		query := fmt.Sprintf(`SELECT COALESCE(json_agg(row_to_json(record) ORDER BY row_to_json(record)::text)::text, '[]') FROM (SELECT * FROM %s WHERE %s = $1) record`, table, column)
		if err := database.Get(&rows, query, sessionID); err != nil {
			t.Fatal(err)
		}
		result += table + ":" + rows + "\n"
	}
	return result
}

func (h *retentionPostgresBinaryHarness) preserveBinaryLogs(t *testing.T, binary, outputPath string) {
	t.Helper()
	backupPath := os.Getenv("KANDEV_RETENTION_BACKUP_OUTPUT")
	if backupPath == "" {
		return
	}
	output, err := os.OpenFile(backupPath+".backend.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Errorf("preserve compatibility log: %v", err)
		return
	}
	defer func() { _ = output.Close() }()
	_, _ = fmt.Fprintf(output, "\nCompatibility binary: %s\n", binary)
	for _, path := range []string{outputPath, filepath.Join(h.home, "logs", "backend-logs.log")} {
		data, err := os.ReadFile(path)
		if err == nil {
			_, _ = output.Write(data)
		}
	}
}

func (h *retentionPostgresBinaryHarness) preserveFailedJournalFrame(t *testing.T) {
	t.Helper()
	backupPath := os.Getenv("KANDEV_RETENTION_BACKUP_OUTPUT")
	if backupPath == "" {
		return
	}
	_ = filepath.WalkDir(h.home, func(path string, entry os.DirEntry, walkError error) error {
		if walkError != nil || entry.IsDir() || entry.Name() != "delivery.bbolt" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		copyPath := backupPath + ".journal." + filepath.Base(filepath.Dir(path)) + ".bbolt"
		if err := os.WriteFile(copyPath, data, 0600); err != nil {
			return nil
		}
		snapshot, err := bolt.Open(copyPath, 0600, &bolt.Options{ReadOnly: true, Timeout: time.Second})
		if err != nil {
			return nil
		}
		defer func() { _ = snapshot.Close() }()
		return snapshot.View(func(tx *bolt.Tx) error {
			streams := tx.Bucket([]byte("events"))
			if streams == nil {
				return nil
			}
			return streams.ForEach(func(streamID, _ []byte) error {
				bucket := streams.Bucket(streamID)
				if bucket == nil {
					return nil
				}
				_, frame := bucket.Cursor().First()
				var identity struct {
					Type              string `json:"type"`
					SessionID         string `json:"session_id"`
					IncarnationID     string `json:"incarnation_id"`
					HarnessGeneration uint64 `json:"harness_generation"`
					Sequence          uint64 `json:"sequence"`
				}
				if err := json.Unmarshal(frame, &identity); err == nil {
					t.Logf("failed live journal first retained frame: type=%s session=%s incarnation=%s generation=%d sequence=%d stream=%s", identity.Type, identity.SessionID, identity.IncarnationID, identity.HarnessGeneration, identity.Sequence, streamID)
				}
				return nil
			})
		})
	})
}
