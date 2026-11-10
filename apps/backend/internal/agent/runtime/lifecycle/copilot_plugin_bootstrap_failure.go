package lifecycle

import (
	"errors"

	"github.com/kandev/kandev/internal/agent/copilotplugins"
)

type copilotPluginPreparationFailure struct {
	Cause          error
	RuntimeVersion string
}

func (e *copilotPluginPreparationFailure) Error() string {
	if e == nil || e.Cause == nil {
		return "copilot plugin preparation failed"
	}
	return e.Cause.Error()
}

func (e *copilotPluginPreparationFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func preserveCopilotPluginRuntimeVersion(err error, version string) error {
	if err == nil {
		return nil
	}
	var pluginErr *copilotplugins.Error
	if !errors.As(err, &pluginErr) || pluginErr == nil {
		return err
	}
	return &copilotPluginPreparationFailure{Cause: err, RuntimeVersion: version}
}
