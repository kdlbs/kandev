package mcp

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// kandevToolRef captures the canonical `<name>_kandev` identifier from either
// its protocol form or the known `mcp__kandev__` client-qualified alias.
var kandevToolRef = regexp.MustCompile(`\b(?:mcp__kandev__)?([a-z][a-z0-9_]*_kandev)\b`)

// extractKandevTools returns the unique set of "<name>_kandev" tool names
// referenced anywhere inside the given prompt text.
func extractKandevTools(prompt string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, match := range kandevToolRef.FindAllStringSubmatch(prompt, -1) {
		out[match[1]] = struct{}{}
	}
	return out
}

func TestExtractKandevTools_NormalizesClientQualifiedAliases(t *testing.T) {
	referenced := extractKandevTools(
		"Canonical step_complete_kandev may appear as mcp__kandev__step_complete_kandev.",
	)
	assert.Equal(t, map[string]struct{}{"step_complete_kandev": {}}, referenced)
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
	for name := range extractKandevTools(sysprompt.PlanMode()) {
		referenced[name] = struct{}{}
	}
	for name := range extractKandevTools(sysprompt.KandevContext()) {
		referenced[name] = struct{}{}
	}
	for name := range extractKandevTools(sysprompt.DefaultPlanPrefix()) {
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

// TestTaskControlDocs_MatchMessageSchemaAndStopChoice keeps the short injected
// context aligned with the task-control choices exposed by task-mode MCP. The
// full tool descriptions remain authoritative for lifecycle detail, but the
// first-turn context must make the queue/interrupt/stop decision discoverable.
func TestTaskControlDocs_MatchMessageSchemaAndStopChoice(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeTask)
	require.NotNil(t, s)
	tools := s.mcpServer.ListTools()
	messageTool, ok := tools["message_task_kandev"]
	require.True(t, ok)
	assert.Contains(t, tools, "stop_task_kandev")

	schema, err := json.Marshal(messageTool.Tool.InputSchema)
	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(schema, &parsed))
	properties, ok := parsed["properties"].(map[string]interface{})
	require.True(t, ok)
	deliveryMode, ok := properties["delivery_mode"].(map[string]interface{})
	require.True(t, ok, "message_task_kandev must expose delivery_mode")
	enumValues, ok := deliveryMode["enum"].([]interface{})
	require.True(t, ok)
	defaultValue, ok := deliveryMode["default"].(string)
	require.True(t, ok)

	context := sysprompt.KandevContext()
	assert.Contains(t, context, "delivery_mode")
	for _, rawValue := range enumValues {
		value, ok := rawValue.(string)
		require.True(t, ok)
		assert.Contains(t, context, fmt.Sprintf(`delivery_mode="%s"`, value))
	}
	assert.Contains(t, context, fmt.Sprintf(`delivery_mode="%s" or omit it`, defaultValue))
	assert.Contains(t, context, "direct parent")
	assert.Contains(t, context, "stop_task_kandev")
	assert.Contains(t, context, "halt-only")
	assert.Contains(t, context, "no replacement turn")
}

