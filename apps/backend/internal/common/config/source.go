package config

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/profiles"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

// InternalConfigFileEnv pins a managed child process to the configuration
// file selected by its launcher. It is process wiring, not an operator-facing
// configuration variable, and is intentionally excluded from the catalog.
const (
	InternalConfigFileEnv     = "KANDEV_INTERNAL_CONFIG_FILE"
	InternalConfigHomeFileEnv = "KANDEV_INTERNAL_CONFIG_HOME_FILE"
	windowsOS                 = "windows"
)

// ConfigSource records configuration provenance without retaining any decoded
// secret values.
type ConfigSource struct {
	FilePath string
	HomeFile bool
	Values   map[string]SettingSource
	Warnings []string
}

// Value returns the source for key. Unknown keys are treated as built-in
// defaults so callers can safely use the result in source-reporting APIs.
func (s ConfigSource) Value(key string) SettingSource {
	if source, ok := s.Values[key]; ok {
		return source
	}
	return SourceDefault
}

// SourceFor returns the source for a canonical YAML key.
func (c *Config) SourceFor(key string) SettingSource {
	if c == nil {
		return SourceDefault
	}
	return c.Source.Value(key)
}

type configCandidate struct {
	path   string
	isHome bool
}

type configSelection struct {
	configCandidate
}

// yamlOnlyStartupKeys are parsed from config.yaml and resolved from the
// compatible environment aliases below. They intentionally do not use Viper's
// AutomaticEnv during Unmarshal: legacy invalid environment values must retain
// their fallback behavior instead of turning into typed-config startup errors.
var yamlOnlyStartupKeys = map[string]struct{}{
	"server.trustedProxies":              {},
	"tasks.preparationTimeout":           {},
	"credentials.file":                   {},
	"limits.ghMaxConcurrent":             {},
	"limits.gitMaxConcurrent":            {},
	"limits.lspMaxConnections":           {},
	"messageQueue.maxPerSession":         {},
	"agentctl.idleTimeout":               {},
	"agentctl.idleReaperInterval":        {},
	"agentctl.notificationQueueCapacity": {},
	"planning.coalesceWindowMs":          {},
	"office.schedulerTickMs":             {},
	officeMaxConcurrentInstanceKey:       {},
	officeMaxConcurrentWorkspaceKey:      {},
	officeWorkspaceBudgetPerHourKey:      {},
	officeRoutineBudgetPerHourKey:        {},
	officePromotionAgeMinutesKey:         {},
	officeMaxCausationDepthKey:           {},
	officeSelfTriggerAllowanceKey:        {},
	officeGateFailureThresholdKey:        {},
	"observability.otlpEndpoint":         {},
	"launcher.webPort":                   {},
	"launcher.healthTimeoutMs":           {},
	"launcher.noBrowser":                 {},
}

func isYAMLOnlyStartupKey(key string) bool {
	_, ok := yamlOnlyStartupKeys[key]
	return ok
}

func configCandidates(configPath, homeDir string) ([]configCandidate, error) {
	if configPath != "" {
		path := explicitConfigFile(configPath)
		return []configCandidate{{path: path, isHome: isHomeConfigFile(path, homeDir) || os.Getenv(InternalConfigHomeFileEnv) == "1"}}, nil
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolve config working directory: %w", err)
	}
	homeDir = bootstrapHomeDirFor(homeDir)
	return []configCandidate{
		{path: filepath.Join(workingDir, "config.yaml")},
		{path: filepath.Join(homeDir, "config.yaml"), isHome: true},
		{path: "/etc/kandev/config.yaml"},
	}, nil
}

func isHomeConfigFile(path, homeDir string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absHome, err := filepath.Abs(bootstrapHomeDirFor(homeDir))
	if err != nil {
		return false
	}
	return filepath.Clean(absPath) == filepath.Join(filepath.Clean(absHome), "config.yaml")
}

func explicitConfigFile(path string) string {
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		return filepath.Join(path, "config.yaml")
	}
	if filepath.Base(path) == "config.yaml" || strings.HasSuffix(strings.ToLower(path), ".yaml") {
		return path
	}
	return filepath.Join(path, "config.yaml")
}

func bootstrapHomeDir() string {
	if raw := strings.TrimSpace(os.Getenv("KANDEV_HOME_DIR")); raw != "" {
		return expandTilde(raw)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return kandevHomeSubdir
	}
	return filepath.Join(home, kandevHomeSubdir)
}

func bootstrapHomeDirFor(homeDir string) string {
	if raw := strings.TrimSpace(homeDir); raw != "" {
		return expandTilde(raw)
	}
	return bootstrapHomeDir()
}

