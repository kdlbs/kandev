package nomad

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	keyNamespace      = "namespace"
	keySSHFingerprint = "ssh_host_fingerprint"
	keyWorkspaceRoot  = "workspace_root"
	keyRegion         = "region"
	keyAddress        = "address"
	keyCPUMHz         = "cpu_mhz"
	keyMemoryMB       = "memory_mb"
	keyDatacenter     = "datacenter"
	keyPlatform       = "platform"
	keyAllocationRoot = "allocation_root"
	keyCAFile         = "ca_file"
	keyMemoryMaxMB    = "memory_max_mb"
	// WorkerPlatform is the only supported worker platform.
	WorkerPlatform = "darwin/arm64"
)

var profileKeys = map[string]struct{}{
	keyAddress: {}, keyNamespace: {}, keyRegion: {}, keyDatacenter: {}, "node_id": {},
	keyPlatform: {}, "ssh_target": {}, "ssh_port": {}, keySSHFingerprint: {},
	keyWorkspaceRoot: {}, keyAllocationRoot: {}, keyCPUMHz: {}, keyMemoryMB: {},
	"nomad_token": {}, "disconnect_lost_after": {},
	"node_pool": {}, "placement": {}, "pin_node_id": {}, "exclude_local": {}, "prefer_local": {},
	"ssh_user": {}, keyMemoryMaxMB: {}, keyCAFile: {},
}

// Profile modes. A profile naming node_id or ssh_target is a legacy pinned
// worker; every other profile places onto a node pool.
const (
	ModeLegacyPinned = "legacy_pinned"
	ModePool         = "pool"
)

// Pool placement algorithms and defaults.
const (
	PlacementSpread  = "spread"
	PlacementBinpack = "binpack"

	DefaultNodePool      = "kandev"
	DefaultPoolCPUMHz    = 100
	DefaultPoolMemoryMB  = 256
	DefaultPoolMemoryMax = 2048
)

var poolOnlyKeys = []string{"node_pool", "placement", "pin_node_id", "exclude_local", "prefer_local", "ssh_user", keyMemoryMaxMB}

var poolRejectedKeys = []string{"ssh_target", "ssh_port", keySSHFingerprint, keyWorkspaceRoot, keyAllocationRoot}

// Bounds and default for how long Nomad waits for a disconnected client before
// marking the single original allocation lost.
const (
	DefaultDisconnectLostAfter = 24 * time.Hour
	MinDisconnectLostAfter     = time.Minute
	MaxDisconnectLostAfter     = 7 * 24 * time.Hour
)

type ProfileConfig struct {
	Address            string
	Namespace          string
	Region             string
	Datacenter         string
	NodeID             string
	Platform           string
	SSHUser            string
	SSHHost            string
	SSHPort            int
	SSHHostFingerprint string
	WorkspaceRoot      string
	AllocationRoot     string
	CPUMHz             int
	MemoryMB           int
	NomadTokenSecretID string
	// CAFile is an optional PEM CA bundle that signs the Nomad API certificate.
	CAFile string
	// DisconnectLostAfter is the bounded disconnect window of the native job.
	DisconnectLostAfter time.Duration
	// Mode is ModeLegacyPinned or ModePool.
	Mode string
	// Pool placement fields; empty for legacy profiles.
	NodePool     string
	Placement    string
	PinNodeID    string
	ExcludeLocal bool
	PreferLocal  bool
	MemoryMaxMB  int
}

var environmentPathIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

// EnvironmentWorkspacePath derives the immutable workspace directory for one
// task environment generation under an operator-configured profile root.
// Reusing the profile root directly would let separate task environments
// share a checkout and runtime files.
func EnvironmentWorkspacePath(profileRoot, environmentID string, generation int64) (string, error) {
	if profileRoot == "" || !filepath.IsAbs(profileRoot) || filepath.Clean(profileRoot) != profileRoot {
		return "", fmt.Errorf("workspace root must be an absolute normalized profile path")
	}
	if !environmentPathIDPattern.MatchString(environmentID) || generation < 1 {
		return "", fmt.Errorf("workspace path requires a safe environment ID and positive generation")
	}
	return filepath.Join(profileRoot, "environments", environmentID, fmt.Sprintf("g%d", generation)), nil
}

func ParseProfileConfig(config map[string]string) (ProfileConfig, error) {
	for key := range config {
		if _, ok := profileKeys[key]; !ok {
			return ProfileConfig{}, fmt.Errorf("unsupported Nomad profile field %q", key)
		}
	}
	_, hasNodeID := config["node_id"]
	_, hasSSHTarget := config["ssh_target"]
	if !hasNodeID && !hasSSHTarget {
		return parsePoolProfile(config)
	}
	for _, key := range poolOnlyKeys {
		if _, ok := config[key]; ok {
			return ProfileConfig{}, fmt.Errorf("nomad profile field %q is only valid for pool profiles", key)
		}
	}
	return parseLegacyProfile(config)
}

