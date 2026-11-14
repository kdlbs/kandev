package utility

import (
	"errors"
	"fmt"
	"slices"

	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/common/launchwrapper"
	"github.com/kandev/kandev/pkg/agent"
)

func prepareACPLaunch(cfg *InferenceConfigDTO, admitted string, args, env []string) ([]string, error) {
	if !cfg.LaunchWrapper.IsZero() && (len(cfg.CommandPrefix) != 0 || (cfg.Protocol != "" && cfg.Protocol != agent.ProtocolACP)) {
		return nil, launchwrapper.ErrInvalid
	}
	inner := append(slices.Clone(args), cfg.CLIFlags...)
	inner[0] = admitted
	if !cfg.LaunchWrapper.IsZero() {
		return launchwrapper.Prepare(cfg.LaunchWrapper, inner, launchwrapper.Context{WorkDir: cfg.WorkDir, Env: env})
	}
	if err := managedruntime.PrepareNPMProjectPrefix(inner); err != nil {
		return nil, err
	}
	if len(cfg.CommandPrefix) == 0 {
		return inner, nil
	}
	prefix := slices.Clone(cfg.CommandPrefix)
	prefix[0] = resolveACPCommandPrefix(prefix[0])
	if prefix[0] == "" {
		return nil, errors.New("profile command prefix is unsupported")
	}
	return append(prefix, inner...), nil
}

func utilityStartError(cfg *InferenceConfigDTO, err error) string {
	if !cfg.LaunchWrapper.IsZero() {
		return "wrapped utility process failed to start"
	}
	return fmt.Sprintf("start: %v", err)
}