func selectConfigFile(configPath, homeDir string) (configSelection, bool, error) {
	candidates, err := configCandidates(configPath, homeDir)
	if err != nil {
		return configSelection{}, false, err
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate.path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			// The candidate exists from the operator's point of view, but
			// cannot be inspected. Select it so the read error names it
			// instead of silently falling through to a lower-priority file.
			return configSelection{configCandidate: candidate}, true, nil
		}
		return configSelection{configCandidate: candidate}, true, nil
	}
	return configSelection{}, false, nil
}

func readSelectedConfig(v *viper.Viper, selection configSelection) error {
	v.SetConfigFile(selection.path)
	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("read configuration file %q: %w", selection.path, err)
	}
	if selection.isHome && v.InConfig("homeDir") {
		return fmt.Errorf("configuration file %q cannot set homeDir because a home configuration file cannot relocate itself", selection.path)
	}
	return nil
}

func bindCatalogEnvironment(v *viper.Viper) {
	for _, entry := range startupCatalog {
		if isYAMLOnlyStartupKey(entry.Key) || len(entry.EnvVars) == 0 {
			continue
		}
		args := append([]string{entry.Key}, entry.EnvVars...)
		_ = v.BindEnv(args...)
	}
}

func setProfileDefaults(v *viper.Viper, profileDefaults map[string]string) {
	for _, entry := range startupCatalog {
		if isYAMLOnlyStartupKey(entry.Key) {
			continue
		}
		for _, envVar := range entry.EnvVars {
			if value := profileDefaults[envVar]; value != "" {
				v.SetDefault(entry.Key, value)
				break
			}
		}
	}
}

func environmentSnapshot() map[string]string {
	out := make(map[string]string)
	for _, entry := range startupCatalog {
		for _, envVar := range entry.EnvVars {
			if value, ok := os.LookupEnv(envVar); ok {
				out[envVar] = value
			}
		}
	}
	return out
}

func nonEmptyEnv(snapshot map[string]string, names ...string) (string, bool) {
	for _, name := range names {
		if value, ok := snapshot[name]; ok && strings.TrimSpace(value) != "" {
			return value, true
		}
	}
	return "", false
}

func envValue(snapshot map[string]string, names ...string) (string, bool) {
	for _, name := range names {
		if value, ok := snapshot[name]; ok {
			return value, true
		}
	}
	return "", false
}

func applyStartupDefaultsAndEnvironment(
	cfg *Config,
	yamlKeys map[string]bool,
	profileDefaults map[string]string,
	envSnapshot map[string]string,
) map[string]SettingSource {
	sources := make(map[string]SettingSource)
	for _, entry := range startupCatalog {
		sources[entry.Key] = sourceForEntry(entry, yamlKeys, profileDefaults, envSnapshot)
	}
	applyStartupDefaults(cfg, yamlKeys, profileDefaults, sources)
	applyStartupEnvironment(cfg, envSnapshot, sources)
	return sources
}

