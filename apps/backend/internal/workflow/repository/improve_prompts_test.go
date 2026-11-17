package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	workflowcfg "github.com/kandev/kandev/config/workflows"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func readPreviousImprovePrompts(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile("testdata/improve-kandev-before-lifecycle.yml")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := workflowcfg.LoadTemplateYAML(data)
	if err != nil {
		t.Fatal(err)
	}
	if fixture.ID != "improve-kandev" {
		t.Fatalf("historical fixture ID = %q", fixture.ID)
	}
	byID := make(map[string]string, len(fixture.Steps))
	for _, step := range fixture.Steps {
		byID[step.ID] = step.Prompt
	}
	return byID
}

func openImprovePromptRepo(t *testing.T, path string, logs ...*logger.Logger) (*Repository, *sqlx.DB, *sqlx.DB) {
	t.Helper()
	writerSQL, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	readerSQL, err := db.OpenSQLiteReader(path)
	if err != nil {
		_ = writerSQL.Close()
		t.Fatal(err)
	}
	writer := sqlx.NewDb(writerSQL, "sqlite3")
	reader := sqlx.NewDb(readerSQL, "sqlite3")
	if _, err := writer.Exec(`
		CREATE TABLE IF NOT EXISTS workflows (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			workflow_template_id TEXT NOT NULL DEFAULT '',
			is_system INTEGER NOT NULL DEFAULT 0,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
		CREATE TABLE IF NOT EXISTS task_sessions (id TEXT PRIMARY KEY);
	`); err != nil {
		_ = writer.Close()
		_ = reader.Close()
		t.Fatal(err)
	}
	var log *logger.Logger
	if len(logs) > 0 {
		log = logs[0]
	}
	repo, err := NewWithDB(writer, reader, log)
	if err != nil {
		_ = writer.Close()
		_ = reader.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = writer.Close()
		_ = reader.Close()
	})
	return repo, writer, reader
}

func TestImprovePromptUpgradePreservesUnknownContentWithBoundedDiagnostic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "improve-prompts-unknown.db")
	previous := readPreviousImprovePrompts(t)
	_, writer, reader := openImprovePromptRepo(t, path)
	var templateJSON string
	if err := writer.Get(&templateJSON, `SELECT steps FROM workflow_templates WHERE id = ?`, improveKandevTemplateID); err != nil {
		t.Fatal(err)
	}
	const customTemplatePrompt = "user-authored Improve instructions"
	templateJSON = setPromptInRawTemplate(t, templateJSON, map[string]string{"improve": customTemplatePrompt})
	if _, err := writer.Exec(`UPDATE workflow_templates SET steps = ? WHERE id = ?`, templateJSON, improveKandevTemplateID); err != nil {
		t.Fatal(err)
	}
	insertImproveWorkflow(t, writer, "wf-custom", improveKandevTemplateID, false, previous)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}

	core, logs := observer.New(zap.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatal(err)
	}
	repo, writer, reader := openImprovePromptRepo(t, path, log)
	template, err := repo.GetTemplate(context.Background(), improveKandevTemplateID)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range template.Steps {
		if step.ID == "improve" && step.Prompt != customTemplatePrompt {
			t.Fatalf("custom template prompt = %q, want unchanged", step.Prompt)
		}
	}
	warnings := logs.FilterMessage("Improve Kandev prompt upgrade preserved unknown or concurrently edited content").All()
	if len(warnings) != 1 {
		t.Fatalf("prompt upgrade warnings = %d, want one bounded summary", len(warnings))
	}
	if strings.Contains(fmt.Sprint(warnings[0]), customTemplatePrompt) || strings.Contains(fmt.Sprint(warnings[0]), "user-custom prompt") {
		t.Fatal("prompt upgrade diagnostic contains prompt content")
	}
	_ = writer.Close()
	_ = reader.Close()
}

