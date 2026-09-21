package executor

import (
	"context"
	"encoding/json"
	"github.com/kandev/kandev/internal/task/models"
	"reflect"
	"strings"
	"testing"
)

func TestRepositoryCheckoutOptionsReachLaunch(t *testing.T) {
	var info repoInfo
	if err := json.Unmarshal([]byte(`{"RepositoryID":"repo","CheckoutOptions":{"version":1,"download_mode":"on_demand","sparse_directories":["app"]}}`), &info); err != nil {
		t.Fatal(err)
	}
	specs := buildRepoSpecs([]*repoInfo{&info})
	field := reflect.ValueOf(specs[0]).FieldByName("CheckoutOptions")
	if !field.IsValid() || field.IsNil() {
		t.Fatal("checkout options lost")
	}
}

func TestRepositoryCheckoutOptionsRejectExecutorCredentialMode(t *testing.T) {
	executor := &Executor{githubCredentialPolicyResolver: fakeTaskGitCredentialPolicyResolver{policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor}}}
	_, err := executor.ensureTaskCheckoutPath(context.Background(), "task", "session", &models.Repository{WorkspaceID: "workspace", Provider: "github", ProviderOwner: "owner", ProviderName: "repo"}, &models.RepositoryCheckoutOptions{Version: 1, DownloadMode: models.DownloadOnDemand})
	if err == nil || !strings.Contains(err.Error(), "Kandev-managed Git credentials") {
		t.Fatalf("credential mode was not rejected: %v", err)
	}
}
