package backendapp

import (
	"context"

	taskquestions "github.com/kandev/kandev/internal/task/questions"
)

func configureAsyncQuestions(p routeParams) {
	if p.services == nil {
		return
	}
	p.services.AsyncQuestions = nil
	if !p.features.AsyncQuestions {
		return
	}

	asyncQuestionSvc := taskquestions.NewService(p.taskRepo, nil, p.log.Zap())
	asyncQuestionSvc.SetDeliveryQueue(&asyncQuestionQueueAdapter{
		queue: p.orchestratorSvc.GetMessageQueue(), sessions: p.taskRepo,
		readiness: p.orchestratorSvc, log: p.log.Zap(),
	})
	asyncQuestionSvc.SetTranscriptWriter(&asyncQuestionTranscriptWriter{reader: p.taskRepo, creator: p.taskSvc})
	asyncQuestionSvc.SetNotifier(asyncQuestionNotifier{bus: p.eventBus, pendingActions: p.taskSvc, log: p.log.Zap()})
	asyncQuestionSvc.SetSessionStateReader(asyncQuestionSessionState{sessions: p.taskRepo})
	asyncQuestionSvc.SetWakeDestination(func(ctx context.Context, taskID, sessionID string) {
		p.orchestratorSvc.NotifyQueuedUserPrompt(ctx, taskID, sessionID)
	})
	p.orchestratorSvc.SetAsyncQuestionGate(asyncQuestionSvc)
	p.orchestratorSvc.SetAsyncQuestionLifecycle(asyncQuestionSvc)
	p.taskSvc.SetAsyncQuestionLifecycle(asyncQuestionSvc)
	p.services.AsyncQuestions = asyncQuestionSvc
	if p.addCleanup != nil {
		p.addCleanup(func() error {
			asyncQuestionSvc.Stop()
			return nil
		})
	}
}

type asyncQuestionWorker interface {
	Start(context.Context)
}

func configuredAsyncQuestionWorker(service *taskquestions.Service) asyncQuestionWorker {
	if service == nil {
		return nil
	}
	return service
}

func prepareStartupAndStartAsyncQuestions(
	ctx context.Context,
	prepare func(context.Context) error,
	worker asyncQuestionWorker,
) error {
	if err := prepare(ctx); err != nil {
		return err
	}
	if worker != nil {
		worker.Start(ctx)
	}
	return nil
}
