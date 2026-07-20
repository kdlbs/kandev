// Package config provides configuration management for Kandev.
// It supports loading configuration from environment variables, config files, and defaults.
package config

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/ports"
	"github.com/kandev/kandev/internal/profiles"
	"github.com/spf13/viper"
)

// kandevHomeSubdir is the hidden directory name used for the Kandev root
// under the user's home directory (e.g. ~/.kandev). This is the single
// source of truth for the dotdir name; derived paths go through
// ResolvedHomeDir / ResolvedDataDir rather than re-constructing it.
const kandevHomeSubdir = ".kandev"

// Config holds all configuration sections for Kandev.
type Config struct {
	// HomeDir is the root Kandev directory (e.g. ~/.kandev in prod, or
	// <repo>/.kandev-dev during local development). When empty, falls back
	// to ~/.kandev. All workspace artifacts (data, tasks, worktrees, repos)
	// live under this root.
	HomeDir                string                       `mapstructure:"homeDir"`
	Server                 ServerConfig                 `mapstructure:"server"`
	Database               DatabaseConfig               `mapstructure:"database"`
	NATS                   NATSConfig                   `mapstructure:"nats"`
	Events                 EventsConfig                 `mapstructure:"events"`
	Docker                 DockerConfig                 `mapstructure:"docker"`
	Agent                  AgentConfig                  `mapstructure:"agent"`
	Auth                   AuthConfig                   `mapstructure:"auth"`
	Logging                LoggingConfig                `mapstructure:"logging"`
	RepositoryDiscovery    RepositoryDiscoveryConfig    `mapstructure:"repositoryDiscovery"`
	Worktree               WorktreeConfig               `mapstructure:"worktree"`
	RepoClone              RepoCloneConfig              `mapstructure:"repoClone"`
	Debug                  DebugConfig                  `mapstructure:"debug"`
	Office                 OfficeConfig                 `mapstructure:"office"`
	Voice                  VoiceConfig                  `mapstructure:"voice"`
	Features               FeaturesConfig               `mapstructure:"features"`
	GitHubApp              GitHubAppConfig              `mapstructure:"githubApp"`
	GitHubCredentialBroker GitHubCredentialBrokerConfig `mapstructure:"githubCredentialBroker"`
}

// GitHubCredentialBrokerConfig holds the externally reachable service URL
// used by managed remote executors. It is independent of GitHub App setup so
// PAT and named gh CLI workspaces can use remote executors.
type GitHubCredentialBrokerConfig struct {
	PublicBaseURL string `mapstructure:"publicBaseUrl" json:"-"`
}

func (c GitHubCredentialBrokerConfig) validate() error {
	if strings.TrimSpace(c.PublicBaseURL) == "" {
		return nil
	}
	return validatePublicBaseURL("githubCredentialBroker.publicBaseUrl", c.PublicBaseURL)
}

// GitHubAppConfig holds deployment-owned GitHub App registration material.
// Workspace APIs must expose Availability rather than serializing this value.
type GitHubAppConfig struct {
	AppID          int64  `mapstructure:"appId" json:"-"`
	ClientID       string `mapstructure:"clientId" json:"-"`
	ClientSecret   string `mapstructure:"clientSecret" json:"-"`
	PrivateKey     string `mapstructure:"privateKey" json:"-"`
	PrivateKeyFile string `mapstructure:"privateKeyFile" json:"-"`
	WebhookSecret  string `mapstructure:"webhookSecret" json:"-"`
	Slug           string `mapstructure:"slug" json:"-"`
	PublicBaseURL  string `mapstructure:"publicBaseUrl" json:"-"`
}

// GitHubAppAvailability is the non-secret deployment status safe for APIs.
type GitHubAppAvailability struct {
	Configured bool   `json:"configured"`
	Available  bool   `json:"available"`
	Reason     string `json:"reason,omitempty"`
}

// Availability reports whether the deployment App is usable without exposing
// registration identifiers, paths, or secret material.
func (c GitHubAppConfig) Availability() GitHubAppAvailability {
	if !c.configured() {
		return GitHubAppAvailability{}
	}
	if err := c.validate(); err != nil {
		return GitHubAppAvailability{Configured: true, Reason: "invalid_configuration"}
	}
	return GitHubAppAvailability{Configured: true, Available: true}
}

