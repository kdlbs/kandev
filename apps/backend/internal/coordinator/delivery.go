package coordinator

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator"
)

var (
	wakeDeliveredTotal  = spendExpvarInt("coordinator_wake_delivered_total")
	wakeSupersededTotal = spendExpvarInt("coordinator_wake_superseded_total")
	admissionHeldTotal  = spendExpvarMap("coordinator_admission_held_total")
)

// bindTimeout bounds a turn-row write made from a send callback, which runs
// after the send's own context may be done.
const bindTimeout = 5 * time.Second

// Deliver runs one delivery attempt for a coordinator: admission, the episode
// re-check, the turn start and the send. An error is left to the next trigger
// or backstop tick.
func (s *Service) Deliver(ctx context.Context, coordinatorID string) error {
	release, err := s.deliverLocks.acquire(ctx, coordinatorID)
	if err != nil {
		return err
	}
	defer release()
	adm := s.Admit(ctx, coordinatorID, AdmitCounting)
	if !adm.OK {
		admissionHeldTotal.Add(adm.Reason, 1)
		return nil
	}
	coord, err := s.store.GetCoordinatorByID(ctx, coordinatorID)
	if err != nil {
		return err
	}
	wakes, err := s.holdingWakes(ctx, coord)
	if err != nil || len(wakes) == 0 {
		return err
	}
	ids := make([]string, len(wakes))
	for i, w := range wakes {
		ids[i] = w.ID
	}
	started, err := s.store.startUnattendedTurn(ctx, turnStart{
		CoordinatorID: coordinatorID, ConvTaskID: adm.TaskID, SessionID: adm.SessionID, WakeIDs: ids,
	})
	if err != nil || started == nil {
		return err
	}
	wakeDeliveredTotal.Add(int64(len(started.Wakes)))
	s.publishCoordinatorUpdatedWith(ctx, started.WorkspaceID, coordinatorID, true)
	return s.sendTurn(ctx, coord, adm, started)
}

func (s *Service) sendTurn(ctx context.Context, coord *Coordinator, adm Admission, started *startedTurn) error {
	if s.wakeSender == nil {
		return errors.New("coordinator delivery: wake sender is not wired")
	}
	turnID := started.ID
	messageID, err := s.wakeSender.PromptUnattendedWake(ctx, orchestrator.UnattendedWakePrompt{
		TaskID:     adm.TaskID,
		SessionID:  adm.SessionID,
		Content:    buildWakeMessage(s.wakeLines(ctx, started.Wakes)),
		WakeTurnID: turnID,
		OnReserved: func(reservedTurnID string) {
			s.writeBinding(ctx, turnID, "reserved", s.store.bindReservedTurn, reservedTurnID)
		},
		OnAccepted: func(sessionTurnID string) {
			s.writeBinding(ctx, turnID, "accepted", s.store.bindAcceptedTurn, sessionTurnID)
		},
	})
	if errors.Is(err, orchestrator.ErrWakePromptNotDispatched) {
		s.logger.Info("coordinator delivery: wake not dispatched",
			zap.String("coordinator_id", coord.ID), zap.String("turn_id", turnID), zap.Error(err))
		s.settleNotSent(ctx, turnID, outcomeSendFailed)
		return nil
	}
	if err != nil {
		if messageID != "" {
			s.writeBinding(ctx, turnID, "message", s.store.setTurnMessage, messageID)
		}
		s.logger.Warn("coordinator delivery: send outcome unknown, left to the backstop",
			zap.String("coordinator_id", coord.ID), zap.String("turn_id", turnID), zap.Error(err))
		return nil
	}
	s.writeBinding(ctx, turnID, "message", s.store.setTurnMessage, messageID)
	return nil
}

// writeBinding runs one conditional turn-row write on a detached, bounded
// context; a failure or no-op only warns.
func (s *Service) writeBinding(ctx context.Context, turnID, what string, write func(context.Context, string, string) (bool, error), value string) {
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), bindTimeout)
	defer cancel()
	changed, err := write(wctx, turnID, value)
	if err != nil || !changed {
		s.logger.Warn("coordinator delivery: turn binding not written",
			zap.String("turn_id", turnID), zap.String("binding", what), zap.Bool("changed", changed), zap.Error(err))
	}
}

// wakeLines resolves each wake's task for the transcript; an unreadable task
// is listed by its id with an empty title.
func (s *Service) wakeLines(ctx context.Context, wakes []deliveredWake) []wakeLine {
	lines := make([]wakeLine, len(wakes))
	for i, w := range wakes {
		lines[i] = wakeLine{Kind: w.Kind, Ref: w.TaskID}
		if s.conversationTasks == nil {
			continue
		}
		task, err := s.conversationTasks.GetTask(ctx, w.TaskID)
		if err != nil || task == nil {
			continue
		}
		lines[i].Title = task.Title
		if task.Identifier != "" {
			lines[i].Ref = task.Identifier
		}
	}
	return lines
}
