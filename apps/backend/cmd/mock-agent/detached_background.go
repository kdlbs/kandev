package main

import (
	"context"

	acp "github.com/coder/acp-go-sdk"
)

type mockDetachedWork struct {
	cancel context.CancelFunc
}

func (a *mockAgent) registerDetachedWork(sessionID acp.SessionId, cancel context.CancelFunc) func() {
	work := &mockDetachedWork{cancel: cancel}
	a.mu.Lock()
	if a.detachedWork == nil {
		a.detachedWork = make(map[acp.SessionId]map[*mockDetachedWork]struct{})
	}
	if a.detachedWork[sessionID] == nil {
		a.detachedWork[sessionID] = make(map[*mockDetachedWork]struct{})
	}
	a.detachedWork[sessionID][work] = struct{}{}
	a.mu.Unlock()
	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		registered := a.detachedWork[sessionID]
		delete(registered, work)
		if len(registered) == 0 {
			delete(a.detachedWork, sessionID)
		}
	}
}

func (a *mockAgent) cancelDetachedWork(sessionID acp.SessionId) {
	a.mu.Lock()
	registered := a.detachedWork[sessionID]
	delete(a.detachedWork, sessionID)
	a.mu.Unlock()
	for work := range registered {
		work.cancel()
	}
}
