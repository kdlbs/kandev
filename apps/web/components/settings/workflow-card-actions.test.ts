import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  createWorkflowAction,
  createWorkflowStepAction,
  deleteWorkflowStepAction,
  getWorkflowTaskCount,
  getStepTaskCount,
  reorderWorkflowStepsAction,
  updateWorkflowAction,
  updateWorkflowStepAction,
} from "@/app/actions/workspaces";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import {
  useWorkflowDeleteHandlers,
  useStepDeleteHandlers,
  createWorkflowDraftSaveProgress,
  persistWorkflowDraft,
  useWorkflowStepActions,
} from "./workflow-card-actions";

vi.mock("@/app/actions/workspaces", () => ({
  createWorkflowAction: vi.fn(),
  createWorkflowStepAction: vi.fn(),
  deleteWorkflowAction: vi.fn(),
  updateWorkflowAction: vi.fn(),
  updateWorkflowStepAction: vi.fn(),
  deleteWorkflowStepAction: vi.fn(),
  reorderWorkflowStepsAction: vi.fn(),
  listWorkflowStepsAction: vi.fn(),
  getStepTaskCount: vi.fn(),
  getWorkflowTaskCount: vi.fn(),
  exportWorkflowAction: vi.fn(),
  bulkMoveTasks: vi.fn(),
}));

beforeEach(() => {
  vi.clearAllMocks();
});

const workflow = {
  id: "wf-1",
  workspace_id: "ws-1",
  name: "Workflow",
  created_at: "",
  updated_at: "",
} as Workflow;
const CLIENT_WORKFLOW_ID = "temp-workflow-1";
const CLIENT_STEP_ONE = "temp-step-1";
const CLIENT_STEP_TWO = "temp-step-2";
const SERVER_STEP_ONE = "server-step-1";
const SERVER_STEP_TWO = "server-step-2";

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

function renderReadOnlyWorkflowStepActions(initialSteps: WorkflowStep[]) {
  let steps = initialSteps;
  const setWorkflowSteps = vi.fn(
    (updater: ((prev: WorkflowStep[]) => WorkflowStep[]) | WorkflowStep[]) => {
      steps = typeof updater === "function" ? updater(steps) : updater;
    },
  );
  const refreshWorkflowSteps = vi.fn();
  const view = renderHook(() =>
    useWorkflowStepActions({
      workflow,
      readOnly: true,
      workflowSteps: steps,
      setWorkflowSteps,
      refreshWorkflowSteps,
      setStepToDelete: vi.fn(),
      setStepTaskCount: vi.fn(),
      setTargetStepForMigration: vi.fn(),
      setStepDeleteOpen: vi.fn(),
      toast: vi.fn(),
    }),
  );
  return { ...view, getSteps: () => steps, refreshWorkflowSteps, setWorkflowSteps };
}

describe("useWorkflowStepActions", () => {
  it("updates a step locally without calling persistence APIs", async () => {
    const original = step("step-1", "Todo", 0, true);
    const setWorkflowSteps = vi.fn();
    const { result } = renderHook(() =>
      useWorkflowStepActions({
        workflow,
        readOnly: false,
        workflowSteps: [original],
        setWorkflowSteps,
        refreshWorkflowSteps: vi.fn(),
        setStepToDelete: vi.fn(),
        setStepTaskCount: vi.fn(),
        setTargetStepForMigration: vi.fn(),
        setStepDeleteOpen: vi.fn(),
        toast: vi.fn(),
      }),
    );

    await act(async () => {
      await result.current.handleUpdateWorkflowStep("step-1", { name: "Renamed" });
    });

    const updater = setWorkflowSteps.mock.calls[0][0] as (steps: WorkflowStep[]) => WorkflowStep[];
    expect(updater([original])[0].name).toBe("Renamed");
    expect(updateWorkflowStepAction).not.toHaveBeenCalled();
  });

  it("adds a client-only step without calling persistence APIs", async () => {
    const setWorkflowSteps = vi.fn();
    const { result } = renderHook(() =>
      useWorkflowStepActions({
        workflow,
        workflowSteps: [step("step-1", "Todo", 0, true)],
        setWorkflowSteps,
        refreshWorkflowSteps: vi.fn(),
        setStepToDelete: vi.fn(),
        setStepTaskCount: vi.fn(),
        setTargetStepForMigration: vi.fn(),
        setStepDeleteOpen: vi.fn(),
        toast: vi.fn(),
      }),
    );

    await act(() => result.current.handleAddWorkflowStep());

    const updater = setWorkflowSteps.mock.calls[0][0] as (steps: WorkflowStep[]) => WorkflowStep[];
    expect(updater([])[0]).toMatchObject({
      id: expect.stringMatching(/^temp-step-/),
      name: "New Step",
    });
    expect(createWorkflowStepAction).not.toHaveBeenCalled();
  });
});