func parseLegacyProfile(config map[string]string) (ProfileConfig, error) {
	get := func(key string) (string, error) {
		value := strings.TrimSpace(config[key])
		if value == "" {
			return "", fmt.Errorf("nomad profile field %q is required", key)
		}
		return value, nil
	}
	var profile ProfileConfig
	var err error
	if profile.Address, err = get(keyAddress); err != nil {
		return profile, err
	}
	if err := validateAddress(profile.Address); err != nil {
		return profile, err
	}
	for key, target := range map[string]*string{
		keyNamespace: &profile.Namespace, keyRegion: &profile.Region, keyDatacenter: &profile.Datacenter,
		"node_id": &profile.NodeID, keyPlatform: &profile.Platform,
		keySSHFingerprint: &profile.SSHHostFingerprint,
		keyWorkspaceRoot:  &profile.WorkspaceRoot, keyAllocationRoot: &profile.AllocationRoot,
	} {
		if *target, err = get(key); err != nil {
			return profile, err
		}
	}
	if profile.Platform != WorkerPlatform {
		return profile, fmt.Errorf("unsupported Nomad worker platform %q", profile.Platform)
	}
	if err := parseProfileSSHAndSizing(&profile, config); err != nil {
		return profile, err
	}
	if err := normalizeProfilePaths(&profile); err != nil {
		return profile, err
	}
	if profile.DisconnectLostAfter, err = parseDisconnectLostAfter(config["disconnect_lost_after"]); err != nil {
		return profile, err
	}
	tokenRef, ok := models.ExecutorProfileSecretID(config["nomad_token"])
	if !ok || strings.TrimSpace(tokenRef) == "" {
		return profile, fmt.Errorf("nomad_token must be a Kandev secret reference")
	}
	profile.NomadTokenSecretID = tokenRef
	profile.Mode = ModeLegacyPinned
	return profile, parseCAFile(&profile, config)
}

func parseProfileSSHAndSizing(profile *ProfileConfig, config map[string]string) error {
	var err error
	if profile.SSHUser, profile.SSHHost, err = parseSSHTarget(config["ssh_target"]); err != nil {
		return err
	}
	if profile.SSHPort, err = positiveInt(config["ssh_port"]); err != nil {
		return fmt.Errorf("ssh_port: %w", err)
	}
	if profile.CPUMHz, err = positiveInt(config[keyCPUMHz]); err != nil {
		return fmt.Errorf("cpu_mhz: %w", err)
	}
	if profile.MemoryMB, err = positiveInt(config[keyMemoryMB]); err != nil {
		return fmt.Errorf("memory_mb: %w", err)
	}
	return nil
}

func normalizeProfilePaths(profile *ProfileConfig) error {
	var err error
	if !strings.HasPrefix(profile.SSHHostFingerprint, "SHA256:") || len(profile.SSHHostFingerprint) < 20 {
		return fmt.Errorf("ssh_host_fingerprint must be a pinned SHA256 fingerprint")
	}
	if profile.WorkspaceRoot, err = cleanAbsolutePath(profile.WorkspaceRoot); err != nil {
		return fmt.Errorf("workspace_root: %w", err)
	}
	if profile.AllocationRoot, err = cleanAbsolutePath(profile.AllocationRoot); err != nil {
		return fmt.Errorf("allocation_root: %w", err)
	}
	return nil
}

// parseCAFile reads the optional ca_file; it is valid only with an https address.
func parseCAFile(profile *ProfileConfig, config map[string]string) error {
	raw := strings.TrimSpace(config[keyCAFile])
	if raw == "" {
		return nil
	}
	path, err := cleanAbsolutePath(raw)
	if err != nil {
		return fmt.Errorf("ca_file: %w", err)
	}
	if !strings.HasPrefix(profile.Address, "https://") {
		return fmt.Errorf("ca_file requires an https nomad address")
	}
	profile.CAFile = path
	return nil
}

func validateAddress(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("nomad address must be an absolute HTTP(S) URL without credentials or fragment")
	}
	if parsed.Scheme == schemeHTTPS {
		return nil
	}
	if parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname()) {
		return nil
	}
	return fmt.Errorf("nomad API must use HTTPS except for loopback development")
}

func parseSSHTarget(raw string) (string, string, error) {
	value := strings.TrimSpace(raw)
	user, host, ok := strings.Cut(value, "@")
	if !ok || user == "" || host == "" || strings.Contains(host, "@") {
		return "", "", fmt.Errorf("ssh_target must be user@host")
	}
	for _, r := range user + host {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return "", "", fmt.Errorf("ssh_target contains invalid characters")
		}
	}
	return user, host, nil
}

func parseDisconnectLostAfter(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultDisconnectLostAfter, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value < MinDisconnectLostAfter || value > MaxDisconnectLostAfter {
		return 0, fmt.Errorf("disconnect_lost_after must be a duration between %s and %s", MinDisconnectLostAfter, MaxDisconnectLostAfter)
	}
	return value, nil
}

func positiveInt(raw string) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("must be a positive integer")
	}
	return value, nil
}

