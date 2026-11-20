package agents

import (
	"path/filepath"
	"regexp"
	"runtime"
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

// Lines that start with an outcome or version marker must match it exactly;
// any other text is informational and ignored.
func (s *ompCheckScan) line(line string) {
	switch {
	case line == "":
	case strings.HasPrefix(line, "Current version:"):
		s.current = s.record(s.current, ompMarkerValue(ompCurrentVersion, line), ompValidVersion)
	case strings.HasPrefix(line, "Current channel:"):
		s.channel = s.record(s.channel, ompMarkerValue(ompCurrentChannel, line), ompChannelName.MatchString)
	case strings.HasPrefix(line, "New version available:"):
		s.available++
		s.latest = s.record(s.latest, ompMarkerValue(ompNewVersion, line), ompValidVersion)
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

// ompMarkerValue returns the marker's captured value, or "" (invalid for every
// validator) when a reserved line does not match its exact syntax.
func ompMarkerValue(marker *regexp.Regexp, line string) string {
	if match := marker.FindStringSubmatch(line); match != nil {
		return match[1]
	}
	return ""
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
			// Install-method classification (update-cli.ts getMiseDataDir and
			// resolveUpdateTarget) chooses npm metadata or the build service;
			// PI_BUILD_URL is the build service origin (build-service.ts).
			"MISE_DATA_DIR", envXDGDataHome, "LOCALAPPDATA", "BUN_INSTALL_GLOBAL_DIR", "PI_BUILD_URL",
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
		filepath.Join(r.WorkingDir, ".env"), filepath.Join(r.HomeDir, ".env"),
		filepath.Join(root, ".env"), filepath.Join(base, ".env"), filepath.Join(agentDir, ".env"),
		filepath.Join(r.HomeDir, ".bunfig.toml"),
	}
	files = append(files, ompConfigOverlays(r)...)
	if r.Executable != "" {
		files = append(files, filepath.Join(filepath.Dir(filepath.Dir(r.Executable)), "manager.json"))
	}
	if r.Launcher != "" {
		// Upstream bunShimMarkerPath: bun's `<name>.bunx` sidecar marks a
		// bun-owned launcher.
		name := strings.TrimSuffix(filepath.Base(r.Launcher), filepath.Ext(r.Launcher))
		files = append(files, filepath.Join(filepath.Dir(r.Launcher), name+".bunx"))
	}
	if userconfig, ok := r.GetenvFold("npm_config_userconfig"); ok && userconfig != "" {
		// OMP reads this path with fs.readFile, so the filesystem resolves
		// symlinks before "..": anchor it without lexical cleanup.
		files = append(files, resolvePathAgainst(r.WorkingDir, userconfig, runtime.GOOS == osWindows))
	} else {
		files = append(files, filepath.Join(r.HomeDir, ".npmrc"))
	}
	if xdg, ok := r.GetenvFold(envXDGConfigHome); ok && xdg != "" {
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

// ompConfigOverlays mirrors upstream: split PI_CONFIG_FILES on the path-list
// separator, drop empty entries, expandTilde each untouched entry, and resolve
// it against the working directory.
func ompConfigOverlays(r NativeUpdateCheckResolver) []string {
	list, _ := r.Getenv("PI_CONFIG_FILES")
	var files []string
	for _, entry := range filepath.SplitList(list) {
		if entry == "" {
			continue
		}
		files = append(files, r.absolute(ompExpandTilde(entry, r.HomeDir)))
	}
	return files
}

// ompExpandTilde mirrors upstream utils/path.ts expandTilde, including a bare
// "~name" resolving to "<home>/name".
func ompExpandTilde(path, home string) string {
	switch {
	case path == "~":
		return home
	case strings.HasPrefix(path, "~/"), strings.HasPrefix(path, `~\`):
		return home + path[1:]
	case strings.HasPrefix(path, "~"):
		return filepath.Join(home, path[1:])
	}
	return path
}

func (r NativeUpdateCheckResolver) absolute(path string) string {
	resolved := resolvePathAgainst(r.WorkingDir, path, runtime.GOOS == osWindows)
	return filepath.Clean(resolved)
}

// resolvePathAgainst mirrors Node's path.resolve(cwd, path). On Windows a
// root-relative path (`\dir`) keeps the working directory's drive, and a
// drive-relative path (`E:dir`) resolves against that drive's root, or
// against the working directory when it is on the same drive. Pure string
// logic so the Windows rules are tested on every platform.
func resolvePathAgainst(cwd, path string, windows bool) string {
	if windows {
		return resolveWindowsPath(cwd, path)
	}
	if strings.HasPrefix(path, "/") {
		return path
	}
	return strings.TrimSuffix(cwd, "/") + "/" + path
}

func resolveWindowsPath(cwd, path string) string {
	vol := windowsVolume(path)
	switch {
	case len(path) >= 2 && windowsSeparator(path[0]) && windowsSeparator(path[1]):
		return path // UNC
	case vol != "" && len(path) > 2 && windowsSeparator(path[2]):
		return path
	case vol == "" && len(path) > 0 && windowsSeparator(path[0]):
		return windowsRootVolume(cwd) + path
	case vol != "" && strings.EqualFold(vol, windowsVolume(cwd)):
		return windowsJoin(cwd, path[2:])
	case vol != "":
		return windowsJoin(vol, path[2:])
	}
	return windowsJoin(cwd, path)
}

func windowsSeparator(b byte) bool { return b == '\\' || b == '/' }

func windowsVolume(p string) string {
	if len(p) >= 2 && p[1] == ':' && 'a' <= p[0]|0x20 && p[0]|0x20 <= 'z' {
		return p[:2]
	}
	return ""
}

// windowsRootVolume returns the drive (`D:`) or UNC share (`\\server\share`)
// that a root-relative path keeps from the working directory.
func windowsRootVolume(p string) string {
	if vol := windowsVolume(p); vol != "" {
		return vol
	}
	if len(p) < 2 || !windowsSeparator(p[0]) || !windowsSeparator(p[1]) {
		return ""
	}
	parts := strings.FieldsFunc(p[2:], func(r rune) bool { return r == '\\' || r == '/' })
	if len(parts) < 2 {
		return ""
	}
	return p[:2] + parts[0] + `\` + parts[1]
}

func windowsJoin(dir, rest string) string {
	return strings.TrimRight(dir, `\/`) + `\` + rest
}

func ompDotenvName(name string) bool {
	return name == ".env" || strings.HasPrefix(name, ".env.")
}
