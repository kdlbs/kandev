package launcher

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/backendapp/ownershiplock"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/maintenance"
)

// @covers AC-PLATFORM-MAINTENANCE-JSON-001.1 AC-PLATFORM-MAINTENANCE-JSON-001.2
func TestRunMaintenanceJSONDryRun(t *testing.T) {
	dbPath := maintenanceJSONFixture(t)
	before, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := captureMaintenanceJSONCommand(t, "database", "--json", "--candidate-limit", "1")
	if code != 0 || stderr != "" {
		t.Fatalf("dry run: exit=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	root := decodeMaintenanceJSONObject(t, stdout)
	assertMaintenanceJSONKeys(t, root, "mode", "report")
	if root["mode"] != "dry_run" {
		t.Fatalf("mode = %v", root["mode"])
	}
	report := maintenanceJSONObject(t, root, "report")
	assertMaintenanceJSONKeys(t, report, "generated_at", "database_size_bytes", "candidates", "delivery_retention", "total_candidate_rows", "total_estimated_bytes")
	generated, ok := report["generated_at"].(string)
	if !ok {
		t.Fatalf("generated_at = %#v", report["generated_at"])
	}
	parsed, err := time.Parse(time.RFC3339, generated)
	if err != nil || parsed.Location() != time.UTC {
		t.Fatalf("generated_at = %q, want UTC RFC3339: %v", generated, err)
	}
	if report["database_size_bytes"] != float64(len(before)) {
		t.Fatalf("database size = %v, want %d", report["database_size_bytes"], len(before))
	}
	candidates := maintenanceJSONObject(t, report, "candidates")
	assertMaintenanceJSONKeys(t, candidates, "duplicate_git_snapshots", "obsolete_plan_revisions", "orphaned_message_payloads")
	groups := []map[string]any{}
	for _, key := range []string{"duplicate_git_snapshots", "obsolete_plan_revisions", "orphaned_message_payloads"} {
		groups = append(groups, maintenanceJSONObject(t, candidates, key))
	}
	if groups[0]["rows"] != float64(1) || groups[0]["sample_limit_reached"] != true {
		t.Fatalf("limited duplicate group = %#v", groups[0])
	}
	delivery := maintenanceJSONObject(t, report, "delivery_retention")
	assertMaintenanceJSONKeys(t, delivery, "available", "unavailable_reason", "candidates")
	if delivery["available"] != true || delivery["unavailable_reason"] != "" {
		t.Fatalf("delivery = %#v", delivery)
	}
	deliveryGroups := maintenanceJSONObject(t, delivery, "candidates")
	assertMaintenanceJSONKeys(t, deliveryGroups, "projected_delivery_payloads", "settled_delivery_prompts", "retired_delivery_event_receipts", "retired_delivery_session_evidence", "orphaned_delivery_identities")
	for _, key := range []string{"projected_delivery_payloads", "settled_delivery_prompts", "retired_delivery_event_receipts", "retired_delivery_session_evidence", "orphaned_delivery_identities"} {
		groups = append(groups, maintenanceJSONObject(t, deliveryGroups, key))
	}
	var rows, estimatedBytes float64
	for _, group := range groups {
		assertMaintenanceJSONKeys(t, group, "rows", "estimated_bytes", "sample_limit_reached")
		count, countOK := group["rows"].(float64)
		size, sizeOK := group["estimated_bytes"].(float64)
		_, limitOK := group["sample_limit_reached"].(bool)
		if !countOK || !sizeOK || !limitOK || count < 0 || size < 0 || count != float64(int64(count)) || size != float64(int64(size)) {
			t.Fatalf("invalid candidate group: %#v", group)
		}
		rows += count
		estimatedBytes += size
	}
	if report["total_candidate_rows"] != rows || report["total_estimated_bytes"] != estimatedBytes {
		t.Fatalf("totals do not sum reported groups: %#v", report)
	}
	if strings.Contains(stdout, "task-cli-fixture") || strings.Contains(stdout, "session-cli-fixture") || strings.Contains(stdout, "a.go") {
		t.Fatalf("row content or identifiers exposed: %s", stdout)
	}
	after, err := os.ReadFile(dbPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("dry run changed database: %v", err)
	}
	if got := maintenanceJSONSnapshotCount(t, dbPath); got != 2 {
		t.Fatalf("dry run snapshots = %d, want 2", got)
	}
	backups, err := filepath.Glob(filepath.Join(filepath.Dir(dbPath), "backups", "*"))
	if err != nil || len(backups) != 0 {
		t.Fatalf("dry run backups = %v, error %v", backups, err)
	}
}

// @covers AC-PLATFORM-MAINTENANCE-JSON-001.3
func TestRunMaintenanceJSONExecuteAndCompact(t *testing.T) {
	dbPath := maintenanceJSONFixture(t)
	stdout, stderr, code := captureMaintenanceJSONCommand(t, "database", "--json", "--execute", "--compact")
	if code != 0 || stderr != "" {
		t.Fatalf("execute: exit=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	root := decodeMaintenanceJSONObject(t, stdout)
	assertMaintenanceJSONKeys(t, root, "mode", "report", "execution", "compaction")
	if root["mode"] != "execute" {
		t.Fatalf("mode = %v", root["mode"])
	}
	execution := maintenanceJSONObject(t, root, "execution")
	assertMaintenanceJSONKeys(t, execution, "backup_path", "partial", "deleted_git_snapshots", "deleted_plan_revisions", "deleted_message_payloads", "deleted_delivery_event_payloads", "deleted_settled_delivery_prompts", "retired_delivery_event_receipts", "purged_retired_delivery_rows", "delivery_orphan_jobs_scheduled", "delivery_retirement_jobs_completed")
	if execution["deleted_git_snapshots"] != float64(1) {
		t.Fatalf("execution = %#v", execution)
	}
	if _, ok := execution["partial"].(bool); !ok {
		t.Fatalf("partial = %#v", execution["partial"])
	}
	backup, ok := execution["backup_path"].(string)
	if !ok || backup == "" {
		t.Fatalf("backup = %#v", execution["backup_path"])
	}
	if got := maintenanceJSONSnapshotCount(t, backup); got != 2 {
		t.Fatalf("backup snapshots = %d, want 2", got)
	}
	if got := maintenanceJSONSnapshotCount(t, dbPath); got != 1 {
		t.Fatalf("live snapshots = %d, want 1", got)
	}
	compaction := maintenanceJSONObject(t, root, "compaction")
	assertMaintenanceJSONKeys(t, compaction, "size_before_bytes", "size_after_bytes", "rollback_path")
	rollback, ok := compaction["rollback_path"].(string)
	if !ok || rollback == "" {
		t.Fatalf("rollback = %#v", compaction["rollback_path"])
	}
	if got := maintenanceJSONSnapshotCount(t, rollback); got != 1 {
		t.Fatalf("rollback snapshots = %d, want 1", got)
	}
	for _, key := range []string{"size_before_bytes", "size_after_bytes"} {
		if value, ok := compaction[key].(float64); !ok || value <= 0 || value != float64(int64(value)) {
			t.Fatalf("%s = %#v", key, compaction[key])
		}
	}
}

// @covers AC-PLATFORM-MAINTENANCE-JSON-001.3
func TestRunMaintenanceJSONExecuteWithoutCompaction(t *testing.T) {
	dbPath := maintenanceJSONFixture(t)
	stdout, stderr, code := captureMaintenanceJSONCommand(t, "database", "--json", "--execute")
	if code != 0 || stderr != "" {
		t.Fatalf("execute: exit=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	root := decodeMaintenanceJSONObject(t, stdout)
	assertMaintenanceJSONKeys(t, root, "mode", "report", "execution")
	execution := maintenanceJSONObject(t, root, "execution")
	if execution["deleted_git_snapshots"] != float64(1) {
		t.Fatalf("execution = %#v", execution)
	}
	if got := maintenanceJSONSnapshotCount(t, dbPath); got != 1 {
		t.Fatalf("live snapshots = %d, want 1", got)
	}
}

// @covers AC-PLATFORM-MAINTENANCE-JSON-001.2 AC-PLATFORM-MAINTENANCE-JSON-001.3
func TestRunMaintenanceJSONUnavailableAndLock(t *testing.T) {
	dbPath := maintenanceJSONFixture(t)
	conn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec("DROP TABLE agent_delivery_retention_state"); err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := captureMaintenanceJSONCommand(t, "database", "--json")
	if code != 0 || stderr != "" {
		t.Fatalf("legacy dry run: exit=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	report := maintenanceJSONObject(t, decodeMaintenanceJSONObject(t, stdout), "report")
	delivery := maintenanceJSONObject(t, report, "delivery_retention")
	assertMaintenanceJSONKeys(t, delivery, "available", "unavailable_reason")
	if delivery["available"] != false || delivery["unavailable_reason"] != "schema_unavailable" {
		t.Fatalf("delivery = %#v", delivery)
	}
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	targets, err := ownershiplock.Targets(workingDir, "sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := ownershiplock.Acquire(targets)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	stdout, stderr, code = captureMaintenanceJSONCommand(t, "database", "--json", "--execute")
	if code != 1 || stdout != "" || stderr == "" {
		t.Fatalf("lock refusal: exit=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	if got := maintenanceJSONSnapshotCount(t, dbPath); got != 2 {
		t.Fatalf("lock refusal snapshots = %d, want 2", got)
	}
}

// @covers AC-PLATFORM-MAINTENANCE-JSON-001.1
func TestRunMaintenanceJSONHelpAndUsage(t *testing.T) {
	stdout, stderr, code := captureMaintenanceJSONCommand(t, "database", "--json", "--help")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "--json") {
		t.Fatalf("help: exit=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
	stdout, stderr, code = captureMaintenanceJSONCommand(t, "database", "--json", "--not-a-real-flag")
	if code != 2 || stdout != "" || !strings.Contains(stderr, "unknown maintenance database flag") {
		t.Fatalf("usage: exit=%d stderr=%q stdout=%q", code, stderr, stdout)
	}
}

// @covers AC-PLATFORM-MAINTENANCE-JSON-001.3 AC-PLATFORM-MAINTENANCE-JSON-001.4
func TestMaintenanceJSONCommittedFailureAndWriteErrors(t *testing.T) {
	outcome := maintenance.Outcome{
		Executed: true, BackupPath: "/fixture/backup.db",
		Execution: maintenance.ExecutionResult{DeletedGitSnapshots: 2, Partial: true},
	}
	runErr := errors.New("retention failed after commit")
	var stdout, stderr bytes.Buffer
	if code := finishMaintenanceOutcome(outcome, runErr, true, &stdout, &stderr); code != 1 {
		t.Fatalf("committed failure exit = %d", code)
	}
	root := decodeMaintenanceJSONObject(t, stdout.String())
	execution := maintenanceJSONObject(t, root, "execution")
	if root["mode"] != "execute" || execution["backup_path"] != "/fixture/backup.db" || execution["deleted_git_snapshots"] != float64(2) || execution["partial"] != true {
		t.Fatalf("committed outcome = %#v", root)
	}
	if _, ok := root["compaction"]; ok || !strings.Contains(stderr.String(), runErr.Error()) {
		t.Fatalf("compaction or diagnostic incorrect: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	for _, tc := range []struct {
		name   string
		runErr error
		writer io.Writer
	}{
		{name: "write_error", writer: maintenanceJSONFailingWriter{}},
		{name: "short_write_and_domain_error", runErr: runErr, writer: maintenanceJSONShortWriter{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var diagnostic bytes.Buffer
			if code := finishMaintenanceOutcome(outcome, tc.runErr, true, tc.writer, &diagnostic); code != 1 {
				t.Fatalf("write failure exit = %d", code)
			}
			if !strings.Contains(diagnostic.String(), "failed to write maintenance output") {
				t.Fatalf("missing write diagnostic: %q", diagnostic.String())
			}
			if tc.runErr != nil && !strings.Contains(diagnostic.String(), tc.runErr.Error()) {
				t.Fatalf("missing domain diagnostic: %q", diagnostic.String())
			}
		})
	}
}

type maintenanceJSONFailingWriter struct{}

func (maintenanceJSONFailingWriter) Write([]byte) (int, error) {
	return 0, errors.New("output unavailable")
}

type maintenanceJSONShortWriter struct{}

func (maintenanceJSONShortWriter) Write(p []byte) (int, error) {
	return len(p) - 1, nil
}

func maintenanceJSONFixture(t *testing.T) string {
	t.Helper()
	clearLauncherConfigurationEnvironment(t)
	dir := t.TempDir()
	t.Chdir(dir)
	dbPath := filepath.Join(dir, "state", "kandev.db")
	writeLauncherConfig(t, filepath.Join(dir, "config.yaml"), "homeDir: "+dir+"\ndatabase:\n  path: "+dbPath+"\n")
	seedMaintenanceFixtureDB(t, dbPath)
	return dbPath
}

func captureMaintenanceJSONCommand(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	stdout, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdout.Close() })
	stderr, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stderr.Close() })
	oldStdout, oldStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdout, stderr
	defer func() { os.Stdout, os.Stderr = oldStdout, oldStderr }()
	code := runMaintenance(args, BuildInfo{})
	out, err := os.ReadFile(stdout.Name())
	if err != nil {
		t.Fatal(err)
	}
	errOut, err := os.ReadFile(stderr.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(out), string(errOut), code
}

func decodeMaintenanceJSONObject(t *testing.T, stdout string) map[string]any {
	t.Helper()
	if !strings.HasSuffix(stdout, "}\n") || strings.HasPrefix(stdout, " ") || strings.HasPrefix(stdout, "\n") {
		t.Fatalf("JSON output must be one object followed by a newline: %q", stdout)
	}
	decoder := json.NewDecoder(strings.NewReader(stdout))
	var result map[string]any
	if err := decoder.Decode(&result); err != nil || result == nil {
		t.Fatalf("decode JSON object: %v; output %q", err, stdout)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("JSON output contains trailing data: %#v, %v", extra, err)
	}
	return result
}

func maintenanceJSONObject(t *testing.T, parent map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := parent[key].(map[string]any)
	if !ok {
		t.Fatalf("%s is not an object: %#v", key, parent[key])
	}
	return value
}

func assertMaintenanceJSONKeys(t *testing.T, object map[string]any, expected ...string) {
	t.Helper()
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	sort.Strings(expected)
	if !reflect.DeepEqual(keys, expected) {
		t.Fatalf("keys = %v, want %v", keys, expected)
	}
}

func maintenanceJSONSnapshotCount(t *testing.T, path string) int {
	t.Helper()
	conn, err := db.OpenSQLiteReader(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	var count int
	if err := conn.QueryRow("SELECT COUNT(*) FROM task_session_git_snapshots").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