describe("useWorkflowStepActions destructive and ordering paths", () => {
  it("refuses to add, update, remove, or reorder steps when readOnly", async () => {
    const initialSteps = [step("step-1", "Todo", 0, true), step("step-2", "Plan", 1, false)];
    const { result, getSteps, refreshWorkflowSteps } =
      renderReadOnlyWorkflowStepActions(initialSteps);

    await act(async () => {
      await result.current.handleAddWorkflowStep();
    });
    await act(async () => {
      await result.current.handleUpdateWorkflowStep("step-2", { name: "Renamed" });
    });
    await act(async () => {
      await result.current.handleRemoveWorkflowStep("step-1");
    });
    await act(async () => {
      await result.current.handleReorderWorkflowSteps([initialSteps[1], initialSteps[0]]);
    });

    expect(getSteps()).toEqual(initialSteps);
    expect(createWorkflowStepAction).not.toHaveBeenCalled();
    expect(updateWorkflowStepAction).not.toHaveBeenCalled();
    expect(deleteWorkflowStepAction).not.toHaveBeenCalled();
    expect(reorderWorkflowStepsAction).not.toHaveBeenCalled();
    expect(refreshWorkflowSteps).not.toHaveBeenCalled();
  });

  it("opens the task migration dialog without reporting a saved mutation", async () => {
    vi.mocked(getStepTaskCount).mockResolvedValue({ task_count: 2 });
    const setStepDeleteOpen = vi.fn();
    const { result } = renderHook(() =>
      useWorkflowStepActions({
        workflow,
        readOnly: false,
        workflowSteps: [step("step-1", "Todo", 0, true), step("step-2", "Done", 1, false)],
        setWorkflowSteps: vi.fn(),
        refreshWorkflowSteps: vi.fn(),
        setStepToDelete: vi.fn(),
        setStepTaskCount: vi.fn(),
        setTargetStepForMigration: vi.fn(),
        setStepDeleteOpen,
        toast: vi.fn(),
      }),
    );

    await act(async () => {
      await result.current.handleRemoveWorkflowStep("step-1");
    });

    expect(setStepDeleteOpen).toHaveBeenCalledWith(true);
    expect(result.current.status).toBe("idle");
  });

  it("requires confirmation before deleting a persisted step with no tasks", async () => {
    vi.mocked(getStepTaskCount).mockResolvedValue({ task_count: 0 });
    const setStepDeleteOpen = vi.fn();
    const { result } = renderHook(() =>
      useWorkflowStepActions({
        workflow,
        workflowSteps: [step("step-1", "Todo", 0, true)],
        setWorkflowSteps: vi.fn(),
        refreshWorkflowSteps: vi.fn(),
        setStepToDelete: vi.fn(),
        setStepTaskCount: vi.fn(),
        setTargetStepForMigration: vi.fn(),
        setStepDeleteOpen,
        toast: vi.fn(),
      }),
    );

    await act(() => result.current.handleRemoveWorkflowStep("step-1"));

    expect(setStepDeleteOpen).toHaveBeenCalledWith(true);
    expect(deleteWorkflowStepAction).not.toHaveBeenCalled();
  });

  it("reorders locally without calling persistence APIs", async () => {
    const originalSteps = [step("step-1", "Todo", 0, true), step("step-2", "Done", 1, false)];
    const reorderedSteps = [originalSteps[1], originalSteps[0]];
    const setWorkflowSteps = vi.fn();
    const { result } = renderHook(() =>
      useWorkflowStepActions({
        workflow,
        workflowSteps: originalSteps,
        setWorkflowSteps,
        refreshWorkflowSteps: vi.fn().mockResolvedValue(undefined),
        setStepToDelete: vi.fn(),
        setStepTaskCount: vi.fn(),
        setTargetStepForMigration: vi.fn(),
        setStepDeleteOpen: vi.fn(),
        toast: vi.fn(),
      }),
    );

    await act(async () => {
      await result.current.handleReorderWorkflowSteps(reorderedSteps);
    });
    expect(setWorkflowSteps).toHaveBeenCalledWith(
      reorderedSteps.map((item, position) => ({ ...item, position })),
    );
    expect(reorderWorkflowStepsAction).not.toHaveBeenCalled();
  });
});

