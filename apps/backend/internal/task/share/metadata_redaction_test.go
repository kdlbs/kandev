package share

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
)

const metadataBodyControl = "ordinary body sentinel"

type metadataRedactionCase struct {
	name  string
	value string
	want  string
	rule  string
}

// @covers AC-AUTH-PUBLIC-SHARE-LINKS-001.6, AC-AUTH-PUBLIC-SHARE-LINKS-001.7, AC-AUTH-PUBLIC-SHARE-LINKS-001.8
func TestShareMetadataRedaction_GistArtifacts(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"title", "agent_type", "model", "executor_type"} {
		t.Run(field, func(t *testing.T) {
			root := filepath.ToSlash(t.TempDir())
			cases := []metadataRedactionCase{
				{"sk", "sk-" + strings.Repeat("B", 20), "[redacted]", RuleSecretSK},
				{"ghp", "ghp_" + strings.Repeat("C", 36), "[redacted]", RuleSecretGHP},
				{"gho", "gho_" + strings.Repeat("D", 36), "[redacted]", RuleSecretGHO},
				{"github_pat", "github_pat_" + strings.Repeat("E", 36), "[redacted]", RuleSecretGitHubPAT},
				{"aws", "AKIA" + strings.Repeat("F", 16), "[redacted]", RuleSecretAWS},
				{"path", root + "/src/private.go", "src/private.go", RuleAbsPath},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					checkMetadataGist(t, root, field, tc)
				})
			}
		})
	}
}

func checkMetadataGist(t *testing.T, root, field string, tc metadataRedactionCase) {
	t.Helper()
	reader := metadataReader(root)
	setExportMetadata(reader, field, "context "+tc.value+" tail")
	before := metadataSource(t, reader)
	snap, err := BuildSnapshot(context.Background(), reader, "s-1", "v-test")
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}
	want := "context " + tc.want + " tail"
	if got := exportMetadata(snap, field); got != want {
		t.Errorf("exported %s = %q, want %q", field, got, want)
	}
	if !reflect.DeepEqual(snap.Redaction.AppliedRules, []string{tc.rule}) {
		t.Errorf("metadata-only rules = %v, want [%s]", snap.Redaction.AppliedRules, tc.rule)
	}
	mock := github.NewMockClient()
	id, _, err := NewGistBackend(mock).Upload(context.Background(), "workspace-1", snap, "en")
	if err != nil {
		t.Fatalf("mock gist upload: %v", err)
	}
	gist, ok := mock.Gists()[id]
	if !ok || gist.Public || len(gist.Files) != 3 {
		t.Fatalf("expected one secret gist with three files")
	}
	for _, name := range []string{"snapshot.json", "share.html", "README.md"} {
		file, exists := gist.Files[name]
		if !exists {
			t.Errorf("missing artifact %s", name)
		}
		if strings.Contains(file.Content, tc.value) {
			t.Errorf("%s exposes matched %s metadata", name, field)
		}
		if !strings.Contains(file.Content, want) || !strings.Contains(file.Content, metadataBodyControl) {
			t.Errorf("%s must retain sanitized metadata and ordinary conversation", name)
		}
	}
	if strings.Contains(gist.Description, tc.value) || !strings.Contains(gist.Description, snap.Task.Title) {
		t.Errorf("gist description must use the sanitized task title")
	}
	if got := metadataSource(t, reader); got != before {
		t.Fatal("snapshot or upload changed source records")
	}
}