// @covers AC-WORKSPACES-IMPROVE-VALIDATION-002.1 AC-WORKSPACES-IMPROVE-VALIDATION-002.3
func TestImprovePromptUpgradeMatchesNormalizedHistoricalContent(t *testing.T) {
	previous := readPreviousImprovePrompts(t)
	path := filepath.Join(t.TempDir(), "improve-prompts-normalized.db")
	_, writer, reader := openImprovePromptRepo(t, path)
	var templateJSON string
	if err := writer.Get(&templateJSON, `SELECT steps FROM workflow_templates WHERE id = ?`, improveKandevTemplateID); err != nil {
		t.Fatal(err)
	}
	stored := map[string]string{
		"improve": " \n" + previous["improve"] + "\n ",
		"test":    " \n" + previous["test"] + "\n ",
	}
	if _, err := writer.Exec(`UPDATE workflow_templates SET steps = ? WHERE id = ?`,
		setPromptInRawTemplate(t, templateJSON, stored), improveKandevTemplateID,
	); err != nil {
		t.Fatal(err)
	}
	insertImproveWorkflow(t, writer, "wf-normalized", improveKandevTemplateID, false, stored)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}

	repo, writer, _ := openImprovePromptRepo(t, path)
	current := currentImprovePrompts(t)
	template, err := repo.GetTemplate(context.Background(), improveKandevTemplateID)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range template.Steps {
		if want, ok := current[step.ID]; ok && step.Prompt != want {
			t.Fatalf("normalized template prompt %q = %q, want current prompt", step.ID, step.Prompt)
		}
	}
	for _, key := range []string{"improve", "test"} {
		var got string
		if err := writer.Get(&got, `SELECT prompt FROM workflow_steps WHERE id = ?`, "wf-normalized-"+key); err != nil {
			t.Fatal(err)
		}
		if got != current[key] {
			t.Fatalf("normalized workflow prompt %q = %q, want current prompt", key, got)
		}
	}
}

// @covers AC-WORKSPACES-IMPROVE-VALIDATION-002.3 AC-WORKSPACES-IMPROVE-VALIDATION-002.5
func TestImprovePromptUpgradePreservesTimestampedEdits(t *testing.T) {
	previous := readPreviousImprovePrompts(t)
	path := filepath.Join(t.TempDir(), "improve-prompts-timestamps.db")
	_, writer, reader := openImprovePromptRepo(t, path)
	var templateJSON string
	if err := writer.Get(&templateJSON, `SELECT steps FROM workflow_templates WHERE id = ?`, improveKandevTemplateID); err != nil {
		t.Fatal(err)
	}
	created := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	updated := created.Add(time.Minute)
	if _, err := writer.Exec(`UPDATE workflow_templates SET steps = ?, created_at = ?, updated_at = ? WHERE id = ?`,
		setPromptInRawTemplate(t, templateJSON, previous), created, updated, improveKandevTemplateID,
	); err != nil {
		t.Fatal(err)
	}
	insertImproveWorkflow(t, writer, "wf-edited", improveKandevTemplateID, false, previous)
	insertImproveWorkflow(t, writer, "wf-unedited", improveKandevTemplateID, false, previous)
	for _, key := range []string{"improve", "test"} {
		if _, err := writer.Exec(`UPDATE workflow_steps SET updated_at = ? WHERE id = ?`, updated, "wf-edited-"+key); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}

	_, writer, _ = openImprovePromptRepo(t, path)
	current := currentImprovePrompts(t)
	var storedTemplate string
	if err := writer.Get(&storedTemplate, `SELECT steps FROM workflow_templates WHERE id = ?`, improveKandevTemplateID); err != nil {
		t.Fatal(err)
	}
	var steps []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(storedTemplate), &steps); err != nil {
		t.Fatal(err)
	}
	for _, step := range steps {
		var id, prompt string
		if err := json.Unmarshal(step["id"], &id); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(step["prompt"], &prompt); err != nil {
			t.Fatal(err)
		}
		if want, ok := previous[id]; ok && prompt != want {
			t.Fatalf("timestamped template prompt %q = %q, want preserved historical text", id, prompt)
		}
	}
	for _, workflow := range []string{"wf-edited", "wf-unedited"} {
		for _, key := range []string{"improve", "test"} {
			var got string
			id := workflow + "-" + key
			if err := writer.Get(&got, `SELECT prompt FROM workflow_steps WHERE id = ?`, id); err != nil {
				t.Fatal(err)
			}
			want := previous[key]
			if workflow == "wf-unedited" {
				want = current[key]
			}
			if got != want {
				t.Fatalf("workflow prompt %q = %q, want %q", id, got, want)
			}
		}
	}
}

