package agents

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	envHome          = "HOME"
	envXDGConfigHome = "XDG_CONFIG_HOME"
	envXDGDataHome   = "XDG_DATA_HOME"
)

// NativeUpdateCheckState is the single outcome of one native update check.
// The values match the runtime status check states.
type NativeUpdateCheckState string

const (
	NativeUpdateCheckStateUpdateAvailable NativeUpdateCheckState = "update_available"
	NativeUpdateCheckStateUpToDate        NativeUpdateCheckState = "up_to_date"
	NativeUpdateCheckStateUnknown         NativeUpdateCheckState = "unknown"
)

// NativeUpdateCheckReason explains an unknown native result. The set is closed;
// it is never derived from raw check output.
type NativeUpdateCheckReason string

const (
	NativeUpdateCheckReasonCheckFailed        NativeUpdateCheckReason = "check_failed"
	NativeUpdateCheckReasonUnsupported        NativeUpdateCheckReason = "unsupported"
	NativeUpdateCheckReasonUnrecognizedOutput NativeUpdateCheckReason = "unrecognized_output"
	NativeUpdateCheckReasonTimeout            NativeUpdateCheckReason = "timeout"
	NativeUpdateCheckReasonOutputLimit        NativeUpdateCheckReason = "output_limit"
	NativeUpdateCheckReasonNotInstalled       NativeUpdateCheckReason = "not_installed"
	NativeUpdateCheckReasonRuntimeMismatch    NativeUpdateCheckReason = "runtime_mismatch"
	NativeUpdateCheckReasonCancelled          NativeUpdateCheckReason = "cancelled"
	NativeUpdateCheckReasonUnavailable        NativeUpdateCheckReason = "unavailable"
)

// NativeUpdateCheckOutput is the complete, bounded output of a finished check.
type NativeUpdateCheckOutput struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// NativeUpdateCheckResult is the normalized adapter result. Reason is set only
// when State is unknown.
type NativeUpdateCheckResult struct {
	State          NativeUpdateCheckState
	CurrentVersion string
	LatestVersion  string
	Channel        string
	Reason         NativeUpdateCheckReason
}

// NativeUpdateCheckResolver supplies the inputs a context declaration may use
// to locate configuration. Env is the check process environment ("KEY=value"
// entries); Launcher is the executable path as resolved from PATH; Executable
// is that path with symlinks evaluated. Both are empty when the executable
// could not be resolved.
type NativeUpdateCheckResolver struct {
	Env        []string
	WorkingDir string
	HomeDir    string
	Launcher   string
	Executable string
	// FoldCase makes Getenv case-insensitive, as process environment lookup
	// is on Windows.
	FoldCase bool
}

// NewNativeUpdateCheckResolver derives the home directory from the check
// environment, the way the checker's own runtime does, and evaluates
// executable symlinks so ownership manifests are found beside the real binary.
func NewNativeUpdateCheckResolver(env []string, workingDir, executable string) NativeUpdateCheckResolver {
	resolver := NativeUpdateCheckResolver{Env: env, WorkingDir: workingDir, Launcher: executable, Executable: executable, FoldCase: runtime.GOOS == osWindows}
	homeKey, lookup := envHome, resolver.Getenv
	if runtime.GOOS == osWindows {
		homeKey, lookup = "USERPROFILE", resolver.GetenvFold
	}
	if home, ok := lookup(homeKey); ok && home != "" {
		resolver.HomeDir = home
	} else {
		resolver.HomeDir = accountHomeDir()
	}
	if executable != "" {
		if real, err := filepath.EvalSymlinks(executable); err == nil {
			resolver.Executable = real
		}
	}
	return resolver
}

// accountHomeDir mirrors Node's os.homedir() fallback when the home variable
// is unset or empty: the effective account's home from the user database.
func accountHomeDir() string {
	if account, err := user.Current(); err == nil && account.HomeDir != "" {
		return account.HomeDir
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return ""
}

// Getenv returns the last match for key, matching process semantics: exact
// on POSIX, case-insensitive when FoldCase (Windows). A present empty value
// is found, so it still masks a fallback key.
func (r NativeUpdateCheckResolver) Getenv(key string) (string, bool) {
	return r.lookupEnv(key, r.FoldCase)
}

func (r NativeUpdateCheckResolver) lookupEnv(key string, fold bool) (string, bool) {
	value, found := "", false
	for _, entry := range r.Env {
		name, v, ok := strings.Cut(entry, "=")
		if ok && (name == key || fold && strings.EqualFold(name, key)) {
			value, found = v, true
		}
	}
	return value, found
}

// GetenvFold mirrors OMP's envLookup: a nonempty exact match wins; otherwise
// the first nonempty case-insensitive match. Empty values never match.
func (r NativeUpdateCheckResolver) GetenvFold(key string) (string, bool) {
	if value, ok := r.lookupEnv(key, false); ok && value != "" {
		return value, true
	}
	for _, entry := range r.Env {
		name, value, ok := strings.Cut(entry, "=")
		if ok && value != "" && strings.EqualFold(name, key) {
			return value, true
		}
	}
	return "", false
}

// NativeUpdateCheckDir names a directory whose direct regular files are
// fingerprinted. Match, when set, selects file names; nil selects every file.
type NativeUpdateCheckDir struct {
	Path  string
	Match func(name string) bool
}

// NativeUpdateCheckSources lists configuration files and directories whose
// metadata identifies the update configuration. Contents are never read.
type NativeUpdateCheckSources struct {
	Files []string
	Dirs  []NativeUpdateCheckDir
}

// NativeUpdateCheckContext declares the update configuration used for cache
// identity: environment keys (matched case-insensitively) and a pure source
// resolver.
type NativeUpdateCheckContext struct {
	EnvKeys []string
	Sources func(NativeUpdateCheckResolver) NativeUpdateCheckSources
}

// NativeUpdateCheckSpec is trusted built-in metadata for a read-only update
// check. Command runs directly without a shell; Parse is a pure adapter.
type NativeUpdateCheckSpec struct {
	Command Command
	Parse   func(NativeUpdateCheckOutput) NativeUpdateCheckResult
	Context NativeUpdateCheckContext
}

// NativeUpdateCheckAgent reports update availability through the agent's own
// checker. It is independent of HarnessUpdateAgent and never enables
// automatic updates.
type NativeUpdateCheckAgent interface {
	NativeUpdateCheck() NativeUpdateCheckSpec
}
