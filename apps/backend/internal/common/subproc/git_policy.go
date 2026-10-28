package subproc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Managed Git runs without repository-supplied code. The policy is carried in
// the argument vector (never the process environment), so the agent process
// and anything it spawns inherit nothing from it.

const (
	enumerationTimeout   = 5 * time.Second
	enumerationMaxOutput = 1 << 20
	maxDriverNameLength  = 256

	gitBinaryName   = "git"
	flagConfigEnv   = "--config-env"
	flagSkip        = "--skip"
	falseValue      = "false"
	propertyClean   = "clean"
	propertySmudge  = "smudge"
	propertyProcess = "process"
	toolGitCrypt    = "git-crypt"
	filterNameLFS   = "lfs"
)

// ManagedGitPolicyCapability is advertised by agentctl builds that apply the
// managed Git policy to every Git command they run.
const ManagedGitPolicyCapability = "managed-git-policy.v1"

var managedGitIsolation atomic.Bool

// SetManagedGitIsolation selects whether this process applies the managed Git
// policy. It is set once at startup (backend) or when the first agentctl
// instance is created; the zero value is off, which leaves every Git command
// exactly as the caller built it.
func SetManagedGitIsolation(enabled bool) { managedGitIsolation.Store(enabled) }

// ManagedGitIsolation reports whether the managed Git policy is applied.
func ManagedGitIsolation() bool { return managedGitIsolation.Load() }

type gitHooksKey struct{}

// WithGitHooks records whether the repository the command runs in has opted in
// to its own hooks. The explicit value replaces any inherited one; a context
// without a value is hardened.
func WithGitHooks(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, gitHooksKey{}, enabled)
}

func gitHooksEnabled(ctx context.Context) bool {
	enabled, _ := ctx.Value(gitHooksKey{}).(bool)
	return enabled
}

type gitCommandClass int

const (
	classContentWriting gitCommandClass = iota
	classFilterReading
	classFilterFree
)

// filterFreeSubcommands are pure reads and object writes: they can neither
// apply a filter or merge driver nor start a hook, so they never enumerate.
var filterFreeSubcommands = setOf(
	"rev-parse", "config", "for-each-ref", "rev-list", "merge-base", "show-ref",
	"ls-remote", "var", "version", "ls-tree", "commit-tree",
)

// filterReadingSubcommands can apply a filter or start a hook but write no
// content.
var filterReadingSubcommands = setOf(
	"symbolic-ref", "remote", "fetch", "update-ref", "write-tree",
	"status", "diff", "diff-tree", "diff-index", "diff-files", "log", "show", "format-patch",
	"range-diff", "ls-files", "push",
)

var diffStyleSubcommands = setOf(
	"diff", "log", "show", "format-patch", "diff-tree", "diff-index", "diff-files", "range-diff",
)

func setOf(values ...string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

// ClassifyGitSubcommand reports the class of a subcommand and its first
// argument. A subcommand on no list is content-writing.
func classifyGitSubcommand(sub string, rest []string) gitCommandClass {
	switch sub {
	case "worktree":
		if len(rest) == 0 {
			return classContentWriting
		}
		switch rest[0] {
		case "list", "prune":
			return classFilterFree
		case "remove":
			return classFilterReading
		}
		return classContentWriting
	case "branch":
		// Only `--list` followed by patterns is a pure read; any further option
		// (for example `--no-list`) can select a ref-updating form that starts a
		// reference-transaction hook.
		if len(rest) > 0 && rest[0] == "--list" && !hasOptionArg(rest[1:]) {
			return classFilterFree
		}
		return classContentWriting
	case "cat-file":
		for _, arg := range rest {
			if arg == "--filters" || strings.HasPrefix(arg, "--filters=") {
				return classContentWriting
			}
		}
		return classFilterFree
	}
	if filterFreeSubcommands[sub] {
		return classFilterFree
	}
	if filterReadingSubcommands[sub] {
		return classFilterReading
	}
	return classContentWriting
}

func hasOptionArg(args []string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			return true
		}
	}
	return false
}

