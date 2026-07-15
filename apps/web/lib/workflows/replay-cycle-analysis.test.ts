import { describe, expect, it } from "vitest";
import { workflowId } from "@/lib/types/ids";
import type { WorkflowStep } from "@/lib/types/http";
import type {
  OnTurnCompleteAction,
  OnTurnStartAction,
  TransitionConfig,
} from "@/lib/types/workflow-actions";
import { analyzeWorkflowReplayCycles } from "./replay-cycle-analysis";

const NOW = "2026-07-15T00:00:00.000Z";
const SHORT_RETURN_STEP_ID = "short-return";

function step(
  id: string,
  position: number,
  options: {
    autoStart?: boolean;
    name?: string;
    prompt?: string;
    onTurnStart?: OnTurnStartAction[];
    onTurnComplete?: OnTurnCompleteAction[];
  } = {},
): WorkflowStep {
  return {
    id,
    workflow_id: workflowId("workflow-1"),
    name: options.name ?? id,
    position,
    color: "bg-neutral-400",
    prompt: options.prompt,
    events: {
      on_enter: options.autoStart ? [{ type: "auto_start_agent" }] : [],
      on_turn_start: options.onTurnStart,
      on_turn_complete: options.onTurnComplete,
    },
    created_at: NOW,
    updated_at: NOW,
  };
}

function moveToStep(target: string, config: TransitionConfig = {}): OnTurnCompleteAction {
  return { type: "move_to_step", config: { ...config, step_id: target } };
}

function guardedMoveToStep(target: string): OnTurnCompleteAction {
  return moveToStep(target, {
    if: { wait_for_quorum: { role: "reviewer", threshold: "all_approve" } },
  });
}

describe("replay cycle detection", () => {
  it("reports a fully automatic next/previous replay cycle as blocking", () => {
    const diagnostics = analyzeWorkflowReplayCycles([
      step("build", 0, {
        autoStart: true,
        onTurnComplete: [{ type: "move_to_next" }],
      }),
      step("review", 1, {
        autoStart: true,
        onTurnComplete: [{ type: "move_to_previous" }],
      }),
    ]);

    expect(diagnostics).toHaveLength(2);
    expect(diagnostics[0]).toEqual({
      identity: expect.any(String),
      severity: "blocking",
      autoStartStepId: "build",
      autoStartStepName: "build",
      affectedStepIds: ["build", "review"],
      trace: [
        {
          sourceStepId: "build",
          sourceStepName: "build",
          trigger: "on_turn_complete",
          actionKind: "move_to_next",
          destinationStepId: "review",
          destinationStepName: "review",
          requiresUserInvolvement: false,
        },
        {
          sourceStepId: "review",
          sourceStepName: "review",
          trigger: "on_turn_complete",
          actionKind: "move_to_previous",
          destinationStepId: "build",
          destinationStepName: "build",
          requiresUserInvolvement: false,
        },
      ],
      promptSource: "task_description",
    });
  });

  it("marks a replay path containing on_turn_start as user-mediated", () => {
    const [diagnostic] = analyzeWorkflowReplayCycles([
      step("work", 0, {
        autoStart: true,
        onTurnStart: [{ type: "move_to_next" }],
      }),
      step("review", 1, { onTurnComplete: [{ type: "move_to_previous" }] }),
    ]);

    expect(diagnostic.severity).toBe("warning");
    expect(diagnostic.trace.map((hop) => hop.requiresUserInvolvement)).toEqual([true, true]);
  });

  it("does not report the built-in Kanban return because on_turn_start skips on_enter", () => {
    const diagnostics = analyzeWorkflowReplayCycles([
      step("backlog", 0, {
        onTurnStart: [{ type: "move_to_next" }],
        onTurnComplete: [moveToStep("review")],
      }),
      step("in-progress", 1, {
        autoStart: true,
        onTurnComplete: [moveToStep("review")],
      }),
      step("review", 2, { onTurnStart: [{ type: "move_to_previous" }] }),
      step("done", 3, {
        onTurnStart: [{ type: "move_to_step", config: { step_id: "in-progress" } }],
      }),
    ]);

    expect(diagnostics).toEqual([]);
  });
});

