package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/service"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateTaskPlanKandev_ToolSchema_ModeHasDefaultAndDocumentsBothValues
// pins mode's advertised default and that its description names both
// accepted values. It deliberately does NOT assert a JSON-schema "enum": the
// server's generic MCP argument-schema validator (internal/mcp/server/tool_
// argument_validation.go) compiles every tool's schema with
// additionalProperties:false and enforces a declared enum strictly, which
// would reject an out-of-enum value itself - with a message naming neither
// accepted value - before service.ParsePlanWriteMode ever ran. Declaring the
// two values only in the description (advisory to well-behaved clients) is
// what lets AC-TASKS-PLAN-APPEND-001.3's "name the two accepted values"
// requirement actually reach the caller.
func TestUpdateTaskPlanKandev_ToolSchema_ModeHasDefaultAndDocumentsBothValues(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	props := toolInputProperties(t, s, "update_task_plan_kandev")
	modeSchema, ok := props["mode"].(map[string]interface{})
	require.True(t, ok, "update_task_plan_kandev schema must expose a 'mode' property")

	assert.Equal(t, string(service.PlanWriteModeReplace), modeSchema["default"])
	assert.NotContains(t, modeSchema, "enum",
		"an enforced schema enum would shadow ParsePlanWriteMode's own message; see comment above")

	description, _ := modeSchema["description"].(string)
	assert.Contains(t, description, string(service.PlanWriteModeReplace))
	assert.Contains(t, description, string(service.PlanWriteModeAppend))
}

// TestUpdateTaskPlanKandev_ToolDescription_CoversAC006 pins
// AC-TASKS-PLAN-APPEND-006.1 through 006.5 directly against
// update_task_plan_kandev's own tool description (not its mode parameter,
// covered separately above): both modes and the default are named, the
// destructive replace-mode warning survives scoped to replace, and append's
// blank-line separator, non-idempotence, and no-read-first properties are
// each stated. Each sub-test asserts a substantive clause rather than the
// whole string, so a wording change that keeps the fact does not fail it.
func TestUpdateTaskPlanKandev_ToolDescription_CoversAC006(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	toolsMap := s.mcpServer.ListTools()
	st, ok := toolsMap["update_task_plan_kandev"]
	require.True(t, ok, "update_task_plan_kandev not registered")
	desc := st.Tool.Description

	t.Run("006.1 names both modes, the default, and what each submits", func(t *testing.T) {
		assert.Contains(t, desc, `mode="replace"`)
		assert.Contains(t, desc, `mode="append"`)
		assert.Contains(t, desc, "default")
		assert.Contains(t, desc, "whole document")
		assert.Contains(t, desc, "only an addition")
	})

	t.Run("006.2 retains the destructive warning scoped to replace", func(t *testing.T) {
		assert.Contains(t, desc, "replace mode")
		assert.Contains(t, desc, "silently delete everything else")
	})

	t.Run("006.3 states the blank-line separator", func(t *testing.T) {
		assert.Contains(t, desc, "append mode")
		assert.Contains(t, desc, "blank line")
	})

	t.Run("006.4 states append is not idempotent", func(t *testing.T) {
		assert.Contains(t, desc, "not idempotent")
		assert.Contains(t, desc, "again")
	})

	t.Run("006.5 states append does not require reading the plan first", func(t *testing.T) {
		assert.Contains(t, desc, "do not need to read the plan first")
	})
}

// TestCreateTaskPlanKandev_ToolSchema_ModeParamIsDocumentedAsRejected pins
// that create_task_plan_kandev's schema deliberately DOES declare a "mode"
// property (unlike a literal reading of REQ-TASKS-PLAN-APPEND-005 might
// suggest): without one, the server's generic argument-schema validator's
// additionalProperties:false would reject any call carrying a "mode" key
// itself, with a generic message that never names update_task_plan_kandev,
// before createTaskPlanHandler's own (correctly worded) rejection ever ran.
// The property exists only so the real, informative rejection is reachable;
// its own description says the value is never accepted.
func TestCreateTaskPlanKandev_ToolSchema_ModeParamIsDocumentedAsRejected(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	props := toolInputProperties(t, s, "create_task_plan_kandev")
	modeSchema, ok := props["mode"].(map[string]interface{})
	require.True(t, ok, "create_task_plan_kandev schema must expose a 'mode' property")
	description, _ := modeSchema["description"].(string)
	assert.Contains(t, strings.ToLower(description), "not supported")
	assert.Contains(t, description, "update_task_plan_kandev")
}

// planWriteDenialPhrases are the literal claims AC-TASKS-PLAN-APPEND-006.9
// forbids: that no partial update or append mode exists anywhere. Both
// phrasings have shipped in production text before this capability existed
// (the truncation warning's "there is no partial update or append mode",
// and create_task_plan_kandev's former "has no append mode"), so both are
// checked rather than one. AC-TASKS-PLAN-APPEND-006.9 carves out exactly one
// exception: create_task_plan_kandev may say it, scoped to itself, as long
// as the same text also redirects to update_task_plan_kandev's append mode -
// checked separately below, not by exempting the tool from this scan.
var planWriteDenialPhrases = []string{"no partial update", "no append mode"}

