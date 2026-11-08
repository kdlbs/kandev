package backendapp

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/coordinator/mergestate"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type fakeMergeRepos struct {
	repos map[string]*taskmodels.Repository
}

func (f fakeMergeRepos) GetRepository(_ context.Context, id string) (*taskmodels.Repository, error) {
	return f.repos[id], nil
}

func mergeRepos() fakeMergeRepos {
	return fakeMergeRepos{repos: map[string]*taskmodels.Repository{
		"gh":       {ID: "gh", WorkspaceID: "ws", Provider: "github", ProviderOwner: "acme", ProviderName: "app"},
		"gl":       {ID: "gl", WorkspaceID: "ws", Provider: "gitlab", ProviderHost: "https://git.example.com", ProviderOwner: "grp/sub", ProviderName: "proj"},
		"other-ws": {ID: "other-ws", WorkspaceID: "ws2", Provider: "github", ProviderOwner: "acme", ProviderName: "app"},
		"no-host":  {ID: "no-host", WorkspaceID: "ws", Provider: "gitlab", ProviderOwner: "grp", ProviderName: "proj"},
	}}
}

type fakeGitHubAPI struct {
	live       *github.PRLiveRead
	mergeRes   github.AutomationMergeResult
	mergeErr   error
	methods    github.RepoMergeMethods
	protection github.BranchProtectionRead
	commits    []github.BranchCommit
	full       bool
	err        error

	gotHost, gotOwner, gotRepo, gotMethod, gotHead, gotBranch string
}

func (f *fakeGitHubAPI) ReadPRLiveForAutomation(_ context.Context, _, host, owner, repo string, _ int) (*github.PRLiveRead, error) {
	f.gotHost, f.gotOwner, f.gotRepo = host, owner, repo
	return f.live, f.err
}

func (f *fakeGitHubAPI) MergePRForAutomationOutcome(_ context.Context, _, host, owner, repo string, _ int, method, head string) (github.AutomationMergeResult, error) {
	f.gotHost, f.gotOwner, f.gotRepo, f.gotMethod, f.gotHead = host, owner, repo, method, head
	return f.mergeRes, f.mergeErr
}

func (f *fakeGitHubAPI) RepoMergeMethodsForAutomation(context.Context, string, string, string, string) (github.RepoMergeMethods, error) {
	return f.methods, f.err
}

func (f *fakeGitHubAPI) ReadBranchProtectionForAutomation(context.Context, string, string, string, string, string) (github.BranchProtectionRead, error) {
	return f.protection, f.err
}

func (f *fakeGitHubAPI) ListBranchCommitsForAutomation(_ context.Context, _, _, _, _, branch string, _ int) ([]github.BranchCommit, bool, error) {
	f.gotBranch = branch
	return f.commits, f.full, f.err
}

type fakeGitLabAPI struct {
	live       *gitlab.MRLiveRead
	merged     *gitlab.MR
	mergeErr   error
	policy     gitlab.ProjectMergePolicy
	protection *gitlab.ProjectProtection
	commits    []gitlab.MRCommitInfo
	full       bool
	err        error

	gotHost, gotPath, gotMethod, gotHead, gotBranch string
}

func (f *fakeGitLabAPI) ReadMRLiveForAutomation(_ context.Context, _, host, path string, _ int) (*gitlab.MRLiveRead, error) {
	f.gotHost, f.gotPath = host, path
	return f.live, f.err
}

func (f *fakeGitLabAPI) MergeMRForAutomation(_ context.Context, _, host, path string, _ int, method, sha string) (*gitlab.MR, error) {
	f.gotHost, f.gotPath, f.gotMethod, f.gotHead = host, path, method, sha
	return f.merged, f.mergeErr
}

func (f *fakeGitLabAPI) ProjectMergePolicyForAutomation(context.Context, string, string, string) (gitlab.ProjectMergePolicy, error) {
	return f.policy, f.err
}

func (f *fakeGitLabAPI) ReadProjectProtectionForAutomation(context.Context, string, string, string, string) (*gitlab.ProjectProtection, error) {
	return f.protection, f.err
}

func (f *fakeGitLabAPI) ListBranchCommitsForAutomation(_ context.Context, _, _, _, branch string, _ int) ([]gitlab.MRCommitInfo, bool, error) {
	f.gotBranch = branch
	return f.commits, f.full, f.err
}

