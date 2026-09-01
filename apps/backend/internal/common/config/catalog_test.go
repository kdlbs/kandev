package config

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestConfigurationCatalogIsComplete(t *testing.T) {
	if err := ValidateCatalog(); err != nil {
		t.Fatalf("ValidateCatalog: %v", err)
	}
	entries := ConfigurationCatalog()
	if len(entries) < 50 {
		t.Fatalf("catalog has %d entries, want the stable startup inventory", len(entries))
	}
	for _, key := range []string{
		"server.trustedProxies",
		"tasks.preparationTimeout",
		"credentials.file",
		"limits.ghMaxConcurrent",
		"messageQueue.maxPerSession",
		"agentctl.notificationQueueCapacity",
		"launcher.noBrowser",
	} {
		if _, ok := CatalogEntryForKey(key); !ok {
			t.Errorf("catalog is missing %q", key)
		}
	}
}

func TestConfigurationCatalogMatchesAuditedEnvironmentInventory(t *testing.T) {
	if err := validateCatalogAgainstAuditedInventory(ConfigurationCatalog(), ConfigurationExclusions(), auditedStartupEnvironmentInventory()); err != nil {
		t.Fatal(err)
	}
}

func TestConfigurationCatalogAuditRejectsUncatalogedEnvironmentVariable(t *testing.T) {
	err := validateCatalogAgainstAuditedInventory(
		[]CatalogEntry{{Key: "test.setting", EnvVars: []string{"KANDEV_UNCATALOGED_TEST"}}},
		nil,
		nil,
	)
	if err == nil {
		t.Fatal("catalog audit accepted an environment variable absent from the audited inventory")
	}
	if !strings.Contains(err.Error(), "KANDEV_UNCATALOGED_TEST") {
		t.Fatalf("catalog audit error = %v, want the uncataloged variable", err)
	}
}

type auditedStartupEnvironment struct {
	envVar string
	class  string
}