// @covers AC-WORKSPACES-IMPROVE-VALIDATION-002.1 AC-WORKSPACES-IMPROVE-VALIDATION-002.5
func TestImprovePromptUpgradeRetriesTemplateAfterUnrelatedEdit(t *testing.T) {
	previous := readPreviousImprovePrompts(t)
	path := filepath.Join(t.TempDir(), "improve-prompts-retry.db")
	_, writer, reader := openImprovePromptRepo(t, path)
	var templateJSON string
	if err := writer.Get(&templateJSON, `SELECT steps FROM workflow_templates WHERE id = ?`, improveKandevTemplateID); err != nil {
		t.Fatal(err)
	}
	staleJSON := setPromptInRawTemplate(t, templateJSON, previous)
	if _, err := writer.Exec(`UPDATE workflow_templates SET steps = ? WHERE id = ?`, staleJSON, improveKandevTemplateID); err != nil {
		t.Fatal(err)
	}
	var concurrentSteps []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(staleJSON), &concurrentSteps); err != nil {
		t.Fatal(err)
	}
	concurrentSteps[0]["custom_extension"] = json.RawMessage(`{"preserved":true}`)
	concurrentJSON, err := json.Marshal(concurrentSteps)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(`CREATE TABLE upgrade_retry (done INTEGER NOT NULL); INSERT INTO upgrade_retry VALUES (0)`); err != nil {
		t.Fatal(err)
	}
	quotedJSON := strings.ReplaceAll(string(concurrentJSON), "'", "''")
	trigger := fmt.Sprintf(`CREATE TRIGGER simulate_unrelated_template_edit
		BEFORE UPDATE OF steps ON workflow_templates
		WHEN (SELECT done FROM upgrade_retry) = 0
		BEGIN
			UPDATE upgrade_retry SET done = 1;
			UPDATE workflow_templates SET steps = '%s' WHERE id = OLD.id;
			SELECT RAISE(IGNORE);
		END`, quotedJSON)
	if _, err := writer.Exec(trigger); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}

	repo, writer, _ := openImprovePromptRepo(t, path)
	current := currentImprovePrompts(t)
	template, err := repo.GetTemplate(context.Background(), improveKandevTemplateID)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range template.Steps {
		if want, ok := current[step.ID]; ok && step.Prompt != want {
			t.Fatalf("retried template prompt %q = %q, want current prompt", step.ID, step.Prompt)
		}
	}
	var storedJSON string
	if err := writer.Get(&storedJSON, `SELECT steps FROM workflow_templates WHERE id = ?`, improveKandevTemplateID); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(storedJSON), &concurrentSteps); err != nil {
		t.Fatal(err)
	}
	if string(concurrentSteps[0]["custom_extension"]) != `{"preserved":true}` {
		t.Fatalf("unrelated concurrent template property = %s, want preserved", concurrentSteps[0]["custom_extension"])
	}
}

