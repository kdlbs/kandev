package coordinator

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

// Creation codes of a refused propose_merge call, in validation order.
const (
	createChangeUnknown = "change_unknown"
	createPolicyDenied  = "policy_denied"
	createPredecessor   = "predecessor"
	createMethod        = "method"
	createUnavailable   = "unavailable"
	createHeadMismatch  = "head_mismatch"
)

const (
	statusRefusedCreation = "refused"
	fieldChange           = "change"
	fieldExpectedHead     = "expected_head_sha"
)

// MergeProposeResult is what propose_merge_kandev returns. A creation refusal
// has a nil ProposalID and the refused status.
type MergeProposeResult struct {
	ProposalID   *string `json:"proposal_id"`
	Status       string  `json:"status"`
	Reason       string  `json:"reason,omitempty"`
	CreationCode string  `json:"creation_code,omitempty"`
	Deduplicated bool    `json:"deduplicated,omitempty"`
	// OwningCoordinatorID names the coordinator that owns a deduplicated
	// proposal when it is not the caller.
	OwningCoordinatorID string `json:"owning_coordinator_id,omitempty"`
}

type mergeArgs struct {
	TaskID          string `json:"task_id"`
	Change          string `json:"change"`
	ExpectedHeadSHA string `json:"expected_head_sha"`
}

func parseMergeArgs(raw json.RawMessage) (mergeArgs, error) {
	var a mergeArgs
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return a, &FieldError{Field: "", Message: messageUnknownField}
	}
	a.TaskID, a.Change, a.ExpectedHeadSHA = strings.TrimSpace(a.TaskID), strings.TrimSpace(a.Change), strings.TrimSpace(a.ExpectedHeadSHA)
	if a.TaskID == "" {
		return a, &FieldError{Field: fieldTaskID, Message: "task_id is required"}
	}
	if a.ExpectedHeadSHA == "" {
		return a, &FieldError{Field: fieldExpectedHead, Message: "expected_head_sha is required"}
	}
	return a, nil
}

// mergeTarget is the change a propose call resolved to.
type mergeTarget struct {
	key    mergestate.ChangeKey
	repo   RepoPolicyRow
	method string
	head   string
}

// ProposeMerge validates and stores a merge proposal for the coordinator's
// watched task. It never dispatches: an automatic proposal is evaluated here
// and dispatched by a later pass.
func (s *Service) ProposeMerge(ctx context.Context, coordinatorID string, raw json.RawMessage) (*MergeProposeResult, error) {
	if !s.merge {
		return nil, &PolicyDeniedError{Action: ActionMerge}
	}
	args, err := parseMergeArgs(raw)
	if err != nil {
		return nil, err
	}
	c, err := s.store.GetCoordinatorByID(ctx, coordinatorID)
	if err != nil {
		return nil, err
	}
	if err := s.requireWatchedTask(ctx, c, args.TaskID); err != nil {
		return nil, err
	}
	target, code, err := s.resolveMergeTarget(ctx, c, args)
	if err != nil {
		return nil, err
	}
	if code != "" {
		return s.refuseCreation(ctx, c, args.TaskID, code)
	}
	return s.insertMergeProposal(ctx, c, args, target)
}

func (s *Service) requireWatchedTask(ctx context.Context, c *Coordinator, taskID string) error {
	watched, err := s.watchedTasksFor(ctx, c, []string{taskID})
	if err != nil {
		return err
	}
	if _, ok := watched[taskID]; !ok {
		return notFoundTarget()
	}
	return nil
}

// resolveMergeTarget runs the creation checks in their fixed order and returns
// the first failing code. A read that cannot complete is a code, never an
// error, except a failure of Kandev's own store.
func (s *Service) resolveMergeTarget(ctx context.Context, c *Coordinator, args mergeArgs) (*mergeTarget, string, error) {
	key, ok, err := s.resolveChange(ctx, c.WorkspaceID, args)
	if err != nil || !ok {
		return nil, createChangeUnknown, err
	}
	row, ok, err := s.repoPolicyOf(ctx, c, key.RepoID)
	if err != nil || !ok {
		return nil, createPolicyDenied, err
	}
	state, _, err := s.predecessorState(ctx, c.WorkspaceID, c.ID, args.TaskID)
	if err != nil || state != mergestate.Met {
		return nil, createPredecessor, nil
	}
	merger := s.changeMerger()
	if merger == nil {
		return nil, createUnavailable, nil
	}
	methods, err := merger.Methods(ctx, c.WorkspaceID, key)
	if err != nil || methods.Default == "" || !methods.allows(methods.Default) {
		return nil, createMethod, nil
	}
	snap, err := merger.ReadLive(ctx, c.WorkspaceID, key)
	if err != nil || snap.State != mergestate.StateOpen || snap.HeadSHA == "" {
		return nil, createUnavailable, nil
	}
	if snap.HeadSHA != args.ExpectedHeadSHA {
		return nil, createHeadMismatch, nil
	}
	return &mergeTarget{key: key, repo: row, method: methods.Default, head: snap.HeadSHA}, "", nil
}