func (c GitHubAppConfig) configured() bool {
	return c.AppID != 0 || c.ClientID != "" || c.ClientSecret != "" ||
		c.PrivateKey != "" || c.PrivateKeyFile != "" || c.WebhookSecret != "" ||
		c.Slug != "" || c.PublicBaseURL != ""
}

// Configured reports whether any deployment App field is present. A partial
// configuration is intentionally authoritative and must fail validation.
func (c GitHubAppConfig) Configured() bool {
	return c.configured()
}

// Validate checks the all-or-none deployment App contract.
func (c GitHubAppConfig) Validate() error {
	return c.validate()
}

// PrivateKeyPEM resolves inline or file-backed private key material. Inline
// values may contain literal newlines or escaped newlines from environment
// configuration.
func (c GitHubAppConfig) PrivateKeyPEM() ([]byte, error) {
	if c.PrivateKey != "" && c.PrivateKeyFile != "" {
		return nil, fmt.Errorf("githubApp.privateKey and githubApp.privateKeyFile are mutually exclusive")
	}
	if c.PrivateKey != "" {
		return []byte(strings.ReplaceAll(c.PrivateKey, `\n`, "\n")), nil
	}
	if c.PrivateKeyFile == "" {
		return nil, fmt.Errorf("githubApp private key is required")
	}
	key, err := os.ReadFile(expandTilde(c.PrivateKeyFile))
	if err != nil {
		return nil, fmt.Errorf("read githubApp private key file: %w", err)
	}
	return key, nil
}

func (c GitHubAppConfig) validate() error {
	if !c.configured() {
		return nil
	}
	var missing []string
	if c.AppID <= 0 {
		missing = append(missing, "appId")
	}
	if strings.TrimSpace(c.ClientID) == "" {
		missing = append(missing, "clientId")
	}
	if c.ClientSecret == "" {
		missing = append(missing, "clientSecret")
	}
	if c.PrivateKey == "" && c.PrivateKeyFile == "" {
		missing = append(missing, "privateKey or privateKeyFile")
	}
	if c.WebhookSecret == "" {
		missing = append(missing, "webhookSecret")
	}
	if strings.TrimSpace(c.Slug) == "" {
		missing = append(missing, "slug")
	}
	if strings.TrimSpace(c.PublicBaseURL) == "" {
		missing = append(missing, "publicBaseUrl")
	}
	if len(missing) > 0 {
		return fmt.Errorf("githubApp requires all fields; missing %s", strings.Join(missing, ", "))
	}

	keyPEM, err := c.PrivateKeyPEM()
	if err != nil {
		return err
	}
	if _, err := parseRSAPrivateKey(keyPEM); err != nil {
		return fmt.Errorf("githubApp private key is not a valid RSA private key")
	}
	if err := validatePublicBaseURL("githubApp.publicBaseUrl", c.PublicBaseURL); err != nil {
		return err
	}
	return nil
}

func parseRSAPrivateKey(keyPEM []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, fmt.Errorf("PEM block not found")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("key is not RSA")
	}
	return key, nil
}

func validatePublicBaseURL(field, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || u.Hostname() == "" {
		return fmt.Errorf("%s must be an absolute URL", field)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%s must not include credentials, query, or fragment", field)
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !IsLoopbackHost(u.Hostname())) {
		return fmt.Errorf("%s must use HTTPS outside loopback development", field)
	}
	return nil
}

// expandTilde expands a leading "~/" to the user's home directory.
// Returns the input unchanged if expansion is unnecessary or fails.
func expandTilde(p string) string {
	if !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, p[2:])
}