func applyStartupDefaults(cfg *Config, yamlKeys map[string]bool, profileDefaults map[string]string, sources map[string]SettingSource) {
	setDefaultDuration := func(key string, target *time.Duration, builtin time.Duration) {
		if yamlKeys[key] {
			return
		}
		*target = builtin
		if entry, ok := CatalogEntryForKey(key); ok {
			if raw, found := nonEmptyEnv(profileDefaults, entry.EnvVars...); found {
				if parsed, err := time.ParseDuration(raw); err == nil {
					*target = parsed
					sources[key] = SourceProfile
				}
			}
		}
	}
	setDefaultInt := func(key string, target *int, builtin int) {
		if yamlKeys[key] {
			return
		}
		*target = builtin
		if entry, ok := CatalogEntryForKey(key); ok {
			if raw, found := nonEmptyEnv(profileDefaults, entry.EnvVars...); found {
				if parsed, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
					*target = parsed
					sources[key] = SourceProfile
				}
			}
		}
	}

	if !yamlKeys["server.trustedProxies"] {
		cfg.Server.TrustedProxies = nil
	}
	setDefaultDuration("tasks.preparationTimeout", &cfg.Tasks.PreparationTimeout, 10*time.Minute)
	if !yamlKeys["credentials.file"] {
		cfg.Credentials.File = ""
	}
	setDefaultInt("limits.ghMaxConcurrent", &cfg.Limits.GHMaxConcurrent, 8)
	setDefaultInt("limits.gitMaxConcurrent", &cfg.Limits.GitMaxConcurrent, 12)
	setDefaultInt("limits.lspMaxConnections", &cfg.Limits.LSPMaxConnections, 8)
	setDefaultInt("messageQueue.maxPerSession", &cfg.MessageQueue.MaxPerSession, 10)
	setDefaultDuration("agentctl.idleTimeout", &cfg.Agentctl.IdleTimeout, time.Hour)
	setDefaultDuration("agentctl.idleReaperInterval", &cfg.Agentctl.IdleReaperInterval, time.Minute)
	setDefaultInt("agentctl.notificationQueueCapacity", &cfg.Agentctl.NotificationQueueCapacity, 131072)
	setDefaultInt("planning.coalesceWindowMs", &cfg.Planning.CoalesceWindowMs, 300000)
	setDefaultInt("office.schedulerTickMs", &cfg.Office.SchedulerTickMs, 5000)
	setDefaultInt("office.maxConcurrentInstance", &cfg.Office.MaxConcurrentInstance, 8)
	setDefaultInt("office.maxConcurrentWorkspace", &cfg.Office.MaxConcurrentWorkspace, 4)
	setDefaultInt("office.workspaceBudgetPerHour", &cfg.Office.WorkspaceBudgetPerHour, 120)
	setDefaultInt("office.routineBudgetPerHour", &cfg.Office.RoutineBudgetPerHour, 20)
	setDefaultInt("office.promotionAgeMinutes", &cfg.Office.PromotionAgeMinutes, 15)
	setDefaultInt("office.maxCausationDepth", &cfg.Office.MaxCausationDepth, 8)
	setDefaultInt("office.selfTriggerAllowance", &cfg.Office.SelfTriggerAllowance, 3)
	setDefaultInt("office.gateFailureThreshold", &cfg.Office.GateFailureThreshold, 3)
	if !yamlKeys["observability.otlpEndpoint"] {
		cfg.Observability.OTLPEndpoint = ""
	}
	setDefaultInt("launcher.webPort", &cfg.Launcher.WebPort, 0)
	setDefaultInt("launcher.healthTimeoutMs", &cfg.Launcher.HealthTimeoutMs, launcherHealthTimeoutDefault())
	if !yamlKeys["launcher.noBrowser"] {
		cfg.Launcher.NoBrowser = false
	}
}

func applyStartupEnvironment(cfg *Config, envSnapshot map[string]string, sources map[string]SettingSource) {
	applyTrustedProxiesEnv(cfg, envSnapshot, sources)
	applyDurationEnv("tasks.preparationTimeout", &cfg.Tasks.PreparationTimeout, 10*time.Minute, envSnapshot, sources)
	applyStringEnvAllowEmpty("credentials.file", &cfg.Credentials.File, envSnapshot, sources)
	applyPositiveIntEnv("limits.ghMaxConcurrent", &cfg.Limits.GHMaxConcurrent, 8, envSnapshot, sources)
	applyPositiveIntEnv("limits.gitMaxConcurrent", &cfg.Limits.GitMaxConcurrent, 12, envSnapshot, sources)
	applyPositiveIntEnv("limits.lspMaxConnections", &cfg.Limits.LSPMaxConnections, 8, envSnapshot, sources)
	applyNonNegativeIntEnv("messageQueue.maxPerSession", &cfg.MessageQueue.MaxPerSession, 10, envSnapshot, sources)
	applyNonNegativeDurationEnv("agentctl.idleTimeout", &cfg.Agentctl.IdleTimeout, time.Hour, envSnapshot, sources)
	applyDurationEnv("agentctl.idleReaperInterval", &cfg.Agentctl.IdleReaperInterval, time.Minute, envSnapshot, sources)
	applyBoundedIntEnv("agentctl.notificationQueueCapacity", &cfg.Agentctl.NotificationQueueCapacity, 131072, 1024, 131072, envSnapshot, sources)
	applyNonNegativeIntEnv("planning.coalesceWindowMs", &cfg.Planning.CoalesceWindowMs, 300000, envSnapshot, sources)
	applyPositiveIntEnv("office.schedulerTickMs", &cfg.Office.SchedulerTickMs, 5000, envSnapshot, sources)
	applyPositiveIntEnv("office.maxConcurrentInstance", &cfg.Office.MaxConcurrentInstance, 8, envSnapshot, sources)
	applyPositiveIntEnv("office.maxConcurrentWorkspace", &cfg.Office.MaxConcurrentWorkspace, 4, envSnapshot, sources)
	applyPositiveIntEnv("office.workspaceBudgetPerHour", &cfg.Office.WorkspaceBudgetPerHour, 120, envSnapshot, sources)
	applyPositiveIntEnv("office.routineBudgetPerHour", &cfg.Office.RoutineBudgetPerHour, 20, envSnapshot, sources)
	applyPositiveIntEnv("office.promotionAgeMinutes", &cfg.Office.PromotionAgeMinutes, 15, envSnapshot, sources)
	applyPositiveIntEnv("office.maxCausationDepth", &cfg.Office.MaxCausationDepth, 8, envSnapshot, sources)
	applyPositiveIntEnv("office.selfTriggerAllowance", &cfg.Office.SelfTriggerAllowance, 3, envSnapshot, sources)
	applyPositiveIntEnv("office.gateFailureThreshold", &cfg.Office.GateFailureThreshold, 3, envSnapshot, sources)
	applyStringEnvAllowEmpty("observability.otlpEndpoint", &cfg.Observability.OTLPEndpoint, envSnapshot, sources)
	applyBoundedIntEnv("launcher.webPort", &cfg.Launcher.WebPort, 0, 1, 65535, envSnapshot, sources)
	applyPositiveIntEnv("launcher.healthTimeoutMs", &cfg.Launcher.HealthTimeoutMs, launcherHealthTimeoutDefault(), envSnapshot, sources)
	applyBoolEnv("launcher.noBrowser", &cfg.Launcher.NoBrowser, false, envSnapshot, sources)
}