// @covers AC-AUTH-PUBLIC-SHARE-LINKS-001.6, AC-AUTH-PUBLIC-SHARE-LINKS-001.7, AC-AUTH-PUBLIC-SHARE-LINKS-001.8
func TestShareMetadataRedaction_Preview(t *testing.T) {
	t.Parallel()
	root := filepath.ToSlash(t.TempDir())
	reader := metadataReader(root)
	reader.task.Title = "Preview " + root + "/title.txt"
	reader.session.AgentProfileSnapshot["model"] = "ghp_" + strings.Repeat("G", 36)
	before := metadataSource(t, reader)
	mock := github.NewMockClient()
	// Nil persistence proves that a preview never writes a share row.
	svc := New(nil, reader, allowShareAccess{}, NewGistBackend(mock), nil, "v-test")
	snap, err := svc.PreviewSnapshot(context.Background(), "t-1", "s-1")
	if err != nil {
		t.Fatalf("preview snapshot: %v", err)
	}
	if snap.Task.Title != "Preview title.txt" || snap.Session.Model != "[redacted]" {
		t.Errorf("preview metadata = %q / %q", snap.Task.Title, snap.Session.Model)
	}
	if !reflect.DeepEqual(snap.Redaction.AppliedRules, []string{RuleAbsPath, RuleSecretGHP}) {
		t.Errorf("preview rules = %v", snap.Redaction.AppliedRules)
	}
	if len(mock.Gists()) != 0 {
		t.Fatal("preview uploaded a gist")
	}
	if got := metadataSource(t, reader); got != before {
		t.Fatal("preview changed source records")
	}
}

// @covers AC-AUTH-PUBLIC-SHARE-LINKS-001.6, AC-AUTH-PUBLIC-SHARE-LINKS-001.7, AC-AUTH-PUBLIC-SHARE-LINKS-001.8
func TestShareMetadataRedaction_Controls(t *testing.T) {
	t.Parallel()
	t.Run("ordinary_metadata_and_body", checkOrdinaryShareMetadata)
	t.Run("empty_and_nonstring_metadata", checkEmptyShareMetadata)
	t.Run("multiple_roots_and_message_rule_union", checkShareMetadataRuleUnion)
}

func checkOrdinaryShareMetadata(t *testing.T) {
	reader := metadataReader(filepath.ToSlash(t.TempDir()))
	before := metadataSource(t, reader)
	snap, err := BuildSnapshot(context.Background(), reader, "s-1", "v-test")
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}
	if snap.Task != (TaskMeta{Title: "Ordinary task", WorkflowStep: "step-existing-id"}) {
		t.Errorf("ordinary task metadata = %+v", snap.Task)
	}
	if snap.Session.AgentType != "claude-acp" || snap.Session.Model != "ordinary-model" || snap.Session.ExecutorType != "local_docker" {
		t.Errorf("ordinary session metadata = %+v", snap.Session)
	}
	if snap.Version != 1 || snap.KandevVersion != "v-test" || snap.ExportedAt.IsZero() {
		t.Errorf("snapshot identity or export timestamp changed")
	}
	if !snap.Session.StartedAt.Equal(reader.session.StartedAt) || !snap.Session.CompletedAt.Equal(*reader.session.CompletedAt) {
		t.Errorf("session timestamps changed")
	}
	if len(snap.Messages) != 1 || snap.Messages[0].Blocks[0].Text != metadataBodyControl || !snap.Messages[0].Ts.Equal(reader.messages[0].CreatedAt) {
		t.Errorf("ordinary conversation changed: %+v", snap.Messages)
	}
	if len(snap.Redaction.AppliedRules) != 0 || metadataSource(t, reader) != before {
		t.Fatal("ordinary snapshot recorded a rule or mutated source data")
	}
}

func checkEmptyShareMetadata(t *testing.T) {
	reader := metadataReader(filepath.ToSlash(t.TempDir()))
	reader.task.Title = ""
	reader.session.AgentProfileSnapshot = map[string]interface{}{"agent_type": 42, "model": nil}
	reader.session.ExecutorSnapshot = nil
	reader.session.CompletedAt = nil
	before := metadataSource(t, reader)
	snap, err := BuildSnapshot(context.Background(), reader, "s-1", "v-test")
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}
	if snap.Task.Title != "" || snap.Session.AgentType != "" || snap.Session.Model != "" || snap.Session.ExecutorType != "" || snap.Session.CompletedAt != nil {
		t.Errorf("missing/non-string metadata changed its projection")
	}
	if len(snap.Redaction.AppliedRules) != 0 || metadataSource(t, reader) != before {
		t.Fatal("empty snapshot recorded a rule or mutated source data")
	}
}