// planWriteResendPhrases are the read-then-resend prescriptions
// AC-TASKS-PLAN-APPEND-006.9 forbids directing at a caller who wants to add
// a section, unless the same text also names update_task_plan_kandev's
// append mode as the alternative (replace-mode guidance keeps this phrasing
// legitimately, since append is mentioned right alongside it). These are the
// literal phrasings that ship in create_task_plan_kandev's own description
// and content parameter today ("read it first with get_task_plan_kandev",
// "call get_task_plan_kandev first and include its content") — checking the
// real production text rather than a hypothetical wording that has never
// shipped.
var planWriteResendPhrases = []string{"read it first with get_task_plan_kandev", "call get_task_plan_kandev first and include its content"}

// TestPlanToolDescriptions_MentionAppendModeConsistently is the AC-006.9
// class-rule test: every plan tool's description and every string-typed
// parameter description, plus the two rendered truncation-warning branches,
// is scanned by iteration (not enumerated per string) so a new plan tool or
// parameter is covered the day it is added. update_task_plan_kandev's own
// text is walked too - it is the tool whose description most recently
// carried the banned claim, so a special case here is exactly what would
// stop checking it again.
func TestPlanToolDescriptions_MentionAppendModeConsistently(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	planTools := []string{
		"create_task_plan_kandev", "get_task_plan_kandev",
		"update_task_plan_kandev", "delete_task_plan_kandev",
	}
	toolsMap := s.mcpServer.ListTools()
	toolTexts := map[string][]string{}
	for _, name := range planTools {
		st, ok := toolsMap[name]
		require.True(t, ok, "tool %q not registered", name)

		texts := []string{st.Tool.Description}
		schema, err := json.Marshal(st.Tool.InputSchema)
		require.NoError(t, err)
		var parsed struct {
			Properties map[string]struct {
				Description string `json:"description"`
			} `json:"properties"`
		}
		require.NoError(t, json.Unmarshal(schema, &parsed))
		for _, prop := range parsed.Properties {
			texts = append(texts, prop.Description)
		}
		toolTexts[name] = texts
	}

	for name, texts := range toolTexts {
		t.Run(name, func(t *testing.T) {
			for _, text := range texts {
				mentionsAppend := strings.Contains(text, "append")
				redirectsToUpdateAppend := mentionsAppend && strings.Contains(text, "update_task_plan_kandev")
				for _, denial := range planWriteDenialPhrases {
					if !strings.Contains(text, denial) {
						continue
					}
					// The one sanctioned exception: create_task_plan_kandev
					// scoping the denial to itself while redirecting to
					// update_task_plan_kandev's append mode in the same text.
					if name == "create_task_plan_kandev" && redirectsToUpdateAppend {
						continue
					}
					t.Errorf("text states or implies no append mode exists (%q): %q", denial, text)
				}
				for _, resend := range planWriteResendPhrases {
					if strings.Contains(text, resend) && !mentionsAppend {
						t.Errorf("text prescribes read-then-resend without naming append: %q", text)
					}
				}
				// Every other tool's mention of "append" must point the
				// caller at the one tool that accepts it. update_task_plan_
				// kandev's own text has no reason to name itself.
				if name != "update_task_plan_kandev" && mentionsAppend && !strings.Contains(text, "update_task_plan_kandev") {
					t.Errorf("text mentions append without naming update_task_plan_kandev: %q", text)
				}
			}
		})
	}

	// create's description is this rule's primary target: it must actively
	// redirect to the append tool, not just avoid contradicting it.
	createDesc := toolsMap["create_task_plan_kandev"].Tool.Description
	if !strings.Contains(createDesc, "append") || !strings.Contains(createDesc, "update_task_plan_kandev") {
		t.Errorf("create_task_plan_kandev's description does not point callers at update_task_plan_kandev's append mode: %q", createDesc)
	}
}

// TestUpdateTaskPlanKandev_BridgesModeToPayload pins that the tool handler
// forwards mode into the outgoing WS payload only when the caller supplied
// one, and that an absent mode reaches the backend as an absent key (not an
// explicit "replace") so a pre-existing browser/back-end contract sees no
// change for every caller that never mentions mode.
func TestUpdateTaskPlanKandev_BridgesModeToPayload(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{"task_id": "task-current", "content": "c"}}
	s := newTaskModeServer(t, backend, "task-current")

	t.Run("mode present", func(t *testing.T) {
		result := callTool(t, s, "update_task_plan_kandev", map[string]interface{}{
			"content": "fragment", "mode": "append",
		})
		require.False(t, result.IsError, "unexpected error result")
		payload, ok := backend.lastPayload.(map[string]interface{})
		require.True(t, ok, "payload should be a string-keyed map")
		assert.Equal(t, "append", payload["mode"])
	})

	t.Run("mode absent", func(t *testing.T) {
		result := callTool(t, s, "update_task_plan_kandev", map[string]interface{}{
			"content": "whole document",
		})
		require.False(t, result.IsError, "unexpected error result")
		payload, ok := backend.lastPayload.(map[string]interface{})
		require.True(t, ok, "payload should be a string-keyed map")
		assert.NotContains(t, payload, "mode")
	})
}

