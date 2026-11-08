package lifecycle

import (
	"context"

	"github.com/kandev/kandev/internal/common/launchwrapper"
)

func recoveredLaunchWrapper(ctx context.Context, execution *AgentExecution, current launchwrapper.Spec) (launchwrapper.Spec, error) {
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if client == nil {
		if !current.IsZero() {
			return launchwrapper.Spec{}, launchwrapper.ErrContext
		}
		return launchwrapper.Spec{}, nil
	}
	retained, supported, err := client.GetLaunchWrapper(ctx)
	if err != nil {
		return launchwrapper.Spec{}, err
	}
	if supported {
		return retained, nil
	}
	if !current.IsZero() {
		return launchwrapper.Spec{}, launchwrapper.ErrContext
	}
	return launchwrapper.Spec{}, nil
}
