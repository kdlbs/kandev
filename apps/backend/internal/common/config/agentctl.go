package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// InternalAgentctlStartupConfigEnv carries the resolved agentctl-owned startup
// settings from the backend to a managed agentctl process. It is a private
// child-process contract, not an operator-facing configuration variable.
const InternalAgentctlStartupConfigEnv = "KANDEV_INTERNAL_AGENTCTL_STARTUP_CONFIG"

// AgentctlStartupConfig is the resolved configuration contract for a managed
// agentctl process. Configured distinguishes the backend's resolved contract
// from zero-valued structs used by embedded callers and tests.
type AgentctlStartupConfig struct {
	Configured                bool          `json:"configured"`
	IdleTimeout               time.Duration `json:"idleTimeout"`
	IdleReaperInterval        time.Duration `json:"idleReaperInterval"`
	NotificationQueueCapacity int           `json:"notificationQueueCapacity"`
	OTLPEndpoint              string        `json:"otlpEndpoint"`
	// UnownedPeriod and DetachedEventLimit are agentctl-owned agent-survival
	// tunables. Zero means "not resolved by an agent-survival-aware backend";
	// agentctl falls back to its own built-in defaults in that case (see
	// Validate below), the same compatibility posture already used for a
	// backend older than this contract.
	UnownedPeriod      time.Duration `json:"unownedPeriod"`
	DetachedEventLimit int           `json:"detachedEventLimit"`
	// AgentSurvivalEnabled carries the current value of the
	// features.agentSurvival runtime flag for this launch. Unlike
	// UnownedPeriod/DetachedEventLimit, false is not "unresolved" -- a
	// managed launch always sets Configured=true, so false is a meaningful
	// "disabled for this launch" answer agentctl must not second-guess with
	// its own default.
	AgentSurvivalEnabled bool `json:"agentSurvivalEnabled"`
}

// ManagedAgentctlStartupConfig returns the agentctl settings resolved by the
// common startup configuration loader.
func (c *Config) ManagedAgentctlStartupConfig() AgentctlStartupConfig {
	if c == nil {
		return AgentctlStartupConfig{}
	}
	return AgentctlStartupConfig{
		Configured:                true,
		IdleTimeout:               c.Agentctl.IdleTimeout,
		IdleReaperInterval:        c.Agentctl.IdleReaperInterval,
		NotificationQueueCapacity: c.Agentctl.NotificationQueueCapacity,
		OTLPEndpoint:              c.Observability.OTLPEndpoint,
		UnownedPeriod:             c.Agentctl.UnownedPeriod,
		DetachedEventLimit:        c.Agentctl.DetachedEventLimit,
		AgentSurvivalEnabled:      c.Features.AgentSurvival,
	}
}

// Validate checks the child contract before it crosses a process boundary.
func (c AgentctlStartupConfig) Validate() error {
	if !c.Configured {
		return fmt.Errorf("agentctl startup configuration is not resolved")
	}
	if c.IdleTimeout < 0 {
		return fmt.Errorf("agentctl idle timeout must be zero or greater")
	}
	if c.IdleReaperInterval <= 0 {
		return fmt.Errorf("agentctl idle reaper interval must be positive")
	}
	if c.NotificationQueueCapacity < 1024 || c.NotificationQueueCapacity > 131072 {
		return fmt.Errorf("agentctl notification queue capacity must be between 1024 and 131072")
	}
	// Zero means "not resolved by an agent-survival-aware caller"; agentctl
	// substitutes its own built-in default in that case. Any other value must
	// already be within the contract's accepted range.
	if c.UnownedPeriod < 0 {
		return fmt.Errorf("agentctl unowned period must be zero (unset) or positive")
	}
	if c.DetachedEventLimit != 0 && (c.DetachedEventLimit < 1 || c.DetachedEventLimit > 10000) {
		return fmt.Errorf("agentctl detached event limit must be zero (unset) or between 1 and 10000")
	}
	return nil
}

// EncodeAgentctlStartupConfig serializes a validated child contract for the
// private environment handoff used by local, container, and remote launches.
func EncodeAgentctlStartupConfig(c AgentctlStartupConfig) (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("encode agentctl startup configuration: %w", err)
	}
	return string(data), nil
}

// DecodeAgentctlStartupConfig parses and validates a private child contract.
func DecodeAgentctlStartupConfig(raw string) (AgentctlStartupConfig, error) {
	if strings.TrimSpace(raw) == "" {
		return AgentctlStartupConfig{}, fmt.Errorf("agentctl startup configuration is empty")
	}
	var c AgentctlStartupConfig
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return AgentctlStartupConfig{}, fmt.Errorf("decode agentctl startup configuration: %w", err)
	}
	if err := c.Validate(); err != nil {
		return AgentctlStartupConfig{}, err
	}
	return c, nil
}
