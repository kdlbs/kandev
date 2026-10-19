package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

const testRaiser = "mgr-1"

// identityAuthorizer authorizes only the listed users; onCall runs before
// each check.
type identityAuthorizer struct {
	mu      sync.Mutex
	allowed map[string]bool
	err     error
	onCall  func()
}

func (a *identityAuthorizer) AuthorizeWorkspaceScope(ctx context.Context, _ string, _ authz.Scope) error {
	a.mu.Lock()
	onCall, err := a.onCall, a.err
	a.onCall = nil
	a.mu.Unlock()
	if onCall != nil {
		onCall()
	}
	if err != nil {
		return err
	}
	if id, ok := authn.IdentityFromContext(ctx); ok && !a.allowed[id.UserID] {
		return taskservice.ErrForbidden
	}
	return nil
}

type fakeIdentities struct {
	known map[string]bool
	err   error
}

func (f fakeIdentities) ResolveUserIdentity(_ context.Context, userID string) (authn.Identity, bool, error) {
	if f.err != nil {
		return authn.Identity{}, false, f.err
	}
	return authn.Identity{UserID: userID}, f.known[userID], nil
}

// raisedFixture is an automaticFixture whose create_task is raised to
// automatic by testRaiser.
func raisedFixture(t *testing.T) (*Store, *Coordinator, *fakeDecisionTaskService, *Service, *identityAuthorizer) {
	t.Helper()
	store, c, tasks, svc, _ := automaticFixture(t)
	az := &identityAuthorizer{allowed: map[string]bool{testRaiser: true}}
	svc.authz = az
	svc.SetAutomaticIdentities(fakeIdentities{known: map[string]bool{testRaiser: true}})
	if _, err := svc.SaveSettings(authedContext(testRaiser), c.WorkspaceID, c.ID, []byte(raiseBody())); err != nil {
		t.Fatalf("raise: %v", err)
	}
	return store, c, tasks, svc, az
}

