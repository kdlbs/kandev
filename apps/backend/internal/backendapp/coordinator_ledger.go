package backendapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/coordinator/ledger"
	"github.com/kandev/kandev/internal/mcp/handlers"
)

// wireCoordinatorLedger starts the turn ledger when coordinator phase 2 is on:
// recording is independent of features.coordinatorPhase31, which gates only the
// read tool. A missing dependency leaves recording off rather than half-built.
func wireCoordinatorLedger(p routeParams, svc *coordinator.Service, mcp *handlers.Handlers) {
	if !svc.Phase2Enabled() || p.dbPool == nil || p.eventBus == nil || p.taskSvc == nil {
		return
	}
	instructions := coordinatorStandingInstructionsReader(svc, p.log)
	reader := &coordinatorConversationReader{tasks: p.taskSvc}
	l := ledger.New(ledger.Deps{
		DB: p.dbPool.Writer(), RO: p.dbPool.Reader(), Log: p.log.Zap(),
		BuildVersion:       resolveVersion(p),
		CoordinatorForTask: svc.CoordinatorForConversationTask,
		WatchSet:           svc.EffectiveWatchSet,
		ActionPending:      reader.ActionPending,
		PromptHash: func(ctx context.Context, coordinatorID string) string {
			workspaceID, err := svc.WorkspaceIDOf(ctx, coordinatorID)
			if err != nil {
				return ""
			}
			name := ""
			if ws, wsErr := p.taskSvc.GetWorkspace(ctx, workspaceID); wsErr == nil && ws != nil {
				name = ws.Name
			}
			content, err := instructions(ctx, coordinatorID, name, workspaceID)
			if err != nil || content == "" {
				return ""
			}
			sum := sha256.Sum256([]byte(content))
			return hex.EncodeToString(sum[:])
		},
	})
	l.Start(p.ctx)
	unsubscribe, err := l.Subscribe(p.eventBus)
	if err != nil {
		p.log.Error("coordinator turn ledger not recording: subscribe failed", zap.Error(err))
		l.Stop()
		return
	}
	svc.SetTurnLedger(l)
	if p.addCleanup != nil {
		p.addCleanup(func() error {
			unsubscribe()
			l.Stop()
			svc.SetTurnLedger(nil)
			return nil
		})
	}
	if svc.Phase31Enabled() {
		mcp.SetCoordinatorTurnReader(l)
	}
}