func ghKey() mergestate.ChangeKey {
	return mergestate.ChangeKey{Provider: "github", Host: "github.com", RepoID: "gh", Number: 7}
}

func glKey() mergestate.ChangeKey {
	return mergestate.ChangeKey{Provider: "gitlab", Host: "git.example.com", RepoID: "gl", Number: 3}
}

func newTestAdapter(gh *fakeGitHubAPI, gl *fakeGitLabAPI) *coordinatorMergeProviders {
	return newCoordinatorMergeProviders(mergeRepos(), githubMerge{api: gh}, gitlabMerge{api: gl})
}

func TestMergeAdapterResolveFailsClosed(t *testing.T) {
	a := newTestAdapter(&fakeGitHubAPI{}, &fakeGitLabAPI{})
	for _, tc := range []struct {
		name string
		ws   string
		key  mergestate.ChangeKey
	}{
		{"unknown repository", "ws", mergestate.ChangeKey{Provider: "github", Host: "github.com", RepoID: "missing", Number: 1}},
		{"other workspace", "ws", mergestate.ChangeKey{Provider: "github", Host: "github.com", RepoID: "other-ws", Number: 1}},
		{"provider mismatch", "ws", mergestate.ChangeKey{Provider: "gitlab", Host: "github.com", RepoID: "gh", Number: 1}},
		{"host mismatch", "ws", mergestate.ChangeKey{Provider: "github", Host: "ghe.example.com", RepoID: "gh", Number: 1}},
		{"gitlab repository without host", "ws", mergestate.ChangeKey{Provider: "gitlab", Host: "git.example.com", RepoID: "no-host", Number: 1}},
		{"unsupported provider", "ws", mergestate.ChangeKey{Provider: "bitbucket", Host: "x", RepoID: "gh", Number: 1}},
	} {
		if _, err := a.ReadLive(context.Background(), tc.ws, tc.key); err == nil {
			t.Errorf("%s: ReadLive succeeded, want error", tc.name)
		}
		if _, err := a.Merge(context.Background(), tc.ws, tc.key, "sha", "squash"); err == nil {
			t.Errorf("%s: Merge succeeded, want error", tc.name)
		}
	}
}

func TestMergeAdapterGitHubMergeOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		res     github.AutomationMergeResult
		err     error
		want    coordinator.MergeResult
		wantErr bool
	}{
		{"merged", github.AutomationMergeResult{Outcome: github.MergeOutcomeMerged, MergeCommit: "m1"}, nil,
			coordinator.MergeResult{Outcome: coordinator.MergeAccepted, MergeCommit: "m1"}, false},
		{"queued", github.AutomationMergeResult{Outcome: github.MergeOutcomeQueued}, nil,
			coordinator.MergeResult{Outcome: coordinator.MergeQueued}, false},
		{"head moved", github.AutomationMergeResult{}, fmt.Errorf("x: %w", github.ErrMergeHeadMoved),
			coordinator.MergeResult{Outcome: coordinator.MergeStaleHead, Reason: "x: github: pull request head moved"}, false},
		{"rejected", github.AutomationMergeResult{}, fmt.Errorf("blocked: %w", github.ErrMergeRejected),
			coordinator.MergeResult{Outcome: coordinator.MergeRejected, Reason: "blocked: github: merge rejected"}, false},
		{"transport", github.AutomationMergeResult{}, errors.New("EOF"), coordinator.MergeResult{}, true},
		{"no outcome", github.AutomationMergeResult{}, nil, coordinator.MergeResult{}, true},
	} {
		fake := &fakeGitHubAPI{mergeRes: tc.res, mergeErr: tc.err}
		got, err := newTestAdapter(fake, &fakeGitLabAPI{}).Merge(context.Background(), "ws", ghKey(), "head1", "squash")
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tc.name, err, tc.wantErr)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: result = %+v, want %+v", tc.name, got, tc.want)
		}
		if fake.gotHead != "head1" || fake.gotMethod != "squash" || fake.gotOwner != "acme" || fake.gotRepo != "app" || fake.gotHost != "github.com" {
			t.Errorf("%s: request = %+v", tc.name, fake)
		}
	}
}