func setPromptInRawTemplate(t *testing.T, data string, prompts map[string]string) string {
	t.Helper()
	var steps []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &steps); err != nil {
		t.Fatal(err)
	}
	for _, step := range steps {
		var id string
		if err := json.Unmarshal(step["id"], &id); err != nil {
			t.Fatal(err)
		}
		if prompt, ok := prompts[id]; ok {
			encoded, err := json.Marshal(prompt)
			if err != nil {
				t.Fatal(err)
			}
			step["prompt"] = encoded
		}
	}
	encoded, err := json.Marshal(steps)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func insertImproveWorkflow(t *testing.T, database *sqlx.DB, id, templateID string, isSystem bool, prompts map[string]string) {
	t.Helper()
	ctx := context.Background()
	created := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if _, err := database.ExecContext(ctx, `
		INSERT INTO workflows (id, workspace_id, workflow_template_id, is_system, name, description, created_at, updated_at)
		VALUES (?, 'workspace-1', ?, ?, ?, 'keep description', ?, ?)
	`, id, templateID, isSystem, "workflow "+id, created, created); err != nil {
		t.Fatal(err)
	}
	position := 0
	for _, step := range []struct{ name, key string }{{"Improve", "improve"}, {"Test", "test"}} {
		prompt := prompts[step.key]
		if prompt == "" {
			prompt = "user-custom prompt"
		}
		stepID := id + "-" + step.key
		if _, err := database.ExecContext(ctx, `
			INSERT INTO workflow_steps (id, workflow_id, name, position, prompt, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, stepID, id, step.name, position, prompt, created, created); err != nil {
			t.Fatal(err)
		}
		position++
	}
}

func currentImprovePrompts(t *testing.T) map[string]string {
	t.Helper()
	templates, err := workflowcfg.LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	var templateID string
	prompts := make(map[string]string)
	for _, template := range templates {
		if template.ID != "improve-kandev" {
			continue
		}
		templateID = template.ID
		for _, step := range template.Steps {
			prompts[step.ID] = step.Prompt
		}
		break
	}
	if templateID == "" {
		t.Fatal("embedded Improve Kandev template not found")
	}
	return prompts
}

func TestImprovePromptUpgrade(t *testing.T) {
	previousByID := readPreviousImprovePrompts(t)
	path := filepath.Join(t.TempDir(), "improve-prompts.db")
	_, writer, reader := openImprovePromptRepo(t, path)
	ctx := context.Background()
	var templateJSON string
	if err := writer.Get(&templateJSON, `SELECT steps FROM workflow_templates WHERE id = ?`, "improve-kandev"); err != nil {
		t.Fatal(err)
	}
	var templateSteps []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(templateJSON), &templateSteps); err != nil {
		t.Fatal(err)
	}
	var preservedEvents json.RawMessage
	for _, step := range templateSteps {
		var id string
		if err := json.Unmarshal(step["id"], &id); err != nil {
			t.Fatal(err)
		}
		if id == "improve" {
			preservedEvents = append(json.RawMessage(nil), step["events"]...)
			step["custom_extension"] = json.RawMessage(`{"keep":true}`)
		}
		if prompt, ok := previousByID[id]; ok {
			encoded, err := json.Marshal(prompt)
			if err != nil {
				t.Fatal(err)
			}
			step["prompt"] = encoded
		}
	}
	templateJSONBytes, err := json.Marshal(templateSteps)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ExecContext(ctx, `UPDATE workflow_templates SET steps = ? WHERE id = ?`, string(templateJSONBytes), "improve-kandev"); err != nil {
		t.Fatal(err)
	}
	insertImproveWorkflow(t, writer, "wf-system", "improve-kandev", true, previousByID)
	insertImproveWorkflow(t, writer, "wf-reused", "improve-kandev", false, map[string]string{"improve": previousByID["improve"]})
	insertImproveWorkflow(t, writer, "wf-other", "simple", false, previousByID)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}

	// A fresh repository models a binary restart against the same persisted store.
	repo, writer, reader := openImprovePromptRepo(t, path)
	want := currentImprovePrompts(t)
	for _, id := range []string{"improve", "test"} {
		if got := want[id]; got == previousByID[id] {
			t.Fatalf("embedded prompt %q did not advance from the historical revision", id)
		}
	}
	var upgradedJSON string
	if err := writer.Get(&upgradedJSON, `SELECT steps FROM workflow_templates WHERE id = ?`, "improve-kandev"); err != nil {
		t.Fatal(err)
	}
	var upgradedSteps []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(upgradedJSON), &upgradedSteps); err != nil {
		t.Fatal(err)
	}
	storedTemplate, err := repo.GetTemplate(ctx, "improve-kandev")
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range storedTemplate.Steps {
		if expected, ok := want[step.ID]; ok && step.Prompt != expected {
			t.Fatalf("stored template prompt %q = %q, want current %q", step.ID, step.Prompt, expected)
		}
	}
	for _, step := range upgradedSteps {
		var id string
		if err := json.Unmarshal(step["id"], &id); err != nil {
			t.Fatal(err)
		}
		if id == "improve" {
			if string(step["events"]) != string(preservedEvents) {
				t.Fatalf("template events changed during prompt upgrade: %s != %s", step["events"], preservedEvents)
			}
			var extension struct {
				Keep bool `json:"keep"`
			}
			if err := json.Unmarshal(step["custom_extension"], &extension); err != nil || !extension.Keep {
				t.Fatalf("unknown template field was not preserved: %s (err %v)", step["custom_extension"], err)
			}
		}
	}

	for _, workflow := range []string{"wf-system", "wf-reused"} {
		for _, step := range []string{"improve", "test"} {
			var got string
			if err := writer.Get(&got, `SELECT prompt FROM workflow_steps WHERE id = ?`, workflow+"-"+step); err != nil {
				t.Fatal(err)
			}
			if workflow == "wf-reused" && step == "test" {
				if got != "user-custom prompt" {
					t.Fatalf("custom prompt = %q, want preservation", got)
				}
				continue
			}
			if got != want[step] {
				t.Fatalf("%s/%s prompt = %q, want current %q", workflow, step, got, want[step])
			}
		}
	}
	var updatedAt, createdAt time.Time
	if err := writer.QueryRowContext(ctx, `SELECT created_at, updated_at FROM workflow_steps WHERE id = ?`, "wf-system-improve").Scan(&createdAt, &updatedAt); err != nil {
		t.Fatal(err)
	}
	if !updatedAt.After(createdAt) {
		t.Fatalf("upgraded step timestamp = %s, want later than creation %s", updatedAt, createdAt)
	}
	if err := writer.QueryRowContext(ctx, `SELECT created_at, updated_at FROM workflow_steps WHERE id = ?`, "wf-reused-test").Scan(&createdAt, &updatedAt); err != nil {
		t.Fatal(err)
	}
	if !updatedAt.Equal(createdAt) {
		t.Fatalf("custom step timestamp changed: created=%s updated=%s", createdAt, updatedAt)
	}
	var unrelated string
	if err := writer.Get(&unrelated, `SELECT prompt FROM workflow_steps WHERE id = ?`, "wf-other-improve"); err != nil {
		t.Fatal(err)
	}
	if unrelated != previousByID["improve"] {
		t.Fatalf("unrelated workflow prompt = %q, want unchanged historical content", unrelated)
	}
	var systemCount, reusedCount int
	if err := writer.Get(&systemCount, `SELECT COUNT(*) FROM workflows WHERE id = ? AND is_system = 1`, "wf-system"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Get(&reusedCount, `SELECT COUNT(*) FROM workflows WHERE id = ? AND is_system = 0 AND workflow_template_id = ?`, "wf-reused", "improve-kandev"); err != nil {
		t.Fatal(err)
	}
	if systemCount != 1 || reusedCount != 1 {
		t.Fatalf("workflow identities or properties changed: system=%d reused=%d", systemCount, reusedCount)
	}

	var beforeRepeat string
	if err := writer.Get(&beforeRepeat, `SELECT steps FROM workflow_templates WHERE id = ?`, "improve-kandev"); err != nil {
		t.Fatal(err)
	}
	var beforeStepUpdate time.Time
	if err := writer.Get(&beforeStepUpdate, `SELECT updated_at FROM workflow_steps WHERE id = ?`, "wf-system-improve"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}

	_, writer, _ = openImprovePromptRepo(t, path)
	var afterRepeat string
	if err := writer.Get(&afterRepeat, `SELECT steps FROM workflow_templates WHERE id = ?`, "improve-kandev"); err != nil {
		t.Fatal(err)
	}
	if afterRepeat != beforeRepeat {
		t.Fatal("repeated startup changed the upgraded template")
	}
	var afterStepUpdate time.Time
	if err := writer.Get(&afterStepUpdate, `SELECT updated_at FROM workflow_steps WHERE id = ?`, "wf-system-improve"); err != nil {
		t.Fatal(err)
	}
	if !afterStepUpdate.Equal(beforeStepUpdate) {
		t.Fatal("repeated startup changed an already-current workflow step")
	}
}

func TestImprovePromptUpgradeAllowsWorkflowsWithoutTemplateBindingColumn(t *testing.T) {
	database, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec(`CREATE TABLE workflows (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		name TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}

	if _, err := NewWithDB(database, database, nil); err != nil {
		t.Fatalf("initialize workflow repository without template binding column: %v", err)
	}
}

func TestImprovePromptUpgradeConcurrentEdit(t *testing.T) {
	previous := readPreviousImprovePrompts(t)
	current := currentImprovePrompts(t)
	path := filepath.Join(t.TempDir(), "improve-prompts-concurrent.db")
	repo, writer, reader := openImprovePromptRepo(t, path)
	ctx := context.Background()
	var currentJSON string
	if err := writer.Get(&currentJSON, `SELECT steps FROM workflow_templates WHERE id = ?`, "improve-kandev"); err != nil {
		t.Fatal(err)
	}
	staleJSON := setPromptInRawTemplate(t, currentJSON, previous)
	if _, err := writer.ExecContext(ctx, `UPDATE workflow_templates SET steps = ? WHERE id = ?`, staleJSON, "improve-kandev"); err != nil {
		t.Fatal(err)
	}
	concurrentTemplateJSON := setPromptInRawTemplate(t, staleJSON, map[string]string{"improve": "concurrent template edit"})
	if _, err := writer.ExecContext(ctx, `UPDATE workflow_templates SET steps = ? WHERE id = ?`, concurrentTemplateJSON, "improve-kandev"); err != nil {
		t.Fatal(err)
	}
	proposedTemplateJSON := setPromptInRawTemplate(t, staleJSON, current)
	var templateCreatedAt, templateUpdatedAt time.Time
	if err := writer.Get(&templateCreatedAt, `SELECT created_at FROM workflow_templates WHERE id = ?`, improveKandevTemplateID); err != nil {
		t.Fatal(err)
	}
	if err := writer.Get(&templateUpdatedAt, `SELECT updated_at FROM workflow_templates WHERE id = ?`, improveKandevTemplateID); err != nil {
		t.Fatal(err)
	}
	updated, err := repo.updateImproveTemplateStepsIfUnchanged(
		staleJSON, templateCreatedAt, templateUpdatedAt, proposedTemplateJSON,
	)
	if err != nil {
		t.Fatal(err)
	}
	if updated {
		t.Fatal("stale template upgrade overwrote the concurrent edit")
	}
	var templateJSON string
	if err := writer.Get(&templateJSON, `SELECT steps FROM workflow_templates WHERE id = ?`, "improve-kandev"); err != nil {
		t.Fatal(err)
	}
	var storedSteps []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(templateJSON), &storedSteps); err != nil {
		t.Fatal(err)
	}
	for _, step := range storedSteps {
		var id, prompt string
		if err := json.Unmarshal(step["id"], &id); err != nil {
			t.Fatal(err)
		}
		if id == "improve" {
			if err := json.Unmarshal(step["prompt"], &prompt); err != nil {
				t.Fatal(err)
			}
			if prompt != "concurrent template edit" {
				t.Fatalf("concurrent template prompt = %q", prompt)
			}
		}
	}

	insertImproveWorkflow(t, writer, "wf-concurrent", "improve-kandev", false, previous)
	if _, err := writer.ExecContext(ctx, `UPDATE workflow_steps SET prompt = ? WHERE id = ?`, "concurrent step edit", "wf-concurrent-improve"); err != nil {
		t.Fatal(err)
	}
	var stepCreatedAt, stepUpdatedAt time.Time
	if err := writer.Get(&stepCreatedAt, `SELECT created_at FROM workflow_steps WHERE id = ?`, "wf-concurrent-improve"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Get(&stepUpdatedAt, `SELECT updated_at FROM workflow_steps WHERE id = ?`, "wf-concurrent-improve"); err != nil {
		t.Fatal(err)
	}
	updated, err = repo.updateImproveStepPromptIfUnchanged(
		"wf-concurrent-improve", previous["improve"], stepCreatedAt, stepUpdatedAt, current["improve"],
	)
	if err != nil {
		t.Fatal(err)
	}
	if updated {
		t.Fatal("stale step upgrade overwrote the concurrent edit")
	}
	var stepPrompt string
	if err := writer.Get(&stepPrompt, `SELECT prompt FROM workflow_steps WHERE id = ?`, "wf-concurrent-improve"); err != nil {
		t.Fatal(err)
	}
	if stepPrompt != "concurrent step edit" {
		t.Fatalf("concurrent step prompt = %q", stepPrompt)
	}
	_ = writer.Close()
	_ = reader.Close()
}

func TestImprovePromptUpgradeFailure(t *testing.T) {
	previousByID := readPreviousImprovePrompts(t)
	path := filepath.Join(t.TempDir(), "improve-prompts-failure.db")
	_, writer, reader := openImprovePromptRepo(t, path)
	ctx := context.Background()
	var templateJSON string
	if err := writer.Get(&templateJSON, `SELECT steps FROM workflow_templates WHERE id = ?`, "improve-kandev"); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ExecContext(ctx, `UPDATE workflow_templates SET steps = ? WHERE id = ?`, setPromptInRawTemplate(t, templateJSON, previousByID), "improve-kandev"); err != nil {
		t.Fatal(err)
	}
	insertImproveWorkflow(t, writer, "wf-failure", "improve-kandev", false, previousByID)
	if _, err := writer.ExecContext(ctx, `CREATE TRIGGER reject_prompt_upgrade BEFORE UPDATE OF prompt ON workflow_steps BEGIN SELECT RAISE(ABORT, 'prompt write rejected'); END`); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}

	writerSQL, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	readerSQL, err := db.OpenSQLiteReader(path)
	if err != nil {
		_ = writerSQL.Close()
		t.Fatal(err)
	}
	_, err = NewWithDB(sqlx.NewDb(writerSQL, "sqlite3"), sqlx.NewDb(readerSQL, "sqlite3"), nil)
	if err == nil {
		_ = writerSQL.Close()
		_ = readerSQL.Close()
		t.Fatal("repository initialization succeeded after a required prompt write failed")
	}
	if !strings.Contains(err.Error(), "prompt write rejected") {
		t.Fatalf("repository initialization error = %v, want prompt write failure", err)
	}
	_ = writerSQL.Close()
	_ = readerSQL.Close()
}
