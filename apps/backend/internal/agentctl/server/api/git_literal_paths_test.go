package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
)

// @covers AC-PLATFORM-WORKSPACE-GIT-STATUS-001.36, AC-PLATFORM-WORKSPACE-GIT-STATUS-001.37
func TestHandleGitLiteralSelections(t *testing.T) {
	for _, operation := range []string{"stage", "unstage"} {
		t.Run(operation, func(t *testing.T) {
			root := t.TempDir()
			for _, repo := range []string{"selected", "other"} {
				seedLiteralAPIRepo(t, root, repo)
				if operation == "unstage" || repo == "other" {
					runGitAPI(t, filepath.Join(root, repo), "add", "-A")
				}
			}
			log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error"})
			cfg := &config.InstanceConfig{WorkDir: root, BaseBranches: map[string]string{"selected": "main", "other": "main"}}
			manager := process.NewManager(cfg, log)
			t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })
			server := NewServer(cfg, manager, nil, nil, log)
			rec := postGitAPI(t, server, "/api/v1/git/"+operation, GitStageRequest{Repo: "selected", Paths: []string{"new[ab].txt"}})
			if rec.Code != http.StatusOK || !decodeGitOperationResult(t, rec).Success {
				t.Fatalf("%s HTTP %d: %s", operation, rec.Code, rec.Body.String())
			}
			checkLiteralAPIBytes(t, root, operation)
			rec = getGitAPI(t, server, "/api/v1/git/status?repo=selected&fresh=true&details=wait")
			var status struct {
				Success     bool                      `json:"success"`
				DetailState string                    `json:"detail_state"`
				Files       map[string]types.FileInfo `json:"files"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
				t.Fatal(err)
			}
			file, ok := status.Files["new[ab].txt"]
			if rec.Code != http.StatusOK || !status.Success || status.DetailState != "ready" || !ok || len(status.Files) != 2 {
				t.Fatalf("detail-wait HTTP %d: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(file.Diff, "+selected-selected-marker") || strings.Contains(file.Diff, "unselected-marker") {
				t.Errorf("selected transport patch: %q", file.Diff)
			}
		})
	}
}

func seedLiteralAPIRepo(t *testing.T, root, repo string) {
	t.Helper()
	dir := filepath.Join(root, repo)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitAPI(t, dir, "init", "--initial-branch=main")
	runGitAPI(t, dir, "config", "user.email", "test@test.com")
	runGitAPI(t, dir, "config", "user.name", "Test User")
	runGitAPI(t, dir, "config", "core.autocrlf", "false")
	writeFileAPI(t, dir, "new[ab].txt", repo+"-base-selected\n")
	writeFileAPI(t, dir, "newa.txt", repo+"-base-unselected\n")
	runGitAPI(t, dir, "add", ".")
	runGitAPI(t, dir, "commit", "-m", "literal API fixture")
	writeFileAPI(t, dir, "new[ab].txt", repo+"-base-selected\n"+repo+"-selected-marker\n")
	writeFileAPI(t, dir, "newa.txt", repo+"-base-unselected\n"+repo+"-unselected-marker\n")
}

func checkLiteralAPIBytes(t *testing.T, root, operation string) {
	t.Helper()
	for _, repo := range []string{"selected", "other"} {
		for _, item := range [][2]string{{"new[ab].txt", "selected"}, {"newa.txt", "unselected"}} {
			base := repo + "-base-" + item[1] + "\n"
			work := base + repo + "-" + item[1] + "-marker\n"
			wantIndex := base
			if repo == "other" || (operation == "stage" && item[1] == "selected") || (operation == "unstage" && item[1] == "unselected") {
				wantIndex = work
			}
			dir := filepath.Join(root, repo)
			if index := runGitAPI(t, dir, "show", ":"+item[0]); index != wantIndex {
				t.Errorf("%s index %q = %q, want %q", repo, item[0], index, wantIndex)
			}
			data, err := os.ReadFile(filepath.Join(dir, item[0]))
			if err != nil || string(data) != work {
				t.Errorf("%s working %q = %q, err %v, want preserved %q", repo, item[0], data, err, work)
			}
		}
	}
}