func checkShareMetadataRuleUnion(t *testing.T) {
	root := filepath.ToSlash(t.TempDir())
	reader := metadataReader(root + "/workspace")
	reader.session.Worktrees = []*models.TaskEnvironmentRepo{
		{WorktreePath: root + "/repo"}, {WorktreePath: root + "/repo/nested"},
	}
	token := "ghp_" + strings.Repeat("H", 36)
	reader.task.Title = "Read " + root + "/repo/nested/a.go and " + root + "/repo/b.go"
	reader.session.AgentProfileSnapshot["model"] = root + "/workspace/model.txt " + token
	reader.messages[0].Content = "Keep body " + token + " and sk-" + strings.Repeat("J", 20)
	reader.messages = append(reader.messages, &models.Message{
		AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeToolExecute,
		Content: "run " + root + "/repo/command.sh",
		Metadata: map[string]interface{}{"args": map[string]interface{}{
			"cmd": "echo ordinary", "env": map[string]interface{}{"SYNTHETIC_SECRET": "test-only"},
		}},
	})
	before := metadataSource(t, reader)
	snap, err := BuildSnapshot(context.Background(), reader, "s-1", "v-test")
	if err != nil {
		t.Fatalf("build snapshot: %v", err)
	}
	if snap.Task.Title != "Read a.go and b.go" || snap.Session.Model != "model.txt [redacted]" {
		t.Errorf("multi-root metadata = %q / %q", snap.Task.Title, snap.Session.Model)
	}
	wantRules := []string{RuleAbsPath, RuleEnvVars, RuleSecretGHP, RuleSecretSK}
	if !reflect.DeepEqual(snap.Redaction.AppliedRules, wantRules) {
		t.Errorf("sorted distinct metadata/message rule union = %v, want %v", snap.Redaction.AppliedRules, wantRules)
	}
	if len(snap.Messages) != 2 || snap.Messages[0].Blocks[0].Text != "Keep body [redacted] and [redacted]" {
		t.Fatalf("existing body redaction changed: %+v", snap.Messages)
	}
	tool := snap.Messages[1].Blocks[0]
	if tool.Text != "run command.sh" || string(tool.Args) != `{"cmd":"echo ordinary"}` {
		t.Errorf("existing tool redaction changed: %+v", tool)
	}
	if metadataSource(t, reader) != before {
		t.Fatal("metadata/message redaction changed source records")
	}
}

func metadataReader(root string) *stubReader {
	completed := time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)
	return &stubReader{
		task: &models.Task{ID: "t-1", WorkspaceID: "workspace-1", Title: "Ordinary task", WorkflowStepID: "step-existing-id"},
		session: &models.TaskSession{
			ID: "s-1", TaskID: "t-1", State: models.TaskSessionStateCompleted,
			WorkspacePath: root, StartedAt: completed.Add(-time.Minute), CompletedAt: &completed,
			AgentProfileSnapshot: map[string]interface{}{"agent_type": "claude-acp", "model": "ordinary-model"},
			ExecutorSnapshot:     map[string]interface{}{"type": "local_docker"},
		},
		messages: []*models.Message{{
			ID: "m-1", TaskSessionID: "s-1", AuthorType: models.MessageAuthorUser,
			Type: models.MessageTypeMessage, Content: metadataBodyControl, CreatedAt: completed.Add(-time.Second),
		}},
	}
}

func setExportMetadata(reader *stubReader, field, value string) {
	switch field {
	case "title":
		reader.task.Title = value
	case "agent_type", "model":
		reader.session.AgentProfileSnapshot[field] = value
	case "executor_type":
		reader.session.ExecutorSnapshot["type"] = value
	}
}

func exportMetadata(snap *Snapshot, field string) string {
	switch field {
	case "title":
		return snap.Task.Title
	case "agent_type":
		return snap.Session.AgentType
	case "model":
		return snap.Session.Model
	case "executor_type":
		return snap.Session.ExecutorType
	}
	return ""
}

func metadataSource(t *testing.T, reader *stubReader) string {
	t.Helper()
	// Include source maps and worktrees omitted by the session's JSON contract.
	raw, err := json.Marshal([]any{reader.task, reader.session, reader.messages,
		reader.session.AgentProfileSnapshot, reader.session.ExecutorSnapshot, reader.session.Worktrees})
	if err != nil {
		t.Fatalf("capture source records: %v", err)
	}
	return string(raw)
}
