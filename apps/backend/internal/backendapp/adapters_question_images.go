package backendapp

import (
	"context"

	"github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/clarification/protocol"
	"github.com/kandev/kandev/internal/common/imagepreview"
)

type questionImageExecutionLookup interface {
	GetOrEnsureExecution(context.Context, string) (*runtime.AgentExecution, error)
}

type questionImageReaderAdapter struct{ lifecycle questionImageExecutionLookup }

func (a *questionImageReaderAdapter) ReadQuestionImage(ctx context.Context, sessionID string, ref protocol.QuestionImage) (*imagepreview.Content, error) {
	execution, err := a.lifecycle.GetOrEnsureExecution(ctx, sessionID)
	if err != nil || execution == nil {
		return nil, imagepreview.Unavailable
	}
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if client == nil {
		return nil, imagepreview.Unavailable
	}
	return client.RequestQuestionImage(ctx, ref.Path, ref.Repo)
}