// TestCreateTaskPlanKandev_RejectsModeArgument pins
// AC-TASKS-PLAN-APPEND-005.2: any non-empty mode argument on
// create_task_plan_kandev is rejected, naming update_task_plan_kandev, before
// the backend round trip.
func TestCreateTaskPlanKandev_RejectsModeArgument(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "create_task_plan_kandev", map[string]interface{}{
		"content": "body", "mode": "append",
	})
	require.True(t, result.IsError, "expected create_task_plan_kandev to reject a mode argument")
	text := resultText(t, result)
	if !strings.Contains(text, "update_task_plan_kandev") {
		t.Errorf("expected the rejection to name update_task_plan_kandev, got: %q", text)
	}
	assert.Empty(t, backend.lastAction, "backend must not be called when mode is rejected")
}

// TestCreateTaskPlanKandev_AbsentOrEmptyModeIsUnaffected pins
// AC-TASKS-PLAN-APPEND-005.2's last sentence: an absent or empty mode leaves
// create's existing behavior unchanged.
func TestCreateTaskPlanKandev_AbsentOrEmptyModeIsUnaffected(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{"task_id": "task-current", "content": "body"}}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "create_task_plan_kandev", map[string]interface{}{
		"content": "body", "mode": "",
	})
	require.False(t, result.IsError, "an empty mode must not be rejected")
}

// TestCreateTaskPlanKandev_RejectsWronglyTypedMode pins that a non-string
// mode value is rejected, never silently read as absent. In practice the
// schema's own type constraint on "mode" (declared so a string value can
// reach the handler's own message; see the schema comment) already catches
// this before createTaskPlanHandler runs, so this only pins the outward
// safety property, not which layer produced the rejection.
func TestCreateTaskPlanKandev_RejectsWronglyTypedMode(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "create_task_plan_kandev", map[string]interface{}{
		"content": "body", "mode": 42,
	})
	require.True(t, result.IsError)
	assert.Empty(t, backend.lastAction)
}

// TestUpdateTaskPlanKandev_RejectsWronglyTypedMode pins the same outward
// safety property as TestCreateTaskPlanKandev_RejectsWronglyTypedMode for
// update_task_plan_kandev: a non-string mode is rejected, never silently
// read as absent (which would default to the destructive replace behavior).
func TestUpdateTaskPlanKandev_RejectsWronglyTypedMode(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "update_task_plan_kandev", map[string]interface{}{
		"content": "fragment", "mode": 42,
	})
	require.True(t, result.IsError)
	assert.Empty(t, backend.lastAction, "backend must not be called for a wrongly-typed mode")
}

// TestUpdateTaskPlanKandev_RejectsUnknownModeValueBeforeContentCheck pins
// AC-TASKS-PLAN-APPEND-001.7's ordering at the MCP tool layer: mode validity
// is resolved entirely from the request before content is even read, so a
// call missing content too still reports the mode rejection, naming both
// accepted values, and never reaches the backend. content is deliberately
// not schema-required (see server.go) so this ordering is actually
// observable rather than shadowed by a generic schema error.
func TestUpdateTaskPlanKandev_RejectsUnknownModeValueBeforeContentCheck(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "update_task_plan_kandev", map[string]interface{}{
		"mode": "merge",
	})
	require.True(t, result.IsError)
	text := resultText(t, result)
	if !strings.Contains(text, "replace") || !strings.Contains(text, "append") {
		t.Errorf("expected the mode error naming both accepted values, got: %q", text)
	}
	assert.Empty(t, backend.lastAction)
}

// TestUpdateTaskPlanKandev_RejectsCaseVariantModeBeforeContentCheck is the
// literal AC-TASKS-PLAN-APPEND-001.3 example ("Append"/"APPEND" differing
// only in letter case), exercised the same way as the unknown-value case
// above.
func TestUpdateTaskPlanKandev_RejectsCaseVariantModeBeforeContentCheck(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	for _, mode := range []string{"Append", "APPEND", "REPLACE"} {
		t.Run(mode, func(t *testing.T) {
			result := callTool(t, s, "update_task_plan_kandev", map[string]interface{}{
				"mode": mode,
			})
			require.True(t, result.IsError)
			text := resultText(t, result)
			if !strings.Contains(text, "replace") || !strings.Contains(text, "append") {
				t.Errorf("expected the mode error naming both accepted values, got: %q", text)
			}
			assert.Empty(t, backend.lastAction)
		})
	}
}

func resultText(t *testing.T, result *mcplib.CallToolResult) string {
	t.Helper()
	require.NotEmpty(t, result.Content, "expected a text content block")
	textBlock, ok := result.Content[0].(mcplib.TextContent)
	require.True(t, ok, "expected a text content block")
	return textBlock.Text
}