// clampOfficeLaunchSafetyConfig clamps a below-minimum office.*
// launch-safety value to its documented default and returns a warning
// naming the key and the rejected value, per
// AC-OFFICE-LAUNCH-SAFETY-001.5/003.1/004.3/005.1 and
// AC-OFFICE-BACKPRESSURE-002.1/003.5: these values must degrade to their
// default and let boot proceed, not fail startup. Defaults here must stay
// in sync with applyStartupDefaults's and applyStartupEnvironment's
// literals for the same keys. Only a YAML-sourced value can still be
// invalid by the point this runs: applyStartupEnvironment's env path and
// applyStartupDefaults's default path already resolve to a value >= 1.
func clampOfficeLaunchSafetyConfig(cfg *Config) []string {
	entries := []struct {
		key   string
		value *int
		def   int
	}{
		{officeMaxConcurrentInstanceKey, &cfg.Office.MaxConcurrentInstance, 8},
		{officeMaxConcurrentWorkspaceKey, &cfg.Office.MaxConcurrentWorkspace, 4},
		{officeWorkspaceBudgetPerHourKey, &cfg.Office.WorkspaceBudgetPerHour, 120},
		{officeRoutineBudgetPerHourKey, &cfg.Office.RoutineBudgetPerHour, 20},
		{officePromotionAgeMinutesKey, &cfg.Office.PromotionAgeMinutes, 15},
		{officeMaxCausationDepthKey, &cfg.Office.MaxCausationDepth, 8},
		{officeSelfTriggerAllowanceKey, &cfg.Office.SelfTriggerAllowance, 3},
		{officeGateFailureThresholdKey, &cfg.Office.GateFailureThreshold, 3},
	}
	var warnings []string
	for _, e := range entries {
		if *e.value >= 1 {
			continue
		}
		warning := fmt.Sprintf("%s is configured as %d, which is below the minimum of 1; using the default %d instead", e.key, *e.value, e.def)
		log.Print(warning)
		warnings = append(warnings, warning)
		*e.value = e.def
	}
	return warnings
}

func launcherHealthTimeoutDefault() int {
	if profiles.DetectEnvironment() == profiles.EnvDev || profiles.DetectEnvironment() == profiles.EnvE2E {
		return 600000
	}
	return 45000
}

func sourceForEntry(entry CatalogEntry, yamlKeys map[string]bool, profileDefaults, envSnapshot map[string]string) SettingSource {
	if _, found := nonEmptyEnv(envSnapshot, entry.EnvVars...); found {
		return SourceEnvironment
	}
	if yamlKeys[entry.Key] {
		return SourceConfiguration
	}
	if _, found := nonEmptyEnv(profileDefaults, entry.EnvVars...); found {
		return SourceProfile
	}
	return SourceDefault
}

func applyTrustedProxiesEnv(cfg *Config, env map[string]string, sources map[string]SettingSource) {
	entry, ok := CatalogEntryForKey("server.trustedProxies")
	if !ok {
		return
	}
	raw, found := nonEmptyEnv(env, entry.EnvVars...)
	if !found {
		return
	}
	parts := strings.Split(raw, ",")
	cfg.Server.TrustedProxies = make([]string, 0, len(parts))
	for _, part := range parts {
		cfg.Server.TrustedProxies = append(cfg.Server.TrustedProxies, strings.TrimSpace(part))
	}
	sources[entry.Key] = SourceEnvironment
}

