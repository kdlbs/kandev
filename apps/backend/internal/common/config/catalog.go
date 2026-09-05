package config

import (
	"fmt"
	"sort"
	"strings"
)

// SettingSource identifies the source that supplied a resolved startup value.
type SettingSource string

const (
	SourceDefault       SettingSource = "default"
	SourceProfile       SettingSource = "profile"
	SourceConfiguration SettingSource = "configuration"
	SourceEnvironment   SettingSource = "environment"
	SourceDatabase      SettingSource = "database"
	SourceFlag          SettingSource = "flag"
)

// CatalogEntry describes one stable operator-facing startup setting.
//
// EnvVars is ordered by compatibility priority. The first non-empty variable
// wins when more than one alias is present. Internal aliases are included when
// a launcher or child process must use the same typed setting.
type CatalogEntry struct {
	Key         string
	EnvVars     []string
	Owner       string
	Default     string
	Sensitive   bool
	Description string
}

// CatalogExclusion records an environment variable that is intentionally not
// exposed as a public YAML setting.
type CatalogExclusion struct {
	EnvVar string
	Class  string
	Reason string
}

var startupCatalog = []CatalogEntry{
	{Key: "homeDir", EnvVars: []string{"KANDEV_HOME_DIR"}, Owner: "common/config", Default: "~/.kandev"},
	{Key: "server.host", EnvVars: []string{"KANDEV_SERVER_HOST"}, Owner: "backend", Default: "0.0.0.0"},
	{Key: "server.port", EnvVars: []string{"KANDEV_SERVER_PORT", "KANDEV_BACKEND_PORT", "KANDEV_PORT"}, Owner: "launcher/backend", Default: "38429"},
	{Key: "server.readTimeout", EnvVars: []string{"KANDEV_SERVER_READTIMEOUT"}, Owner: "backend", Default: "30"},
	{Key: "server.writeTimeout", EnvVars: []string{"KANDEV_SERVER_WRITETIMEOUT"}, Owner: "backend", Default: "30"},
	{Key: "server.webInternalUrl", EnvVars: []string{"KANDEV_WEB_INTERNAL_URL"}, Owner: "launcher/backend", Default: ""},
	{Key: "server.webTitlePrefix", EnvVars: []string{"KANDEV_WEB_TITLE_PREFIX"}, Owner: "backend", Default: ""},
	{Key: "server.trustedProxies", EnvVars: []string{"KANDEV_TRUSTED_PROXIES"}, Owner: "backend", Default: ""},
	{Key: "database.driver", EnvVars: []string{"KANDEV_DATABASE_DRIVER"}, Owner: "database", Default: "sqlite"},
	{Key: "database.path", EnvVars: []string{"KANDEV_DATABASE_PATH"}, Owner: "launcher/backend", Default: "<home>/data/kandev.db"},
	{Key: "database.host", EnvVars: []string{"KANDEV_DATABASE_HOST"}, Owner: "database", Default: "localhost"},
	{Key: "database.port", EnvVars: []string{"KANDEV_DATABASE_PORT"}, Owner: "database", Default: "5432"},
	{Key: "database.user", EnvVars: []string{"KANDEV_DATABASE_USER"}, Owner: "database", Default: "kandev"},
	{Key: "database.password", EnvVars: []string{"KANDEV_DATABASE_PASSWORD"}, Owner: "database", Default: "", Sensitive: true},
	{Key: "database.dbName", EnvVars: []string{"KANDEV_DATABASE_DBNAME"}, Owner: "database", Default: "kandev"},
	{Key: "database.sslMode", EnvVars: []string{"KANDEV_DATABASE_SSLMODE"}, Owner: "database", Default: "disable"},
	{Key: "database.maxConns", EnvVars: []string{"KANDEV_DATABASE_MAXCONNS"}, Owner: "database", Default: "25"},
	{Key: "database.minConns", EnvVars: []string{"KANDEV_DATABASE_MINCONNS"}, Owner: "database", Default: "5"},
	{Key: "nats.url", EnvVars: []string{"KANDEV_NATS_URL"}, Owner: "events", Default: "", Sensitive: true},
	{Key: "nats.clusterId", EnvVars: []string{"KANDEV_NATS_CLUSTERID"}, Owner: "events", Default: "kandev-cluster"},
	{Key: "nats.clientId", EnvVars: []string{"KANDEV_NATS_CLIENTID"}, Owner: "events", Default: "kandev-client"},
	{Key: "nats.maxReconnects", EnvVars: []string{"KANDEV_NATS_MAXRECONNECTS"}, Owner: "events", Default: "10"},
	{Key: "events.namespace", EnvVars: []string{"KANDEV_EVENTS_NAMESPACE"}, Owner: "events", Default: ""},
	{Key: "docker.enabled", EnvVars: []string{"KANDEV_DOCKER_ENABLED"}, Owner: "docker", Default: "true"},
	{Key: "docker.host", EnvVars: []string{"KANDEV_DOCKER_HOST", "DOCKER_HOST"}, Owner: "docker", Default: "platform socket"},
	{Key: "docker.apiVersion", EnvVars: []string{"KANDEV_DOCKER_APIVERSION"}, Owner: "docker", Default: ""},
	{Key: "docker.tlsVerify", EnvVars: []string{"KANDEV_DOCKER_TLSVERIFY"}, Owner: "docker", Default: "false"},
	{Key: "docker.defaultNetwork", EnvVars: []string{"KANDEV_DOCKER_DEFAULTNETWORK"}, Owner: "docker", Default: "kandev-network"},
	{Key: "docker.volumeBasePath", EnvVars: []string{"KANDEV_DOCKER_VOLUMEBASEPATH"}, Owner: "docker", Default: "platform default"},
	{Key: "agent.standaloneHost", EnvVars: []string{"KANDEV_AGENT_STANDALONE_HOST"}, Owner: "agentctl", Default: "localhost"},
	{Key: "agent.standalonePort", EnvVars: []string{"AGENTCTL_PORT", "KANDEV_AGENT_STANDALONE_PORT"}, Owner: "agentctl", Default: "39429"},
	{Key: "auth.jwtSecret", EnvVars: []string{"KANDEV_AUTH_JWTSECRET"}, Owner: "auth", Default: "generated", Sensitive: true},
	{Key: "auth.tokenDuration", EnvVars: []string{"KANDEV_AUTH_TOKENDURATION"}, Owner: "auth", Default: "3600"},
	{Key: "auth.sessionTTLHours", EnvVars: []string{"KANDEV_AUTH_SESSIONTTLHOURS"}, Owner: "auth", Default: "720"},
	{Key: "auth.cookieName", EnvVars: []string{"KANDEV_AUTH_COOKIE_NAME"}, Owner: "auth", Default: ""},
	{Key: "logging.level", EnvVars: []string{"KANDEV_LOG_LEVEL"}, Owner: "logging", Default: "info"},
	{Key: "logging.format", EnvVars: []string{"KANDEV_LOGGING_FORMAT"}, Owner: "logging", Default: "text"},
	{Key: "repositoryDiscovery.roots", EnvVars: []string{"KANDEV_REPOSITORYDISCOVERY_ROOTS"}, Owner: "repository discovery", Default: "[]"},
	{Key: "repositoryDiscovery.maxDepth", EnvVars: []string{"KANDEV_REPOSITORYDISCOVERY_MAXDEPTH"}, Owner: "repository discovery", Default: "5"},
	{Key: "worktree.enabled", EnvVars: []string{"KANDEV_WORKTREE_ENABLED"}, Owner: "worktree", Default: "true"},
	{Key: "worktree.defaultBranch", EnvVars: []string{"KANDEV_WORKTREE_DEFAULTBRANCH"}, Owner: "worktree", Default: "main"},
	{Key: "worktree.cleanupOnRemove", EnvVars: []string{"KANDEV_WORKTREE_CLEANUPONREMOVE"}, Owner: "worktree", Default: "true"},
	{Key: "worktree.fetchTimeoutSeconds", EnvVars: []string{"KANDEV_WORKTREE_FETCHTIMEOUTSECONDS"}, Owner: "worktree", Default: "60"},
	{Key: "worktree.pullTimeoutSeconds", EnvVars: []string{"KANDEV_WORKTREE_PULLTIMEOUTSECONDS"}, Owner: "worktree", Default: "60"},
	{Key: "repoClone.basePath", EnvVars: []string{"KANDEV_REPOCLONE_BASEPATH"}, Owner: "repository clone", Default: "<home>/repos"},
	{Key: "debug.devMode", EnvVars: []string{"KANDEV_DEBUG_DEV_MODE"}, Owner: "debug", Default: "false"},
	{Key: "debug.pprofEnabled", EnvVars: []string{"KANDEV_DEBUG_PPROF_ENABLED"}, Owner: "debug", Default: "false"},
	{Key: "office.jwtSigningKey", EnvVars: []string{"KANDEV_OFFICE_JWTSIGNINGKEY"}, Owner: "office", Default: "random per start", Sensitive: true},
	{Key: "office.schedulerTickMs", EnvVars: []string{"KANDEV_OFFICE_SCHEDULER_TICK_MS"}, Owner: "office", Default: "5000"},
	{Key: "githubCredentialBroker.publicBaseUrl", EnvVars: []string{"KANDEV_GITHUB_CREDENTIAL_BROKER_PUBLIC_BASE_URL"}, Owner: "github credential broker", Default: ""},
	{Key: "tasks.preparationTimeout", EnvVars: []string{"KANDEV_TASK_PREPARATION_TIMEOUT"}, Owner: "task lifecycle", Default: "10m"},
	{Key: "credentials.file", EnvVars: []string{"KANDEV_CREDENTIALS_FILE"}, Owner: "credentials", Default: ""},
	{Key: "limits.ghMaxConcurrent", EnvVars: []string{"KANDEV_GH_MAX_CONCURRENT"}, Owner: "subprocess admission", Default: "8"},
	{Key: "limits.gitMaxConcurrent", EnvVars: []string{"KANDEV_GIT_MAX_CONCURRENT"}, Owner: "subprocess admission", Default: "12"},
	{Key: "limits.lspMaxConnections", EnvVars: []string{"KANDEV_LSP_MAX_CONNECTIONS"}, Owner: "LSP gateway", Default: "8"},
	{Key: "messageQueue.maxPerSession", EnvVars: []string{"KANDEV_QUEUE_MAX_PER_SESSION"}, Owner: "message queue", Default: "10"},
	{Key: "agentctl.idleTimeout", EnvVars: []string{"KANDEV_ACP_IDLE_TIMEOUT"}, Owner: "agentctl", Default: "1h"},
	{Key: "agentctl.idleReaperInterval", EnvVars: []string{"KANDEV_ACP_IDLE_REAPER_INTERVAL"}, Owner: "agentctl", Default: "1m"},
	{Key: "agentctl.notificationQueueCapacity", EnvVars: []string{"KANDEV_ACP_NOTIF_QUEUE"}, Owner: "agentctl", Default: "131072"},
	{Key: "planning.coalesceWindowMs", EnvVars: []string{"KANDEV_PLAN_COALESCE_WINDOW_MS"}, Owner: "planning", Default: "300000"},
	{Key: "observability.otlpEndpoint", EnvVars: []string{"OTEL_EXPORTER_OTLP_ENDPOINT"}, Owner: "observability", Default: "", Sensitive: true},
	{Key: "launcher.webPort", EnvVars: []string{"KANDEV_WEB_PORT"}, Owner: "launcher", Default: "automatic"},
	{Key: "launcher.healthTimeoutMs", EnvVars: []string{"KANDEV_HEALTH_TIMEOUT_MS"}, Owner: "launcher", Default: "45000"},
	{Key: "launcher.noBrowser", EnvVars: []string{"KANDEV_NO_BROWSER"}, Owner: "launcher", Default: "false"},
}

