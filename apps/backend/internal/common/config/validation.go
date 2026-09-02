package config

import (
	"fmt"
	"net"
	"strings"
	"time"
)

func validateStartupSettings(cfg *Config) error {
	errs := validateTrustedProxies(cfg)
	errs = append(errs, configuredValidation(cfg, "tasks.preparationTimeout", cfg.Tasks.PreparationTimeout <= 0, "tasks.preparationTimeout must be positive")...)
	errs = append(errs, configuredValidation(cfg, "limits.ghMaxConcurrent", cfg.Limits.GHMaxConcurrent <= 0, "limits.ghMaxConcurrent must be positive")...)
	errs = append(errs, configuredValidation(cfg, "limits.gitMaxConcurrent", cfg.Limits.GitMaxConcurrent <= 0, "limits.gitMaxConcurrent must be positive")...)
	errs = append(errs, configuredValidation(cfg, "limits.lspMaxConnections", cfg.Limits.LSPMaxConnections <= 0, "limits.lspMaxConnections must be positive")...)
	errs = append(errs, configuredValidation(cfg, "messageQueue.maxPerSession", cfg.MessageQueue.MaxPerSession < 0, "messageQueue.maxPerSession must be zero or greater")...)
	errs = append(errs, configuredValidation(cfg, "agentctl.idleTimeout", cfg.Agentctl.IdleTimeout < 0, "agentctl.idleTimeout must be zero or greater")...)
	errs = append(errs, configuredValidation(cfg, "agentctl.idleReaperInterval", cfg.Agentctl.IdleReaperInterval <= 0, "agentctl.idleReaperInterval must be positive")...)
	errs = append(errs, configuredValidation(cfg, "agentctl.notificationQueueCapacity", cfg.Agentctl.NotificationQueueCapacity < 1024 || cfg.Agentctl.NotificationQueueCapacity > 131072, "agentctl.notificationQueueCapacity must be between 1024 and 131072")...)
	errs = append(errs, configuredValidation(cfg, "agentctl.recoveryDeadline", cfg.Agentctl.RecoveryDeadline < time.Second || cfg.Agentctl.RecoveryDeadline > 5*time.Minute, "agentctl.recoveryDeadline must be between 1s and 5m")...)
	errs = append(errs, configuredValidation(cfg, "agentctl.recoveryReadTimeout", cfg.Agentctl.RecoveryReadTimeout < 100*time.Millisecond, "agentctl.recoveryReadTimeout must be at least 100ms")...)
	errs = append(errs, configuredValidation(cfg, "agentctl.recoveryReadRetries", cfg.Agentctl.RecoveryReadRetries < 0 || cfg.Agentctl.RecoveryReadRetries > 10, "agentctl.recoveryReadRetries must be between 0 and 10")...)
	errs = append(errs, configuredValidation(cfg, "agentctl.detachedEventLimit", cfg.Agentctl.DetachedEventLimit < 1 || cfg.Agentctl.DetachedEventLimit > 10000, "agentctl.detachedEventLimit must be between 1 and 10000")...)
	errs = append(errs, validateRecoveryReadBudget(cfg)...)
	errs = append(errs, configuredValidation(cfg, "planning.coalesceWindowMs", cfg.Planning.CoalesceWindowMs < 0, "planning.coalesceWindowMs must be zero or greater")...)
	errs = append(errs, configuredValidation(cfg, "office.schedulerTickMs", cfg.Office.SchedulerTickMs <= 0, "office.schedulerTickMs must be positive")...)
	errs = append(errs, configuredValidation(cfg, "launcher.webPort", cfg.Launcher.WebPort < 1 || cfg.Launcher.WebPort > 65535, "launcher.webPort must be between 1 and 65535")...)
	errs = append(errs, configuredValidation(cfg, "launcher.healthTimeoutMs", cfg.Launcher.HealthTimeoutMs <= 0, "launcher.healthTimeoutMs must be positive")...)
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// validateRecoveryReadBudget rejects a configured combination of
// agentctl.recoveryReadTimeout and agentctl.recoveryReadRetries whose total
// attempt duration exceeds the lesser of 6 seconds or one fifth of the
// actually-configured agentctl.recoveryDeadline. Unlike configuredValidation,
// this check is unconditional: the three values it compares can each come
// from a different source (YAML, environment, or built-in default), and the
// contract requires validating the combination regardless of where any one
// of them came from.
func validateRecoveryReadBudget(cfg *Config) []string {
	attempts := time.Duration(cfg.Agentctl.RecoveryReadRetries+1) * cfg.Agentctl.RecoveryReadTimeout
	ceiling := cfg.Agentctl.RecoveryDeadline / 5
	if 6*time.Second < ceiling {
		ceiling = 6 * time.Second
	}
	if attempts > ceiling {
		return []string{fmt.Sprintf(
			"agentctl.recoveryReadTimeout * (agentctl.recoveryReadRetries + 1) = %s exceeds the allowed ceiling of %s (the lesser of 6s and one fifth of agentctl.recoveryDeadline = %s)",
			attempts, ceiling, cfg.Agentctl.RecoveryDeadline,
		)}
	}
	return nil
}

func configuredValidation(cfg *Config, key string, invalid bool, message string) []string {
	if cfg.SourceFor(key) == SourceConfiguration && invalid {
		return []string{message}
	}
	return nil
}

func validateTrustedProxies(cfg *Config) []string {
	if cfg.SourceFor("server.trustedProxies") != SourceConfiguration {
		return nil
	}
	var errs []string
	for _, raw := range cfg.Server.TrustedProxies {
		value := strings.TrimSpace(raw)
		if net.ParseIP(value) != nil {
			continue
		}
		if _, _, err := net.ParseCIDR(value); err != nil {
			errs = append(errs, fmt.Sprintf("server.trustedProxies contains invalid IP or CIDR %q", raw))
		}
	}
	return errs
}