// ResolvedHomeDir returns the Kandev root directory — the parent of data,
// tasks, worktrees, repos, sessions, quick-chat and lsp-servers.
//
// Resolution order:
//  1. KANDEV_HOME_DIR (explicit override — e.g. /data in Docker, /kandev in K8s,
//     or <repo>/.kandev-dev during local development).
//  2. ~/.kandev (default).
func (c *Config) ResolvedHomeDir() string {
	if c.HomeDir != "" {
		return expandTilde(c.HomeDir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return kandevHomeSubdir
	}
	return filepath.Join(home, kandevHomeSubdir)
}

// ResolvedDataDir returns the base data directory (where the SQLite DB lives).
// Always <ResolvedHomeDir>/data — relocate via KANDEV_HOME_DIR, not a separate knob.
func (c *Config) ResolvedDataDir() string {
	return filepath.Join(c.ResolvedHomeDir(), "data")
}

// defaultServerHost is the wildcard host the server binds to when no host is
// configured — every interface.
const defaultServerHost = "0.0.0.0"

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	// Host is the bind address. A single value (e.g. "127.0.0.1") behaves as
	// it always has; a comma-separated list (e.g. "127.0.0.1,100.64.0.1")
	// binds one listener per address. Empty means the wildcard default.
	//
	// Host carries the KANDEV_SERVER_HOST env override, so when it is set it
	// wins over Hosts to preserve env-over-file precedence (see ResolvedBinds).
	Host string `mapstructure:"host"`
	// Hosts is the YAML-array form of Host, for config files that prefer an
	// array to a comma-separated string. It is used only when Host is unset.
	Hosts          []string `mapstructure:"hosts"`
	Port           int      `mapstructure:"port"`
	ReadTimeout    int      `mapstructure:"readTimeout"`  // in seconds
	WriteTimeout   int      `mapstructure:"writeTimeout"` // in seconds
	WebInternalURL string   `mapstructure:"webInternalUrl"`
}