func TestTaskControlDocs_OmittedFromRestrictedModes(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	context := sysprompt.FormatKandevContextWithOptions("task", "session", sysprompt.KandevContextOptions{})
	for _, mode := range []string{ModeOffice, ModeConfig} {
		t.Run(mode, func(t *testing.T) {
			s := New(backend, "session", "task", 10005, log, "", false, mode)
			assert.NotContains(t, s.mcpServer.ListTools(), "stop_task_kandev")
			assert.NotContains(t, context, "stop_task_kandev")
			assert.NotContains(t, context, "delivery_mode")
		})
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

	referenced := extractKandevTools(sysprompt.ConfigContext())
	require.NotEmpty(t, referenced, "expected ConfigContext to reference at least one _kandev tool")

	for name := range referenced {
		_, ok := registered[name]
		assert.True(t, ok,
			"tool %q is referenced in ConfigContext but not registered by the MCP server in ModeConfig",
			name)
	}
}

func TestSyspromptToolNames_ExactlyMatchMCPOfficeMode(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeOffice)
	require.NotNil(t, s)

	registered := make(map[string]struct{})
	for _, name := range getRegisteredToolNames(s) {
		registered[name] = struct{}{}
	}
	referenced := extractKandevTools(sysprompt.OfficeContext())

	assert.Equal(t, registered, referenced,
		"Office first-turn context must advertise exactly the ModeOffice tool inventory")
	assert.NotContains(t, referenced, "step_complete_kandev")
	assert.NotContains(t, sysprompt.OfficeContext(), "mcp__kandev__step_complete_kandev")
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
		"PlanMode":          sysprompt.PlanMode(),
		"KandevContext":     sysprompt.KandevContext(),
		"OfficeContext":     sysprompt.OfficeContext(),
		"DefaultPlanPrefix": sysprompt.DefaultPlanPrefix(),
		"ConfigContext":     sysprompt.ConfigContext(),
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

// askUserQuestionSchemaFacts pulls the load-bearing structural facts from the
// registered ask_user_question_kandev tool schema: the top-level array param
// name, its min/max bounds, and the required sub-fields of each question
// object. These are the facts the embedded prompt contexts must mirror.
type askUserQuestionSchemaFacts struct {
	paramName      string
	minItems       int
	maxItems       int
	requiredFields []string
}

func extractAskUserQuestionFacts(t *testing.T, s *Server) askUserQuestionSchemaFacts {
	t.Helper()

	tool, ok := s.mcpServer.ListTools()["ask_user_question_kandev"]
	require.True(t, ok, "ask_user_question_kandev not registered on this server")

	raw, err := json.Marshal(tool.Tool.InputSchema)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(raw, &parsed))

	props, ok := parsed["properties"].(map[string]any)
	require.True(t, ok, "schema must expose 'properties'")

	// Collect all top-level array params and assert exactly one exists.
	// Using a collect-then-assert pattern (rather than break-on-first) makes
	// the selection deterministic even if Go's map iteration visits properties
	// in a different order between runs or Go versions.
	var arrayParams []string
	paramSpecs := make(map[string]map[string]any)
	for name, spec := range props {
		m, ok := spec.(map[string]any)
		if !ok {
			continue
		}
		if m["type"] == "array" {
			arrayParams = append(arrayParams, name)
			paramSpecs[name] = m
		}
	}
	sort.Strings(arrayParams)
	require.Len(t, arrayParams, 1,
		"ask_user_question schema must have exactly one top-level array parameter; got %v", arrayParams)

	arrayParam := arrayParams[0]
	arraySpec := paramSpecs[arrayParam]

	minF, _ := arraySpec["minItems"].(float64)
	maxF, _ := arraySpec["maxItems"].(float64)
	require.NotZero(t, maxF,
		"ask_user_question schema must declare maxItems on the %q array — if absent, bounds would silently become \"0-0\" and mask schema drift", arrayParam)

	items, ok := arraySpec["items"].(map[string]any)
	require.True(t, ok, "ask_user_question schema must declare an items schema on the %q array", arrayParam)
	requiredRaw, ok := items["required"].([]any)
	require.True(t, ok, "ask_user_question schema items must declare a required field list")
	required := make([]string, 0, len(requiredRaw))
	for _, r := range requiredRaw {
		if field, ok := r.(string); ok {
			required = append(required, field)
		}
	}
	sort.Strings(required)

	return askUserQuestionSchemaFacts{
		paramName:      arrayParam,
		minItems:       int(minF),
		maxItems:       int(maxF),
		requiredFields: required,
	}
}

// TestAskUserQuestionDocs_MatchSchema binds the embedded prompt documentation
// for ask_user_question_kandev to the actual MCP tool schema. If the schema
// changes (param renamed, bounds adjusted, required sub-fields added or
// removed) the prompt files must change with it — otherwise agents read stale
// docs and send malformed payloads, which is exactly what cancelled task
// 399f3f98 in v0.28.
func TestAskUserQuestionDocs_MatchSchema(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	taskServer := New(backend, "test-session", "test-task", 10005, log, "", false, ModeTask)
	require.NotNil(t, taskServer)
	configServer := New(backend, "test-session", "test-task", 10005, log, "", false, ModeConfig)
	require.NotNil(t, configServer)

	// Both modes register the same tool; assert that and pick one set of facts.
	taskFacts := extractAskUserQuestionFacts(t, taskServer)
	configFacts := extractAskUserQuestionFacts(t, configServer)
	require.Equal(t, taskFacts, configFacts,
		"ask_user_question_kandev schema differs between task and config modes — pick one source of truth")

	facts := taskFacts
	bounds := fmt.Sprintf("%d-%d", facts.minItems, facts.maxItems)

	// Each prompt must mention the array param name, the bounds, and every
	// required sub-field name. We assert phrases (not exact wording) so authors
	// can rewrite the prose freely as long as the load-bearing facts survive.
	cases := map[string]string{
		"KandevContext": sysprompt.KandevContext(),
		"ConfigContext": sysprompt.ConfigContext(),
	}
	for name, prompt := range cases {
		assert.Contains(t, prompt, facts.paramName,
			"%s must mention ask_user_question param name %q", name, facts.paramName)
		assert.Contains(t, prompt, bounds,
			"%s must mention the question-count bounds %q from the schema", name, bounds)
		for _, field := range facts.requiredFields {
			assert.Contains(t, prompt, field,
				"%s must mention required question sub-field %q", name, field)
		}
	}
}