func applyDurationEnv(key string, target *time.Duration, fallback time.Duration, env map[string]string, sources map[string]SettingSource) {
	entry, ok := CatalogEntryForKey(key)
	if !ok {
		return
	}
	raw, found := nonEmptyEnv(env, entry.EnvVars...)
	if !found {
		return
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || parsed <= 0 {
		*target = fallback
		sources[key] = SourceDefault
		return
	}
	*target = parsed
	sources[key] = SourceEnvironment
}

func applyNonNegativeDurationEnv(key string, target *time.Duration, fallback time.Duration, env map[string]string, sources map[string]SettingSource) {
	entry, ok := CatalogEntryForKey(key)
	if !ok {
		return
	}
	raw, found := nonEmptyEnv(env, entry.EnvVars...)
	if !found {
		return
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || parsed < 0 {
		*target = fallback
		sources[key] = SourceDefault
		return
	}
	*target = parsed
	sources[key] = SourceEnvironment
}

func applyStringEnvAllowEmpty(key string, target *string, env map[string]string, sources map[string]SettingSource) {
	entry, ok := CatalogEntryForKey(key)
	if !ok {
		return
	}
	if raw, found := envValue(env, entry.EnvVars...); found {
		*target = strings.TrimSpace(raw)
		sources[key] = SourceEnvironment
	}
}

func applyPositiveIntEnv(key string, target *int, fallback int, env map[string]string, sources map[string]SettingSource) {
	applyBoundedIntEnv(key, target, fallback, 1, int(^uint(0)>>1), env, sources)
}

func applyNonNegativeIntEnv(key string, target *int, fallback int, env map[string]string, sources map[string]SettingSource) {
	applyBoundedIntEnv(key, target, fallback, 0, int(^uint(0)>>1), env, sources)
}

func applyBoundedIntEnv(key string, target *int, fallback, minimum, maximum int, env map[string]string, sources map[string]SettingSource) {
	entry, ok := CatalogEntryForKey(key)
	if !ok {
		return
	}
	raw, found := nonEmptyEnv(env, entry.EnvVars...)
	if !found {
		return
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || parsed < minimum || parsed > maximum {
		log.Printf("environment override for %s (%q) is out of range [%d,%d]; using default %d", key, raw, minimum, maximum, fallback)
		*target = fallback
		sources[key] = SourceDefault
		return
	}
	*target = parsed
	sources[key] = SourceEnvironment
}

func applyBoolEnv(key string, target *bool, fallback bool, env map[string]string, sources map[string]SettingSource) {
	entry, ok := CatalogEntryForKey(key)
	if !ok {
		return
	}
	raw, found := nonEmptyEnv(env, entry.EnvVars...)
	if !found {
		return
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		*target = fallback
		sources[key] = SourceDefault
		return
	}
	*target = parsed
	sources[key] = SourceEnvironment
}

func configKeys(v *viper.Viper) map[string]bool {
	keys := make(map[string]bool)
	for _, entry := range startupCatalog {
		keys[entry.Key] = v.InConfig(entry.Key)
	}
	return keys
}

func buildConfigSource(selection configSelection, v *viper.Viper, sources map[string]SettingSource, warnings []string) ConfigSource {
	if sources == nil {
		sources = make(map[string]SettingSource)
	}
	for _, entry := range startupCatalog {
		if _, ok := sources[entry.Key]; !ok {
			if v.InConfig(entry.Key) {
				sources[entry.Key] = SourceConfiguration
			} else {
				sources[entry.Key] = SourceDefault
			}
		}
	}
	path := ""
	if selection.path != "" {
		path = selection.path
	}
	return ConfigSource{FilePath: path, HomeFile: selection.isHome, Values: sources, Warnings: warnings}
}

func inspectSecretPermissions(selection configSelection, v *viper.Viper) []string {
	if selection.path == "" || runtime.GOOS == windowsOS {
		return nil
	}
	info, err := os.Stat(selection.path)
	if err != nil || info.Mode().Perm()&0o077 == 0 {
		return nil
	}
	for _, entry := range startupCatalog {
		if entry.Sensitive && v.InConfig(entry.Key) {
			warning := fmt.Sprintf("configuration file %q contains a secret setting readable by group or other users; use mode 0600", selection.path)
			log.Print(warning)
			return []string{warning}
		}
	}
	return nil
}

func decodeConfig(v *viper.Viper, cfg *Config) error {
	return v.Unmarshal(cfg, viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
	)))
}