func cleanAbsolutePath(raw string) (string, error) {
	cleaned := filepath.Clean(raw)
	if !filepath.IsAbs(raw) || cleaned != raw {
		return "", fmt.Errorf("must be a normalized absolute path")
	}
	return cleaned, nil
}

func parsePoolProfile(config map[string]string) (ProfileConfig, error) {
	for _, key := range poolRejectedKeys {
		if _, ok := config[key]; ok {
			return ProfileConfig{}, fmt.Errorf("nomad pool profile field %q is not allowed: host, port and host key now come from node metadata", key)
		}
	}
	profile := ProfileConfig{Mode: ModePool}
	var err error
	if err = parsePoolRequired(&profile, config); err != nil {
		return profile, err
	}
	if err = parsePoolPlacement(&profile, config); err != nil {
		return profile, err
	}
	if err = parsePoolSizing(&profile, config); err != nil {
		return profile, err
	}
	if profile.DisconnectLostAfter, err = parseDisconnectLostAfter(config["disconnect_lost_after"]); err != nil {
		return profile, err
	}
	tokenRef, ok := models.ExecutorProfileSecretID(config["nomad_token"])
	if !ok || strings.TrimSpace(tokenRef) == "" {
		return profile, fmt.Errorf("nomad_token must be a Kandev secret reference")
	}
	profile.NomadTokenSecretID = tokenRef
	return profile, parseCAFile(&profile, config)
}

func parsePoolRequired(profile *ProfileConfig, config map[string]string) error {
	for key, target := range map[string]*string{
		keyAddress: &profile.Address, keyNamespace: &profile.Namespace, keyRegion: &profile.Region,
		keyDatacenter: &profile.Datacenter, keyPlatform: &profile.Platform,
	} {
		value := strings.TrimSpace(config[key])
		if value == "" {
			return fmt.Errorf("nomad profile field %q is required", key)
		}
		*target = value
	}
	if err := validateAddress(profile.Address); err != nil {
		return err
	}
	if profile.Platform != WorkerPlatform {
		return fmt.Errorf("unsupported Nomad worker platform %q", profile.Platform)
	}
	return nil
}

func parsePoolPlacement(profile *ProfileConfig, config map[string]string) error {
	var err error
	profile.NodePool = strings.TrimSpace(config["node_pool"])
	if profile.NodePool == "" {
		profile.NodePool = DefaultNodePool
	}
	if !safeNodeID.MatchString(profile.NodePool) {
		return fmt.Errorf("node_pool must be a safe Nomad node pool name")
	}
	switch profile.Placement = strings.TrimSpace(config["placement"]); profile.Placement {
	case "":
		profile.Placement = PlacementSpread
	case PlacementSpread, PlacementBinpack:
	default:
		return fmt.Errorf("placement must be %q or %q", PlacementSpread, PlacementBinpack)
	}
	if profile.PinNodeID = strings.TrimSpace(config["pin_node_id"]); profile.PinNodeID != "" && !safeNodeID.MatchString(profile.PinNodeID) {
		return fmt.Errorf("pin_node_id must be a safe node ID")
	}
	if profile.ExcludeLocal, err = parseOptionalBool(config["exclude_local"]); err != nil {
		return fmt.Errorf("exclude_local: %w", err)
	}
	if profile.PreferLocal, err = parseOptionalBool(config["prefer_local"]); err != nil {
		return fmt.Errorf("prefer_local: %w", err)
	}
	if profile.ExcludeLocal && profile.PreferLocal {
		return fmt.Errorf("exclude_local and prefer_local are mutually exclusive")
	}
	if profile.SSHUser = strings.TrimSpace(config["ssh_user"]); strings.IndexFunc(profile.SSHUser, func(r rune) bool {
		return unicode.IsControl(r) || unicode.IsSpace(r) || r == '@'
	}) >= 0 {
		return fmt.Errorf("ssh_user contains invalid characters")
	}
	return nil
}

func parsePoolSizing(profile *ProfileConfig, config map[string]string) error {
	var err error
	for key, spec := range map[string]struct {
		target *int
		def    int
	}{
		keyCPUMHz: {&profile.CPUMHz, DefaultPoolCPUMHz}, keyMemoryMB: {&profile.MemoryMB, DefaultPoolMemoryMB},
		keyMemoryMaxMB: {&profile.MemoryMaxMB, DefaultPoolMemoryMax},
	} {
		if strings.TrimSpace(config[key]) == "" {
			*spec.target = spec.def
			continue
		}
		if *spec.target, err = positiveInt(config[key]); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}
	if profile.MemoryMaxMB < profile.MemoryMB {
		return fmt.Errorf("memory_max_mb must be at least memory_mb")
	}
	return nil
}

func parseOptionalBool(raw string) (bool, error) {
	switch strings.TrimSpace(raw) {
	case "", "false":
		return false, nil
	case "true":
		return true, nil
	}
	return false, fmt.Errorf("must be true or false")
}
