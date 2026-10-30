package utility

import (
	"errors"
	"fmt"
	"slices"

	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/common/launchwrapper"
	"github.com/kandev/kandev/pkg/agent"
)

// wrapped reports whether a profile or node wrapper shapes this launch, so
// responses and logs stay redacted.
func (c *InferenceConfigDTO) wrapped() bool {
	return !c.LaunchWrapper.IsZero() || c.node.Active()
}

// observationPrefix is the wrapper argv that makes runtime observation skip
// executables it would otherwise run directly. It is empty without a wrapper.
func observationPrefix(cfg *InferenceConfigDTO) []string {
	if !cfg.wrapped() {
		return cfg.CommandPrefix
	}
	prefix := slices.Clone(cfg.node.Spec.Argv)
	prefix = append(prefix, cfg.LaunchWrapper.Argv...)
	if len(prefix) == 0 {
		prefix = []string{"[node wrapper]"}
	}
	return prefix
}

func prepareACPLaunch(cfg *InferenceConfigDTO, admitted string, args, env []string) ([]string, error) {
	if cfg.wrapped() && (len(cfg.CommandPrefix) != 0 || (cfg.Protocol != "" && cfg.Protocol != agent.ProtocolACP)) {
		return nil, launchwrapper.ErrInvalid
	}
	inner := append(slices.Clone(args), cfg.CLIFlags...)
	inner[0] = admitted
	if cfg.wrapped() {
		return cfg.node.Prepare(cfg.LaunchWrapper, inner, launchwrapper.Context{WorkDir: cfg.WorkDir, Env: env})
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
	if cfg.wrapped() {
		return "wrapped utility process failed to start"
	}
	return fmt.Sprintf("start: %v", err)
}
