package agents

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// The OMP adapter and context mirror upstream oh-my-pi at commit 2b562c5b7e3f:
// packages/coding-agent/src/cli/update-cli.ts (output markers),
// packages/utils/src/dirs.ts and env.ts (profile and dotenv locations),
// packages/coding-agent/src/config/settings.ts (settings files),
// packages/coding-agent/src/cli/managed-install.ts (ownership manifest), and
// packages/coding-agent/src/cli/npm-registry.ts (release feed). Re-check these
// sources when upstream changes its update command.

var (
	ompANSIEscape      = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)
	ompCurrentVersion  = regexp.MustCompile(`^Current version: (\S+)$`)
	ompCurrentChannel  = regexp.MustCompile(`^Current channel: (.+)$`)
	ompNewVersion      = regexp.MustCompile(`^New version available: (\S+)$`)
	ompAlreadyUpToDate = regexp.MustCompile(`^(?:\S+ )?Already up to date$`)
	ompManagedInstall  = regexp.MustCompile(`^.+ is installed and kept up to date by .+; update it from .+\.$`)
	ompCheckFailed     = regexp.MustCompile(`^(?:Error: )?Failed to check for updates(?::.*)?$`)
	ompChannelName     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	ompProfileName     = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	ompReservedProfile = regexp.MustCompile(`(?i)^(?:CON|PRN|AUX|NUL|COM[0-9]|LPT[0-9])(?:\..*)?$`)
)

type ompCheckScan struct {
	current, channel, latest     string
	invalid, conflict, transient bool
	available, upToDate          int
	managed, failed              int
}

// ParseOmpUpdateCheck adapts `omp update --check` output to one normalized
// result. Exit status never upgrades a result, and anything ambiguous is
// unknown rather than up to date.
func ParseOmpUpdateCheck(output NativeUpdateCheckOutput) NativeUpdateCheckResult {
	scan := scanOmpCheckOutput(output.Stdout + "\n" + output.Stderr)
	unknown := func(reason NativeUpdateCheckReason) NativeUpdateCheckResult {
		return NativeUpdateCheckResult{State: NativeUpdateCheckStateUnknown, Reason: reason, CurrentVersion: scan.current, Channel: scan.channel}
	}
	if output.ExitCode != 0 {
		return unknown(NativeUpdateCheckReasonCheckFailed)
	}
	if scan.invalid || scan.conflict || scan.transient || scan.outcomes() != 1 {
		return NativeUpdateCheckResult{State: NativeUpdateCheckStateUnknown, Reason: NativeUpdateCheckReasonUnrecognizedOutput}
	}
	switch {
	case scan.failed == 1:
		return unknown(NativeUpdateCheckReasonCheckFailed)
	case scan.managed == 1:
		return unknown(NativeUpdateCheckReasonUnsupported)
	case scan.current == "":
		return NativeUpdateCheckResult{State: NativeUpdateCheckStateUnknown, Reason: NativeUpdateCheckReasonUnrecognizedOutput}
	case scan.available == 1:
		return NativeUpdateCheckResult{State: NativeUpdateCheckStateUpdateAvailable, CurrentVersion: scan.current, LatestVersion: scan.latest, Channel: scan.channel}
	default:
		return NativeUpdateCheckResult{State: NativeUpdateCheckStateUpToDate, CurrentVersion: scan.current, Channel: scan.channel}
	}
}

func (s ompCheckScan) outcomes() int { return s.available + s.upToDate + s.managed + s.failed }

func scanOmpCheckOutput(text string) ompCheckScan {
	var scan ompCheckScan
	for _, raw := range strings.Split(ompANSIEscape.ReplaceAllString(text, ""), "\n") {
		scan.line(strings.TrimSpace(raw))
	}
	if scan.invalid || scan.conflict {
		scan.current, scan.channel = "", ""
	}
	return scan
}

func (s *ompCheckScan) line(line string) {
	switch {
	case line == "":
	case ompCurrentVersion.MatchString(line):
		s.current = s.record(s.current, ompCurrentVersion.FindStringSubmatch(line)[1], ompValidVersion)
	case ompCurrentChannel.MatchString(line):
		s.channel = s.record(s.channel, ompCurrentChannel.FindStringSubmatch(line)[1], ompChannelName.MatchString)
	case ompNewVersion.MatchString(line):
		s.available++
		s.latest = s.record(s.latest, ompNewVersion.FindStringSubmatch(line)[1], ompValidVersion)
	case ompAlreadyUpToDate.MatchString(line):
		s.upToDate++
	case ompManagedInstall.MatchString(line):
		s.managed++
	case ompCheckFailed.MatchString(line):
		s.failed++
	case strings.HasPrefix(line, "Switching to "), strings.HasPrefix(line, "Forcing reinstall"):
		s.transient = true
	}
}

// record keeps the first valid value and flags malformed or conflicting ones.
func (s *ompCheckScan) record(previous, value string, valid func(string) bool) string {
	if !valid(value) {
		s.invalid = true
		return previous
	}
	if previous != "" && previous != value {
		s.conflict = true
	}
	if previous != "" {
		return previous
	}
	return value
}