// splitHosts splits a comma-separated host string, trimming whitespace and
// dropping empty entries.
func splitHosts(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// normalizeHost validates h and returns its canonical form plus whether it is a
// wildcard (binds every interface). IP addresses are canonicalized via
// net.ParseIP so equivalent forms dedupe (e.g. 0:0:0:0:0:0:0:1 and ::1); any
// unspecified IP (0.0.0.0, ::, or a longhand form) is treated as a wildcard.
// Hostnames are returned as-is. An invalid entry returns an error.
func normalizeHost(h string) (canonical string, wildcard bool, err error) {
	if ip := net.ParseIP(h); ip != nil {
		if ip.IsUnspecified() {
			return ip.String(), true, nil
		}
		return ip.String(), false, nil
	}
	if isValidHostname(h) {
		return h, false, nil
	}
	return "", false, fmt.Errorf("server bind host %q is not a valid IP address or hostname", h)
}

// isValidHostname reports whether h is a syntactically valid RFC 1123 hostname.
func isValidHostname(h string) bool {
	if len(h) == 0 || len(h) > 253 {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(h, "."), ".") {
		if !isValidHostnameLabel(label) {
			return false
		}
	}
	return true
}

// isValidHostnameLabel reports whether label is a valid single hostname label
// (letters, digits, and interior hyphens, 1–63 chars).
func isValidHostnameLabel(label string) bool {
	if len(label) == 0 || len(label) > 63 {
		return false
	}
	if label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for i := 0; i < len(label); i++ {
		c := label[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') &&
			(c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

// IsLoopbackHost reports whether h refers only to the local loopback
// interface. A wildcard (empty/0.0.0.0/::) is NOT loopback because it also
// binds routable interfaces. Unresolvable hostnames are treated as
// non-loopback (fail-closed) so callers gating on "any non-loopback bind"
// don't under-report exposure.
func IsLoopbackHost(h string) bool {
	h = strings.TrimSpace(h)
	if h == "" {
		return false
	}
	if strings.EqualFold(h, "localhost") {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// ResolvedBinds returns the de-duplicated, validated list of hosts the HTTP
// server should bind to. server.host (comma-separated string) takes precedence
// over server.hosts (YAML array) when set, because Host carries the
// KANDEV_SERVER_HOST env override and env must beat a config-file server.hosts
// (env-over-file precedence, and the desktop loopback contract). server.hosts
// is used only when host is unset. Whitespace is trimmed and empty/duplicate
// entries dropped, and IP addresses are canonicalized so equivalent forms
// dedupe. Every entry is validated before the set is collapsed, so an invalid
// host is rejected regardless of its position relative to a wildcard. If any
// entry is a wildcard (empty/0.0.0.0/:: or an unspecified IP) the whole set
// collapses to that single wildcard, since it already binds every interface. An
// empty result falls back to the wildcard default.
func (c *ServerConfig) ResolvedBinds() ([]string, error) {
	raw := splitHosts(c.Host)
	if len(raw) == 0 {
		for _, h := range c.Hosts {
			raw = append(raw, splitHosts(h)...)
		}
	}

	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	wildcard := ""
	for _, h := range raw {
		norm, isWild, err := normalizeHost(h)
		if err != nil {
			return nil, err
		}
		if isWild {
			if wildcard == "" {
				wildcard = norm
			}
			continue
		}
		if _, dup := seen[norm]; dup {
			continue
		}
		seen[norm] = struct{}{}
		out = append(out, norm)
	}
	if wildcard != "" {
		return []string{wildcard}, nil
	}
	if len(out) == 0 {
		return []string{defaultServerHost}, nil
	}
	return out, nil
}

// NonLoopbackBinds returns the resolved bind hosts that are not loopback-only.
// The sibling app-auth work uses this to decide whether to require auth (fail
// closed when the server is reachable off-box). A wildcard bind counts as
// non-loopback.
func (c *ServerConfig) NonLoopbackBinds() ([]string, error) {
	binds, err := c.ResolvedBinds()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(binds))
	for _, h := range binds {
		if !IsLoopbackHost(h) {
			out = append(out, h)
		}
	}
	return out, nil
}

// DatabaseConfig holds database connection configuration.
type DatabaseConfig struct {
	Driver   string `mapstructure:"driver"`
	Path     string `mapstructure:"path"`
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbName"`
	SSLMode  string `mapstructure:"sslMode"`
	MaxConns int    `mapstructure:"maxConns"`
	MinConns int    `mapstructure:"minConns"`
}

// NATSConfig holds NATS messaging configuration.
type NATSConfig struct {
	URL           string `mapstructure:"url"`
	ClusterID     string `mapstructure:"clusterId"`
	ClientID      string `mapstructure:"clientId"`
	MaxReconnects int    `mapstructure:"maxReconnects"`
}

// EventsConfig holds event bus namespace configuration.
type EventsConfig struct {
	// Namespace isolates queue-group subscribers across deployments/instances.
	// Empty value means derive from runtime data identity.
	Namespace string `mapstructure:"namespace"`
}

// DockerConfig holds Docker client configuration.
type DockerConfig struct {
	// Enabled controls whether the Docker runtime is available for task execution.
	// When true and Docker is accessible, tasks can use Docker-based executors.
	// Default: true (Docker runtime is enabled if Docker is available)
	Enabled        bool   `mapstructure:"enabled"`
	Host           string `mapstructure:"host"`
	APIVersion     string `mapstructure:"apiVersion"`
	TLSVerify      bool   `mapstructure:"tlsVerify"`
	DefaultNetwork string `mapstructure:"defaultNetwork"`
	VolumeBasePath string `mapstructure:"volumeBasePath"`
}

// AuthConfig holds authentication configuration.
type AuthConfig struct {
	JWTSecret     string `mapstructure:"jwtSecret"`
	TokenDuration int    `mapstructure:"tokenDuration"` // in seconds
}

// OfficeConfig holds configuration for the office (autonomous agents) feature.
type OfficeConfig struct {
	// JWTSigningKey is the HMAC key used to sign agent runtime JWTs.
	// When empty, a random key is generated at startup — fine for dev, but
	// means every restart invalidates outstanding agent tokens. Production
	// deployments should set a stable value (e.g. via KANDEV_OFFICE_JWTSIGNINGKEY).
	JWTSigningKey string `mapstructure:"jwtSigningKey"`
}

// VoiceConfig holds configuration for the chat voice-input transcription
// fallback. The primary voice-input engine runs entirely in the browser
// (Web Speech API); this server-side fallback is only used when the browser
// has no SpeechRecognition support (e.g. Firefox).
//
// When OpenAIAPIKey is empty the /api/v1/transcribe endpoint returns 503
// and the frontend hides the fallback path, so the feature is safe to
// ship un-configured.
type VoiceConfig struct {
	// OpenAIAPIKey is the API key used to call OpenAI's Whisper transcription
	// endpoint. Set via KANDEV_VOICE_OPENAI_API_KEY.
	OpenAIAPIKey string `mapstructure:"openAIApiKey"`
}

// FeaturesConfig is the central registry of runtime feature flags. Every flag
// defaults to false so production binaries ship with new work hidden until a
// deployment explicitly opts in (env var, e.g. KANDEV_FEATURES_OFFICE=true).
//
// The struct doubles as the wire shape for GET /api/v1/features — `json` tags
// keep the field names lowercase and the handler in helpers.go just calls
// `c.JSON(200, p.features)` so new fields are picked up automatically.
//
// See docs/decisions/0007-runtime-feature-flags.md for the pattern and rollout policy.
type FeaturesConfig struct {
	// Office gates the autonomous-agent feature: backend service construction,
	// HTTP/WS route registration, and frontend nav/route visibility.
	Office bool `mapstructure:"office" json:"office"`

	// Plugins gates the extensible plugin system: backend service
	// construction, HTTP/WS route registration, and frontend nav/route
	// visibility.
	Plugins bool `mapstructure:"plugins" json:"plugins"`
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"`
	OutputPath string `mapstructure:"outputPath"`

	// Rotation options - apply only when OutputPath is a file path
	// (ignored for stdout/stderr). Backed by lumberjack.
	//
	// Note: lumberjack creates the active log file with mode 0600 (owner read/write
	// only); the previous os.OpenFile path used 0644. External log shippers or
	// sidecars running as a different user will need to run as the same user.
	MaxSizeMB  int  `mapstructure:"maxSizeMb"`  // rotate when file exceeds this size; 0 = lumberjack default (100MB)
	MaxBackups int  `mapstructure:"maxBackups"` // max number of rotated files to retain; 0 = unlimited
	MaxAgeDays int  `mapstructure:"maxAgeDays"` // max age in days of rotated files; 0 = unlimited
	Compress   bool `mapstructure:"compress"`   // gzip rotated files
}

// RepositoryDiscoveryConfig holds configuration for local repository scanning.
type RepositoryDiscoveryConfig struct {
	Roots    []string `mapstructure:"roots"`
	MaxDepth int      `mapstructure:"maxDepth"`
}

// WorktreeConfig holds Git worktree configuration for concurrent agent execution.
type WorktreeConfig struct {
	Enabled             bool   `mapstructure:"enabled"`             // Enable worktree mode
	DefaultBranch       string `mapstructure:"defaultBranch"`       // Default base branch (default: main)
	CleanupOnRemove     bool   `mapstructure:"cleanupOnRemove"`     // Remove worktree directory on task deletion
	FetchTimeoutSeconds int    `mapstructure:"fetchTimeoutSeconds"` // Git fetch timeout before worktree creation
	PullTimeoutSeconds  int    `mapstructure:"pullTimeoutSeconds"`  // Git pull timeout before worktree creation
}

// RepoCloneConfig holds configuration for automatic repository cloning.
type RepoCloneConfig struct {
	BasePath string `mapstructure:"basePath"` // Base directory for cloned repos (default: ~/.kandev/repos)
}

// DebugConfig holds debug/profiling configuration.
type DebugConfig struct {
	// DevMode enables all developer-only endpoints (pprof, memory, debug export).
	// Controlled via KANDEV_DEBUG_DEV_MODE env var. Default: false.
	DevMode bool `mapstructure:"devMode"`

	// PprofEnabled is a legacy alias — if set, it also enables DevMode.
	// Controlled via KANDEV_DEBUG_PPROF_ENABLED env var. Default: false.
	PprofEnabled bool `mapstructure:"pprofEnabled"`
}

// AgentConfig holds agent runtime configuration.
// Note: Runtime selection is now per-task based on executor type, not global.
// The Standalone runtime (agentctl) always runs as a core service.
// Docker runtime is available when docker.enabled=true.
type AgentConfig struct {
	// StandaloneHost is the host where standalone agentctl is running (default: localhost)
	StandaloneHost string `mapstructure:"standaloneHost"`

	// StandalonePort is the control port for standalone agentctl (default: 39429)
	StandalonePort int `mapstructure:"standalonePort"`

	// StandaloneAuthToken is the per-launch auth token retrieved via handshake.
	// Set at runtime after agentctl starts; not persisted in config files.
	StandaloneAuthToken string `mapstructure:"-"`

	// StandalonePID is the OS process id of the standalone agentctl control-server
	// this backend spawned. Set at runtime after agentctl starts (from the
	// launcher); not persisted in config files. Used as the host-local liveness
	// handle recorded in executors_running.local_pid for local/standalone rows.
	StandalonePID int `mapstructure:"-"`
}

// ReadTimeoutDuration returns the read timeout as a time.Duration.
func (s *ServerConfig) ReadTimeoutDuration() time.Duration {
	return time.Duration(s.ReadTimeout) * time.Second
}

// WriteTimeoutDuration returns the write timeout as a time.Duration.
func (s *ServerConfig) WriteTimeoutDuration() time.Duration {
	return time.Duration(s.WriteTimeout) * time.Second
}

// TokenDurationTime returns the token duration as a time.Duration.
func (a *AuthConfig) TokenDurationTime() time.Duration {
	return time.Duration(a.TokenDuration) * time.Second
}

// detectDefaultLogFormat returns the appropriate log format based on environment.
// Returns "json" if running in Kubernetes or other production environments.
// Returns "text" for terminal/development use (human-readable console format).
func detectDefaultLogFormat() string {
	// Check if running in Kubernetes
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return "json"
	}

	// Check for explicit production environment
	if env := os.Getenv("KANDEV_ENV"); env == "production" || env == "prod" {
		return "json"
	}

	// Default to text format for terminal use (more readable than JSON)
	return "text"
}

// setDefaults configures default values for all configuration options.
func setDefaults(v *viper.Viper) {
	// Server defaults. Host defaults to empty (not the wildcard) so an unset
	// host is distinguishable from an explicit one: ResolvedBinds falls back to
	// server.hosts only when host is unset, and empty resolves to the wildcard
	// default. This keeps KANDEV_SERVER_HOST winning over a config-file
	// server.hosts.
	v.SetDefault("server.host", "")
	v.SetDefault("server.port", ports.Backend)
	v.SetDefault("server.readTimeout", 30)
	v.SetDefault("server.writeTimeout", 30)
	v.SetDefault("server.webInternalUrl", "")

	// HomeDir default — empty means resolve from KANDEV_HOME_DIR env or ~/.kandev
	v.SetDefault("homeDir", "")

	// Database defaults
	v.SetDefault("database.driver", "sqlite")
	v.SetDefault("database.path", "")
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.user", "kandev")
	v.SetDefault("database.password", "")
	v.SetDefault("database.dbName", "kandev")
	v.SetDefault("database.sslMode", "disable")
	v.SetDefault("database.maxConns", 25)
	v.SetDefault("database.minConns", 5)

	// NATS defaults - empty URL means use in-memory event bus
	v.SetDefault("nats.url", "")
	v.SetDefault("nats.clusterId", "kandev-cluster")
	v.SetDefault("nats.clientId", "kandev-client")
	v.SetDefault("nats.maxReconnects", 10)

	// Events defaults
	v.SetDefault("events.namespace", "")

	// Docker defaults — platform-aware host and volume path
	v.SetDefault("docker.enabled", true) // Docker runtime enabled by default if Docker is available
	v.SetDefault("docker.host", DefaultDockerHost())
	v.SetDefault("docker.apiVersion", "") // Empty = auto-negotiate with daemon
	v.SetDefault("docker.tlsVerify", false)
	v.SetDefault("docker.defaultNetwork", "kandev-network")
	v.SetDefault("docker.volumeBasePath", defaultDockerVolumePath())

	// Agent defaults (runtime selection is now per-task based on executor type)
	v.SetDefault("agent.standaloneHost", "localhost")
	v.SetDefault("agent.standalonePort", ports.AgentCtl)

	// Auth defaults
	v.SetDefault("auth.jwtSecret", "")
	v.SetDefault("auth.tokenDuration", 3600) // 1 hour

	// Office defaults
	v.SetDefault("office.jwtSigningKey", "")

	// Voice defaults
	v.SetDefault("voice.openAIApiKey", "")

	// Feature-flag defaults live in ./features.yaml (symlinked to
	// apps/backend/internal/features/features.yaml). LoadWithPath applies
	// them via features.ApplyDefaults after this function returns so the
	// embedded YAML, not a Go literal, is the source of truth. Env vars
	// (KANDEV_FEATURES_<NAME>) and the deployment's config.yaml still
	// override. See docs/decisions/0007-runtime-feature-flags.md.

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", detectDefaultLogFormat())
	v.SetDefault("logging.outputPath", "stdout")
	v.SetDefault("logging.maxSizeMb", 100)
	v.SetDefault("logging.maxBackups", 5)
	v.SetDefault("logging.maxAgeDays", 30)
	v.SetDefault("logging.compress", true)

	// Repository discovery defaults
	v.SetDefault("repositoryDiscovery.roots", []string{})
	v.SetDefault("repositoryDiscovery.maxDepth", 5)

	// Worktree defaults
	v.SetDefault("worktree.enabled", true)
	v.SetDefault("worktree.defaultBranch", "main")
	v.SetDefault("worktree.cleanupOnRemove", true)
	v.SetDefault("worktree.fetchTimeoutSeconds", 60)
	v.SetDefault("worktree.pullTimeoutSeconds", 60)

	// RepoClone defaults
	v.SetDefault("repoClone.basePath", "")

	// Debug defaults
	v.SetDefault("debug.pprofEnabled", false)
}

// DefaultDockerHost returns the platform-appropriate Docker socket path.
// Respects DOCKER_HOST env var as override (standard Docker convention).
func DefaultDockerHost() string {
	if host := os.Getenv("DOCKER_HOST"); host != "" {
		return host
	}
	if runtime.GOOS == "windows" {
		return "npipe:////./pipe/docker_engine"
	}
	return "unix:///var/run/docker.sock"
}

// defaultDockerVolumePath returns the platform-appropriate volume base path.
func defaultDockerVolumePath() string {
	if runtime.GOOS == "windows" {
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			localAppData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
		}
		return filepath.Join(localAppData, "kandev", "volumes")
	}
	return "/var/lib/kandev/volumes"
}

// Load reads configuration from environment variables, config file, and defaults.
// Environment variables use the prefix KANDEV_ with snake_case naming.
// Config file should be named config.yaml and placed in the current directory or /etc/kandev/.
func Load() (*Config, error) {
	return LoadWithPath("")
}

// LoadWithPath reads configuration from the specified path or default locations.
func LoadWithPath(configPath string) (*Config, error) {
	v := viper.New()

	// Apply the active runtime profile (prod / dev / e2e) from the
	// embedded profiles.yaml. This writes env vars onto our own
	// process so the subsequent AutomaticEnv and the rest of the
	// codebase's os.Getenv reads see the YAML-declared values.
	// Vars already set by the launcher / shell / per-spec override
	// are left alone, giving precedence:
	//
	//   shell env / launcher env > profiles.yaml > Go zero values
	//
	// A parse error here means someone committed a malformed
	// profiles.yaml; fail loud so CI catches it before a release ships.
	if _, _, err := profiles.ApplyProfile(); err != nil {
		return nil, fmt.Errorf("apply profile defaults: %w", err)
	}

	// Set defaults next. setDefaults seeds non-feature config
	// (server, database, logging, …); feature-flag defaults flow
	// through env via ApplyProfile + AutomaticEnv below.
	setDefaults(v)

	// Seed Viper's features.* keyspace from profiles.yaml so the
	// typed Config struct populates correctly even in tests that
	// bypass AutomaticEnv. AutomaticEnv still wins at runtime.
	flags, err := profiles.FeatureFlagDefaults()
	if err != nil {
		return nil, fmt.Errorf("read feature flag defaults: %w", err)
	}
	for name, value := range flags {
		v.SetDefault("features."+name, value == "true")
	}

	// Configure environment variables
	v.SetEnvPrefix("KANDEV")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Explicit bindings for snake_case env vars (camelCase config keys)
	// AutomaticEnv does not handle camelCase to SNAKE_CASE conversion,
	// so we explicitly bind keys where env var naming differs from config key naming.
	_ = v.BindEnv("agent.standalonePort", "AGENTCTL_PORT", "KANDEV_AGENT_STANDALONE_PORT")
	_ = v.BindEnv("agent.standaloneHost", "KANDEV_AGENT_STANDALONE_HOST")
	_ = v.BindEnv("server.webInternalUrl", "KANDEV_WEB_INTERNAL_URL")
	_ = v.BindEnv("homeDir", "KANDEV_HOME_DIR")
	_ = v.BindEnv("logging.level", "KANDEV_LOG_LEVEL")
	_ = v.BindEnv("events.namespace", "KANDEV_EVENTS_NAMESPACE")
	_ = v.BindEnv("debug.devMode", "KANDEV_DEBUG_DEV_MODE")
	_ = v.BindEnv("debug.pprofEnabled", "KANDEV_DEBUG_PPROF_ENABLED")
	_ = v.BindEnv("voice.openAIApiKey", "KANDEV_VOICE_OPENAI_API_KEY")
	_ = v.BindEnv("githubApp.appId", "KANDEV_GITHUB_APP_APP_ID")
	_ = v.BindEnv("githubApp.clientId", "KANDEV_GITHUB_APP_CLIENT_ID")
	_ = v.BindEnv("githubApp.clientSecret", "KANDEV_GITHUB_APP_CLIENT_SECRET")
	_ = v.BindEnv("githubApp.privateKey", "KANDEV_GITHUB_APP_PRIVATE_KEY")
	_ = v.BindEnv("githubApp.privateKeyFile", "KANDEV_GITHUB_APP_PRIVATE_KEY_FILE")
	_ = v.BindEnv("githubApp.webhookSecret", "KANDEV_GITHUB_APP_WEBHOOK_SECRET")
	_ = v.BindEnv("githubApp.slug", "KANDEV_GITHUB_APP_SLUG")
	_ = v.BindEnv("githubApp.publicBaseUrl", "KANDEV_GITHUB_APP_PUBLIC_BASE_URL")
	_ = v.BindEnv(
		"githubCredentialBroker.publicBaseUrl",
		"KANDEV_GITHUB_CREDENTIAL_BROKER_PUBLIC_BASE_URL",
	)

	// Configure config file
	v.SetConfigName("config")
	v.SetConfigType("yaml")

	if configPath != "" {
		v.AddConfigPath(configPath)
	}
	v.AddConfigPath(".")
	v.AddConfigPath("/etc/kandev/")

	// Read config file (ignore if not found)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// validate checks that all required configuration fields are set.
// In development mode (default), most fields are optional.
func validate(cfg *Config) error {
	var errs []string

	// Server validation - always required
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		errs = append(errs, "server.port must be between 1 and 65535")
	}
	if _, err := cfg.Server.ResolvedBinds(); err != nil {
		errs = append(errs, err.Error())
	}

	// Database validation. Normalize the driver in place so downstream
	// case-sensitive comparisons (and the postgres-only branch below) see
	// a canonical value.
	cfg.Database.Driver = strings.ToLower(cfg.Database.Driver)
	validDrivers := map[string]bool{"sqlite": true, "postgres": true}
	if !validDrivers[cfg.Database.Driver] {
		errs = append(errs, "database.driver must be one of: sqlite, postgres")
	}
	if cfg.Database.Driver == "postgres" {
		if cfg.Database.Port <= 0 || cfg.Database.Port > 65535 {
			errs = append(errs, "database.port must be between 1 and 65535")
		}
		if cfg.Database.User == "" {
			errs = append(errs, "database.user is required for postgres driver")
		}
		if cfg.Database.DBName == "" {
			errs = append(errs, "database.dbName is required for postgres driver")
		}
		validSSLModes := map[string]bool{
			"disable": true, "require": true, "verify-ca": true, "verify-full": true,
		}
		if !validSSLModes[strings.ToLower(cfg.Database.SSLMode)] {
			errs = append(errs, "database.sslMode must be one of: disable, require, verify-ca, verify-full")
		}
	}

	// NATS validation - optional (uses in-memory event bus if not set)
	// No validation needed - empty URL means use in-memory

	// Docker validation - optional (agent features disabled if not available)
	// No validation needed - will gracefully degrade

	// Auth validation - generate random secret if not set (dev mode)
	if cfg.Auth.JWTSecret == "" {
		cfg.Auth.JWTSecret = generateDevSecret()
	}
	if cfg.Auth.TokenDuration <= 0 {
		errs = append(errs, "auth.tokenDuration must be positive")
	}

	// Logging validation
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[strings.ToLower(cfg.Logging.Level)] {
		errs = append(errs, "logging.level must be one of: debug, info, warn, error")
	}
	validFormats := map[string]bool{"json": true, "text": true}
	if !validFormats[strings.ToLower(cfg.Logging.Format)] {
		errs = append(errs, "logging.format must be one of: json, text")
	}

	if cfg.RepositoryDiscovery.MaxDepth <= 0 {
		errs = append(errs, "repositoryDiscovery.maxDepth must be positive")
	}

	if err := cfg.GitHubApp.validate(); err != nil {
		errs = append(errs, err.Error())
	}
	if err := cfg.GitHubCredentialBroker.validate(); err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}

	return nil
}

// DSN returns the PostgreSQL connection string.
func (d *DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode,
	)
}

// generateDevSecret generates a random secret for development mode.
func generateDevSecret() string {
	// Use a fixed dev secret with a warning prefix
	// In production, users should set KANDEV_AUTH_JWTSECRET
	return "dev-secret-change-in-production-" + fmt.Sprintf("%d", time.Now().UnixNano())
}
