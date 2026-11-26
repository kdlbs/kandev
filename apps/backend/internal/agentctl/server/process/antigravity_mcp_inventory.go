package process

import (
	"os"
	"strings"

	"github.com/kandev/kandev/internal/agent/antigravityruntime"
	"github.com/kandev/kandev/internal/agentctl/server/config"
)

func scanAntigravityNativeMCPNames(cfg *config.InstanceConfig) ([]string, bool) {
	if cfg == nil {
		return nil, false
	}
	home := ""
	for _, entry := range cfg.AgentEnv {
		key, value, found := strings.Cut(entry, "=")
		if found && key == "HOME" {
			home = value
		}
	}
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return antigravityruntime.ScanNativeMCPServerNames(cfg.WorkDir, home)
}