func ompValidVersion(value string) bool {
	version, err := semver.NewVersion(value)
	return err == nil && version.Metadata() == ""
}

func ompUpdateCheckContext() NativeUpdateCheckContext {
	return NativeUpdateCheckContext{
		EnvKeys: []string{
			"OMP_PROFILE", "PI_PROFILE", "PI_CODING_AGENT_DIR", "PI_CONFIG_DIR", "PI_CONFIG_FILES",
			envHome, envXDGConfigHome, "npm_config_userconfig", "npm_config_registry", "BUN_CONFIG_REGISTRY",
		},
		Sources: ompUpdateCheckSources,
	}
}

func ompUpdateCheckSources(r NativeUpdateCheckResolver) NativeUpdateCheckSources {
	dirName, _ := r.Getenv("PI_CONFIG_DIR")
	if dirName == "" {
		dirName = ".omp"
	}
	base := filepath.Join(r.HomeDir, dirName)
	root, agentDir := ompProfileDirs(r, base)
	files := []string{
		filepath.Join(agentDir, "config.yml"), filepath.Join(agentDir, "config.yaml"),
		filepath.Join(r.WorkingDir, ".omp", "config.yml"), filepath.Join(r.WorkingDir, ".omp", "settings.json"),
		filepath.Join(r.WorkingDir, ".claude", "settings.json"), filepath.Join(r.WorkingDir, ".claude", "settings.yml"),
		filepath.Join(r.HomeDir, ".env"), filepath.Join(root, ".env"), filepath.Join(base, ".env"), filepath.Join(agentDir, ".env"),
		filepath.Join(r.HomeDir, ".bunfig.toml"),
	}
	files = append(files, ompConfigOverlays(r)...)
	if r.Executable != "" {
		files = append(files, filepath.Join(filepath.Dir(filepath.Dir(r.Executable)), "manager.json"))
	}
	if userconfig, ok := r.GetenvFold("npm_config_userconfig"); ok && userconfig != "" {
		files = append(files, r.absolute(userconfig))
	} else {
		files = append(files, filepath.Join(r.HomeDir, ".npmrc"))
	}
	if xdg, ok := r.Getenv(envXDGConfigHome); ok && xdg != "" {
		files = append(files, filepath.Join(xdg, ".bunfig.toml"))
	}
	return NativeUpdateCheckSources{
		Files: files,
		Dirs: []NativeUpdateCheckDir{
			{Path: filepath.Join(r.WorkingDir, ".omp")},
			{Path: filepath.Join(r.WorkingDir, ".claude")},
			{Path: r.WorkingDir, Match: ompDotenvName},
		},
	}
}

// ompProfileDirs mirrors upstream getConfigRootDir and getAgentDir, including
// the module-load fallback that treats an invalid profile as the default.
func ompProfileDirs(r NativeUpdateCheckResolver, base string) (root, agentDir string) {
	raw, ompDefined := r.Getenv("OMP_PROFILE")
	if !ompDefined {
		raw, _ = r.Getenv("PI_PROFILE")
	}
	if profile := ompNormalizeProfile(raw); profile != "" {
		root = filepath.Join(base, "profiles", profile)
		return root, filepath.Join(root, "agent")
	}
	root, agentDir = base, filepath.Join(base, "agent")
	override, ok := r.Getenv("PI_CODING_AGENT_DIR")
	if !ok || override == "" {
		return root, agentDir
	}
	piRaw, _ := r.Getenv("PI_PROFILE")
	if piProfile := ompNormalizeProfile(piRaw); piProfile != "" && override == filepath.Join(base, "profiles", piProfile, "agent") {
		return root, agentDir
	}
	return root, r.absolute(override)
}

func ompNormalizeProfile(raw string) string {
	profile := strings.TrimSpace(raw)
	if profile == "" || profile == "default" || profile == "." || profile == ".." || strings.HasSuffix(profile, ".") ||
		!ompProfileName.MatchString(profile) || ompReservedProfile.MatchString(profile) {
		return ""
	}
	return profile
}

func ompConfigOverlays(r NativeUpdateCheckResolver) []string {
	list, _ := r.Getenv("PI_CONFIG_FILES")
	var files []string
	for _, entry := range filepath.SplitList(list) {
		entry = strings.TrimSpace(entry)
		switch {
		case entry == "":
			continue
		case entry == "~":
			entry = r.HomeDir
		case strings.HasPrefix(entry, "~/"), strings.HasPrefix(entry, `~\`):
			entry = filepath.Join(r.HomeDir, entry[2:])
		}
		files = append(files, r.absolute(entry))
	}
	return files
}

func (r NativeUpdateCheckResolver) absolute(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(r.WorkingDir, path)
}

func ompDotenvName(name string) bool {
	return name == ".env" || strings.HasPrefix(name, ".env.")
}