describe("replay cycle severity", () => {
  it("marks an on_turn_complete hop from a non-auto-start source as user-mediated", () => {
    const [diagnostic] = analyzeWorkflowReplayCycles([
      step("work", 0, {
        autoStart: true,
        onTurnComplete: [{ type: "move_to_next" }],
      }),
      step("review", 1, { onTurnComplete: [{ type: "move_to_previous" }] }),
    ]);

    expect(diagnostic.severity).toBe("warning");
    expect(diagnostic.trace.map((hop) => hop.requiresUserInvolvement)).toEqual([false, true]);
  });

  it("marks an approval-gated hop as user-mediated", () => {
    const [diagnostic] = analyzeWorkflowReplayCycles([
      step("work", 0, {
        autoStart: true,
        onTurnComplete: [{ type: "move_to_next", config: { requires_approval: true } }],
      }),
      step("review", 1, {
        autoStart: true,
        onTurnComplete: [{ type: "move_to_previous" }],
      }),
    ]);

    expect(diagnostic.severity).toBe("warning");
    expect(diagnostic.trace.map((hop) => hop.requiresUserInvolvement)).toEqual([true, false]);
  });

  it("treats guarded transitions as possible automatic paths", () => {
    const [diagnostic] = analyzeWorkflowReplayCycles([
      step("work", 0, {
        autoStart: true,
        onTurnComplete: [
          {
            type: "move_to_next",
            config: { if: { wait_for_quorum: { role: "reviewer", threshold: "all_approve" } } },
          },
        ],
      }),
      step("review", 1, {
        autoStart: true,
        onTurnComplete: [{ type: "move_to_previous" }],
      }),
    ]);

    expect(diagnostic.severity).toBe("blocking");
  });
});

describe("replay transition resolution", () => {
  it("ignores lower-priority moves after an unconditional transition", () => {
    const diagnostics = analyzeWorkflowReplayCycles([
      step("work", 0, {
        autoStart: true,
        onTurnComplete: [{ type: "move_to_next" }, moveToStep("work")],
      }),
      step("done", 1),
    ]);

    expect(diagnostics).toEqual([]);
  });

  it("keeps fallback moves after a guarded transition", () => {
    const [diagnostic] = analyzeWorkflowReplayCycles([
      step("work", 0, {
        autoStart: true,
        onTurnComplete: [
          {
            type: "move_to_next",
            config: { if: { wait_for_quorum: { role: "reviewer", threshold: "all_approve" } } },
          },
          moveToStep("work"),
        ],
      }),
      step("done", 1),
    ]);

    expect(diagnostic.severity).toBe("blocking");
    expect(diagnostic.trace).toHaveLength(1);
    expect(diagnostic.trace[0].actionKind).toBe("move_to_step");
  });

  it("resolves explicit targets and ignores dangling targets", () => {
    const valid = analyzeWorkflowReplayCycles([
      step("work", 0, { autoStart: true, onTurnComplete: [moveToStep("review")] }),
      step("review", 1, {
        autoStart: true,
        onTurnComplete: [moveToStep("work")],
      }),
    ]);
    const dangling = analyzeWorkflowReplayCycles([
      step("work", 0, { autoStart: true, onTurnComplete: [moveToStep("missing")] }),
      step("review", 1, {
        autoStart: true,
        onTurnComplete: [moveToStep("work")],
      }),
    ]);

    expect(valid).toHaveLength(2);
    expect(dangling).toEqual([]);
  });

  it("ignores malformed explicit targets without throwing", () => {
    const malformed = { type: "move_to_step" } as OnTurnCompleteAction;

    expect(
      analyzeWorkflowReplayCycles([
        step("work", 0, { autoStart: true, onTurnComplete: [malformed] }),
      ]),
    ).toEqual([]);
  });

  it("uses position order for relative transitions after a reorder", () => {
    const before = analyzeWorkflowReplayCycles([
      step("work", 0, {
        autoStart: true,
        onTurnComplete: [{ type: "move_to_next" }],
      }),
      step("review", 1, {
        autoStart: true,
        onTurnComplete: [{ type: "move_to_previous" }],
      }),
      step("done", 2),
    ]);
    const after = analyzeWorkflowReplayCycles([
      step("work", 0, {
        autoStart: true,
        onTurnComplete: [{ type: "move_to_next" }],
      }),
      step("review", 2, {
        autoStart: true,
        onTurnComplete: [{ type: "move_to_previous" }],
      }),
      step("done", 1),
    ]);

    expect(before.map((diagnostic) => diagnostic.autoStartStepId)).toEqual(["work", "review"]);
    expect(after).toEqual([]);
  });

  it("does not report cycles that never re-enter an auto-start step", () => {
    const diagnostics = analyzeWorkflowReplayCycles([
      step("backlog", 0, { onTurnComplete: [{ type: "move_to_next" }] }),
      step("review", 1, { onTurnComplete: [{ type: "move_to_previous" }] }),
    ]);

    expect(diagnostics).toEqual([]);
  });
});