// resolveChange finds the one change the call names: the given reference when
// the task links it, else the task's only linked change.
func (s *Service) resolveChange(ctx context.Context, workspaceID string, args mergeArgs) (mergestate.ChangeKey, bool, error) {
	links := s.changeLinkReader()
	if links == nil {
		return mergestate.ChangeKey{}, false, nil
	}
	keys, err := links.ChangesLinkedToTask(ctx, workspaceID, args.TaskID)
	if err != nil {
		return mergestate.ChangeKey{}, false, err
	}
	chosen := ""
	switch {
	case args.Change != "":
		for _, k := range keys {
			if k == args.Change {
				chosen = k
			}
		}
	case len(keys) == 1:
		chosen = keys[0]
	}
	key, ok := mergestate.ParseChangeKey(chosen)
	return key, ok && chosen != "", nil
}

// repoPolicyOf returns the coordinator's stored policy of a live repository and
// reports false for a denied, missing or deleted one.
func (s *Service) repoPolicyOf(ctx context.Context, c *Coordinator, repositoryID string) (RepoPolicyRow, bool, error) {
	policies, err := s.store.loadRepoPolicies(ctx, s.store.ro, c.ID)
	if err != nil {
		return RepoPolicyRow{}, false, err
	}
	row, ok := policies[repositoryID]
	if !ok || row.Policy == RepoDenied {
		return RepoPolicyRow{}, false, nil
	}
	live, err := s.store.liveRepositories(ctx, s.store.ro, c.WorkspaceID)
	if err != nil {
		return RepoPolicyRow{}, false, err
	}
	for _, r := range live {
		if r.ID == repositoryID {
			return row, true, nil
		}
	}
	return RepoPolicyRow{}, false, nil
}

// refuseCreation records the refusal and answers without a proposal.
func (s *Service) refuseCreation(ctx context.Context, c *Coordinator, taskID, code string) (*MergeProposeResult, error) {
	row := ActivityRow{
		CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID, ActionClass: ActionMerge, Outcome: ActivityRefused,
		Authorization: AuthRequiresApproval, ReasonCode: optString(code), TargetTaskID: optString(taskID),
		UnattendedTurnID: s.currentUnattendedTurn(ctx, c.ID),
	}
	err := s.store.withCoordinatorLock(ctx, c.ID, func(tx coordinatorExec) error { return s.Record(ctx, tx, row) })
	if err != nil {
		s.logger.Warn("merge creation refusal not recorded", zap.String("code", code), zap.Error(err))
	}
	return &MergeProposeResult{Status: statusRefusedCreation, CreationCode: code, Reason: code}, nil
}

// insertMergeProposal stores the proposal, or returns the open one for the
// change. A pending proposal frozen on another head is settled stale first.
func (s *Service) insertMergeProposal(ctx context.Context, c *Coordinator, args mergeArgs, t *mergeTarget) (*MergeProposeResult, error) {
	spec, err := json.Marshal(mergeSpec{
		TaskID: args.TaskID, RepositoryID: t.key.RepoID, ChangeKey: t.key.String(), HeadSHA: t.head, Method: t.method,
	})
	if err != nil {
		return nil, err
	}
	p := &Proposal{CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID, Kind: ProposalKindMerge, TargetTaskID: &args.TaskID, RawSpec: string(spec)}
	var existing *Proposal
	pre := func(ctx context.Context, tx coordinatorExec) (*Proposal, error) {
		open, err := s.store.openMergeForChange(ctx, tx, c.WorkspaceID, t.key.String())
		if err != nil || open == nil {
			return nil, err
		}
		row, err := s.store.proposalByIDTx(ctx, tx, open.ProposalID)
		if err != nil {
			return nil, err
		}
		if row.Status == ProposalStatusPending && open.HeadSHA != t.head {
			settled, err := s.settleRefusedTx(ctx, tx, row, refuseStaleHead)
			if err != nil || settled {
				return nil, err
			}
		}
		existing = row
		return row, nil
	}
	turnID := s.currentUnattendedTurn(ctx, c.ID)
	m := MergeProposal{WorkspaceID: c.WorkspaceID, CoordinatorID: c.ID, RepositoryID: t.key.RepoID, ChangeKey: t.key.String(), HeadSHA: t.head, Method: t.method}
	if t.repo.Policy == RepoAutomatic {
		rev := t.repo.AutomaticSinceRevision
		m.AutomaticRevision = &rev
	}
	inTx := func(ctx context.Context, tx coordinatorExec, p *Proposal) error {
		m.ProposalID = p.ID
		if err := s.store.insertMergeRowTx(ctx, tx, m); err != nil {
			return err
		}
		return s.recordMergeProposed(ctx, tx, p, args, t, turnID)
	}
	if err := s.store.InsertProposalWith(ctx, p, s.phase2, pre, inTx); err != nil {
		return nil, err
	}
	if existing != nil {
		return dedupedResult(c, existing), nil
	}
	s.publishCoordinatorUpdated(ctx, c.WorkspaceID, c.ID)
	return s.evaluateCreated(ctx, c, p, &m)
}

func dedupedResult(c *Coordinator, p *Proposal) *MergeProposeResult {
	res := &MergeProposeResult{ProposalID: &p.ID, Status: string(p.Status), Deduplicated: true}
	if p.CoordinatorID != c.ID {
		res.OwningCoordinatorID = p.CoordinatorID
	}
	return res
}
