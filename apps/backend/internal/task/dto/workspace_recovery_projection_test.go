package dto

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestTaskSessionWorkspaceRecoverySerializationIsExplicitAndPathFree(t *testing.T) {
	withoutOperation, err := json.Marshal(FromTaskSession(&models.TaskSession{ID: "session-empty", TaskID: "task-empty"}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(withoutOperation), `"workspace_recovery":null`) {
		t.Fatalf("empty session omitted its explicit workspace recovery field: %s", withoutOperation)
	}

	dto := FromTaskSession(&models.TaskSession{ID: "session-recovery", TaskID: "task-recovery"})
	EnrichWorkspaceRecovery(&dto, &models.TaskEnvironmentRecoveryOperation{
		TaskEnvironmentID: "environment-recovery", OwnerTaskID: "task-recovery",
		OwnershipGeneration: 9007199254740993, SessionID: "session-recovery",
		OperationID: "operation-recovery", AttemptID: "attempt-recovery",
		ErrorStamp: "private-error-stamp", RunnerInstanceID: "private-runner-id",
		Kind: "managed_clone_relocation", Revision: 9007199254740995,
		State: "running", Phase: "snapshotting", RepositoryTotal: 2,
		SelectedRepositoryIDs: []string{"private-repository-inventory"},
	}, true)
	encoded, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if err := json.Unmarshal(encoded, &response); err != nil {
		t.Fatal(err)
	}
	projection, ok := response["workspace_recovery"].(map[string]any)
	if !ok {
		t.Fatalf("workspace recovery DTO = %#v", response["workspace_recovery"])
	}
	if projection["ownership_generation"] != "9007199254740993" || projection["revision"] != "9007199254740995" {
		t.Fatalf("large operation identities lost precision: %+v", projection)
	}
	if projection["error_stamp"] != "private-error-stamp" {
		t.Fatalf("recovery error correlation stamp = %#v, want the persisted stamp", projection["error_stamp"])
	}
	for _, privateValue := range []string{"private-runner-id", "private-repository-inventory"} {
		if strings.Contains(string(encoded), privateValue) {
			t.Fatalf("public session DTO leaked %q: %s", privateValue, encoded)
		}
	}
}