describe("replay cycle search bounds", () => {
  it("selects deterministically without enumerating every path in a dense branching graph", () => {
    const layerCount = 16;
    const branchCount = 3;
    const layers = Array.from({ length: layerCount }, (_, layerIndex) =>
      Array.from(
        { length: branchCount },
        (_, branchIndex) => `layer-${String(layerIndex).padStart(2, "0")}-${branchIndex}`,
      ),
    );
    const denseSteps = [
      step("work", 0, {
        autoStart: true,
        onTurnComplete: [...layers[0]].reverse().map(guardedMoveToStep),
      }),
      ...layers.flatMap((layer, layerIndex) =>
        layer.map((id, branchIndex) =>
          step(id, layerIndex * branchCount + branchIndex + 1, {
            onTurnComplete:
              layerIndex === layerCount - 1
                ? [moveToStep("work")]
                : [...layers[layerIndex + 1]].reverse().map(guardedMoveToStep),
          }),
        ),
      ),
    ];

    const [diagnostic] = analyzeWorkflowReplayCycles(denseSteps);

    expect(diagnostic.trace.map((hop) => hop.destinationStepId)).toEqual([
      ...layers.map(([lexicallyFirst]) => lexicallyFirst),
      "work",
    ]);
  });
});

describe("replay diagnostic selection", () => {
  it("selects a blocking trace before a warning trace", () => {
    const [diagnostic] = analyzeWorkflowReplayCycles([
      step("work", 0, {
        autoStart: true,
        onTurnStart: [{ type: "move_to_step", config: { step_id: "manual-return" } }],
        onTurnComplete: [moveToStep(SHORT_RETURN_STEP_ID)],
      }),
      step("manual-return", 1, { onTurnComplete: [moveToStep("work")] }),
      step(SHORT_RETURN_STEP_ID, 2, {
        autoStart: true,
        onTurnComplete: [moveToStep("work")],
      }),
    ]);

    expect(diagnostic.severity).toBe("blocking");
    expect(diagnostic.trace.map((hop) => hop.destinationStepId)).toEqual([
      SHORT_RETURN_STEP_ID,
      "work",
    ]);
  });

  it("selects the shortest trace when candidates have equal severity", () => {
    const [diagnostic] = analyzeWorkflowReplayCycles([
      step("work", 0, {
        autoStart: true,
        onTurnStart: [{ type: "move_to_step", config: { step_id: SHORT_RETURN_STEP_ID } }],
        onTurnComplete: [moveToStep("long-a")],
      }),
      step(SHORT_RETURN_STEP_ID, 1, { onTurnComplete: [moveToStep("work")] }),
      step("long-a", 2, { onTurnComplete: [moveToStep("long-b")] }),
      step("long-b", 3, {
        autoStart: true,
        onTurnComplete: [moveToStep("work")],
      }),
    ]);

    expect(diagnostic.severity).toBe("warning");
    expect(diagnostic.trace.map((hop) => hop.destinationStepId)).toEqual([
      SHORT_RETURN_STEP_ID,
      "work",
    ]);
  });

  it("keeps identity stable across names and prompt changes", () => {
    const original = analyzeWorkflowReplayCycles([
      step("work", 0, { autoStart: true, onTurnComplete: [moveToStep("review")] }),
      step("review", 1, {
        autoStart: true,
        onTurnComplete: [moveToStep("work")],
      }),
    ]);
    const renamed = analyzeWorkflowReplayCycles([
      step("work", 0, {
        autoStart: true,
        name: "Implementation",
        prompt: "Use {{task_prompt}} carefully",
        onTurnComplete: [moveToStep("review")],
      }),
      step("review", 1, {
        autoStart: true,
        name: "Code review",
        onTurnComplete: [moveToStep("work")],
      }),
    ]);

    expect(renamed.map((diagnostic) => diagnostic.identity)).toEqual(
      original.map((diagnostic) => diagnostic.identity),
    );
  });

  it.each([
    [undefined, "task_description"],
    ["", "task_description"],
    ["Before\n{{task_prompt}}\nAfter", "step_prompt_with_task_description"],
    ["Review the implementation", "step_prompt"],
  ] as const)("classifies prompt %j as %s", (prompt, expected) => {
    const [diagnostic] = analyzeWorkflowReplayCycles([
      step("work", 0, {
        autoStart: true,
        prompt,
        onTurnComplete: [moveToStep("work")],
      }),
    ]);

    expect(diagnostic.promptSource).toBe(expected);
  });

  it("uses input order to break equal-position ties and returns diagnostics in that order", () => {
    const steps = [
      step("second", 0, { autoStart: true, onTurnComplete: [moveToStep("second")] }),
      step("first", 0, { autoStart: true, onTurnComplete: [moveToStep("first")] }),
    ];

    expect(
      analyzeWorkflowReplayCycles(steps).map((diagnostic) => diagnostic.autoStartStepId),
    ).toEqual(["second", "first"]);
  });
});
