import { describe, expect, it } from "vitest";

import {
  createWorkflowAction,
  getWorkflowActionCatalog,
  normalizeWorkflowScriptAction,
  validateWorkflowScriptAction,
} from "./workflow-action-catalog";

describe("workflow action catalog", () => {
  it("lists only actions compatible with the selected lifecycle trigger", () => {
    expect(getWorkflowActionCatalog("on_enter").map((item) => item.type)).toEqual([
      "enable_plan_mode",
      "auto_start_agent",
      "reset_agent_context",
      "configure_session",
      "set_session_mode",
      "clear_decisions",
      "queue_run_for_each_participant",
      "queue_run",
      "ensure_participant_seat",
      "run_code_review",
      "run_script",
    ]);
    expect(getWorkflowActionCatalog("on_turn_complete").map((item) => item.type)).toEqual([
      "move_to_next",
      "move_to_previous",
      "move_to_step",
      "disable_plan_mode",
      "run_script",
    ]);
    expect(getWorkflowActionCatalog("on_exit").map((item) => item.type)).toEqual([
      "disable_plan_mode",
      "run_script",
    ]);
  });

  it("creates a portable script action with documented defaults", () => {
    expect(createWorkflowAction("on_enter", "run_script")).toEqual({
      type: "run_script",
      config: {
        command: "",
        timeout_seconds: 600,
        failure_policy: "block",
      },
    });
  });

  it("normalizes a script without dropping portable extension fields", () => {
    const action = {
      type: "run_script",
      config: { command: "  ./check.sh  ", future_field: "keep-me" },
    };
    expect(normalizeWorkflowScriptAction(action)).toEqual({
      type: "run_script",
      config: {
        command: "  ./check.sh  ",
        timeout_seconds: 600,
        failure_policy: "block",
        future_field: "keep-me",
      },
    });
  });

  it("validates command, timeout, and failure policy before save", () => {
    expect(validateWorkflowScriptAction({ type: "run_script", config: {} })).toMatchObject({
      valid: false,
      errorKey: "workflows:scriptCommandRequired",
    });
    expect(
      validateWorkflowScriptAction({
        type: "run_script",
        config: { command: "echo ok", timeout_seconds: 0 },
      }),
    ).toMatchObject({ valid: false, errorKey: "workflows:scriptTimeoutInvalid" });
    expect(
      validateWorkflowScriptAction({
        type: "run_script",
        config: { command: "echo ok", failure_policy: "retry" },
      }),
    ).toMatchObject({ valid: false, errorKey: "workflows:scriptFailurePolicyInvalid" });
    expect(
      validateWorkflowScriptAction({
        type: "run_script",
        config: { command: "echo ok", timeout_seconds: 30, failure_policy: "continue" },
      }),
    ).toEqual({ valid: true });
  });
});
