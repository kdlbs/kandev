package coordinator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
)

// AdmitMode selects whether an admission run counts containment failures.
type AdmitMode int

const (
	// AdmitCounting counts each unmet containment condition.
	AdmitCounting AdmitMode = iota + 1
	// AdmitReadOnly writes and counts nothing.
	AdmitReadOnly
)

// unattendedCooldown is the minimum gap between the end of one unattended turn
// and the start of the next.
const unattendedCooldown = 5 * time.Minute

// Admission reasons and details; the reason set is closed.
const (
	admitAutonomyOff      = "autonomy_off"
	admitContainment      = "containment"
	admitSpendUnmeasured  = "spend_unmeasured"
	admitCeilingReached   = "ceiling_reached"
	admitNoConversation   = "no_conversation"
	admitConvUnavailable  = "conversation_unavailable"
	admitConvBusy         = "conversation_busy"
	admitCooldown         = "cooldown"
	admitDetailReadError  = "read_error"
	admitDetailNotFound   = "coordinator_not_found"
	admitDetailNotStarted = "session_not_started"
	admitDetailTurnOpen   = "turn_open"
)

// Admission is the result of the ordered admission checks.
type Admission struct {
	OK     bool
	Reason string
	Detail string
	// TaskID and SessionID name the admitted conversation when OK.
	TaskID    string
	SessionID string
	// Until is set for a cooldown hold: the newest settled turn's finish plus the cooldown.
	Until *time.Time
}

func held(reason, detail string) Admission { return Admission{Reason: reason, Detail: detail} }

// Admit runs the eight ordered admission checks; the first failure wins. It
// writes nothing. A read error inside a check fails that check.
func (s *Service) Admit(ctx context.Context, coordinatorID string, mode AdmitMode) Admission {
	coord, a, ok := s.admitThroughSpend(ctx, coordinatorID, mode)
	if !ok {
		return a
	}
	conv, ok := s.admitConversation(ctx, coord)
	if !ok {
		return conv
	}
	res := s.admitCooldown(ctx, coordinatorID)
	if res.OK {
		res.TaskID, res.SessionID = conv.TaskID, conv.SessionID
	}
	return res
}

// AdmitAction runs the checks of Admit up to the cost ceiling (autonomy on,
// containment, spend measured, ceiling not reached) and skips the conversation
// and cooldown checks: an automatic action happens inside the coordinator's
// own turn. It starts no turn and writes and counts nothing.
func (s *Service) AdmitAction(ctx context.Context, coordinatorID string) Admission {
	_, a, ok := s.admitThroughSpend(ctx, coordinatorID, AdmitReadOnly)
	if !ok {
		return a
	}
	return Admission{OK: true}
}

// admitThroughSpend is the shared prefix of Admit and AdmitAction.
func (s *Service) admitThroughSpend(ctx context.Context, coordinatorID string, mode AdmitMode) (*Coordinator, Admission, bool) {
	if mode != AdmitCounting && mode != AdmitReadOnly {
		return nil, held(admitAutonomyOff, admitDetailReadError), false
	}
	coord, err := s.store.GetCoordinatorByID(ctx, coordinatorID)
	if errors.Is(err, ErrNotFound) {
		return nil, held(admitAutonomyOff, admitDetailNotFound), false
	}
	if err != nil {
		return nil, held(admitAutonomyOff, admitDetailReadError), false
	}
	if !coord.AutonomyEnabled {
		return nil, held(admitAutonomyOff, ""), false
	}
	if a, ok := s.admitContainment(ctx, coord, mode); !ok {
		return nil, a, false
	}
	if a, ok := s.admitSpend(ctx, coord); !ok {
		return nil, a, false
	}
	return coord, Admission{}, true
}

func (s *Service) admitContainment(ctx context.Context, coord *Coordinator, mode AdmitMode) (Admission, bool) {
	if s.containment == nil {
		return held(admitContainment, admitDetailReadError), false
	}
	var res ContainmentResult
	if mode == AdmitCounting {
		res = s.containment.CheckForAdmission(ctx, coord, nil)
	} else {
		res = s.containment.Check(ctx, coord)
	}
	if res.Contained {
		return Admission{}, true
	}
	first, _ := res.FirstUnmet()
	return held(admitContainment, first.Name), false
}

func (s *Service) admitSpend(ctx context.Context, coord *Coordinator) (Admission, bool) {
	reading, err := s.Spend(ctx, coord, s.store.now())
	if !CheckSpendMeasurable(reading, err) {
		return held(admitSpendUnmeasured, ""), false
	}
	if coord.CostCeilingSubcents == nil || !CheckCeilingNotReached(reading, *coord.CostCeilingSubcents) {
		return held(admitCeilingReached, ""), false
	}
	return Admission{}, true
}

