package main

import (
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/logger"
)

// F71 bound values for agentctl's own detached diagnostic log
// (AC-EXECUTORS-SURVIVAL-001.3): fixed implementation constants, not
// operator settings -- the sink adds no configuration key and no
// environment variable of its own. Chosen to mirror the backend's own
// logger's order of magnitude while staying a visibly distinct budget.
const (
	diagnosticLogMaxSizeMB  = 16
	diagnosticLogMaxBackups = 10
	diagnosticLogMaxAgeDays = 14
)

// diagnosticLoggingConfig builds the file-backed, bounded logging
// configuration for agentctl's own diagnostic output while it may be
// running detached from any backend. Not yet wired into runMain's logger
// construction: switching away from stdout is a decision that depends on
// whether the agent-survival capability is engaged for this launch, which
// Layer 5.9 threads into agentctl's config (kill-path #6, "inherited
// stdout", is only a problem once that capability is active). This function
// exists so 5.9 has a ready, already-tested LoggingConfig to switch to
// instead of hand-assembling one at the call site.
func diagnosticLoggingConfig(level, format, diagnosticLogPath string) logger.LoggingConfig {
	return logger.LoggingConfig{
		Level:      level,
		Format:     format,
		OutputPath: diagnosticLogPath,
		MaxSizeMB:  diagnosticLogMaxSizeMB,
		MaxBackups: diagnosticLogMaxBackups,
		MaxAgeDays: diagnosticLogMaxAgeDays,
	}
}

// resolveRunLoggingConfig is Layer 5.9's kill-path #6 gate ("inherited
// stdout", design 01's kill paths): stdout is only a liveness hazard once
// the agent-survival capability is actually engaged for this launch, so
// every other case -- capability disabled, or a diagnostic log path that
// failed to resolve for some reason -- keeps today's stdout logging
// unchanged rather than risk an unwritable sink.
func resolveRunLoggingConfig(cfg *config.Config) logger.LoggingConfig {
	if cfg.AgentSurvivalEnabled && cfg.DiagnosticLogPath != "" {
		return diagnosticLoggingConfig(cfg.LogLevel, cfg.LogFormat, cfg.DiagnosticLogPath)
	}
	return logger.LoggingConfig{
		Level:      cfg.LogLevel,
		Format:     cfg.LogFormat,
		OutputPath: "stdout",
	}
}
