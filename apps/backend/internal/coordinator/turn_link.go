package coordinator

import "context"

// TurnLedger is the seam to the turn ledger: the open ledger turn of a session
// and the digest hook of the guarded-call layer. Neither method can fail.
type TurnLedger interface {
	ActiveTurnID(sessionID string) string
	Call(sessionID, action, targetTaskID string, allowed bool)
}

type callerSessionKey struct{}

// WithCallerSession carries the calling coordinator session through a guarded
// call, so the proposal and log writes it makes can name the ledger turn.
func WithCallerSession(ctx context.Context, sessionID string) context.Context {
	if sessionID == "" {
		return ctx
	}
	return context.WithValue(ctx, callerSessionKey{}, sessionID)
}

// SetTurnLedger installs the ledger; nil clears it.
func (s *Service) SetTurnLedger(l TurnLedger) { s.store.setTurnLedger(l) }

// RecordCall forwards one guarded-call decision to the ledger, if any.
func (s *Service) RecordCall(sessionID, action, targetTaskID string, allowed bool) {
	if l := s.store.turnLedger(); l != nil {
		l.Call(sessionID, action, targetTaskID, allowed)
	}
}

func (s *Store) setTurnLedger(l TurnLedger) {
	s.ledgerMu.Lock()
	s.ledger = l
	s.ledgerMu.Unlock()
}

func (s *Store) turnLedger() TurnLedger {
	s.ledgerMu.RLock()
	defer s.ledgerMu.RUnlock()
	return s.ledger
}

// ledgerTurnID is the open ledger turn of the guarded call carried by ctx, or
// nil: no ledger, no caller session, or no open turn all leave the link empty.
func (s *Store) ledgerTurnID(ctx context.Context) *string {
	sessionID, _ := ctx.Value(callerSessionKey{}).(string)
	l := s.turnLedger()
	if sessionID == "" || l == nil {
		return nil
	}
	if id := l.ActiveTurnID(sessionID); id != "" {
		return &id
	}
	return nil
}
