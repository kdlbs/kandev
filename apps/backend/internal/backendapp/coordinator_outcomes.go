package backendapp

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/coordinator/outcomes/recorder"
	userstore "github.com/kandev/kandev/internal/user/store"
)

// coordinatorActorChecker classifies the actor of an override: while
// authentication is disabled the default user is the manager, as the automatic
// class treats an empty raiser; any other id is a principal. While it is
// enabled the actor goes through the same manager check as an automatic raiser.
type coordinatorActorChecker struct {
	svc  *coordinator.Service
	auth *auth.Service
}

func (a coordinatorActorChecker) Check(ctx context.Context, workspaceID, actorID string) (recorder.Verdict, error) {
	if a.auth == nil || a.auth.Mode() == auth.ModeDisabled {
		if actorID == "" || actorID == userstore.DefaultUserID {
			return recorder.VerdictManager, nil
		}
		return recorder.VerdictPrincipal, nil
	}
	if actorID == "" {
		return recorder.VerdictSystem, nil
	}
	ok, err := a.svc.IsWorkspaceManager(ctx, workspaceID, actorID)
	if err != nil {
		return recorder.VerdictNotManager, err
	}
	if !ok {
		return recorder.VerdictNotManager, nil
	}
	return recorder.VerdictManager, nil
}

// wireCoordinatorOutcomes starts outcome grading and override capture when
// coordinator phase 2 is on, and serves the measures when phase 3.1 is also
// on. A missing dependency leaves recording off rather than half-built.
func wireCoordinatorOutcomes(p routeParams, svc *coordinator.Service) {
	if !svc.Phase2Enabled() || p.dbPool == nil || p.eventBus == nil {
		return
	}
	writer, reader, log := p.dbPool.Writer(), p.dbPool.Reader(), p.log.Zap()
	queue := recorder.NewQueue(recorder.NewGrader(writer, log), log)
	capture := recorder.NewCapture(writer, coordinatorActorChecker{svc: svc, auth: p.authSvc}, log, nil)
	scanner := recorder.NewScanner(capture, reader, log)
	unsubGrade, err := queue.Subscribe(p.eventBus, recorder.SQLProposals{DB: reader})
	if err != nil {
		p.log.Error("coordinator outcomes not recording: subscribe failed", zap.Error(err))
		return
	}
	unsubScan, err := scanner.Subscribe(p.eventBus)
	if err != nil {
		unsubGrade()
		p.log.Error("coordinator overrides not recording: subscribe failed", zap.Error(err))
		return
	}
	queue.Start(p.ctx)
	queue.RunSweeps(p.ctx, recorder.SQLProposals{DB: reader})
	scanner.Start(p.ctx)
	svc.SetDecisionObserver(recorder.NewObservers(log, queue, capture))
	if svc.Phase31Enabled() {
		svc.SetOutcomeMeasures(recorder.NewMeasures(reader, dreamAgreement{store: svc.Store()}))
	}
	if p.addCleanup != nil {
		p.addCleanup(func() error {
			unsubGrade()
			unsubScan()
			svc.SetDecisionObserver(nil)
			scanner.Stop()
			queue.Stop()
			return nil
		})
	}
}
