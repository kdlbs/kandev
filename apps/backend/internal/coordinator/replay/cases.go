package replay

import (
	"context"
	"errors"
	"sort"
)

// titleLine is the title of a task an expected proposal targets, a label read
// when the replay runs.
type titleLine struct{ taskID, title string }

// caseInput is what a case's prompt is built from besides the instruction
// render.
type caseInput struct {
	snapshot  string
	trigger   string
	wake      bool
	wakeKinds []string
	titles    []titleLine
}

// preparedCase is a selected turn after its reads: either skipped or ready.
type preparedCase struct {
	record CaseRecord
	state  *caseState
	input  caseInput
}

// prepareCase reads what a turn needs. A skip is recorded in the case; a read
// that fails other than by not-found is returned as an error and never a skip.
func prepareCase(ctx context.Context, c Cases, turn Turn) (preparedCase, error) {
	out := preparedCase{record: CaseRecord{TurnID: turn.ID}}
	if turn.Trigger == TriggerDream {
		out.record.Skip = SkipDreamTurn
		return out, nil
	}
	snapshot, found, err := readSnapshot(ctx, c, turn.SnapshotHash)
	if err != nil {
		return out, err
	}
	if !found {
		out.record.Skip = SkipNoSnapshot
		return out, nil
	}
	exp, notes, err := expectations(ctx, c, turn)
	if err != nil {
		return out, err
	}
	out.record.Notes = notes
	in := caseInput{snapshot: snapshot, wake: turn.Trigger == TriggerWake, wakeKinds: turn.WakeKinds}
	gone, err := readTrigger(ctx, c, turn, &in)
	if err != nil {
		return out, err
	}
	if !gone {
		gone, err = readTitles(ctx, c, exp, &in)
		if err != nil {
			return out, err
		}
	}
	switch {
	case gone:
		out.record.Skip = SkipInputGone
	case exp.empty():
		out.record.Skip = SkipNoExpectation
	default:
		out.state = &caseState{id: turn.ID, exp: exp}
		out.input = in
	}
	return out, nil
}

func readSnapshot(ctx context.Context, c Cases, hash string) (string, bool, error) {
	if hash == "" {
		return "", false, nil
	}
	body, err := c.Snapshot(ctx, hash)
	if errors.Is(err, ErrNotFound) {
		return "", false, nil
	}
	return body, err == nil, err
}

// readTrigger fills the trigger text of a message turn; a wake turn has none.
// It reports true when the text is confirmed absent.
func readTrigger(ctx context.Context, c Cases, turn Turn, in *caseInput) (bool, error) {
	if turn.Trigger != TriggerMessage {
		return false, nil
	}
	text, err := c.TriggerText(ctx, turn)
	if errors.Is(err, ErrNotFound) {
		return true, nil
	}
	in.trigger = text
	return false, err
}

// readTitles fills the titles of the tasks the expected proposals of every
// kind but create_task target, once each in task id order. It reports true
// when a title is confirmed absent.
func readTitles(ctx context.Context, c Cases, e expectation, in *caseInput) (bool, error) {
	seen := map[string]bool{}
	var ids []string
	for _, x := range append(append([]expected(nil), e.reproduced...), e.avoided...) {
		if x.kind != ProposalCreateTask && x.target != "" && !seen[x.target] {
			seen[x.target] = true
			ids = append(ids, x.target)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		title, err := c.TaskTitle(ctx, id)
		if errors.Is(err, ErrNotFound) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		in.titles = append(in.titles, titleLine{taskID: id, title: title})
	}
	return false, nil
}

const (
	noteProposalGone = "proposal_gone"
	noteAmbiguousKey = "ambiguous_key"
)

// expectations derives what a manager decided from the turn's outcome rows,
// each outcome's decision key coming from its proposal as the coordinator made
// it. An improvement proposal is never an expectation.
func expectations(ctx context.Context, c Cases, turn Turn) (expectation, []string, error) {
	var notes []string
	reproduced := map[string]expected{}
	avoided := map[string]expected{}
	for _, o := range turn.Outcomes {
		p, err := c.Proposal(ctx, o.ProposalID)
		if errors.Is(err, ErrNotFound) {
			notes = append(notes, noteProposalGone)
			continue
		}
		if err != nil {
			return expectation{}, nil, err
		}
		if p.Kind == ProposalImprovement {
			continue
		}
		x := expected{key: Key(p.Kind, p.TargetTaskID, p.WorkflowID, p.Title), proposalID: o.ProposalID, kind: p.Kind, target: p.TargetTaskID}
		switch {
		case o.Decision == DecisionRejected || o.Decision == DecisionUndone:
			keepSmallest(avoided, x)
		case o.Decision == DecisionApproved && !o.Automatic && len(o.EditedFields) == 0:
			keepSmallest(reproduced, x)
		}
	}
	for key := range reproduced {
		if _, both := avoided[key]; both {
			delete(reproduced, key)
			delete(avoided, key)
			notes = append(notes, noteAmbiguousKey)
		}
	}
	return expectation{reproduced: sortedExpected(reproduced), avoided: sortedExpected(avoided)}, notes, nil
}

func keepSmallest(m map[string]expected, x expected) {
	if cur, ok := m[x.key]; !ok || x.proposalID < cur.proposalID {
		m[x.key] = x
	}
}

func sortedExpected(m map[string]expected) []expected {
	out := make([]expected, 0, len(m))
	for _, x := range m {
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].key < out[j].key })
	return out
}
