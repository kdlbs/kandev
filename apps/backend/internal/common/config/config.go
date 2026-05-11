// Package config provides configuration management for Kandev.
// It supports loading configuration from environment variables, config files, and defaults.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/ports"
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
	HomeDir             string                    `mapstructure:"homeDir"`
	Server              ServerConfig              `mapstructure:"server"`
	Database            DatabaseConfig            `mapstructure:"database"`
	NATS                NATSConfig                `mapstructure:"nats"`
	Events              EventsConfig              `mapstructure:"events"`
	Docker              DockerConfig              `mapstructure:"docker"`
	Agent               AgentConfig               `mapstructure:"agent"`
	Auth                AuthConfig                `mapstructure:"auth"`
	Logging             LoggingConfig             `mapstructure:"logging"`
	RepositoryDiscovery RepositoryDiscoveryConfig `mapstructure:"repositoryDiscovery"`
	Worktree            WorktreeConfig            `mapstructure:"worktree"`
	RepoClone           RepoCloneConfig           `mapstructure:"repoClone"`
	Debug               DebugConfig               `mapstructure:"debug"`
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

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Host           string `mapstructure:"host"`
	Port           int    `mapstructure:"port"`
	ReadTimeout    int    `mapstructure:"readTimeout"`  // in seconds
	WriteTimeout   int    `mapstructure:"writeTimeout"` // in seconds
	WebInternalURL string `mapstructure:"webInternalUrl"`
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
	// PprofEnabled enables pprof endpoints at /debug/pprof/ and /api/v1/debug/memory.
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
	// Server defaults
	v.SetDefault("server.host", "0.0.0.0")
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

	// Set defaults first
	setDefaults(v)

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
	_ = v.BindEnv("debug.pprofEnabled", "KANDEV_DEBUG_PPROF_ENABLED")

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