func TestMergeAdapterMergeRequiresHead(t *testing.T) {
	fake := &fakeGitHubAPI{}
	if _, err := newTestAdapter(fake, &fakeGitLabAPI{}).Merge(context.Background(), "ws", ghKey(), " ", "squash"); err == nil {
		t.Fatal("Merge without a head succeeded")
	}
	if fake.gotHead != "" {
		t.Fatal("provider was called without an expected head")
	}
}

func TestMergeAdapterGitLabMergeOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		method     string
		mr         *gitlab.MR
		err        error
		want       coordinator.MergeOutcome
		wantErr    bool
		wantMethod string
		wantCommit string
	}{
		{name: "merged squash", method: "squash", mr: &gitlab.MR{State: "merged", MergeCommitSHA: "m2"}, want: coordinator.MergeAccepted, wantMethod: "squash", wantCommit: "m2"},
		{name: "merged merge", method: "merge", mr: &gitlab.MR{State: "merged"}, want: coordinator.MergeAccepted, wantMethod: ""},
		{name: "locked is queued", method: "merge", mr: &gitlab.MR{State: "locked"}, want: coordinator.MergeQueued},
		{name: "still open is queued", method: "merge", mr: &gitlab.MR{State: "opened"}, want: coordinator.MergeQueued},
		{name: "head moved", method: "merge", err: &gitlab.MergeRefusedError{Status: 409, Err: errors.New("sha mismatch")}, want: coordinator.MergeStaleHead},
		{name: "refused", method: "merge", err: &gitlab.MergeRefusedError{Status: 405, Err: errors.New("not mergeable")}, want: coordinator.MergeRejected},
		{name: "draft", method: "merge", err: gitlab.ErrMRIsDraft, want: coordinator.MergeRejected},
		{name: "transport", method: "merge", err: errors.New("EOF"), wantErr: true},
		{name: "nil answer", method: "merge", wantErr: true},
		{name: "unknown method", method: "rebase", wantErr: true},
	} {
		fake := &fakeGitLabAPI{merged: tc.mr, mergeErr: tc.err}
		got, err := newTestAdapter(&fakeGitHubAPI{}, fake).Merge(context.Background(), "ws", glKey(), "head2", tc.method)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tc.name, err, tc.wantErr)
			continue
		}
		if tc.wantErr {
			continue
		}
		if got.Outcome != tc.want || got.MergeCommit != tc.wantCommit {
			t.Errorf("%s: result = %+v, want outcome %v commit %q", tc.name, got, tc.want, tc.wantCommit)
		}
		if fake.gotHead != "head2" || fake.gotMethod != tc.wantMethod || fake.gotPath != "grp/sub/proj" || fake.gotHost != "https://git.example.com" {
			t.Errorf("%s: request = %+v", tc.name, fake)
		}
	}
}

func TestMergeAdapterMethods(t *testing.T) {
	gh := &fakeGitHubAPI{methods: github.RepoMergeMethods{Merge: true, Rebase: true}}
	got, err := newTestAdapter(gh, &fakeGitLabAPI{}).Methods(context.Background(), "ws", ghKey())
	if err != nil || got.Default != "merge" || len(got.Allowed) != 2 || got.Allowed[0] != "merge" || got.Allowed[1] != "rebase" {
		t.Fatalf("github methods = %+v, %v", got, err)
	}
	for _, tc := range []struct {
		option      string
		wantDefault string
		wantAllowed []string
	}{
		{"never", "merge", []string{"merge"}},
		{"always", "squash", []string{"squash"}},
		{"default_on", "squash", []string{"squash", "merge"}},
		{"default_off", "merge", []string{"merge", "squash"}},
		{"", "merge", []string{"merge", "squash"}},
	} {
		gl := &fakeGitLabAPI{policy: gitlab.ProjectMergePolicy{SquashOption: tc.option}}
		got, err := newTestAdapter(&fakeGitHubAPI{}, gl).Methods(context.Background(), "ws", glKey())
		if err != nil || got.Default != tc.wantDefault || fmt.Sprint(got.Allowed) != fmt.Sprint(tc.wantAllowed) {
			t.Errorf("gitlab %q methods = %+v, %v", tc.option, got, err)
		}
	}
	if _, err := newTestAdapter(&fakeGitHubAPI{err: errors.New("down")}, &fakeGitLabAPI{}).Methods(context.Background(), "ws", ghKey()); err == nil {
		t.Error("provider error was swallowed")
	}
}

