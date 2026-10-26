package lifecycle

import (
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
)

func claudeForTimeout() agents.Agent { return agents.NewClaudeACP() }

func budgetMeta(v string) map[string]interface{} {
	return map[string]interface{}{MetadataKeyOfflineBudgetMinutes: v}
}

func TestToolTimeoutManagedDefaultFollowsBudget(t *testing.T) {
	cases := map[string]string{
		"":     "7200000",  // 15 min budget: default 120 min already exceeds 25 min
		"60":   "7200000",  // 70 min < 120 min
		"1440": "87000000", // 1450 min in ms
	}
	for budget, want := range cases {
		env := map[string]string{"MCP_TOOL_TIMEOUT": "7200000"}
		if err := applyToolTimeoutFloor(env, claudeForTimeout(), budgetMeta(budget)); err != nil {
			t.Fatalf("budget %q: %v", budget, err)
		}
		if env["MCP_TOOL_TIMEOUT"] != want {
			t.Errorf("budget %q: timeout = %s, want %s", budget, env["MCP_TOOL_TIMEOUT"], want)
		}
	}
}

func TestToolTimeoutAbsentIsFilledFromManagedDefault(t *testing.T) {
	env := map[string]string{}
	if err := applyToolTimeoutFloor(env, claudeForTimeout(), nil); err != nil {
		t.Fatal(err)
	}
	if env["MCP_TOOL_TIMEOUT"] != "7200000" {
		t.Errorf("timeout = %q", env["MCP_TOOL_TIMEOUT"])
	}
}

func TestToolTimeoutConfiguredValueIsValidatedAgainstBudget(t *testing.T) {
	// 15 minute budget: the value must be at least 16 minutes = 960000 ms.
	if err := applyToolTimeoutFloor(map[string]string{"MCP_TOOL_TIMEOUT": "960000"}, claudeForTimeout(), nil); err != nil {
		t.Errorf("exactly budget+1m must pass: %v", err)
	}
	err := applyToolTimeoutFloor(map[string]string{"MCP_TOOL_TIMEOUT": "959999"}, claudeForTimeout(), nil)
	if !errors.Is(err, ErrToolTimeoutBelowOfflineBudget) {
		t.Errorf("below budget+1m: %v", err)
	}
	for _, bad := range []string{"abc", "0", "-5", "1.5", ""} {
		err := applyToolTimeoutFloor(map[string]string{"MCP_TOOL_TIMEOUT": bad}, claudeForTimeout(), nil)
		if !errors.Is(err, ErrToolTimeoutInvalid) {
			t.Errorf("value %q: %v, want invalid", bad, err)
		}
	}
}

func TestToolTimeoutErrorsNameKeyAndBudgetButNotInvalidValue(t *testing.T) {
	err := applyToolTimeoutFloor(map[string]string{"MCP_TOOL_TIMEOUT": "secret-ref"}, claudeForTimeout(), budgetMeta("30"))
	if err == nil {
		t.Fatal("want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "MCP_TOOL_TIMEOUT") || !strings.Contains(msg, "30") || strings.Contains(msg, "secret-ref") {
		t.Errorf("message = %q", msg)
	}
}

func TestToolTimeoutNoFloorForHarnessWithoutKey(t *testing.T) {
	env := map[string]string{"X": "1"}
	if err := applyToolTimeoutFloor(env, agents.NewCodexACP(), nil); err != nil || len(env) != 1 {
		t.Errorf("env = %v err = %v", env, err)
	}
}

func TestToolTimeoutHugeConfiguredValueDoesNotOverflow(t *testing.T) {
	env := map[string]string{"MCP_TOOL_TIMEOUT": "9999999999999"}
	if err := applyToolTimeoutFloor(env, claudeForTimeout(), budgetMeta("15")); err != nil {
		t.Fatalf("a very large timeout must pass the floor: %v", err)
	}
	if env["MCP_TOOL_TIMEOUT"] != "9999999999999" {
		t.Errorf("configured value rewritten to %q", env["MCP_TOOL_TIMEOUT"])
	}
}
