import { describe, expect, it } from "vitest";
import type { WorkflowStep } from "@/lib/types/http";
import { buildStepSectionSummaries, isStepSectionDirty } from "./workflow-step-section-summary";

const first = {
  id: "work",
  workflow_id: "wf",
  name: "Work",
  position: 0,
  color: "bg-blue-500",
  created_at: "",
  updated_at: "",
} as WorkflowStep;
const review = { ...first, id: "review", name: "Review", position: 1 };
const profiles = [{ id: "reviewer", label: "Reviewer" }];

describe("workflow step section summaries", () => {
  it("marks changed values even when the visible summary stays the same", () => {
    const saved = {
      ...first,
      events: { on_enter: [{ type: "run_script", config: { command: "echo before" } }] },
    } as WorkflowStep;
    const draft = {
      ...saved,
      events: { on_enter: [{ type: "run_script", config: { command: "echo after" } }] },
    } as WorkflowStep;
    expect(buildStepSectionSummaries(draft, [draft], profiles).automation).toEqual(
      buildStepSectionSummaries(saved, [saved], profiles).automation,
    );
    expect(isStepSectionDirty(draft, saved, "automation")).toBe(true);
    expect(isStepSectionDirty(draft, saved, "agent")).toBe(false);
    expect(isStepSectionDirty({ ...first, allow_manual_move: false }, first, "board")).toBe(true);
    expect(
      isStepSectionDirty({ ...first, prompt: "New instructions" }, first, "instructions"),
    ).toBe(true);
    expect(isStepSectionDirty({ ...first, auto_archive_after_hours: 4 }, first, "advanced")).toBe(
      true,
    );
    expect(isStepSectionDirty(first, undefined, "agent")).toBe(true);
  });
  it("makes inherited defaults and empty sections understandable", () => {
    const summary = buildStepSectionSummaries(first, [first, review], profiles);
    expect(summary.agent).toEqual([
      { key: "workflows:inheritedStepAgent" },
      { key: "workflows:manualAgentStart" },
    ]);
    expect(summary.instructions).toEqual([{ key: "workflows:noStepInstructions" }]);
    expect(summary.automation).toEqual([
      { key: "workflows:stepActionCount", values: { count: 0 } },
    ]);
    expect(summary.board).toEqual([
      { key: "workflows:allowManualMove" },
      { key: "workflows:showInCommandPanel" },
      { key: "workflows:noWipLimit" },
    ]);
    expect(summary.advanced).toEqual([{ key: "workflows:defaultStepSettings" }]);
  });
});

describe("configured workflow step summaries", () => {
  it("summarizes all lifecycle actions, profile policies, prompts, and board rules", () => {
    const step = {
      ...first,
      agent_profile_id: "reviewer",
      profile_session_start_policy: "new",
      profile_session_end_policy: "park",
      prompt: "  Implement the change.\nThen test.  ",
      is_start_step: true,
      wip_limit: 3,
      pull_from_step_id: review.id,
      auto_archive_after_hours: 24,
      auto_advance_requires_signal: true,
      events: {
        on_enter: [{ type: "auto_start_agent" }, { type: "enable_plan_mode" }],
        on_turn_start: [{ type: "move_to_next" }],
        on_turn_complete: [
          { type: "run_script", config: { command: "pnpm test" } },
          { type: "move_to_step", config: { step_id: review.id } },
        ],
        on_exit: [{ type: "run_script", config: { command: "echo exit" } }],
        on_children_completed: [{ type: "move_to_next" }],
      },
    } as WorkflowStep;
    const summary = buildStepSectionSummaries(step, [review, step], profiles);
    expect(summary.agent).toEqual([
      { key: "workflows:profileValue", values: { profile: "Reviewer" } },
      { key: "workflows:profileSessionStartNewShort" },
      { key: "workflows:profileSessionEndParkShort" },
      { key: "workflows:autoStartAgent" },
      { key: "workflows:planMode" },
    ]);
    expect(summary.instructions).toEqual([
      {
        key: "workflows:stepInstructionsPreview",
        values: { prompt: "Implement the change. Then test." },
      },
    ]);
    expect(summary.automation).toEqual([
      { key: "workflows:stepActionCount", values: { count: 7 } },
      { key: "workflows:stepCompletionDestination", values: { step: "Review" } },
    ]);
    expect(summary.board).toContainEqual({ key: "workflows:stepWipSummary", values: { count: 3 } });
    expect(summary.board).toContainEqual({
      key: "workflows:stepPullSummary",
      values: { step: "Review" },
    });
    expect(summary.advanced).toEqual([
      { key: "workflows:requireCompletionSignal" },
      { key: "workflows:stepArchiveSummary", values: { count: 24 } },
    ]);
  });

  it("never exposes identifiers for unavailable profiles or destinations", () => {
    const summary = buildStepSectionSummaries(
      {
        ...first,
        agent_profile_id: "secret-profile-id",
        pull_from_step_id: "missing-step-id",
        events: {
          on_turn_complete: [{ type: "move_to_step", config: { step_id: "missing-step-id" } }],
        },
      },
      [first],
      profiles,
    );
    expect(summary.agent[0]).toEqual({ key: "workflows:profileUnavailable" });
    expect(summary.automation[1]).toEqual({ key: "workflows:stepDestinationUnavailable" });
    expect(summary.board).toContainEqual({ key: "workflows:stepDestinationUnavailable" });
    expect(JSON.stringify(summary)).not.toContain("secret-profile-id");
    expect(JSON.stringify(summary)).not.toContain("missing-step-id");
  });
});

describe("workflow transition and session summaries", () => {
  it("resolves next/previous transitions by position and shows referenced sessions", () => {
    const next = buildStepSectionSummaries(
      {
        ...first,
        session_target: { kind: "initial" },
        events: { on_turn_complete: [{ type: "move_to_next" }] },
      },
      [review, first],
      profiles,
    );
    expect(next.agent[0]).toEqual({ key: "workflows:initialAgentSession" });
    expect(next.automation[1]).toEqual({
      key: "workflows:stepCompletionDestination",
      values: { step: "Review" },
    });
    const previous = buildStepSectionSummaries(
      {
        ...review,
        session_target: { kind: "step", step_id: first.id },
        events: { on_turn_complete: [{ type: "move_to_previous" }] },
      },
      [review, first],
      profiles,
    );
    expect(previous.agent[0]).toEqual({
      key: "workflows:stepSessionSummary",
      values: { step: "Work" },
    });
    expect(previous.automation[1]).toEqual({
      key: "workflows:stepCompletionDestination",
      values: { step: "Work" },
    });
  });
});