func TestMergeAdapterProtection(t *testing.T) {
	enf, not, unk := mergestate.Enforced, mergestate.NotEnforced, mergestate.UnknownEnf
	for _, tc := range []struct {
		name string
		repo string
		gh   github.BranchProtectionRead
		gl   *gitlab.ProjectProtection
		err  error
		want mergestate.Protection
		fail bool
	}{
		{name: "github classic and rulesets", repo: "gh",
			gh: github.BranchProtectionRead{Classic: github.ClassicProtectionFound, ClassicRequire: github.ProtectionRequirements{Pipeline: true},
				RulesetsOK: true, RulesetsRequire: github.ProtectionRequirements{Approvals: true}},
			want: mergestate.Protection{Pipeline: enf, Approvals: enf, Threads: not}},
		{name: "github absent without admin is unknown", repo: "gh",
			gh:   github.BranchProtectionRead{Classic: github.ClassicProtectionAbsent, RulesetsOK: true},
			want: mergestate.Protection{Pipeline: unk, Approvals: unk, Threads: unk}},
		{name: "github absent with admin", repo: "gh",
			gh:   github.BranchProtectionRead{Classic: github.ClassicProtectionAbsent, AdminConfirmed: true, RulesetsOK: true},
			want: mergestate.Protection{Pipeline: not, Approvals: not, Threads: not}},
		{name: "github failed classic read is unknown", repo: "gh",
			gh:   github.BranchProtectionRead{Classic: github.ClassicProtectionFailed, RulesetsOK: true},
			want: mergestate.Protection{Pipeline: unk, Approvals: unk, Threads: unk}},
		{name: "github read error", repo: "gh", err: errors.New("403"), fail: true},
		{name: "gitlab known", repo: "gl",
			gl:   &gitlab.ProjectProtection{PipelineRequired: true, ApprovalsKnown: true},
			want: mergestate.Protection{Pipeline: enf, Approvals: not, Threads: not}},
		{name: "gitlab approvals unknown", repo: "gl",
			gl:   &gitlab.ProjectProtection{ThreadsRequired: true},
			want: mergestate.Protection{Pipeline: not, Approvals: unk, Threads: enf}},
		{name: "gitlab nil answer", repo: "gl", fail: true},
		{name: "gitlab read error", repo: "gl", err: errors.New("500"), fail: true},
		{name: "other workspace", repo: "other-ws", fail: true},
		{name: "missing repository", repo: "missing", fail: true},
	} {
		a := newTestAdapter(&fakeGitHubAPI{protection: tc.gh, err: tc.err}, &fakeGitLabAPI{protection: tc.gl, err: tc.err})
		got, err := a.ReadProtection(context.Background(), "ws", tc.repo, "main")
		if tc.fail {
			if err == nil {
				t.Errorf("%s: want error", tc.name)
			}
			if got != mergestate.UnknownProtection() {
				t.Errorf("%s: failed read = %+v, want unknown", tc.name, got)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("%s: got %+v, %v want %+v", tc.name, got, err, tc.want)
		}
	}
}

func TestMergeAdapterCommitMessages(t *testing.T) {
	live := &github.PRLiveRead{Status: &github.PRStatus{PR: &github.PR{State: "merged", BaseBranch: "main"}}}
	commits := []github.BranchCommit{{SHA: "c3", Message: "third"}, {SHA: "c2", Message: "second"}, {SHA: "m1", Message: "merge"}, {SHA: "c0", Message: "old"}}
	for _, tc := range []struct {
		name     string
		full     bool
		after    string
		want     []string
		wantMore bool
	}{
		{"stops at the merge commit, oldest first", true, "m1", []string{"second", "third"}, false},
		{"merge commit not on a full page", true, "zz", []string{"old", "merge", "second", "third"}, true},
		{"merge commit not on a short page", false, "zz", []string{"old", "merge", "second", "third"}, false},
	} {
		gh := &fakeGitHubAPI{live: live, commits: commits, full: tc.full}
		got, more, err := newTestAdapter(gh, &fakeGitLabAPI{}).CommitMessages(context.Background(), "ws", ghKey(), tc.after, 1)
		if err != nil || more != tc.wantMore || fmt.Sprint(got) != fmt.Sprint(tc.want) {
			t.Errorf("%s: got %v more=%v err=%v", tc.name, got, more, err)
		}
		if gh.gotBranch != "main" {
			t.Errorf("%s: branch = %q, want the change's target branch", tc.name, gh.gotBranch)
		}
	}
	gl := &fakeGitLabAPI{
		live:    &gitlab.MRLiveRead{Status: &gitlab.MRStatus{MR: &gitlab.MR{BaseBranch: "release"}}},
		commits: []gitlab.MRCommitInfo{{SHA: "b", Message: "newer"}, {SHA: "a", Message: "merge"}},
	}
	got, more, err := newTestAdapter(&fakeGitHubAPI{}, gl).CommitMessages(context.Background(), "ws", glKey(), "a", 1)
	if err != nil || more || fmt.Sprint(got) != "[newer]" || gl.gotBranch != "release" {
		t.Errorf("gitlab commits = %v more=%v err=%v branch=%q", got, more, err, gl.gotBranch)
	}
	if _, _, err := newTestAdapter(&fakeGitHubAPI{err: errors.New("down")}, &fakeGitLabAPI{}).CommitMessages(context.Background(), "ws", ghKey(), "m1", 1); err == nil {
		t.Error("provider error was swallowed")
	}
}

func TestMergeAdapterGitHubReadLive(t *testing.T) {
	yes, no := true, false
	for _, tc := range []struct {
		name string
		pr   github.PR
		st   github.PRStatus
		read github.PRLiveRead
		want mergestate.Snapshot
	}{
		{
			name: "clean and approved",
			pr:   github.PR{State: "open", BaseBranch: "main", HeadSHA: "h1"},
			st: github.PRStatus{MergeableState: "clean", ChecksState: "success", ChecksPopulated: true, ChecksTotal: 2, ReviewState: "approved",
				UnresolvedReviewThreadsPopulated: true, UnresolvedReviewThreads: 0},
			read: github.PRLiveRead{HasMergeConflicts: &no},
			want: mergestate.Snapshot{State: mergestate.StateOpen, Draft: mergestate.Met, TargetBranch: "main", HeadSHA: "h1",
				Pipeline: mergestate.PipelineGreen, Mergeable: mergestate.MergeableReading{Mergeable: mergestate.Met},
				Conflict: mergestate.ConflictClear, UnresolvedThreads: 0, Approvals: mergestate.Met, Behind: 0},
		},
		{
			name: "draft, dirty, threads unread",
			pr:   github.PR{State: "open", BaseBranch: "main", HeadSHA: "h2", Draft: true},
			st:   github.PRStatus{MergeableState: "dirty", ChecksState: "failure", ReviewState: "changes_requested"},
			read: github.PRLiveRead{HasMergeConflicts: &yes},
			want: mergestate.Snapshot{State: mergestate.StateOpen, Draft: mergestate.Unmet, TargetBranch: "main", HeadSHA: "h2",
				Pipeline: mergestate.PipelineRed, Mergeable: mergestate.MergeableReading{Mergeable: mergestate.Unmet},
				Conflict: mergestate.ConflictPresent, UnresolvedThreads: -1, Approvals: mergestate.Unmet, Behind: 0},
		},
		{
			name: "merged with commit, nothing else known",
			pr:   github.PR{State: "merged", BaseBranch: "main", HeadSHA: "h3"},
			st:   github.PRStatus{},
			read: github.PRLiveRead{MergeCommit: "mc"},
			want: mergestate.Snapshot{State: mergestate.StateMerged, Draft: mergestate.Met, TargetBranch: "main", HeadSHA: "h3", MergedHeadSHA: "h3",
				MergeCommit: "mc", Pipeline: mergestate.PipelineUnavailable, Mergeable: mergestate.MergeableReading{Mergeable: mergestate.Unavailable},
				Conflict: mergestate.ConflictUnknown, UnresolvedThreads: -1, Approvals: mergestate.Unavailable, Behind: -1},
		},
	} {
		st := tc.st
		st.PR = &tc.pr
		read := tc.read
		read.Status = &st
		got, err := newTestAdapter(&fakeGitHubAPI{live: &read}, &fakeGitLabAPI{}).ReadLive(context.Background(), "ws", ghKey())
		tc.want.Key = ghKey()
		if err != nil || got != tc.want {
			t.Errorf("%s:\n got %+v err=%v\nwant %+v", tc.name, got, err, tc.want)
		}
	}
	if _, err := newTestAdapter(&fakeGitHubAPI{live: &github.PRLiveRead{}}, &fakeGitLabAPI{}).ReadLive(context.Background(), "ws", ghKey()); err == nil {
		t.Error("empty provider answer was accepted")
	}
}

func TestMergeAdapterGitLabReadLive(t *testing.T) {
	for _, tc := range []struct {
		name string
		mr   gitlab.MR
		st   gitlab.MRStatus
		disc int
		want mergestate.Snapshot
	}{
		{
			name: "mergeable",
			mr:   gitlab.MR{State: "opened", BaseBranch: "main", HeadSHA: "h1", DetailedMergeStatus: "mergeable"},
			st:   gitlab.MRStatus{PipelineState: "success", ApprovalState: "approved"},
			want: mergestate.Snapshot{State: mergestate.StateOpen, Draft: mergestate.Met, TargetBranch: "main", HeadSHA: "h1",
				Pipeline: mergestate.PipelineGreen, Mergeable: mergestate.MergeableReading{Mergeable: mergestate.Met},
				Conflict: mergestate.ConflictClear, Approvals: mergestate.Met, Behind: -1},
		},
		{
			name: "draft with threads and conflicts",
			mr:   gitlab.MR{State: "opened", BaseBranch: "main", HeadSHA: "h2", Draft: true, HasConflicts: true, DetailedMergeStatus: "draft_status"},
			st:   gitlab.MRStatus{PipelineState: "pending", ApprovalState: "pending"},
			disc: 2,
			want: mergestate.Snapshot{State: mergestate.StateOpen, Draft: mergestate.Unmet, TargetBranch: "main", HeadSHA: "h2",
				Pipeline: mergestate.PipelinePending, Mergeable: mergestate.MergeableReading{Mergeable: mergestate.Unmet, Explains: mergestate.CondDraft},
				Conflict: mergestate.ConflictPresent, UnresolvedThreads: 2, Approvals: mergestate.Unmet, Behind: -1},
		},
		{
			name: "locked is in progress",
			mr:   gitlab.MR{State: "locked", BaseBranch: "main", HeadSHA: "h3", DetailedMergeStatus: "locked"},
			st:   gitlab.MRStatus{},
			want: mergestate.Snapshot{State: mergestate.StateOpen, Draft: mergestate.Met, TargetBranch: "main", HeadSHA: "h3", InProgress: true,
				Pipeline: mergestate.PipelineNone, Mergeable: mergestate.MergeableReading{Mergeable: mergestate.Unavailable},
				Conflict: mergestate.ConflictUnknown, Approvals: mergestate.Met, Behind: -1},
		},
		{
			name: "merged",
			mr:   gitlab.MR{State: "merged", BaseBranch: "main", HeadSHA: "h4", MergeCommitSHA: "mc"},
			st:   gitlab.MRStatus{PipelineState: "failure", RequiredApprovals: 1},
			want: mergestate.Snapshot{State: mergestate.StateMerged, Draft: mergestate.Met, TargetBranch: "main", HeadSHA: "h4", MergedHeadSHA: "h4",
				MergeCommit: "mc", Pipeline: mergestate.PipelineRed, Mergeable: mergestate.MergeableReading{Mergeable: mergestate.Unavailable},
				Conflict: mergestate.ConflictUnknown, Approvals: mergestate.Unavailable, Behind: -1},
		},
	} {
		st := tc.st
		st.MR = &tc.mr
		live := &gitlab.MRLiveRead{Status: &st, UnresolvedDiscussions: tc.disc}
		got, err := newTestAdapter(&fakeGitHubAPI{}, &fakeGitLabAPI{live: live}).ReadLive(context.Background(), "ws", glKey())
		tc.want.Key = glKey()
		if err != nil || got != tc.want {
			t.Errorf("%s:\n got %+v err=%v\nwant %+v", tc.name, got, err, tc.want)
		}
	}
}

func TestWireCoordinatorMergeProvidersIgnoresNilService(t *testing.T) {
	wireCoordinatorMergeProviders(nil, mergeRepos(), nil, nil)
}