// auditedStartupEnvironmentInventory is intentionally independent of
// startupCatalog. Update it only after auditing every stable environment
// consumer and every reviewed exclusion.
func auditedStartupEnvironmentInventory() []auditedStartupEnvironment {
	return []auditedStartupEnvironment{
		{envVar: "KANDEV_HOME_DIR", class: "catalog"},
		{envVar: "KANDEV_SERVER_HOST", class: "catalog"},
		{envVar: "KANDEV_SERVER_PORT", class: "catalog"},
		{envVar: "KANDEV_BACKEND_PORT", class: "catalog"},
		{envVar: "KANDEV_PORT", class: "catalog"},
		{envVar: "KANDEV_SERVER_READTIMEOUT", class: "catalog"},
		{envVar: "KANDEV_SERVER_WRITETIMEOUT", class: "catalog"},
		{envVar: "KANDEV_WEB_INTERNAL_URL", class: "catalog"},
		{envVar: "KANDEV_WEB_TITLE_PREFIX", class: "catalog"},
		{envVar: "KANDEV_TRUSTED_PROXIES", class: "catalog"},
		{envVar: "KANDEV_DATABASE_DRIVER", class: "catalog"},
		{envVar: "KANDEV_DATABASE_PATH", class: "catalog"},
		{envVar: "KANDEV_DATABASE_HOST", class: "catalog"},
		{envVar: "KANDEV_DATABASE_PORT", class: "catalog"},
		{envVar: "KANDEV_DATABASE_USER", class: "catalog"},
		{envVar: "KANDEV_DATABASE_PASSWORD", class: "catalog"},
		{envVar: "KANDEV_DATABASE_DBNAME", class: "catalog"},
		{envVar: "KANDEV_DATABASE_SSLMODE", class: "catalog"},
		{envVar: "KANDEV_DATABASE_MAXCONNS", class: "catalog"},
		{envVar: "KANDEV_DATABASE_MINCONNS", class: "catalog"},
		{envVar: "KANDEV_NATS_URL", class: "catalog"},
		{envVar: "KANDEV_NATS_CLUSTERID", class: "catalog"},
		{envVar: "KANDEV_NATS_CLIENTID", class: "catalog"},
		{envVar: "KANDEV_NATS_MAXRECONNECTS", class: "catalog"},
		{envVar: "KANDEV_EVENTS_NAMESPACE", class: "catalog"},
		{envVar: "KANDEV_DOCKER_ENABLED", class: "catalog"},
		{envVar: "KANDEV_DOCKER_HOST", class: "catalog"},
		{envVar: "DOCKER_HOST", class: "catalog"},
		{envVar: "KANDEV_DOCKER_APIVERSION", class: "catalog"},
		{envVar: "KANDEV_DOCKER_TLSVERIFY", class: "catalog"},
		{envVar: "KANDEV_DOCKER_DEFAULTNETWORK", class: "catalog"},
		{envVar: "KANDEV_DOCKER_VOLUMEBASEPATH", class: "catalog"},
		{envVar: "KANDEV_AGENT_STANDALONE_HOST", class: "catalog"},
		{envVar: "AGENTCTL_PORT", class: "catalog"},
		{envVar: "KANDEV_AGENT_STANDALONE_PORT", class: "catalog"},
		{envVar: "KANDEV_AUTH_JWTSECRET", class: "catalog"},
		{envVar: "KANDEV_AUTH_TOKENDURATION", class: "catalog"},
		{envVar: "KANDEV_AUTH_SESSIONTTLHOURS", class: "catalog"},
		{envVar: "KANDEV_AUTH_COOKIE_NAME", class: "catalog"},
		{envVar: "KANDEV_LOG_LEVEL", class: "catalog"},
		{envVar: "KANDEV_LOGGING_FORMAT", class: "catalog"},
		{envVar: "KANDEV_REPOSITORYDISCOVERY_ROOTS", class: "catalog"},
		{envVar: "KANDEV_REPOSITORYDISCOVERY_MAXDEPTH", class: "catalog"},
		{envVar: "KANDEV_WORKTREE_ENABLED", class: "catalog"},
		{envVar: "KANDEV_WORKTREE_DEFAULTBRANCH", class: "catalog"},
		{envVar: "KANDEV_WORKTREE_CLEANUPONREMOVE", class: "catalog"},
		{envVar: "KANDEV_WORKTREE_FETCHTIMEOUTSECONDS", class: "catalog"},
		{envVar: "KANDEV_WORKTREE_PULLTIMEOUTSECONDS", class: "catalog"},
		{envVar: "KANDEV_REPOCLONE_BASEPATH", class: "catalog"},
		{envVar: "KANDEV_DEBUG_DEV_MODE", class: "catalog"},
		{envVar: "KANDEV_DEBUG_PPROF_ENABLED", class: "catalog"},
		{envVar: "KANDEV_OFFICE_JWTSIGNINGKEY", class: "catalog"},
		{envVar: "KANDEV_OFFICE_SCHEDULER_TICK_MS", class: "catalog"},
		{envVar: "KANDEV_GITHUB_CREDENTIAL_BROKER_PUBLIC_BASE_URL", class: "catalog"},
		{envVar: "KANDEV_TASK_PREPARATION_TIMEOUT", class: "catalog"},
		{envVar: "KANDEV_CREDENTIALS_FILE", class: "catalog"},
		{envVar: "KANDEV_GH_MAX_CONCURRENT", class: "catalog"},
		{envVar: "KANDEV_GIT_MAX_CONCURRENT", class: "catalog"},
		{envVar: "KANDEV_LSP_MAX_CONNECTIONS", class: "catalog"},
		{envVar: "KANDEV_QUEUE_MAX_PER_SESSION", class: "catalog"},
		{envVar: "KANDEV_ACP_IDLE_TIMEOUT", class: "catalog"},
		{envVar: "KANDEV_ACP_IDLE_REAPER_INTERVAL", class: "catalog"},
		{envVar: "KANDEV_ACP_NOTIF_QUEUE", class: "catalog"},
		{envVar: "KANDEV_PLAN_COALESCE_WINDOW_MS", class: "catalog"},
		{envVar: "OTEL_EXPORTER_OTLP_ENDPOINT", class: "catalog"},
		{envVar: "KANDEV_WEB_PORT", class: "catalog"},
		{envVar: "KANDEV_HEALTH_TIMEOUT_MS", class: "catalog"},
		{envVar: "KANDEV_NO_BROWSER", class: "catalog"},
		{envVar: InternalConfigFileEnv, class: "exclusion"},
		{envVar: InternalConfigHomeFileEnv, class: "exclusion"},
		{envVar: InternalAgentctlStartupConfigEnv, class: "exclusion"},
		{envVar: "KANDEV_LAUNCHER_PARENT_PID", class: "exclusion"},
		{envVar: "KANDEV_BACKEND_PID_FILE", class: "exclusion"},
		{envVar: "KANDEV_DESKTOP_HEALTH_TOKEN", class: "exclusion"},
		{envVar: "KANDEV_DESKTOP_NATIVE_NOTIFICATIONS", class: "exclusion"},
		{envVar: "KANDEV_BUNDLE_DIR", class: "exclusion"},
		{envVar: "KANDEV_WEB_DIST_DIR", class: "exclusion"},
		{envVar: "KANDEV_TASK_ID", class: "exclusion"},
		{envVar: "KANDEV_SESSION_ID", class: "exclusion"},
		{envVar: "KANDEV_WORKSPACE_ID", class: "exclusion"},
		{envVar: "KANDEV_E2E_MOCK", class: "exclusion"},
		{envVar: "KANDEV_MOCK_AGENT", class: "exclusion"},
		{envVar: "KANDEV_MOCK_GITHUB", class: "exclusion"},
		{envVar: "KANDEV_MOCK_GITLAB", class: "exclusion"},
		{envVar: "KANDEV_MOCK_JIRA", class: "exclusion"},
		{envVar: "KANDEV_MOCK_LINEAR", class: "exclusion"},
		{envVar: "KANDEV_FEATURES_OFFICE", class: "exclusion"},
		{envVar: "KANDEV_FEATURES_AUTH", class: "exclusion"},
		{envVar: "KANDEV_FEATURES_CLAUDE_BACKGROUND_PROMPT_HANDOFF", class: "exclusion"},
		{envVar: "KANDEV_FEATURES_CLAUDE_MID_TURN_STEERING", class: "exclusion"},
		{envVar: "KANDEV_FEATURES_OFFICE_SESSION_IDENTITY", class: "exclusion"},
		{envVar: "KANDEV_DEBUG_AGENT_MESSAGES", class: "exclusion"},
		{envVar: "KANDEV_DEBUG_ACP_MAX_FILES", class: "exclusion"},
		{envVar: "KANDEV_DEBUG_ACP_RETENTION_HOURS", class: "exclusion"},
		{envVar: "KANDEV_DEBUG_ACP_MAX_FILE_BYTES", class: "exclusion"},
		{envVar: "KANDEV_MCP_LOG_FILE", class: "exclusion"},
		{envVar: "KANDEV_DEBUG_LOG_DIR", class: "exclusion"},
		{envVar: "AGENTCTL_AUTO_APPROVE_PERMISSIONS", class: "exclusion"},
	}
}

