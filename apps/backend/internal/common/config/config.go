// Package config provides configuration management for Kandev.
// It supports loading configuration from environment variables, config files, and defaults.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration sections for Kandev.
type Config struct {
	Server              ServerConfig              `mapstructure:"server"`
	Database            DatabaseConfig            `mapstructure:"database"`
	NATS                NATSConfig                `mapstructure:"nats"`
	Docker              DockerConfig              `mapstructure:"docker"`
	Agent               AgentConfig               `mapstructure:"agent"`
	Auth                AuthConfig                `mapstructure:"auth"`
	Logging             LoggingConfig             `mapstructure:"logging"`
	RepositoryDiscovery RepositoryDiscoveryConfig `mapstructure:"repositoryDiscovery"`
	Worktree            WorktreeConfig            `mapstructure:"worktree"`
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Host         string `mapstructure:"host"`
	Port         int    `mapstructure:"port"`
	ReadTimeout  int    `mapstructure:"readTimeout"`  // in seconds
	WriteTimeout int    `mapstructure:"writeTimeout"` // in seconds
}

// DatabaseConfig holds database connection configuration.
type DatabaseConfig struct {
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

// DockerConfig holds Docker client configuration.
type DockerConfig struct {
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
}

// RepositoryDiscoveryConfig holds configuration for local repository scanning.
type RepositoryDiscoveryConfig struct {
	Roots    []string `mapstructure:"roots"`
	MaxDepth int      `mapstructure:"maxDepth"`
}

// WorktreeConfig holds Git worktree configuration for concurrent agent execution.
type WorktreeConfig struct {
	Enabled         bool   `mapstructure:"enabled"`         // Enable worktree mode
	BasePath        string `mapstructure:"basePath"`        // Base directory for worktrees (default: ~/.kandev/worktrees)
	MaxPerRepo      int    `mapstructure:"maxPerRepo"`      // Max worktrees per repository (default: 10)
	DefaultBranch   string `mapstructure:"defaultBranch"`   // Default base branch (default: main)
	CleanupOnRemove bool   `mapstructure:"cleanupOnRemove"` // Remove worktree directory on task deletion
}

// AgentConfig holds agent runtime configuration.
type AgentConfig struct {
	// Runtime specifies the agent runtime mode: "docker" or "standalone"
	// - "standalone": Agents run via standalone agentctl on the host machine (default)
	// - "docker": Agents run in Docker containers
	Runtime string `mapstructure:"runtime"`

	// StandaloneHost is the host where standalone agentctl is running (default: localhost)
	StandaloneHost string `mapstructure:"standaloneHost"`

	// StandalonePort is the control port for standalone agentctl (default: 9999)
	StandalonePort int `mapstructure:"standalonePort"`
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

// setDefaults configures default values for all configuration options.
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.readTimeout", 30)
	v.SetDefault("server.writeTimeout", 30)

	// Database defaults
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

	// Docker defaults
	v.SetDefault("docker.host", "unix:///var/run/docker.sock")
	v.SetDefault("docker.apiVersion", "1.41")
	v.SetDefault("docker.tlsVerify", false)
	v.SetDefault("docker.defaultNetwork", "kandev-network")
	v.SetDefault("docker.volumeBasePath", "/var/lib/kandev/volumes")

	// Agent defaults
	v.SetDefault("agent.runtime", "standalone")
	v.SetDefault("agent.standaloneHost", "localhost")
	v.SetDefault("agent.standalonePort", 9999)

	// Auth defaults
	v.SetDefault("auth.jwtSecret", "")
	v.SetDefault("auth.tokenDuration", 3600) // 1 hour

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.outputPath", "stdout")

	// Repository discovery defaults
	v.SetDefault("repositoryDiscovery.roots", []string{})
	v.SetDefault("repositoryDiscovery.maxDepth", 5)

	// Worktree defaults
	v.SetDefault("worktree.enabled", true)
	v.SetDefault("worktree.basePath", "~/.kandev/worktrees")
	v.SetDefault("worktree.maxPerRepo", 10)
	v.SetDefault("worktree.defaultBranch", "main")
	v.SetDefault("worktree.cleanupOnRemove", true)
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
	_ = v.BindEnv("agent.standalonePort", "KANDEV_AGENT_STANDALONE_PORT")
	_ = v.BindEnv("agent.standaloneHost", "KANDEV_AGENT_STANDALONE_HOST")

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

	// Database validation - only if host is set (optional for in-memory mode)
	if cfg.Database.Host != "" {
		if cfg.Database.Port <= 0 || cfg.Database.Port > 65535 {
			errs = append(errs, "database.port must be between 1 and 65535")
		}
		if cfg.Database.User == "" {
			errs = append(errs, "database.user is required when database.host is set")
		}
		if cfg.Database.DBName == "" {
			errs = append(errs, "database.dbName is required when database.host is set")
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