func (s *Service) admitConversation(ctx context.Context, coord *Coordinator) (Admission, bool) {
	if coord.ConversationTaskID == nil || *coord.ConversationTaskID == "" || s.conversationTasks == nil {
		return held(admitNoConversation, ""), false
	}
	task, err := s.conversationTasks.GetTask(ctx, *coord.ConversationTaskID)
	if err != nil && !errors.Is(err, taskrepo.ErrTaskNotFound) {
		return held(admitNoConversation, admitDetailReadError), false
	}
	if err != nil || task == nil || task.ArchivedAt != nil {
		return held(admitNoConversation, ""), false
	}
	if !ownsConversationTask(coord, task) {
		return held(admitNoConversation, ""), false
	}
	if s.convReader == nil {
		return held(admitConvUnavailable, admitDetailReadError), false
	}
	sess, err := s.convReader.PrimarySession(ctx, task.ID)
	if err != nil {
		return held(admitConvUnavailable, admitDetailReadError), false
	}
	if sess == nil {
		return held(admitConvUnavailable, ""), false
	}
	switch taskmodels.TaskSessionState(sess.State) {
	case taskmodels.TaskSessionStateFailed, taskmodels.TaskSessionStateCancelled, taskmodels.TaskSessionStateCompleted:
		return held(admitConvUnavailable, ""), false
	case taskmodels.TaskSessionStateCreated:
		return held(admitConvUnavailable, admitDetailNotStarted), false
	}
	a, ok := s.admitIdle(ctx, coord.ID, sess)
	a.TaskID, a.SessionID = task.ID, sess.ID
	return a, ok
}

func ownsConversationTask(coord *Coordinator, task *taskmodels.Task) bool {
	return task.ID == *coord.ConversationTaskID && task.WorkspaceID == coord.WorkspaceID &&
		task.Origin == taskmodels.TaskOriginCoordinator && conversationTaskCoordinatorID(task) == coord.ID && !isDreamTask(task)
}

func (s *Service) admitIdle(ctx context.Context, coordinatorID string, sess *ConversationSession) (Admission, bool) {
	state := taskmodels.TaskSessionState(sess.State)
	if state != taskmodels.TaskSessionStateWaitingForInput && state != taskmodels.TaskSessionStateIdle {
		return held(admitConvBusy, ""), false
	}
	pending, err := s.convReader.ActionPending(ctx, sess.ID)
	if err != nil {
		return held(admitConvBusy, admitDetailReadError), false
	}
	if pending {
		return held(admitConvBusy, ""), false
	}
	queued, err := s.convReader.Queued(ctx, sess.ID)
	if err != nil {
		return held(admitConvBusy, admitDetailReadError), false
	}
	if queued {
		return held(admitConvBusy, ""), false
	}
	if s.activeTurns == nil {
		return held(admitConvBusy, admitDetailReadError), false
	}
	turn, err := s.activeTurns.GetActiveTurn(ctx, sess.ID)
	if err != nil {
		return held(admitConvBusy, admitDetailReadError), false
	}
	if turn != nil {
		return held(admitConvBusy, ""), false
	}
	open, err := s.store.openCeilingTurn(ctx, coordinatorID)
	if err != nil {
		return held(admitConvBusy, admitDetailReadError), false
	}
	if open != nil {
		return held(admitConvBusy, admitDetailTurnOpen), false
	}
	return Admission{}, true
}

func (s *Service) admitCooldown(ctx context.Context, coordinatorID string) Admission {
	finished, err := s.store.lastTurnFinishedAt(ctx, coordinatorID)
	if err != nil {
		return held(admitCooldown, admitDetailReadError)
	}
	if finished == nil {
		return Admission{OK: true}
	}
	until := finished.Add(unattendedCooldown)
	if s.store.now().Before(until) {
		a := held(admitCooldown, "")
		a.Until = &until
		return a
	}
	return Admission{OK: true}
}

// lastTurnFinishedAt is the newest settled turn's finished_at, nil when none.
func (s *Store) lastTurnFinishedAt(ctx context.Context, coordinatorID string) (*time.Time, error) {
	var at sql.NullTime
	err := s.ro.QueryRowContext(ctx, s.ro.Rebind(`
		SELECT finished_at FROM coordinator_unattended_turns
		WHERE coordinator_id = ? AND finished_at IS NOT NULL
		ORDER BY finished_at DESC, id DESC LIMIT 1`), coordinatorID).Scan(&at)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !at.Valid) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read last unattended turn: %w", err)
	}
	t := at.Time.UTC()
	return &t, nil
}
