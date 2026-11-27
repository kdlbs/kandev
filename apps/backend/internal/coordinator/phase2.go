package coordinator

import (
	"context"
	"time"
)

// ClassSetting is one class's stored setting and who last set it.
type ClassSetting struct {
	Value     Setting
	ChangedBy string
	ChangedAt *time.Time
}

// ActionSettings is the per-class settings surface the automatic path reads.
type ActionSettings interface {
	Setting(ctx context.Context, coordinatorID, class string) (ClassSetting, error)
	// Lower writes the class to requires_approval as a system change and
	// records reason. It writes nothing when the class is not automatic.
	Lower(ctx context.Context, coordinatorID, class, reason string) error
}

// Decision is one decided log row of a class.
type Decision struct {
	ProposalID string
	Outcome    string
	DecidedAt  time.Time
}

// Decision outcomes as the eligibility computation reads them.
const (
	DecisionApproved          = "approved"
	DecisionApprovedWithEdits = "approved_with_edits"
)

// DecisionLog is the decided-row surface the eligibility computation reads.
type DecisionLog interface {
	EarliestDecision(ctx context.Context, coordinatorID, class string) (time.Time, bool, error)
	Decisions(ctx context.Context, coordinatorID, class string, since, before time.Time) ([]Decision, error)
	UndoneTaskIDs(ctx context.Context, coordinatorID string, since, before time.Time) ([]string, error)
}

// storeActionSettings reads the coordinator row and the class change history.
type storeActionSettings struct{ svc *Service }

func (a storeActionSettings) Setting(ctx context.Context, coordinatorID, class string) (ClassSetting, error) {
	c, err := a.svc.store.GetCoordinatorByID(ctx, coordinatorID)
	if err != nil {
		return ClassSetting{}, err
	}
	value := a.svc.policyFor(c).Actions[Action(class)]
	out := ClassSetting{Value: value}
	change, err := a.svc.store.NewestClassChangeTx(ctx, a.svc.store.ro, coordinatorID, Action(class), value)
	if err != nil || change == nil {
		return out, err
	}
	at := change.ChangedAt.UTC()
	out.ChangedBy, out.ChangedAt = change.ChangedBy, &at
	return out, nil
}

func (a storeActionSettings) Lower(ctx context.Context, coordinatorID, class, reason string) error {
	return a.svc.LowerClass(ctx, coordinatorID, Action(class), reason)
}

// storeDecisionLog reads the decided rows of coordinator_activity.
type storeDecisionLog struct{ store *Store }

func (l storeDecisionLog) EarliestDecision(ctx context.Context, coordinatorID, class string) (time.Time, bool, error) {
	if Action(class) != ActionCreateTask {
		return time.Time{}, false, nil
	}
	at, err := l.store.EarliestDecisionAt(ctx, coordinatorID)
	if err != nil || at == nil {
		return time.Time{}, false, err
	}
	return *at, true, nil
}

func (l storeDecisionLog) Decisions(ctx context.Context, coordinatorID, class string, since, before time.Time) ([]Decision, error) {
	if Action(class) != ActionCreateTask {
		return nil, nil
	}
	rows, err := l.store.DecidedRows(ctx, coordinatorID, since, before)
	if err != nil {
		return nil, err
	}
	out := make([]Decision, len(rows))
	for i, r := range rows {
		outcome := r.Outcome
		if outcome == string(ActivityApproved) && r.Edited {
			outcome = DecisionApprovedWithEdits
		}
		out[i] = Decision{ProposalID: r.ProposalID, Outcome: outcome, DecidedAt: r.DecidedAt}
	}
	return out, nil
}

func (l storeDecisionLog) UndoneTaskIDs(ctx context.Context, coordinatorID string, since, before time.Time) ([]string, error) {
	return l.store.UndoneCreateTaskIDs(ctx, coordinatorID, since, before)
}

// SetAutomaticPorts replaces the settings and decision-log adapters; a nil
// argument keeps the store-backed default.
func (s *Service) SetAutomaticPorts(settings ActionSettings, log DecisionLog) {
	s.automatic.mu.Lock()
	defer s.automatic.mu.Unlock()
	s.automatic.settings, s.automatic.log = settings, log
}

func (s *Service) actionSettings() ActionSettings {
	s.automatic.mu.Lock()
	defer s.automatic.mu.Unlock()
	if s.automatic.settings != nil {
		return s.automatic.settings
	}
	return storeActionSettings{svc: s}
}

func (s *Service) decisionLog() DecisionLog {
	s.automatic.mu.Lock()
	defer s.automatic.mu.Unlock()
	if s.automatic.log != nil {
		return s.automatic.log
	}
	return storeDecisionLog{store: s.store}
}
