import { describe, expect, it } from "vitest";
import type { WorkflowStep } from "@/lib/types/http";
import type { ConfigureSessionRule, OnTurnCompleteAction } from "@/lib/types/workflow-actions";
import { analyzeSessionConfigCarryForward } from "./session-config-carry-analysis";

function step(id: string, position: number, action?: Record<string, unknown>): WorkflowStep {
  return {
    id,
    workflow_id: "workflow-1" as WorkflowStep["workflow_id"],
    name: id,
    position,
    color: "bg-muted",
    events: action
      ? {
          on_enter: [
            {
              type: "configure_session",
              config: action as { rules: ConfigureSessionRule[] },
            },
          ],
        }
      : {},
    created_at: "",
    updated_at: "",
  };
}

function moveToNext(source: WorkflowStep): WorkflowStep {
  return withTurnComplete(source, [{ type: "move_to_next" }]);
}

function withTurnComplete(source: WorkflowStep, actions: OnTurnCompleteAction[]): WorkflowStep {
  return {
    ...source,
    events: { ...source.events, on_turn_complete: actions },
  };
}

describe("analyzeSessionConfigCarryForward", () => {
  it("warns when a later step can inherit a changed family configuration", () => {
    const warnings = analyzeSessionConfigCarryForward(
      [
        moveToNext(
          step("work", 0, {
            rules: [{ agent_name: "codex", operation: "set", model: "gpt-5.6-sol" }],
          }),
        ),
        step("review", 1),
      ],
      "review",
    );

    expect(warnings).toEqual([
      expect.objectContaining({
        agentName: "codex",
        sourceStepId: "work",
        model: "gpt-5.6-sol",
      }),
    ]);
  });

  it("does not warn when the later step explicitly keeps, restores, or sets values", () => {
    const base = moveToNext(
      step("work", 0, {
        rules: [{ agent_name: "codex", operation: "set", model: "gpt-5.6-sol" }],
      }),
    );
    expect(
      analyzeSessionConfigCarryForward(
        [base, step("keep", 1, { rules: [{ agent_name: "codex", operation: "keep" }] })],
        "keep",
      ),
    ).toEqual([]);
    expect(
      analyzeSessionConfigCarryForward(
        [
          base,
          step("restore", 1, { rules: [{ agent_name: "codex", operation: "restore_original" }] }),
        ],
        "restore",
      ),
    ).toEqual([]);
    expect(
      analyzeSessionConfigCarryForward(
        [
          base,
          step("set", 1, {
            rules: [
              {
                agent_name: "codex",
                operation: "set",
                config_options: { reasoning_effort: "max" },
              },
            ],
          }),
        ],
        "set",
      ),
    ).toEqual([]);
  });

  it("keeps a warning when another family is configured because the original family may still be active", () => {
    expect(
      analyzeSessionConfigCarryForward(
        [
          moveToNext(
            step("work", 0, { rules: [{ agent_name: "codex", operation: "set", model: "sol" }] }),
          ),
          step("review", 1, { rules: [{ agent_name: "claude", operation: "keep" }] }),
        ],
        "review",
      ),
    ).toEqual([expect.objectContaining({ agentName: "codex" })]);
  });

  it("follows explicit branches and converges through cycles", () => {
    const work = moveToNext(
      step("work", 0, { rules: [{ agent_name: "codex", operation: "set", model: "sol" }] }),
    );
    const review = withTurnComplete(step("review", 1), [
      { type: "move_to_step", config: { step_id: "done" } },
      { type: "move_to_previous" },
    ]);
    const done = step("done", 2);

    expect(analyzeSessionConfigCarryForward([work, review, done], "done")).toEqual([
      expect.objectContaining({ agentName: "codex", sourceStepId: "work" }),
    ]);
  });

  it("does not warn when every reachable branch restores before the target", () => {
    const work = moveToNext(
      step("work", 0, { rules: [{ agent_name: "codex", operation: "set", model: "sol" }] }),
    );
    const restoreBase = step("restore", 1, {
      rules: [{ agent_name: "codex", operation: "restore_original" }],
    });
    const restore = withTurnComplete(restoreBase, [{ type: "move_to_next" }]);
    expect(analyzeSessionConfigCarryForward([work, restore, step("done", 2)], "done")).toEqual([]);
  });
});
