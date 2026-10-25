import { beforeEach, describe, expect, it, vi } from "vitest";
import { updateWorkflowAction, updateWorkflowStepAction } from "@/app/actions/workspaces";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import { createWorkflowDraftSaveProgress, persistWorkflowDraft } from "./workflow-card-actions";

vi.mock("@/app/actions/workspaces", () => ({
  bulkMoveTasks: vi.fn(),
  createWorkflowAction: vi.fn(),
  createWorkflowStepAction: vi.fn(),
  deleteWorkflowStepAction: vi.fn(),
  getStepTaskCount: vi.fn(),
  getWorkflowTaskCount: vi.fn(),
  listWorkflowStepsAction: vi.fn(),
  reorderWorkflowStepsAction: vi.fn(),
  updateWorkflowAction: vi.fn(),
  updateWorkflowStepAction: vi.fn(),
}));

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(updateWorkflowStepAction).mockImplementation(async (_stepID, payload) => {
    if (payload.session_target != null && payload.agent_profile_id) {
      throw new Error("session_target cannot be combined with agent_profile_id");
    }
    return {} as WorkflowStep;
  });
});

const workflow = {
  id: "wf-1",
  workspace_id: "ws-1",
  name: "Workflow",
  created_at: "",
  updated_at: "",
} as Workflow;
const SOURCE_PROFILE_ID = "profile-source";

function step(id: string, name: string, position: number, isStartStep: boolean): WorkflowStep {
  return {
    id,
    workflow_id: workflow.id,
    name,
    position,
    color: "bg-slate-500",
    allow_manual_move: true,
    is_start_step: isStartStep,
    created_at: "",
    updated_at: "",
  };
}

describe("persistWorkflowDraft dependency ordering", () => {
  it("repairs dependent targets before persisting a source profile change", async () => {
    vi.mocked(updateWorkflowAction).mockResolvedValue(workflow);
    const sourceSaved = { ...step("source", "Implement", 0, true), agent_profile_id: "profile-a" };
    const dependentSaved = {
      ...step("dependent", "Review", 1, false),
      session_target: { kind: "step" as const, step_id: "source" },
    };
    await persistWorkflowDraft({
      workflow,
      draftSteps: [
        { ...sourceSaved, agent_profile_id: "" },
        { ...dependentSaved, session_target: null },
      ],
      savedSteps: [sourceSaved, dependentSaved],
      progress: createWorkflowDraftSaveProgress(),
    });
    expect(vi.mocked(updateWorkflowStepAction).mock.calls[0]).toEqual([
      "dependent",
      { session_target: null },
    ]);
    expect(updateWorkflowStepAction).toHaveBeenCalledWith(
      "source",
      expect.objectContaining({ agent_profile_id: "" }),
    );
  });

  it("clears an existing direct profile with a newly attached target", async () => {
    vi.mocked(updateWorkflowAction).mockResolvedValue(workflow);
    const saved = {
      ...step("step-review", "Review", 1, false),
      agent_profile_id: "profile-review",
      session_target: null,
    };
    const draft = { ...saved, agent_profile_id: "", session_target: { kind: "initial" as const } };
    await persistWorkflowDraft({
      workflow,
      draftSteps: [draft],
      savedSteps: [saved],
      progress: createWorkflowDraftSaveProgress(),
    });
    expect(updateWorkflowStepAction).toHaveBeenCalledOnce();
    expect(updateWorkflowStepAction).toHaveBeenCalledWith(
      "step-review",
      expect.objectContaining({ session_target: { kind: "initial" }, agent_profile_id: "" }),
    );
  });
});

describe("persistWorkflowDraft dependency ordering", () => {
  it("persists a newly enabled source profile before attaching its target", async () => {
    vi.mocked(updateWorkflowAction).mockResolvedValue(workflow);
    const sourceSaved = { ...step("source", "Implement", 0, true), agent_profile_id: "" };
    const dependentSaved = { ...step("dependent", "Review", 1, false), session_target: null };
    const sourceDraft = { ...sourceSaved, agent_profile_id: SOURCE_PROFILE_ID };
    const dependentDraft = {
      ...dependentSaved,
      session_target: { kind: "step" as const, step_id: "source" },
    };
    await persistWorkflowDraft({
      workflow,
      draftSteps: [sourceDraft, dependentDraft],
      savedSteps: [sourceSaved, dependentSaved],
      progress: createWorkflowDraftSaveProgress(),
    });
    const calls = vi.mocked(updateWorkflowStepAction).mock.calls;
    expect(calls[0]).toEqual([
      "source",
      expect.objectContaining({ agent_profile_id: SOURCE_PROFILE_ID }),
    ]);
    expect(calls.at(-1)).toEqual([
      "dependent",
      expect.objectContaining({
        agent_profile_id: "",
        session_target: { kind: "step", step_id: "source" },
      }),
    ]);
  });

  it("detaches an old target before enabling its new source profile", async () => {
    vi.mocked(updateWorkflowAction).mockResolvedValue(workflow);
    const sourceSaved = { ...step("source", "Implement", 0, true), agent_profile_id: "" };
    const oldSourceSaved = {
      ...step("old-source", "Plan", 1, false),
      agent_profile_id: "profile-old",
    };
    const dependentSaved = {
      ...step("dependent", "Review", 2, false),
      session_target: { kind: "step" as const, step_id: "old-source" },
    };
    const sourceDraft = { ...sourceSaved, agent_profile_id: SOURCE_PROFILE_ID };
    const dependentDraft = {
      ...dependentSaved,
      session_target: { kind: "step" as const, step_id: "source" },
    };
    await persistWorkflowDraft({
      workflow,
      draftSteps: [sourceDraft, oldSourceSaved, dependentDraft],
      savedSteps: [sourceSaved, oldSourceSaved, dependentSaved],
      progress: createWorkflowDraftSaveProgress(),
    });
    const calls = vi.mocked(updateWorkflowStepAction).mock.calls;
    expect(calls[0]).toEqual(["dependent", { session_target: null }]);
    expect(calls[1]).toEqual([
      "source",
      expect.objectContaining({ agent_profile_id: SOURCE_PROFILE_ID }),
    ]);
    expect(calls.at(-1)).toEqual([
      "dependent",
      expect.objectContaining({ session_target: { kind: "step", step_id: "source" } }),
    ]);
  });
});