var startupExclusions = []CatalogExclusion{
	{EnvVar: InternalConfigFileEnv, Class: "internal wiring", Reason: "launcher-selected configuration file handoff"},
	{EnvVar: InternalConfigHomeFileEnv, Class: "internal wiring", Reason: "launcher-selected home-file handoff"},
	{EnvVar: InternalAgentctlStartupConfigEnv, Class: "internal wiring", Reason: "resolved agentctl child configuration handoff"},
	{EnvVar: "KANDEV_LAUNCHER_PARENT_PID", Class: "internal wiring", Reason: "launcher parent-watchdog handoff"},
	{EnvVar: "KANDEV_BACKEND_PID_FILE", Class: "internal wiring", Reason: "supervisor-owned process state"},
	{EnvVar: "KANDEV_DESKTOP_HEALTH_TOKEN", Class: "generated", Reason: "per-launch health authentication token"},
	{EnvVar: "KANDEV_DESKTOP_NATIVE_NOTIFICATIONS", Class: "internal wiring", Reason: "desktop shell capability handoff"},
	{EnvVar: "KANDEV_DESKTOP_RUNTIME", Class: "internal wiring", Reason: "desktop shell launch-policy handoff"},
	{EnvVar: "KANDEV_BUNDLE_DIR", Class: "packaging", Reason: "runtime bundle discovery"},
	{EnvVar: "KANDEV_WEB_DIST_DIR", Class: "packaging", Reason: "embedded web asset override"},
	{EnvVar: "KANDEV_TASK_ID", Class: "workspace injection", Reason: "task-owned child process context"},
	{EnvVar: "KANDEV_SESSION_ID", Class: "workspace injection", Reason: "session-owned child process context"},
	{EnvVar: "KANDEV_WORKSPACE_ID", Class: "workspace injection", Reason: "workspace-owned child process context"},
	{EnvVar: "KANDEV_E2E_MOCK", Class: "test", Reason: "profile selector and test harness"},
	{EnvVar: "KANDEV_MOCK_AGENT", Class: "test", Reason: "profile-selected mock provider"},
	{EnvVar: "KANDEV_MOCK_GITHUB", Class: "test", Reason: "test-only provider replacement"},
	{EnvVar: "KANDEV_MOCK_GITLAB", Class: "test", Reason: "test-only provider replacement"},
	{EnvVar: "KANDEV_MOCK_JIRA", Class: "test", Reason: "test-only provider replacement"},
	{EnvVar: "KANDEV_MOCK_LINEAR", Class: "test", Reason: "test-only provider replacement"},
	{EnvVar: "KANDEV_FEATURES_OFFICE", Class: "profile", Reason: "runtime feature flag registry"},
	{EnvVar: "KANDEV_FEATURES_AUTH", Class: "profile", Reason: "runtime feature flag registry"},
	{EnvVar: "KANDEV_FEATURES_CLAUDE_BACKGROUND_PROMPT_HANDOFF", Class: "debug", Reason: "runtime feature flag registry"},
	{EnvVar: "KANDEV_FEATURES_CLAUDE_MID_TURN_STEERING", Class: "debug", Reason: "runtime feature flag registry"},
	{EnvVar: "KANDEV_FEATURES_OFFICE_SESSION_IDENTITY", Class: "debug", Reason: "runtime feature flag registry"},
	{EnvVar: "KANDEV_DEBUG_AGENT_MESSAGES", Class: "debug", Reason: "ACP frame diagnostics"},
	{EnvVar: "KANDEV_DEBUG_ACP_MAX_FILES", Class: "debug", Reason: "ACP debug retention"},
	{EnvVar: "KANDEV_DEBUG_ACP_RETENTION_HOURS", Class: "debug", Reason: "ACP debug retention"},
	{EnvVar: "KANDEV_DEBUG_ACP_MAX_FILE_BYTES", Class: "debug", Reason: "ACP debug retention"},
	{EnvVar: "KANDEV_MCP_LOG_FILE", Class: "debug", Reason: "agentctl MCP debug logging"},
	{EnvVar: "KANDEV_DEBUG_LOG_DIR", Class: "debug", Reason: "ACP debug logging directory"},
	{EnvVar: "AGENTCTL_AUTO_APPROVE_PERMISSIONS", Class: "test", Reason: "profile-selected E2E behavior"},
}