// policyOwnedKeys are the configuration keys the policy sets. A caller entry
// for one of them is dropped.
var policyOwnedKeys = setOf(
	"core.hookspath", "core.fsmonitor", "protocol.ext.allow",
	"submodule.recurse", "fetch.recursesubmodules", "push.recursesubmodules",
)

func isPolicyOwnedKey(key string) bool {
	return policyOwnedKeys[strings.ToLower(key)]
}

var plainGlobalFlags = setOf(
	"--no-pager", "-P", "--no-optional-locks", "--no-replace-objects", "--literal-pathspecs",
	"--glob-pathspecs", "--noglob-pathspecs", "--icase-pathspecs", "--bare", "--no-lazy-fetch",
)

var repoSelectionOptions = []string{"-C", "--git-dir", "--work-tree", "--namespace"}

// parsedGitArgs is the split of a git argument vector into the leading global
// options and the subcommand tail.
type parsedGitArgs struct {
	configOptions []string // surviving caller -c / --config-env tokens
	selection     []string // repository selection options
	plain         []string // other recognised global flags
	sub           string
	rest          []string
}

func parseGitArgs(args []string) (parsedGitArgs, error) {
	var parsed parsedGitArgs
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			parsed.sub, parsed.rest = arg, args[i+1:]
			return parsed, nil
		}
		next, err := parseGlobalOption(&parsed, args, i)
		if err != nil {
			return parsed, err
		}
		i = next
	}
	return parsed, errors.New("managed Git command has no subcommand")
}

// parseGlobalOption consumes the global option at args[i] and returns the index
// of its last token.
func parseGlobalOption(parsed *parsedGitArgs, args []string, i int) (int, error) {
	arg := args[i]
	if plainGlobalFlags[arg] {
		parsed.plain = append(parsed.plain, arg)
		return i, nil
	}
	if strings.HasPrefix(arg, "-c") && !strings.HasPrefix(arg, "--") {
		return parseConfigOption(parsed, args, i, arg == "-c", arg[2:], 1)
	}
	if arg == flagConfigEnv || strings.HasPrefix(arg, "--config-env=") {
		inline := strings.TrimPrefix(arg, "--config-env=")
		return parseConfigOption(parsed, args, i, arg == flagConfigEnv, inline, 2)
	}
	for _, name := range repoSelectionOptions {
		if arg == name {
			if i+1 >= len(args) {
				return i, fmt.Errorf("managed Git refuses %s without an argument", name)
			}
			parsed.selection = append(parsed.selection, arg, args[i+1])
			return i + 1, nil
		}
		if strings.HasPrefix(name, "--") && strings.HasPrefix(arg, name+"=") {
			parsed.selection = append(parsed.selection, arg)
			return i, nil
		}
	}
	return i, fmt.Errorf("managed Git refuses global option %s", arg)
}

// parseConfigOption handles -c and --config-env. separate reports that the
// entry is the next argument; kind distinguishes the two spellings when
// re-emitting a surviving entry.
func parseConfigOption(parsed *parsedGitArgs, args []string, i int, separate bool, inline string, kind int) (int, error) {
	entry, last := inline, i
	if separate {
		if i+1 >= len(args) {
			return i, fmt.Errorf("managed Git refuses %s without an argument", args[i])
		}
		entry, last = args[i+1], i+1
	}
	key, _, _ := strings.Cut(entry, "=")
	if isPolicyOwnedKey(key) {
		return last, nil
	}
	flag := "-c"
	if kind == 2 {
		flag = flagConfigEnv
	}
	parsed.configOptions = append(parsed.configOptions, flag, entry)
	return last, nil
}

func refusedSubcommandArgument(arg string) bool {
	for _, option := range []string{"--textconv", "--filters", "--ext-diff"} {
		if arg == option || strings.HasPrefix(arg, option+"=") {
			return true
		}
	}
	return false
}