describe("persistWorkflowDraft", () => {
  const persistedWorkflow = { ...workflow, id: "wf-created" } as Workflow;

  beforeEach(() => {
    vi.mocked(createWorkflowAction).mockResolvedValue(persistedWorkflow);
    vi.mocked(updateWorkflowAction).mockResolvedValue(persistedWorkflow);
    vi.mocked(updateWorkflowStepAction).mockImplementation(async (id, updates) => ({
      ...step(id, updates.name ?? "Step", updates.position ?? 0, updates.is_start_step ?? false),
      ...updates,
    }));
    vi.mocked(reorderWorkflowStepsAction).mockResolvedValue({ steps: [], total: 0 });
  });

  it("does not duplicate a workflow or successful steps when a partial create is retried", async () => {
    const draftWorkflow = { ...workflow, id: CLIENT_WORKFLOW_ID } as Workflow;
    const drafts = [
      step(CLIENT_STEP_ONE, "Todo", 0, true),
      step(CLIENT_STEP_TWO, "Done", 1, false),
    ].map((item) => ({ ...item, workflow_id: draftWorkflow.id }) as WorkflowStep);
    vi.mocked(createWorkflowStepAction)
      .mockResolvedValueOnce(step(SERVER_STEP_ONE, "Todo", 0, true))
      .mockRejectedValueOnce(new Error("network down"))
      .mockResolvedValueOnce(step(SERVER_STEP_TWO, "Done", 1, false));
    const progress = createWorkflowDraftSaveProgress();

    await expect(
      persistWorkflowDraft({
        workflow: draftWorkflow,
        draftSteps: drafts,
        savedSteps: [],
        progress,
      }),
    ).rejects.toThrow("network down");
    await persistWorkflowDraft({
      workflow: draftWorkflow,
      draftSteps: drafts,
      savedSteps: [],
      progress,
    });

    expect(createWorkflowAction).toHaveBeenCalledOnce();
    expect(createWorkflowStepAction).toHaveBeenCalledTimes(3);
    expect(progress.stepIds.get(CLIENT_STEP_ONE)).toBe(SERVER_STEP_ONE);
    expect(progress.stepIds.get(CLIENT_STEP_TWO)).toBe(SERVER_STEP_TWO);
  });

  it("remaps draft step references before updating the server", async () => {
    const draftWorkflow = { ...workflow, id: CLIENT_WORKFLOW_ID } as Workflow;
    const drafts = [
      {
        ...step(CLIENT_STEP_ONE, "Todo", 0, true),
        workflow_id: draftWorkflow.id,
        events: {
          on_turn_complete: [{ type: "move_to_step", config: { step_id: CLIENT_STEP_TWO } }],
        },
      },
      {
        ...step(CLIENT_STEP_TWO, "Done", 1, false),
        workflow_id: draftWorkflow.id,
        pull_from_step_id: CLIENT_STEP_ONE,
      },
    ] as WorkflowStep[];
    vi.mocked(createWorkflowStepAction)
      .mockResolvedValueOnce(step(SERVER_STEP_ONE, "Todo", 0, true))
      .mockResolvedValueOnce(step(SERVER_STEP_TWO, "Done", 1, false));

    await persistWorkflowDraft({
      workflow: draftWorkflow,
      draftSteps: drafts,
      savedSteps: [],
      progress: createWorkflowDraftSaveProgress(),
    });

    expect(updateWorkflowStepAction).toHaveBeenCalledWith(
      SERVER_STEP_ONE,
      expect.objectContaining({
        events: {
          on_turn_complete: [{ type: "move_to_step", config: { step_id: SERVER_STEP_TWO } }],
        },
      }),
    );
    expect(updateWorkflowStepAction).toHaveBeenCalledWith(
      SERVER_STEP_TWO,
      expect.objectContaining({ pull_from_step_id: SERVER_STEP_ONE }),
    );
  });
});

describe("useWorkflowDeleteHandlers", () => {
  it("refuses to open the delete-workflow dialog when readOnly", async () => {
    const wfDel = {
      setDeleteOpen: vi.fn(),
      setWorkflowTaskCount: vi.fn(),
      setWorkflowDeleteLoading: vi.fn(),
      setTargetWorkflowId: vi.fn(),
      setTargetWorkflowSteps: vi.fn(),
      setTargetStepId: vi.fn(),
      targetWorkflowId: "",
      targetStepId: "",
      setMigrateLoading: vi.fn(),
    };
    const { result } = renderHook(() =>
      useWorkflowDeleteHandlers({
        workflow,
        readOnly: true,
        otherWorkflows: [],
        wfDel,
        deleteWorkflowRun: vi.fn(),
        toast: vi.fn(),
      }),
    );

    await act(async () => {
      await result.current.handleDeleteWorkflowClick();
    });

    expect(getWorkflowTaskCount).not.toHaveBeenCalled();
    expect(wfDel.setDeleteOpen).not.toHaveBeenCalled();
  });
});

