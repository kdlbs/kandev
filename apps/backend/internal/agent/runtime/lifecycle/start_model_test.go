package lifecycle

import (
	"context"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
)

type fakeModelApplier struct {
	calls []string
	errs  []error
}

func (f *fakeModelApplier) SetModel(_ context.Context, modelID string) error {
	f.calls = append(f.calls, modelID)
	if len(f.errs) == 0 {
		return nil
	}
	err := f.errs[0]
	if len(f.errs) > 1 {
		f.errs = f.errs[1:]
	}
	return err
}

func newPolicyTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	return log
}

func modelState(models ...string) *CachedModelState {
	infos := make([]streams.SessionModelInfo, 0, len(models))
	for _, id := range models {
		infos = append(infos, streams.SessionModelInfo{ModelID: id})
	}
	return &CachedModelState{Models: infos}
}

func methodNotFoundErr() error {
	return &acp.RequestError{Code: -32601, Message: "method not found"}
}