// ConfigurationCatalog returns a defensive copy of the stable startup
// catalog. Callers may sort or edit the returned slices.
func ConfigurationCatalog() []CatalogEntry {
	out := make([]CatalogEntry, len(startupCatalog))
	for i, entry := range startupCatalog {
		out[i] = entry
		out[i].EnvVars = append([]string(nil), entry.EnvVars...)
	}
	return out
}

// ConfigurationExclusions returns a defensive copy of the reviewed
// environment-variable exclusions.
func ConfigurationExclusions() []CatalogExclusion {
	return append([]CatalogExclusion(nil), startupExclusions...)
}

// CatalogEntryForKey returns the catalog entry for a canonical YAML key.
func CatalogEntryForKey(key string) (CatalogEntry, bool) {
	for _, entry := range startupCatalog {
		if entry.Key == key {
			entry.EnvVars = append([]string(nil), entry.EnvVars...)
			return entry, true
		}
	}
	return CatalogEntry{}, false
}

// ValidateCatalog checks that canonical keys and environment aliases are
// unique and that every audited environment variable has an owner.
func ValidateCatalog() error {
	keys := make(map[string]struct{}, len(startupCatalog))
	envs := make(map[string]string)
	for _, entry := range startupCatalog {
		if entry.Key == "" || entry.Owner == "" {
			return fmt.Errorf("catalog entry has empty key or owner: %#v", entry)
		}
		if _, exists := keys[entry.Key]; exists {
			return fmt.Errorf("duplicate catalog key %q", entry.Key)
		}
		keys[entry.Key] = struct{}{}
		for _, envVar := range entry.EnvVars {
			if envVar == "" {
				return fmt.Errorf("catalog key %q has an empty environment alias", entry.Key)
			}
			if previous, exists := envs[envVar]; exists {
				return fmt.Errorf("environment alias %q belongs to both %q and %q", envVar, previous, entry.Key)
			}
			envs[envVar] = entry.Key
		}
	}
	for _, exclusion := range startupExclusions {
		if exclusion.EnvVar == "" || exclusion.Class == "" || strings.TrimSpace(exclusion.Reason) == "" {
			return fmt.Errorf("incomplete catalog exclusion: %#v", exclusion)
		}
		if owner, exists := envs[exclusion.EnvVar]; exists {
			return fmt.Errorf("environment variable %q is both cataloged as %q and excluded", exclusion.EnvVar, owner)
		}
		envs[exclusion.EnvVar] = "excluded"
	}
	return nil
}

// CatalogEnvironmentVariables returns all cataloged and excluded variables in
// stable order. It is used by inventory and documentation completeness tests.
func CatalogEnvironmentVariables() []string {
	seen := make(map[string]struct{})
	var out []string
	for _, entry := range startupCatalog {
		for _, envVar := range entry.EnvVars {
			if _, ok := seen[envVar]; !ok {
				seen[envVar] = struct{}{}
				out = append(out, envVar)
			}
		}
	}
	for _, exclusion := range startupExclusions {
		if _, ok := seen[exclusion.EnvVar]; !ok {
			seen[exclusion.EnvVar] = struct{}{}
			out = append(out, exclusion.EnvVar)
		}
	}
	sort.Strings(out)
	return out
}