func checkManagedArguments(parsed parsedGitArgs, hooksEnabled bool) error {
	if parsed.sub == "blame" {
		return errors.New("managed Git refuses blame: it applies text conversion that cannot be disabled")
	}
	for _, arg := range parsed.rest {
		if arg == "--" {
			break
		}
		if refusedSubcommandArgument(arg) {
			return fmt.Errorf("managed Git refuses %s: it re-enables text conversion or external diff", arg)
		}
		if hooksEnabled && isRecurseSubmodulesEnable(arg) {
			return fmt.Errorf("managed Git refuses %s in a repository with hooks enabled: submodule recursion is disabled", arg)
		}
	}
	return nil
}

func isRecurseSubmodulesEnable(arg string) bool {
	if arg == "--recurse-submodules" {
		return true
	}
	value, ok := strings.CutPrefix(arg, "--recurse-submodules=")
	return ok && value != "no"
}

// isDryRunPush reports whether a push argument list asks for a dry run
// (--dry-run, or -n alone or clustered with other short flags).
func isDryRunPush(rest []string) bool {
	for _, arg := range rest {
		if arg == "--" {
			return false
		}
		if arg == "--dry-run" {
			return true
		}
		if len(arg) > 1 && arg[0] == '-' && arg[1] != '-' && strings.Trim(arg[1:], "nvqfud46") == "" && strings.Contains(arg, "n") {
			return true
		}
	}
	return false
}

func hasNoVerify(rest []string) bool {
	for _, arg := range rest {
		if arg == "--" {
			return false
		}
		if arg == "--no-verify" {
			return true
		}
	}
	return false
}

// ManagedGitArgs returns the policy vector for a Git argument list (without the
// leading "git"), enumerating drivers in the current working directory. The
// directory-entering helper uses it after changing directory.
func ManagedGitArgs(ctx context.Context, env []string, gitArgs []string) ([]string, error) {
	if !ManagedGitIsolation() {
		return gitArgs, nil
	}
	return buildManagedGitVector(ctx, "", env, gitArgs)
}

// ManagedGitArgsIn is ManagedGitArgs for a command that runs in dir, for callers
// that wrap the Git invocation in another executable and so cannot reach the
// policy applied to a direct Git start.
func ManagedGitArgsIn(ctx context.Context, dir string, env []string, gitArgs []string) ([]string, error) {
	if !ManagedGitIsolation() {
		return gitArgs, nil
	}
	return buildManagedGitVector(ctx, dir, env, gitArgs)
}

