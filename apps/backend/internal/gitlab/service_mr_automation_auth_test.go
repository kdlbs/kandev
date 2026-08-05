package gitlab

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newMRAutomationAuthFixture(t *testing.T) (*Service, *MockClient) {
	t.Helper()
	store := newTestStore(t)
	seedWorkspace(t, store, "ws-1")
	if err := store.SaveConfigForWorkspace(context.Background(), "ws-1", &GitLabConfig{
		Host: "https://gitlab.example.com", AuthMethod: AuthMethodPAT,
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	secrets := &configTestSecrets{values: map[string]string{SecretKeyForWorkspace("ws-1"): "token"}}
	svc := newWorkspaceConfigService(t, store, secrets)
	mock := NewMockClient("https://gitlab.example.com")
	mock.SetUser("alice")
	svc.workspaceClientFn = func(_ context.Context, _ *GitLabConfig, _ string) (Client, error) {
		return mock, nil
	}
	return svc, mock
}

func TestService_GetMRAutomationSnapshot_IncludesFailingJobsAndUnresolvedDiscussions(t *testing.T) {
	svc, mock := newMRAutomationAuthFixture(t)
	mock.SeedMR("group/project", &MR{IID: 1, HeadBranch: "feature", State: mrStateOpen})
	pipelineID := int64(99)
	mock.SeedPipelines("group/project", []Pipeline{{ID: pipelineID, Status: "failed"}})
	mock.SeedPipelineJobs(pipelineID, []PipelineJob{
		{ID: 1, Name: "build", Status: "success"},
		{ID: 2, Name: "test", Status: "failed"},
		{ID: 3, Name: "lint", Status: "failed", AllowFailure: true},
	})
	mock.SeedDiscussions("group/project", 1, []MRDiscussion{
		{ID: "d1", Resolvable: true, Resolved: false},
		{ID: "d2", Resolvable: true, Resolved: true},
	})

	snapshot, err := svc.GetMRAutomationSnapshot(context.Background(), "ws-1", "https://gitlab.example.com", "group/project", 1)
	if err != nil {
		t.Fatalf("GetMRAutomationSnapshot() error = %v", err)
	}
	if len(snapshot.FailingJobs) != 1 || snapshot.FailingJobs[0].Name != "test" {
		t.Errorf("FailingJobs = %+v, want only the non-allow_failure failed job", snapshot.FailingJobs)
	}
	if snapshot.UnresolvedDiscussions != 1 {
		t.Errorf("UnresolvedDiscussions = %d, want 1", snapshot.UnresolvedDiscussions)
	}
	if snapshot.MR == nil || snapshot.MR.IID != 1 {
		t.Errorf("MR = %+v, want IID 1", snapshot.MR)
	}
}

func TestService_GetMRAutomationSnapshot_NoPipelineYet(t *testing.T) {
	svc, mock := newMRAutomationAuthFixture(t)
	mock.SeedMR("group/project", &MR{IID: 1, HeadBranch: "feature", State: mrStateOpen})

	snapshot, err := svc.GetMRAutomationSnapshot(context.Background(), "ws-1", "https://gitlab.example.com", "group/project", 1)
	if err != nil {
		t.Fatalf("GetMRAutomationSnapshot() error = %v", err)
	}
	if len(snapshot.FailingJobs) != 0 {
		t.Errorf("FailingJobs = %+v, want empty when there is no pipeline yet", snapshot.FailingJobs)
	}
}

func TestService_GetMRAutomationSnapshot_HostMismatchFailsClosed(t *testing.T) {
	svc, mock := newMRAutomationAuthFixture(t)
	mock.SeedMR("group/project", &MR{IID: 1, HeadBranch: "feature", State: mrStateOpen})

	_, err := svc.GetMRAutomationSnapshot(context.Background(), "ws-1", "https://gitlab.other.example.com", "group/project", 1)
	if !errors.Is(err, ErrWorkspaceHostMismatch) {
		t.Fatalf("err = %v, want ErrWorkspaceHostMismatch", err)
	}
}

func TestService_MergeMRForAutomation_MergesAndReturnsMR(t *testing.T) {
	svc, mock := newMRAutomationAuthFixture(t)
	mock.SeedMR("group/project", &MR{IID: 1, HeadBranch: "feature", State: mrStateOpen})

	merged, err := svc.MergeMRForAutomation(context.Background(), "ws-1", "https://gitlab.example.com", "group/project", 1)
	if err != nil {
		t.Fatalf("MergeMRForAutomation() error = %v", err)
	}
	if merged.State != gitlabStateMerged {
		t.Errorf("merged.State = %q, want merged", merged.State)
	}
	if merged.MergedAt == nil || merged.MergedAt.After(time.Now()) {
		t.Errorf("merged.MergedAt = %v, want a recent timestamp", merged.MergedAt)
	}
}

func TestService_MergeMRForAutomation_HostMismatchNeverMerges(t *testing.T) {
	svc, mock := newMRAutomationAuthFixture(t)
	mock.SeedMR("group/project", &MR{IID: 1, HeadBranch: "feature", State: mrStateOpen})

	_, err := svc.MergeMRForAutomation(context.Background(), "ws-1", "https://gitlab.other.example.com", "group/project", 1)
	if !errors.Is(err, ErrWorkspaceHostMismatch) {
		t.Fatalf("err = %v, want ErrWorkspaceHostMismatch", err)
	}
	status, statusErr := mock.GetMRStatus(context.Background(), "group/project", 1)
	if statusErr != nil {
		t.Fatalf("GetMRStatus() error = %v", statusErr)
	}
	if status.MR.State == gitlabStateMerged {
		t.Fatal("MR was merged despite the host mismatch — automation must fail closed")
	}
}