func validateCatalogAgainstAuditedInventory(entries []CatalogEntry, exclusions []CatalogExclusion, audited []auditedStartupEnvironment) error {
	tracked := make(map[string]string, len(entries)+len(exclusions))
	for _, entry := range entries {
		for _, envVar := range entry.EnvVars {
			if previous, exists := tracked[envVar]; exists {
				return fmt.Errorf("environment variable %q is tracked more than once (%s and catalog)", envVar, previous)
			}
			tracked[envVar] = "catalog"
		}
	}
	for _, exclusion := range exclusions {
		if previous, exists := tracked[exclusion.EnvVar]; exists {
			return fmt.Errorf("environment variable %q is tracked more than once (%s and exclusion)", exclusion.EnvVar, previous)
		}
		tracked[exclusion.EnvVar] = "exclusion"
	}

	want := make(map[string]string, len(audited))
	for _, item := range audited {
		if previous, exists := want[item.envVar]; exists {
			return fmt.Errorf("audited environment variable %q is listed more than once (%s and %s)", item.envVar, previous, item.class)
		}
		want[item.envVar] = item.class
	}
	for envVar, class := range tracked {
		if auditedClass, exists := want[envVar]; !exists {
			return fmt.Errorf("environment variable %q is %s but absent from the audited inventory", envVar, class)
		} else if auditedClass != class {
			return fmt.Errorf("environment variable %q is %s, audited as %s", envVar, class, auditedClass)
		}
	}
	for envVar, class := range want {
		if trackedClass, exists := tracked[envVar]; !exists {
			return fmt.Errorf("audited environment variable %q (%s) is neither cataloged nor excluded", envVar, class)
		} else if trackedClass != class {
			return fmt.Errorf("audited environment variable %q is %s, tracked as %s", envVar, class, trackedClass)
		}
	}
	return nil
}

func TestConfigurationCatalogMarksSecretsAndExclusions(t *testing.T) {
	for _, key := range []string{"database.password", "auth.jwtSecret", "office.jwtSigningKey"} {
		entry, ok := CatalogEntryForKey(key)
		if !ok || !entry.Sensitive {
			t.Errorf("catalog entry %q is not marked sensitive: %#v", key, entry)
		}
	}
	exclusions := ConfigurationExclusions()
	foundGenerated := false
	for _, exclusion := range exclusions {
		if exclusion.EnvVar == "KANDEV_DESKTOP_HEALTH_TOKEN" && exclusion.Class == "generated" && strings.TrimSpace(exclusion.Reason) != "" {
			foundGenerated = true
		}
	}
	if !foundGenerated {
		t.Fatal("generated health token exclusion is missing")
	}
}

func TestAgentctlStartupConfigRoundTripsAndRejectsInvalidValues(t *testing.T) {
	want := AgentctlStartupConfig{
		Configured:                true,
		IdleTimeout:               2 * time.Hour,
		IdleReaperInterval:        3 * time.Minute,
		NotificationQueueCapacity: 4096,
		OTLPEndpoint:              "http://collector:4318",
	}
	raw, err := EncodeAgentctlStartupConfig(want)
	if err != nil {
		t.Fatalf("EncodeAgentctlStartupConfig: %v", err)
	}
	got, err := DecodeAgentctlStartupConfig(raw)
	if err != nil {
		t.Fatalf("DecodeAgentctlStartupConfig: %v", err)
	}
	if got != want {
		t.Fatalf("decoded contract = %#v, want %#v", got, want)
	}

	invalid := want
	invalid.NotificationQueueCapacity = 1
	if _, err := EncodeAgentctlStartupConfig(invalid); err == nil {
		t.Fatal("EncodeAgentctlStartupConfig accepted an invalid queue capacity")
	}
}