// ManagedGitEnv removes environment values that would run repository-selected
// programs.
func ManagedGitEnv(env []string) []string {
	if !ManagedGitIsolation() {
		return env
	}
	if env == nil {
		env = os.Environ()
	}
	out := make([]string, 0, len(env))
	for _, entry := range env {
		key, _ := environmentAssignmentKey(entry)
		if environmentKeysEqual(key, "GIT_EXTERNAL_DIFF") || environmentKeysEqual(key, "GIT_ALLOW_PROTOCOL") {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func buildManagedGitVector(ctx context.Context, dir string, env []string, args []string) ([]string, error) {
	parsed, err := parseGitArgs(args)
	if err != nil {
		return nil, err
	}
	hooksEnabled := gitHooksEnabled(ctx)
	if err := checkManagedArguments(parsed, hooksEnabled); err != nil {
		return nil, err
	}
	class := classifyGitSubcommand(parsed.sub, parsed.rest)
	pairs := map[string]string{
		"core.fsmonitor":     falseValue,
		"protocol.ext.allow": "never",
	}
	if hooksEnabled {
		pairs["submodule.recurse"] = "false"
		pairs["fetch.recurseSubmodules"] = "no"
		pairs["push.recurseSubmodules"] = "no"
	} else {
		pairs["core.hooksPath"] = os.DevNull
	}
	var driverPairs []string
	lfsAllowlisted := false
	if class != classFilterFree {
		drivers, err := enumerateDrivers(ctx, dir, env, parsed, pairs)
		if err != nil {
			return nil, err
		}
		driverPairs, lfsAllowlisted, err = driverPolicy(drivers, class, env, hooksEnabled)
		if err != nil {
			return nil, err
		}
	}
	if err := selectPushHooks(parsed, hooksEnabled, lfsAllowlisted, pairs); err != nil {
		return nil, err
	}
	return assembleVector(parsed, pairs, driverPairs), nil
}

// selectPushHooks points a hooks-off push at the Kandev-owned directory that
// runs Git LFS's pre-push step when an allowlisted lfs filter is configured.
func selectPushHooks(parsed parsedGitArgs, hooksEnabled, lfsAllowlisted bool, pairs map[string]string) error {
	if parsed.sub != "push" || hooksEnabled || !lfsAllowlisted || isDryRunPush(parsed.rest) {
		return nil
	}
	if hasNoVerify(parsed.rest) {
		return errors.New("managed Git refuses --no-verify on push: Git LFS objects must be uploaded before refs are sent")
	}
	lfsPath, err := exec.LookPath("git-lfs")
	if err != nil {
		return fmt.Errorf("resolve git-lfs: %w", err)
	}
	dir, err := lfsHooksDir(lfsPath)
	if err != nil {
		return err
	}
	pairs["core.hooksPath"] = dir
	return nil
}

func assembleVector(parsed parsedGitArgs, pairs map[string]string, driverPairs []string) []string {
	keys := make([]string, 0, len(pairs))
	for key := range pairs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	vector := append([]string{}, parsed.selection...)
	vector = append(vector, parsed.plain...)
	vector = append(vector, parsed.configOptions...)
	for _, key := range keys {
		vector = append(vector, "-c", key+"="+pairs[key])
	}
	for _, pair := range driverPairs {
		vector = append(vector, "-c", pair)
	}
	vector = append(vector, parsed.sub)
	if diffStyleSubcommands[parsed.sub] {
		vector = append(vector, "--no-textconv", "--no-ext-diff")
	}
	return append(vector, parsed.rest...)
}

// driverEntry holds the effective configuration of one filter or merge driver.
type driverEntry struct {
	values map[string]*string // nil pointer: key present without a value
}

type driverSet struct {
	filters map[string]*driverEntry
	merges  map[string]*driverEntry
	hooks   map[string]bool // config-defined hook names
	// lfsAgent reports an lfs.standalonetransferagent value; lfsTransfers
	// holds lfs.customtransfer names; lfsExtensions reports any
	// lfs.extension.<name>.clean/smudge command.
	lfsAgent      bool
	lfsTransfers  map[string]bool
	lfsExtensions bool
}

// driverKeyPattern selects every configuration key that names a program Git or
// Git LFS would run on behalf of the repository.
const driverKeyPattern = `^((filter|merge)\..*\.(clean|smudge|process|required|driver|name|recursive)` +
	`|hook\..*\.(command|event|enabled)` +
	`|lfs\.(standalonetransferagent|customtransfer\..*\.path|extension\..*\.(clean|smudge)))$`

func enumerateDrivers(ctx context.Context, dir string, env []string, parsed parsedGitArgs, pairs map[string]string) (*driverSet, error) {
	args := append([]string{}, parsed.selection...)
	args = append(args, parsed.configOptions...)
	for _, key := range []string{"core.fsmonitor", "protocol.ext.allow", "core.hooksPath"} {
		if value, ok := pairs[key]; ok {
			args = append(args, "-c", key+"="+value)
		} else if key == "core.hooksPath" {
			args = append(args, "-c", key+"="+os.DevNull)
		}
	}
	args = append(args, "config", "--null", "--get-regexp", driverKeyPattern)
	output, err := runEnumeration(ctx, dir, env, args)
	if err != nil {
		return nil, fmt.Errorf("managed Git could not determine filter and merge drivers: %w", err)
	}
	return parseDriverOutput(output)
}

func runEnumeration(ctx context.Context, dir string, env []string, args []string) ([]byte, error) {
	execCtx, cancel := context.WithTimeout(ctx, enumerationTimeout)
	defer cancel()
	name := gitBinaryName
	if resolved, err := GitExecutablePath(); err == nil {
		name = resolved
	}
	cmd := exec.CommandContext(execCtx, name, args...)
	cmd.Dir = dir
	cmd.Env = env
	var stdout limitedBuffer
	var stderr synchronizedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	process, err := startManagedGitUnpolicied(execCtx, cmd)
	if err != nil {
		return nil, err
	}
	runErr := process.wait()
	if stdout.exceeded {
		return nil, fmt.Errorf("driver listing exceeds %d bytes", enumerationMaxOutput)
	}
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) && exitErr.ExitCode() == 1 && len(stdout.Bytes()) == 0 && len(stderr.Bytes()) == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: %s", runErr, strings.TrimSpace(string(stderr.Bytes())))
	}
	return stdout.Bytes(), nil
}

type limitedBuffer struct {
	buf      bytes.Buffer
	exceeded bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.buf.Len()+len(p) > enumerationMaxOutput {
		b.exceeded = true
		return 0, errors.New("output limit exceeded")
	}
	return b.buf.Write(p)
}

func (b *limitedBuffer) Bytes() []byte { return b.buf.Bytes() }

func parseDriverOutput(output []byte) (*driverSet, error) {
	set := &driverSet{
		filters: map[string]*driverEntry{}, merges: map[string]*driverEntry{},
		hooks: map[string]bool{}, lfsTransfers: map[string]bool{},
	}
	for _, record := range bytes.Split(output, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		key, value, hasValue := strings.Cut(string(record), "\n")
		family, rest, _ := strings.Cut(key, ".")
		if family == filterNameLFS {
			if err := set.addLFSKey(rest); err != nil {
				return nil, err
			}
			continue
		}
		last := strings.LastIndex(rest, ".")
		if last <= 0 {
			return nil, fmt.Errorf("managed Git could not parse driver key %q", key)
		}
		name, property := rest[:last], rest[last+1:]
		if strings.Contains(name, "=") {
			return nil, fmt.Errorf("managed Git cannot neutralize driver %q: the name contains '='", name)
		}
		if len(name) > maxDriverNameLength {
			return nil, fmt.Errorf("managed Git refuses a driver name longer than %d characters", maxDriverNameLength)
		}
		if family == "hook" {
			set.hooks[name] = true
			continue
		}
		target := set.filters
		if family == "merge" {
			target = set.merges
		}
		entry := target[name]
		if entry == nil {
			entry = &driverEntry{values: map[string]*string{}}
			target[name] = entry
		}
		if hasValue {
			entry.values[property] = &value
		} else {
			entry.values[property] = nil
		}
	}
	return set, nil
}

// addLFSKey records an lfs.* key naming a program: rest is the key without its
// "lfs." section.
func (s *driverSet) addLFSKey(rest string) error {
	if rest == "standalonetransferagent" {
		s.lfsAgent = true
		return nil
	}
	section, tail, _ := strings.Cut(rest, ".")
	last := strings.LastIndex(tail, ".")
	if last <= 0 {
		return fmt.Errorf("managed Git could not parse lfs key %q", rest)
	}
	name := tail[:last]
	if strings.Contains(name, "=") {
		return fmt.Errorf("managed Git cannot neutralize lfs entry %q: the name contains '='", name)
	}
	if len(name) > maxDriverNameLength {
		return fmt.Errorf("managed Git refuses an lfs entry name longer than %d characters", maxDriverNameLength)
	}
	switch section {
	case "customtransfer":
		s.lfsTransfers[name] = true
	case "extension":
		s.lfsExtensions = true
	}
	return nil
}

// effective returns the configured command for a property and whether it is set
// (a missing or empty value is unset).
func (e *driverEntry) effective(property string) (string, bool) {
	value, ok := e.values[property]
	if !ok || value == nil || *value == "" {
		return "", false
	}
	return *value, true
}

func (e *driverEntry) required() (bool, error) {
	value, ok := e.values["required"]
	if !ok {
		return false, nil
	}
	if value == nil {
		return true, nil
	}
	switch strings.ToLower(*value) {
	case "true", "yes", "on":
		return true, nil
	case "false", "no", "off", "":
		return false, nil
	}
	number, err := strconv.ParseInt(*value, 10, 64)
	if err != nil {
		return false, fmt.Errorf("invalid boolean %q for a filter's required setting", *value)
	}
	return number != 0, nil
}

// driverPolicy returns the neutralizing pairs for the enumerated drivers and
// whether an allowlisted lfs filter is configured.
func driverPolicy(set *driverSet, class gitCommandClass, env []string, hooksEnabled bool) ([]string, bool, error) {
	var pairs []string
	lfs := false
	for _, name := range sortedKeys(set.filters) {
		entry := set.filters[name]
		required, err := entry.required()
		if err != nil {
			return nil, false, fmt.Errorf("filter %q: %w", name, err)
		}
		if filterAllowlisted(name, entry, env) && (name != filterNameLFS || !set.lfsExtensions) {
			lfs = lfs || name == filterNameLFS
			continue
		}
		if required && class == classContentWriting {
			return nil, false, fmt.Errorf("filter %q is required but not permitted in managed Git", name)
		}
		for _, property := range []string{propertyClean, propertyProcess, propertySmudge} {
			pairs = append(pairs, "filter."+name+"."+property+"=")
		}
		pairs = append(pairs, "filter."+name+".required=false")
	}
	for _, name := range sortedKeys(set.merges) {
		pairs = append(pairs, "merge."+name+".driver=")
	}
	if !hooksEnabled {
		for _, name := range sortedNames(set.hooks) {
			pairs = append(pairs, "hook."+name+".enabled=false")
		}
	}
	if set.lfsAgent {
		pairs = append(pairs, "lfs.standalonetransferagent=")
	}
	for _, name := range sortedNames(set.lfsTransfers) {
		pairs = append(pairs, "lfs.customtransfer."+name+".path=")
	}
	return pairs, lfs, nil
}

func sortedNames(m map[string]bool) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedKeys(m map[string]*driverEntry) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// canonicalFilterForms maps a tool to the argument lists its install writes.
func canonicalFilterForms(name string) (tool string, forms map[string][][]string) {
	switch {
	case name == filterNameLFS:
		return "git-lfs", map[string][][]string{
			propertyClean:   {{propertyClean, "--", "%f"}, {propertyClean, "%f"}},
			propertySmudge:  {{propertySmudge, "--", "%f"}, {propertySmudge, flagSkip, "--", "%f"}, {propertySmudge, "%f"}, {propertySmudge, flagSkip, "%f"}},
			propertyProcess: {{"filter-process"}, {"filter-process", flagSkip}},
		}
	case name == toolGitCrypt:
		return toolGitCrypt, map[string][][]string{
			propertyClean: {{propertyClean}}, propertySmudge: {{propertySmudge}},
		}
	case strings.HasPrefix(name, "git-crypt-") && len(name) > len("git-crypt-"):
		key := "--key-name=" + strings.TrimPrefix(name, "git-crypt-")
		return toolGitCrypt, map[string][][]string{
			propertyClean: {{propertyClean, key}}, propertySmudge: {{propertySmudge, key}},
		}
	}
	return "", nil
}

func filterAllowlisted(name string, entry *driverEntry, env []string) bool {
	tool, forms := canonicalFilterForms(name)
	if tool == "" {
		return false
	}
	anySet := false
	for _, property := range []string{"clean", "smudge", "process"} {
		command, set := entry.effective(property)
		if !set {
			continue
		}
		anySet = true
		words, ok := tokenizeFilterCommand(command)
		if !ok || len(words) == 0 || !toolMatches(words[0], tool, env) {
			return false
		}
		if !matchesAnyForm(words[1:], forms[property]) {
			return false
		}
	}
	return anySet
}

func matchesAnyForm(args []string, forms [][]string) bool {
	for _, form := range forms {
		if len(form) != len(args) {
			continue
		}
		same := true
		for i := range form {
			if form[i] != args[i] {
				same = false
				break
			}
		}
		if same {
			return true
		}
	}
	return false
}

// toolMatches reports whether word names the same file as Kandev's own lookup
// of tool: an absolute path resolving to it, or a bare name that resolves to it
// under both the child's PATH and Kandev's.
func toolMatches(word, tool string, env []string) bool {
	own, err := exec.LookPath(tool)
	if err != nil {
		return false
	}
	ownReal, err := filepath.EvalSymlinks(own)
	if err != nil {
		return false
	}
	if filepath.IsAbs(word) {
		real, err := filepath.EvalSymlinks(word)
		return err == nil && real == ownReal
	}
	if word != tool {
		return false
	}
	childPath, set := environmentValue(env, "PATH")
	if !set {
		return true
	}
	return sameFileOnPath(tool, childPath, ownReal)
}

func sameFileOnPath(tool, pathValue, wantReal string) bool {
	for _, dir := range filepath.SplitList(pathValue) {
		if dir == "" {
			dir = "."
		}
		for _, name := range executableNames(tool) {
			candidate := filepath.Join(dir, name)
			info, err := os.Stat(candidate)
			if err != nil || info.IsDir() || (!onWindows() && info.Mode().Perm()&0o111 == 0) {
				continue
			}
			real, err := filepath.EvalSymlinks(candidate)
			return err == nil && real == wantReal
		}
	}
	return false
}

// executableNames lists the file names a PATH lookup of tool tries: the bare
// name, plus the PATHEXT variants on Windows, where installs ship git-lfs.exe.
func onWindows() bool { return runtime.GOOS == "windows" }

func executableNames(tool string) []string {
	names := []string{tool}
	if !onWindows() {
		return names
	}
	exts := os.Getenv("PATHEXT")
	if exts == "" {
		exts = ".COM;.EXE;.BAT;.CMD"
	}
	for _, ext := range strings.Split(exts, ";") {
		if ext != "" {
			names = append(names, tool+strings.ToLower(ext))
		}
	}
	return names
}

const filterCommandMeta = "$`;&|<>()*?[]{}~#!\n"

// tokenizeFilterCommand splits a filter command as a shell word list and
// reports false for anything a shell would expand or that does not parse.
func tokenizeFilterCommand(command string) ([]string, bool) {
	var words []string
	var current strings.Builder
	inWord := false
	for i := 0; i < len(command); i++ {
		c := command[i]
		switch {
		case c == '\'':
			text, next, ok := readSingleQuoted(command, i)
			if !ok {
				return nil, false
			}
			current.WriteString(text)
			i, inWord = next, true
		case c == '"':
			text, next, ok := readDoubleQuoted(command, i)
			if !ok {
				return nil, false
			}
			current.WriteString(text)
			i, inWord = next, true
		case c == '\\':
			if i+1 >= len(command) || command[i+1] == '\n' {
				return nil, false
			}
			i++
			current.WriteByte(command[i])
			inWord = true
		case c == ' ' || c == '\t':
			if inWord {
				words = append(words, current.String())
				current.Reset()
				inWord = false
			}
		case strings.IndexByte(filterCommandMeta, c) >= 0:
			return nil, false
		default:
			current.WriteByte(c)
			inWord = true
		}
	}
	if inWord {
		words = append(words, current.String())
	}
	return words, len(words) > 0
}

// readSingleQuoted reads the single-quoted segment opening at command[open]
// and returns its text and the index of the closing quote.
func readSingleQuoted(command string, open int) (string, int, bool) {
	end := strings.IndexByte(command[open+1:], '\'')
	if end < 0 {
		return "", 0, false
	}
	return command[open+1 : open+1+end], open + 1 + end, true
}

// readDoubleQuoted reads the double-quoted segment opening at command[open];
// it rejects expansion metacharacters and returns the index of the closing quote.
func readDoubleQuoted(command string, open int) (string, int, bool) {
	var text strings.Builder
	for i := open + 1; i < len(command); i++ {
		d := command[i]
		if d == '"' {
			return text.String(), i, true
		}
		if strings.IndexByte(filterCommandMeta, d) >= 0 {
			return "", 0, false
		}
		if d == '\\' && i+1 < len(command) && (command[i+1] == '"' || command[i+1] == '\\') {
			i++
			d = command[i]
		}
		text.WriteByte(d)
	}
	return "", 0, false
}