func settingOf(t *testing.T, store *Store, svc *Service, c *Coordinator) Setting {
	t.Helper()
	stored, err := store.GetCoordinatorByID(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	return svc.policyFor(stored).Actions[ActionCreateTask]
}

func proposeAuto(t *testing.T, store *Store, c *Coordinator, svc *Service) (*Proposal, *AutomaticResult) {
	t.Helper()
	p := insertKind(t, store, c, ProposalKindCreateTask)
	res, err := svc.TryAutomaticApproval(context.Background(), p)
	if err != nil {
		t.Fatalf("TryAutomaticApproval: %v", err)
	}
	return p, res
}

func reload(t *testing.T, store *Store, c *Coordinator, id string) *Proposal {
	t.Helper()
	p, err := store.GetProposal(context.Background(), c.WorkspaceID, c.ID, id, true)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAutomaticApproval_ApprovesAndRecordsRaiser(t *testing.T) {
	store, c, tasks, svc, _ := raisedFixture(t)
	p, res := proposeAuto(t, store, c, svc)
	if res == nil || res.Status != ProposalStatusApproved || res.TaskID != "task-new" || res.Note != "" {
		t.Fatalf("res = %+v", res)
	}
	got := reload(t, store, c, p.ID)
	if got.Status != ProposalStatusApproved || !got.DecidedAutomatically || !got.ClaimedAutomatically || got.AutomaticAt == nil {
		t.Fatalf("proposal = %+v", got)
	}
	if len(tasks.createCalls) != 1 || tasks.createCalls[0].Metadata != nil {
		t.Fatalf("createCalls = %+v", tasks.createCalls)
	}
	rows := listActivity(t, store, c.ID)
	if len(rows) != 1 || rows[0].Outcome != ActivityApproved || rows[0].Authorization != AuthAutomatic || rows[0].ActorUserID == nil || *rows[0].ActorUserID != testRaiser {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestAutomaticApproval_NilWhenNotAutomatic(t *testing.T) {
	store, c, _, svc, _ := automaticFixture(t)
	p := insertKind(t, store, c, ProposalKindCreateTask)
	res, err := svc.TryAutomaticApproval(context.Background(), p)
	if err != nil || res != nil {
		t.Fatalf("res = %+v err = %v", res, err)
	}
	if reload(t, store, c, p.ID).Status != ProposalStatusPending {
		t.Fatal("proposal left pending expected")
	}
}

func TestAutomaticApproval_FailedApprovalIsRecordedAndCounted(t *testing.T) {
	store, c, tasks, svc, _ := raisedFixture(t)
	tasks.createErr = errors.New("create failed")
	p, res := proposeAuto(t, store, c, svc)
	if res == nil || res.Status != ProposalStatusFailed {
		t.Fatalf("res = %+v", res)
	}
	got := reload(t, store, c, p.ID)
	if got.Status != ProposalStatusFailed || !got.DecidedAutomatically || got.AutomaticAt == nil {
		t.Fatalf("proposal = %+v", got)
	}
	rows := listActivity(t, store, c.ID)
	if len(rows) != 1 || rows[0].Outcome != ActivityFailed || rows[0].Authorization != AuthAutomatic || rows[0].ActorUserID == nil || *rows[0].ActorUserID != testRaiser {
		t.Fatalf("rows = %+v", rows)
	}
	n, err := store.CountAutomaticDecidedTx(context.Background(), store.db, c.ID, automaticNow.Add(-time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("count = %d err = %v", n, err)
	}
}

func TestAutomaticApproval_EleventhInADayStaysPending(t *testing.T) {
	store, c, tasks, svc, _ := raisedFixture(t)
	for i := 0; i < automaticDailyLimit; i++ {
		tasks.createErr = nil
		if i%3 == 0 {
			tasks.createErr = errors.New("create failed")
		}
		if _, res := proposeAuto(t, store, c, svc); res == nil || res.Note != "" {
			t.Fatalf("approval %d: %+v", i, res)
		}
	}
	tasks.createErr = nil
	calls := len(tasks.createCalls)
	p, res := proposeAuto(t, store, c, svc)
	if res == nil || res.Status != ProposalStatusPending || res.Note != limitReachedNote {
		t.Fatalf("res = %+v", res)
	}
	if got := reload(t, store, c, p.ID); got.Status != ProposalStatusPending || got.AutomaticAt != nil || len(tasks.createCalls) != calls {
		t.Fatalf("limited proposal was claimed: %+v", got)
	}
}

func TestAutomaticApproval_LimitWindowRolls(t *testing.T) {
	store, c, _, svc, _ := raisedFixture(t)
	for i := 0; i < automaticDailyLimit; i++ {
		proposeAuto(t, store, c, svc)
	}
	store.now = func() time.Time { return automaticNow.Add(25 * time.Hour) }
	_, res := proposeAuto(t, store, c, svc)
	if res == nil || res.Status != ProposalStatusApproved {
		t.Fatalf("res = %+v", res)
	}
}

func TestAutomaticApproval_InvalidRaiserLowersAndLeavesPending(t *testing.T) {
	cases := map[string]func(*identityAuthorizer, *Service){
		"no longer a manager": func(az *identityAuthorizer, _ *Service) { az.allowed = map[string]bool{} },
		"deleted or disabled": func(_ *identityAuthorizer, svc *Service) { svc.SetAutomaticIdentities(fakeIdentities{}) },
	}
	for name, spoil := range cases {
		t.Run(name, func(t *testing.T) {
			store, c, tasks, svc, az := raisedFixture(t)
			spoil(az, svc)
			p, res := proposeAuto(t, store, c, svc)
			if res == nil || res.Status != ProposalStatusPending || res.Note != unavailableNote {
				t.Fatalf("res = %+v", res)
			}
			if got := reload(t, store, c, p.ID); got.Status != ProposalStatusPending || got.AutomaticAt != nil || len(tasks.createCalls) != 0 {
				t.Fatalf("proposal claimed: %+v", got)
			}
			if settingOf(t, store, svc, c) != SettingRequiresApproval {
				t.Fatal("class not lowered")
			}
			change, _ := store.NewestClassChangeTx(context.Background(), store.ro, c.ID, ActionCreateTask, SettingRequiresApproval)
			if change == nil || change.Reason != raiserLoweredReason || change.ChangedBy != "" {
				t.Fatalf("change = %+v", change)
			}
		})
	}
}

func TestAutomaticApproval_RaiserCheckErrorDoesNotLower(t *testing.T) {
	store, c, _, svc, az := raisedFixture(t)
	az.err = errors.New("authz backend down")
	p, res := proposeAuto(t, store, c, svc)
	if res == nil || res.Status != ProposalStatusPending || res.Note != unavailableNote {
		t.Fatalf("res = %+v", res)
	}
	if reload(t, store, c, p.ID).AutomaticAt != nil || settingOf(t, store, svc, c) != SettingAutomatic {
		t.Fatal("check error must neither claim nor lower")
	}
}

func TestAutomaticApproval_LowerBetweenReadAndLockLeavesUnclaimed(t *testing.T) {
	store, c, tasks, svc, az := raisedFixture(t)
	az.onCall = func() {
		if err := svc.LowerClass(context.Background(), c.ID, ActionCreateTask, "manager"); err != nil {
			t.Error(err)
		}
	}
	p, res := proposeAuto(t, store, c, svc)
	if res != nil {
		t.Fatalf("res = %+v, want nil (phase 1 stands)", res)
	}
	if got := reload(t, store, c, p.ID); got.Status != ProposalStatusPending || got.AutomaticAt != nil || len(tasks.createCalls) != 0 {
		t.Fatalf("proposal claimed after lower: %+v", got)
	}
}

func TestAutomaticApproval_SectionFailuresLeavePendingUncounted(t *testing.T) {
	cases := map[string]func(cancel context.CancelFunc) func(context.Context) error{
		"forced error": func(context.CancelFunc) func(context.Context) error {
			return func(context.Context) error { return errors.New("forced") }
		},
		"panic": func(context.CancelFunc) func(context.Context) error {
			return func(context.Context) error { panic("forced panic") }
		},
		"cancelled context": func(cancel context.CancelFunc) func(context.Context) error {
			return func(context.Context) error { cancel(); return context.Canceled }
		},
	}
	for name, mk := range cases {
		t.Run(name, func(t *testing.T) {
			store, c, tasks, svc, _ := raisedFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			svc.automatic.afterClaim = mk(cancel)
			p := insertKind(t, store, c, ProposalKindCreateTask)
			res, err := svc.TryAutomaticApproval(ctx, p)
			if err != nil {
				t.Fatal(err)
			}
			if res == nil || res.Status != ProposalStatusPending || res.Note != unavailableNote {
				t.Fatalf("res = %+v", res)
			}
			got := reload(t, store, c, p.ID)
			if got.Status != ProposalStatusPending || got.AutomaticAt != nil || got.DecidedAutomatically || got.ClaimedAutomatically {
				t.Fatalf("proposal stamped: %+v", got)
			}
			n, _ := store.CountAutomaticDecidedTx(context.Background(), store.db, c.ID, automaticNow.Add(-time.Hour))
			if n != 0 || len(tasks.createCalls) != 0 {
				t.Fatalf("counted %d, created %d", n, len(tasks.createCalls))
			}
		})
	}
}

func TestAutomaticApproval_ManagerDecidedBeforeLockWritesNothing(t *testing.T) {
	store, c, tasks, svc, _ := raisedFixture(t)
	p := insertKind(t, store, c, ProposalKindCreateTask)
	mustExec(t, store, `UPDATE coordinator_proposals SET status = 'rejected' WHERE id = ?`, p.ID)
	res, err := svc.TryAutomaticApproval(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.Status != ProposalStatusRejected {
		t.Fatalf("res = %+v", res)
	}
	if got := reload(t, store, c, p.ID); got.AutomaticAt != nil || got.ClaimedAutomatically || len(tasks.createCalls) != 0 || len(listActivity(t, store, c.ID)) != 0 {
		t.Fatalf("wrote after a manager decided: %+v", got)
	}
}

func unattendedStamp(t *testing.T, store *Store, coordinatorID string) sql.NullString {
	t.Helper()
	var got sql.NullString
	err := store.db.GetContext(context.Background(), &got, store.db.Rebind(
		`SELECT unattended_turn_id FROM coordinator_activity WHERE coordinator_id = ?`), coordinatorID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestAutomaticApproval_StampsOpenUnattendedTurn(t *testing.T) {
	store, c, _, svc, _ := raisedFixture(t)
	svc.SetSpendDeps(nil, &fakeActiveTurns{turns: map[string]string{"sess": "st"}}, nil)
	mustExec(t, store, `INSERT INTO coordinator_unattended_turns
		(id, coordinator_id, conversation_task_id, session_id, session_turn_id, wake_count, start_ceiling_subcents, started_at)
		VALUES ('ut', ?, 'conv', 'sess', 'st', 1, 100, ?)`, c.ID, automaticNow)
	proposeAuto(t, store, c, svc)
	if got := unattendedStamp(t, store, c.ID); got.String != "ut" {
		t.Fatalf("stamp = %+v", got)
	}
}

func TestAutomaticApproval_NoStampOutsideAnUnattendedTurn(t *testing.T) {
	store, c, _, svc, _ := raisedFixture(t)
	svc.SetSpendDeps(nil, &fakeActiveTurns{turns: map[string]string{"sess": "manager-turn"}}, nil)
	mustExec(t, store, `INSERT INTO coordinator_unattended_turns
		(id, coordinator_id, conversation_task_id, session_id, session_turn_id, wake_count, start_ceiling_subcents, started_at)
		VALUES ('ut', ?, 'conv', 'sess', 'st', 1, 100, ?)`, c.ID, automaticNow)
	proposeAuto(t, store, c, svc)
	if got := unattendedStamp(t, store, c.ID); got.Valid {
		t.Fatalf("stamp = %+v, want none", got)
	}
}

func TestAutomaticApproval_ManagerApprovalAfterFailureKeepsAutomaticAtAndAuthorization(t *testing.T) {
	store, c, tasks, svc, az := raisedFixture(t)
	az.allowed["mgr-2"] = true
	tasks.createErr = errors.New("create failed")
	p, _ := proposeAuto(t, store, c, svc)
	failedAt := reload(t, store, c, p.ID).AutomaticAt
	tasks.createErr = nil
	if _, err := svc.ApproveProposal(authedContext("mgr-2"), c.WorkspaceID, c.ID, p.ID, ApproveProposalRequest{}); err != nil {
		t.Fatalf("manager approve: %v", err)
	}
	got := reload(t, store, c, p.ID)
	if got.Status != ProposalStatusApproved || got.ClaimedAutomatically || !got.DecidedAutomatically || got.AutomaticAt == nil || !got.AutomaticAt.Equal(*failedAt) {
		t.Fatalf("proposal = %+v", got)
	}
	for _, r := range listActivity(t, store, c.ID) {
		switch r.Outcome {
		case ActivityApproved:
			if r.Authorization != AuthRequiresApproval {
				t.Fatalf("approved row authorization = %s", r.Authorization)
			}
		case ActivityFailed:
			if r.Authorization != AuthAutomatic {
				t.Fatalf("failed row authorization = %s", r.Authorization)
			}
		}
	}
}

func noTurnStartAutomaticApproval(t *testing.T) {
	store, c, tasks, svc, _ := raisedFixture(t)
	messenger, resumer := &fakeMessenger{}, &fakeResumer{}
	svc.SetKindDeps(KindDeps{Tasks: idleKindTasks{}, Messenger: messenger, Resumer: resumer})
	mustExec(t, store, `UPDATE coordinators SET conversation_task_id = 'conv-1' WHERE id = ?`, c.ID)
	if _, res := proposeAuto(t, store, c, svc); res == nil || res.Status != ProposalStatusApproved {
		t.Fatalf("res = %+v", res)
	}
	if len(messenger.prompts) != 0 || resumer.calls != 0 {
		t.Fatalf("automatic approval started a turn: prompts=%v resumes=%d", messenger.prompts, resumer.calls)
	}
	for _, call := range tasks.createCalls {
		if call.Metadata != nil {
			t.Fatalf("automatic approval asked the created task to start an agent: %v", call.Metadata)
		}
	}
}

type idleKindTasks struct{}

func (idleKindTasks) GetTarget(context.Context, string) (*TargetTask, error) {
	return nil, ErrNotFound
}
func (idleKindTasks) HasLiveExecution(context.Context, string) bool { return false }

func TestAutomaticApproval_StartsAgentProposalStaysPendingForAManager(t *testing.T) {
	store, c, tasks, svc, _ := raisedFixture(t)
	p := insertKind(t, store, c, ProposalKindCreateTask)
	p.StartsAgent = true
	res, err := svc.TryAutomaticApproval(context.Background(), p)
	if err != nil || res != nil {
		t.Fatalf("res = %+v err = %v, want nil (phase 1 stands)", res, err)
	}
	got := reload(t, store, c, p.ID)
	if got.Status != ProposalStatusPending || got.AutomaticAt != nil || got.ClaimedAutomatically || len(tasks.createCalls) != 0 {
		t.Fatalf("agent-starting proposal handled automatically: %+v", got)
	}
	if settingOf(t, store, svc, c) != SettingAutomatic {
		t.Fatal("class must not be lowered")
	}
}

func TestAutomaticApproval_IdentityLookupErrorDoesNotLower(t *testing.T) {
	store, c, tasks, svc, _ := raisedFixture(t)
	svc.SetAutomaticIdentities(fakeIdentities{err: errors.New("db down")})
	p, res := proposeAuto(t, store, c, svc)
	if res == nil || res.Status != ProposalStatusPending || res.Note != unavailableNote {
		t.Fatalf("res = %+v", res)
	}
	if got := reload(t, store, c, p.ID); got.AutomaticAt != nil || len(tasks.createCalls) != 0 {
		t.Fatalf("claimed on lookup error: %+v", got)
	}
	if settingOf(t, store, svc, c) != SettingAutomatic {
		t.Fatal("a lookup error must not lower the class")
	}
}

func TestAutomaticApproval_DailyLimitIgnoresProjectScope(t *testing.T) {
	store, c, _, svc, _ := raisedFixture(t)
	for i := 0; i < automaticDailyLimit; i++ {
		proposeAuto(t, store, c, svc)
	}
	mustExec(t, store, `UPDATE coordinators SET project_scope = 'selected', include_no_repository = 0 WHERE id = ?`, c.ID)
	mustExec(t, store, `INSERT INTO coordinator_watch_projects (coordinator_id, entry_kind, entry_id, workspace_id, created_at) VALUES (?, 'repository', 'elsewhere', ?, ?)`,
		c.ID, c.WorkspaceID, automaticNow)
	n, err := store.CountAutomaticDecidedTx(context.Background(), store.db, c.ID, automaticNow.Add(-automaticLimitWindow))
	if err != nil || n != automaticDailyLimit {
		t.Fatalf("count after narrowing the scope = %d err = %v, want %d", n, err, automaticDailyLimit)
	}
	if _, res := proposeAuto(t, store, c, svc); res == nil || res.Note != limitReachedNote {
		t.Fatalf("res = %+v", res)
	}
}