describe("useStepDeleteHandlers", () => {
  it("closes a failed delete and defers loading until its retry executes", async () => {
    const setStepDeleteOpen = vi.fn();
    const setStepToDelete = vi.fn();
    const setStepMigrateLoading = vi.fn();
    const setStepDeletePending = vi.fn();
    let failedOperation: (() => Promise<void>) | undefined;
    const { result } = renderHook(() =>
      useStepDeleteHandlers({
        workflow,
        stepDel: {
          stepToDelete: "step-1",
          targetStepForMigration: "step-2",
          setStepMigrateLoading,
          setStepDeletePending,
          setStepDeleteOpen,
          setStepToDelete,
        },
        refreshWorkflowSteps: vi.fn(),
        runMutation: vi.fn().mockImplementation(async (operation: () => Promise<void>) => {
          failedOperation = operation;
          return false;
        }),
      }),
    );

    await act(async () => {
      await result.current.handleDeleteStepAndTasks();
    });

    expect(setStepToDelete).toHaveBeenCalledWith(null);
    expect(setStepDeleteOpen).toHaveBeenCalledWith(false);
    expect(setStepMigrateLoading).not.toHaveBeenCalled();
    expect(setStepDeletePending.mock.calls).toEqual([[true], [false]]);
    setStepToDelete.mockClear();
    setStepDeleteOpen.mockClear();

    await act(async () => {
      await failedOperation?.();
    });

    expect(setStepMigrateLoading.mock.calls).toEqual([[true], [false]]);
    expect(setStepToDelete).toHaveBeenCalledWith(null);
    expect(setStepDeleteOpen).toHaveBeenCalledWith(false);
  });
});

describe("useStepDeleteHandlers retry safety", () => {
  it("ignores duplicate delete submissions while the first is queued", async () => {
    let finishMutation!: (saved: boolean) => void;
    const runMutation = vi.fn(
      () =>
        new Promise<boolean>((resolve) => {
          finishMutation = resolve;
        }),
    );
    const setStepDeleteOpen = vi.fn();
    const { result } = renderHook(() =>
      useStepDeleteHandlers({
        workflow,
        stepDel: {
          stepToDelete: "step-1",
          targetStepForMigration: "step-2",
          setStepMigrateLoading: vi.fn(),
          setStepDeletePending: vi.fn(),
          setStepDeleteOpen,
          setStepToDelete: vi.fn(),
        },
        refreshWorkflowSteps: vi.fn(),
        runMutation,
      }),
    );

    let firstSubmission!: Promise<void>;
    act(() => {
      firstSubmission = result.current.handleDeleteStepAndTasks();
    });
    await act(async () => {
      await result.current.handleDeleteStepAndTasks();
    });

    expect(runMutation).toHaveBeenCalledOnce();

    await act(async () => {
      finishMutation(false);
      await firstSubmission;
    });
    expect(setStepDeleteOpen).toHaveBeenCalledWith(false);
  });

  it("retries only refresh after a completed delete", async () => {
    const refreshWorkflowSteps = vi
      .fn()
      .mockRejectedValueOnce(new Error("refresh failed"))
      .mockResolvedValue(undefined);
    const runMutation = vi.fn(async (operation: () => Promise<void>) => {
      try {
        await operation();
      } catch {
        await operation();
      }
      return true;
    });
    const { result } = renderHook(() =>
      useStepDeleteHandlers({
        workflow,
        stepDel: {
          stepToDelete: "step-1",
          targetStepForMigration: "step-2",
          setStepMigrateLoading: vi.fn(),
          setStepDeletePending: vi.fn(),
          setStepDeleteOpen: vi.fn(),
          setStepToDelete: vi.fn(),
        },
        refreshWorkflowSteps,
        runMutation,
      }),
    );

    await act(async () => result.current.handleDeleteStepAndTasks());

    expect(deleteWorkflowStepAction).toHaveBeenCalledOnce();
    expect(refreshWorkflowSteps).toHaveBeenCalledTimes(2);
  });
});
