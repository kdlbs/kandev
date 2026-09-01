import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  agentProfileId,
  workflowId,
  workspaceId,
  type Workflow,
  type WorkflowStep,
} from "@/lib/types/http";
import { createWorkflowDuplication, getWorkflowCopyName } from "./workflow-duplication";

const workspace = workspaceId("workspace-1");
const REVIEW_STEP_ID = "step-review";
const DONE_STEP_ID = "step-done";
const REVIEW_COPY_NAME = "Review (copy)";
const REVIEW_COPY_TWO_NAME = "Review (copy 2)";

function sourceWorkflow(name = "Review"): Workflow {
  return {
    id: workflowId("workflow-source"),
    workspace_id: workspace,
    name,
    description: "Review configuration",
    prompt: "Review the task",
    agent_profile_id: agentProfileId("agent-source"),
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-02T00:00:00Z",
  };
}

function sourceSteps(): WorkflowStep[] {
  return [
    {
      id: REVIEW_STEP_ID,
      workflow_id: workflowId("workflow-source"),
      name: "Review",
      position: 0,
      color: "bg-purple-500",
      prompt: "Review this task",
      stage_type: "review",
      events: {
        on_turn_complete: [
          { type: "move_to_step", config: { step_id: DONE_STEP_ID, requires_approval: true } },
        ],
      },
      allow_manual_move: false,
      is_start_step: true,
      show_in_command_panel: true,
      auto_archive_after_hours: 24,
      agent_profile_id: "step-agent",
      profile_session_start_policy: "reuse",
      profile_session_end_policy: "complete",
      auto_advance_requires_signal: true,
      cancel_triggers_turn_complete: true,
      wip_limit: 2,
      pull_from_step_id: null,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-02T00:00:00Z",
    },
    {
      id: DONE_STEP_ID,
      workflow_id: workflowId("workflow-source"),
      name: "Done",
      position: 1,
      color: "bg-green-500",
      events: {
        on_children_completed: [{ type: "move_to_step", config: { step_id: REVIEW_STEP_ID } }],
      },
      allow_manual_move: true,
      is_start_step: false,
      show_in_command_panel: false,
      auto_archive_after_hours: 0,
      agent_profile_id: "",
      auto_advance_requires_signal: false,
      cancel_triggers_turn_complete: false,
      wip_limit: 0,
      pull_from_step_id: REVIEW_STEP_ID,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-02T00:00:00Z",
    },
  ];
}

let uuidCounter = 0;

beforeEach(() => {
  uuidCounter = 0;
  vi.spyOn(crypto, "randomUUID").mockImplementation(() => {
    return `00000000-0000-4000-8000-${String(uuidCounter++).padStart(12, "0")}`;
  });
});

afterEach(() => vi.restoreAllMocks());

describe("getWorkflowCopyName", () => {
  it.each([
    {
      name: "Review",
      existing: ["Review"],
      expected: REVIEW_COPY_NAME,
    },
    {
      name: "Review",
      existing: ["Review", REVIEW_COPY_NAME],
      expected: REVIEW_COPY_TWO_NAME,
    },
    {
      name: REVIEW_COPY_TWO_NAME,
      existing: ["Review", REVIEW_COPY_NAME, REVIEW_COPY_TWO_NAME],
      expected: "Review (copy 3)",
    },
    {
      name: "Review",
      existing: ["Review", REVIEW_COPY_NAME, "Review (copy 3)"],
      expected: REVIEW_COPY_TWO_NAME,
    },
  ])("chooses the lowest available suffix for $name", ({ name, existing, expected }) => {
    expect(
      getWorkflowCopyName(
        name,
        existing.map((item) => ({ name: item })),
      ),
    ).toBe(expected);
  });
});

describe("createWorkflowDuplication", () => {
  it("copies editable metadata and remaps every internal step reference", () => {
    const source = sourceWorkflow();
    const steps = sourceSteps();
    const result = createWorkflowDuplication(source, [source], steps);

    expect(result.workflow).toMatchObject({
      workspace_id: source.workspace_id,
      name: REVIEW_COPY_NAME,
      description: source.description,
      prompt: source.prompt,
      agent_profile_id: source.agent_profile_id,
      created_at: "",
      updated_at: "",
    });
    expect(result.workflow.id).toMatch(/^temp-workflow-/);
    expect(result.workflow).not.toHaveProperty("workflow_template_id");
    expect(result.workflow).not.toHaveProperty("source");
    expect(result.workflow).not.toHaveProperty("source_path");

    const [copiedReview, copiedDone] = result.steps;
    expect(copiedReview).toMatchObject({
      name: "Review",
      position: 0,
      color: "bg-purple-500",
      prompt: "Review this task",
      stage_type: "review",
      allow_manual_move: false,
      is_start_step: true,
      show_in_command_panel: true,
      auto_archive_after_hours: 24,
      agent_profile_id: "step-agent",
      profile_session_start_policy: "reuse",
      profile_session_end_policy: "complete",
      auto_advance_requires_signal: true,
      cancel_triggers_turn_complete: true,
      wip_limit: 2,
      pull_from_step_id: null,
      created_at: "",
      updated_at: "",
      workflow_id: result.workflow.id,
    });
    expect(copiedDone.pull_from_step_id).toBe(copiedReview.id);
    expect(copiedReview.events?.on_turn_complete).toEqual([
      {
        type: "move_to_step",
        config: { step_id: copiedDone.id, requires_approval: true },
      },
    ]);
    expect(copiedDone.events?.on_children_completed).toEqual([
      { type: "move_to_step", config: { step_id: copiedReview.id } },
    ]);
    expect(result.steps.map((step) => step.id)).not.toContain(REVIEW_STEP_ID);
    expect(result.steps.map((step) => step.id)).not.toContain(DONE_STEP_ID);
  });

  it("returns a deeply independent graph", () => {
    const source = sourceWorkflow();
    const steps = sourceSteps();
    const result = createWorkflowDuplication(source, [source], steps);

    result.workflow.description = "Changed copy";
    (
      result.steps[0].events!.on_turn_complete![0] as { config: { step_id: string } }
    ).config.step_id = "changed";
    result.steps[1].name = "Changed step";

    expect(source.description).toBe("Review configuration");
    expect(
      (steps[0].events?.on_turn_complete?.[0] as { config: { step_id: string } }).config.step_id,
    ).toBe(DONE_STEP_ID);
    expect(steps[1].name).toBe("Done");
  });
});
