package mcp

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// kandevToolRef matches any `<name>_kandev` identifier appearing in a prompt.
// The regex enforces snake_case and requires the _kandev suffix at a word boundary.
var kandevToolRef = regexp.MustCompile(`\b[a-z][a-z0-9_]*_kandev\b`)

// extractKandevTools returns the unique set of "<name>_kandev" tool names
// referenced anywhere inside the given prompt text.
func extractKandevTools(prompt string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, m := range kandevToolRef.FindAllString(prompt, -1) {
		out[m] = struct{}{}
	}
	return out
}

// findBareToolReferences scans `prompt` for any whole-word occurrence of a
// `bareName`. Returns the sorted list of bare names found. Used to catch
// sysprompt drift in the opposite direction of the v0.28 bug — i.e. a prompt
// that says `get_task_plan` (no suffix) when the registered tool is
// `get_task_plan_kandev`.
//
// `\b` in Go's RE2 fires at a transition between a word char (`[A-Za-z0-9_]`)
// and a non-word char. Since `_` is a word char, `\b<bare>\b` cannot match
// inside `<bare>_kandev` — the trailing `\b` requires a non-word char after the
// last letter of `bare`, and `_` fails that test. So this regex naturally
// distinguishes bare references from the suffixed form without an explicit
// suffix guard.
func findBareToolReferences(prompt string, bareNames map[string]struct{}) []string {
	var found []string
	for bare := range bareNames {
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(bare) + `\b`)
		if re.MatchString(prompt) {
			found = append(found, bare)
		}
	}
	sort.Strings(found)
	return found
}

// bareNamesOf strips the `_kandev` suffix from every registered tool name and
// returns the bare set, used by findBareToolReferences as the haystack of
// candidates to scan for.
func bareNamesOf(registered map[string]struct{}) map[string]struct{} {
	const suffix = "_kandev"
	out := make(map[string]struct{}, len(registered))
	for name := range registered {
		out[strings.TrimSuffix(name, suffix)] = struct{}{}
	}
	return out
}

// TestSyspromptToolNames_MatchMCPTaskMode verifies that every `<name>_kandev`
// tool referenced in the task-mode prompts (PlanMode, KandevContext,
// DefaultPlanPrefix) is actually registered by an MCP server in ModeTask.
//
// This is the regression test for the v0.28 bug where the sysprompt told
// agents to call tools like `get_task_plan_kandev` but the MCP server
// registered them as `get_task_plan` (no suffix), causing "Tool not found"
// errors at runtime.
func TestSyspromptToolNames_MatchMCPTaskMode(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeTask)
	require.NotNil(t, s)

	registered := make(map[string]struct{})
	for _, name := range getRegisteredToolNames(s) {
		registered[name] = struct{}{}
	}

	referenced := make(map[string]struct{})
	for name := range extractKandevTools(sysprompt.PlanMode) {
		referenced[name] = struct{}{}
	}
	for name := range extractKandevTools(sysprompt.KandevContext) {
		referenced[name] = struct{}{}
	}
	for name := range extractKandevTools(sysprompt.DefaultPlanPrefix) {
		referenced[name] = struct{}{}
	}

	require.NotEmpty(t, referenced, "expected task-mode prompts to reference at least one _kandev tool")

	for name := range referenced {
		_, ok := registered[name]
		assert.True(t, ok,
			"tool %q is referenced in task-mode sysprompt but not registered by the MCP server in ModeTask",
			name)
	}
}

// TestSyspromptToolNames_MatchMCPConfigMode verifies that every `<name>_kandev`
// tool referenced in ConfigContext is registered by an MCP server in ModeConfig.
func TestSyspromptToolNames_MatchMCPConfigMode(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeConfig)
	require.NotNil(t, s)

	registered := make(map[string]struct{})
	for _, name := range getRegisteredToolNames(s) {
		registered[name] = struct{}{}
	}

	referenced := extractKandevTools(sysprompt.ConfigContext)
	require.NotEmpty(t, referenced, "expected ConfigContext to reference at least one _kandev tool")

	for name := range referenced {
		_, ok := registered[name]
		assert.True(t, ok,
			"tool %q is referenced in ConfigContext but not registered by the MCP server in ModeConfig",
			name)
	}
}

// TestSyspromptToolNames_NoBareToolReferences catches the opposite drift: a
// prompt that mentions a registered tool by its bare name (without the
// `_kandev` suffix). Without this check, a typo like
// `get_task_plan` in a sysprompt would silently slip past the other tests
// because they only inspect `_kandev`-suffixed mentions.
func TestSyspromptToolNames_NoBareToolReferences(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	taskServer := New(backend, "test-session", "test-task", 10005, log, "", false, ModeTask)
	require.NotNil(t, taskServer)
	configServer := New(backend, "test-session", "test-task", 10005, log, "", false, ModeConfig)
	require.NotNil(t, configServer)

	registered := make(map[string]struct{})
	for _, name := range getRegisteredToolNames(taskServer) {
		registered[name] = struct{}{}
	}
	for _, name := range getRegisteredToolNames(configServer) {
		registered[name] = struct{}{}
	}
	bareNames := bareNamesOf(registered)

	cases := map[string]string{
		"PlanMode":          sysprompt.PlanMode,
		"KandevContext":     sysprompt.KandevContext,
		"DefaultPlanPrefix": sysprompt.DefaultPlanPrefix,
		"ConfigContext":     sysprompt.ConfigContext,
	}

	for name, prompt := range cases {
		bare := findBareToolReferences(prompt, bareNames)
		assert.Empty(t, bare,
			"sysprompt %s contains tool name(s) without the _kandev suffix: %v — every reference must use the suffixed form",
			name, bare)
	}
}

// TestFindBareToolReferences_DistinguishesSuffixedFromBare locks in the
// regex-word-boundary contract that findBareToolReferences relies on. If a
// future Go release changes `\b` semantics for `_`, the production tests above
// would silently start passing or failing for the wrong reasons; this unit
// test surfaces the change at its source.
func TestFindBareToolReferences_DistinguishesSuffixedFromBare(t *testing.T) {
	bareNames := map[string]struct{}{
		"create_task_plan": {},
		"list_executors":   {},
	}

	cases := []struct {
		name   string
		prompt string
		want   []string
	}{
		{
			name:   "bare name in prose is flagged",
			prompt: "Use list_executors to find IDs.",
			want:   []string{"list_executors"},
		},
		{
			name:   "suffixed name is not flagged",
			prompt: "Use list_executors_kandev to find IDs.",
			want:   nil,
		},
		{
			name:   "both forms in same prompt: only bare is flagged",
			prompt: "list_executors_kandev (bare: list_executors) returns IDs.",
			want:   []string{"list_executors"},
		},
		{
			name:   "multiple bare names sorted in output",
			prompt: "Call create_task_plan then list_executors.",
			want:   []string{"create_task_plan", "list_executors"},
		},
		{
			name:   "no references",
			prompt: "Hello world.",
			want:   nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := findBareToolReferences(tc.prompt, bareNames)
			assert.Equal(t, tc.want, got)
		})
	}
}
