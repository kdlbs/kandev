package config

import (
	"fmt"
	"net"
	"strings"
)

func validateStartupSettings(cfg *Config) error {
	errs := validateTrustedProxies(cfg)
	errs = append(errs, configuredValidation(cfg, "tasks.preparationTimeout", cfg.Tasks.PreparationTimeout <= 0, "tasks.preparationTimeout must be positive")...)
	errs = append(errs, configuredValidation(cfg, "limits.ghMaxConcurrent", cfg.Limits.GHMaxConcurrent <= 0, "limits.ghMaxConcurrent must be positive")...)
	errs = append(errs, configuredValidation(cfg, "limits.gitMaxConcurrent", cfg.Limits.GitMaxConcurrent <= 0, "limits.gitMaxConcurrent must be positive")...)
	errs = append(errs, configuredValidation(cfg, taskLSPMaxServersConfigKey, cfg.Limits.LSPMaxServers <= 0, "limits.lspMaxServers must be positive")...)
	errs = append(errs, configuredValidation(cfg, "messageQueue.maxPerSession", cfg.MessageQueue.MaxPerSession < 0, "messageQueue.maxPerSession must be zero or greater")...)
	errs = append(errs, configuredValidation(cfg, "agentctl.idleTimeout", cfg.Agentctl.IdleTimeout < 0, "agentctl.idleTimeout must be zero or greater")...)
	errs = append(errs, configuredValidation(cfg, "agentctl.idleReaperInterval", cfg.Agentctl.IdleReaperInterval <= 0, "agentctl.idleReaperInterval must be positive")...)
	errs = append(errs, configuredValidation(cfg, "agentctl.notificationQueueCapacity", cfg.Agentctl.NotificationQueueCapacity < 1024 || cfg.Agentctl.NotificationQueueCapacity > 131072, "agentctl.notificationQueueCapacity must be between 1024 and 131072")...)
	errs = append(errs, configuredValidation(cfg, "planning.coalesceWindowMs", cfg.Planning.CoalesceWindowMs < 0, "planning.coalesceWindowMs must be zero or greater")...)
	errs = append(errs, configuredValidation(cfg, "office.schedulerTickMs", cfg.Office.SchedulerTickMs <= 0, "office.schedulerTickMs must be positive")...)
	errs = append(errs, configuredValidation(cfg, "launcher.webPort", cfg.Launcher.WebPort < 1 || cfg.Launcher.WebPort > 65535, "launcher.webPort must be between 1 and 65535")...)
	errs = append(errs, configuredValidation(cfg, "launcher.healthTimeoutMs", cfg.Launcher.HealthTimeoutMs <= 0, "launcher.healthTimeoutMs must be positive")...)
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
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
