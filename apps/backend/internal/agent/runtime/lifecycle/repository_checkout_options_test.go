package lifecycle

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestRepositoryCheckoutOptionsReachWorktree(t *testing.T) {
	var req EnvPrepareRequest
	if err := json.Unmarshal([]byte(`{"CheckoutOptions":{"version":1,"download_mode":"on_demand","sparse_directories":["app"]}}`), &req); err != nil {
		t.Fatal(err)
	}
	got := buildWorktreeCreateRequest(&req).CheckoutOptions
	if got == nil || got.DownloadMode != "on_demand" || len(got.SparseDirectories) != 1 {
		t.Fatalf("options lost: %#v", got)
	}
}

func TestRepositoryCheckoutCapabilities(t *testing.T) {
	manager := &Manager{}
	for _, tt := range []struct {
		executor, script string
		want             bool
	}{
		{"worktree", "", true}, {"worktree", DefaultPrepareScript("worktree"), true},
		{"local", "", false}, {"worktree", "echo custom", false}, {"ssh", "", false},
	} {
		demand, sparse := manager.SupportsRepositoryCheckoutOptions(tt.executor, tt.script)
		if demand != tt.want || sparse != tt.want {
			t.Fatalf("%s capability = %v,%v", tt.executor, demand, sparse)
		}
	}
}

func TestRepositoryCheckoutOptionsDockerPrimary(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(root, "gitconfig"))
	source := filepath.Join(root, "source")
	destination := filepath.Join(root, "checkout")
	if err := os.Mkdir(source, 0755); err != nil {
		t.Fatal(err)
	}
	runIn(t, source, "git", "init", "-b", "main")
	for _, dir := range []string{"app", "other"} {
		if err := os.Mkdir(filepath.Join(source, dir), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, dir, "file.txt"), []byte(dir), 0600); err != nil {
			t.Fatal(err)
		}
	}
	runIn(t, source, "git", "add", ".")
	runIn(t, source, "git", "-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "-m", "folders")
	options := &models.RepositoryCheckoutOptions{Version: 1, DownloadMode: "on_demand", SparseDirectories: []string{"app"}}
	runIn(t, source, "git", "config", "uploadpack.allowFilter", "true")
	script, err := checkoutOptionsPrepareScript(DefaultPrepareScript("local_docker"), options)
	if err != nil {
		t.Fatal(err)
	}
	script = strings.NewReplacer("{{repository.clone_url}}", shellQuote("file://"+source), "{{workspace.path}}", shellQuote(destination), "{{repository.branch}}", "main", "{{repository.setup_script}}", ":", "{{git.identity_setup}}", ":", "{{github.auth_setup}}", ":").Replace(script)
	cmd := exec.Command("bash", "-e", "-c", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(destination, "app", "file.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, "other")); !os.IsNotExist(err) {
		t.Fatal("excluded folder populated")
	}
	if got := strings.TrimSpace(string(runIn(t, destination, "git", "rev-parse", "--is-shallow-repository"))); got != "false" {
		t.Fatal("shallow checkout")
	}
}
