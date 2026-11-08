package sqlite

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestSessionSummaryModelLabelCases(t *testing.T) {
	assertSessionSummaryModelLabelCases(t, newRepoForSessionTests(t))
}

func TestSessionSummaryModelLabelCasesPostgres(t *testing.T) {
	assertSessionSummaryModelLabelCases(t, openPostgresRepo(t))
}

type sessionSummaryModelLabelCase struct {
	name        string
	metadata    map[string]interface{}
	rawMetadata *string
	wantID      string
	wantName    string
}

func sessionSummaryModelLabelCases() []sessionSummaryModelLabelCase {
	catalog := []interface{}{
		map[string]interface{}{"model_id": "model-a", "name": "Model A"},
		map[string]interface{}{"model_id": "model-b", "name": "Model B"},
		map[string]interface{}{"model_id": "model-c", "name": "Model C"},
	}
	empty, null := "", "null"
	return []sessionSummaryModelLabelCase{
		{
			name: "runtime config used when overrides are empty",
			metadata: map[string]interface{}{
				models.SessionMetaKeyACPModelState: map[string]interface{}{"current_model_id": "model-a", "models": catalog},
				"runtime_config_overrides":         map[string]interface{}{"model": ""},
				"runtime_config":                   map[string]interface{}{"model": "model-b"},
			},
			wantID: "model-b", wantName: "Model B",
		},
		{
			name: "current model used without runtime config",
			metadata: map[string]interface{}{
				models.SessionMetaKeyACPModelState: map[string]interface{}{"current_model_id": "model-c", "models": catalog},
			},
			wantID: "model-c", wantName: "Model C",
		},
		{
			name: "provider restored ignores local overrides",
			metadata: map[string]interface{}{
				models.SessionMetaKeyACPModelState: map[string]interface{}{
					"current_model_id": "model-a", "settings_policy": "provider_restored", "models": catalog,
				},
				"runtime_config_overrides": map[string]interface{}{"model": "model-b"},
				"runtime_config":           map[string]interface{}{"model": "model-c"},
			},
			wantID: "model-a", wantName: "Model A",
		},
		{
			name: "model missing from catalog falls back to its ID",
			metadata: map[string]interface{}{
				models.SessionMetaKeyACPModelState: map[string]interface{}{"current_model_id": "model-z", "models": catalog},
			},
			wantID: "model-z", wantName: "model-z",
		},
		{
			name: "non-array models catalog is ignored",
			metadata: map[string]interface{}{
				models.SessionMetaKeyACPModelState: map[string]interface{}{
					"current_model_id": "model-a", "models": map[string]interface{}{"model_id": "model-a", "name": "Wrong"},
				},
			},
			wantID: "model-a", wantName: "model-a",
		},
		{name: "empty metadata text has no label", rawMetadata: &empty},
		{name: "null metadata text has no label", rawMetadata: &null},
	}
}

func assertSessionSummaryModelLabelCases(t *testing.T, repo *Repository) {
	t.Helper()
	for _, testCase := range sessionSummaryModelLabelCases() {
		t.Run(testCase.name, func(t *testing.T) {
			taskID := "summary-model-" + t.Name()
			sessionID := taskID + "-session"
			if err := repo.CreateTask(t.Context(), &models.Task{ID: taskID, Title: "Model label"}); err != nil {
				t.Fatalf("create task: %v", err)
			}
			if err := repo.CreateTaskSession(t.Context(), &models.TaskSession{
				ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput, Metadata: testCase.metadata,
			}); err != nil {
				t.Fatalf("create session: %v", err)
			}
			if testCase.rawMetadata != nil {
				if _, err := repo.db.ExecContext(t.Context(), repo.db.Rebind(
					`UPDATE task_sessions SET metadata = ? WHERE id = ?`), *testCase.rawMetadata, sessionID); err != nil {
					t.Fatalf("store raw metadata: %v", err)
				}
			}
			rows, err := repo.ListTaskSessionSummaryObservations(t.Context(), taskID)
			if err != nil {
				t.Fatalf("list summary observations: %v", err)
			}
			if len(rows) != 1 {
				t.Fatalf("summary rows = %d, want 1", len(rows))
			}
			label, _ := rows[0].Metadata[models.SessionMetaKeyACPModelState].(map[string]interface{})
			if testCase.wantID == "" {
				if label != nil {
					t.Fatalf("model label = %#v, want none", label)
				}
				return
			}
			want := models.SessionModelLabelMetadata(testCase.wantID, testCase.wantName)
			if label["current_model_id"] != want["current_model_id"] {
				t.Fatalf("model label = %#v, want %#v", label, want)
			}
			gotModels, _ := label["models"].([]interface{})
			if len(gotModels) != 1 || gotModels[0].(map[string]interface{})["name"] != testCase.wantName {
				t.Fatalf("model label = %#v, want name %q", label, testCase.wantName)
			}
		})
	}
}
