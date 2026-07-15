import { act, renderHook } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import {
  createWorkflowAction,
  createWorkflowStepAction,
  deleteWorkflowStepAction,
  getStepTaskCount,
  listWorkflowStepsAction,
  reorderWorkflowStepsAction,
  updateWorkflowStepAction,
} from "@/app/actions/workspaces";
import type { OnTurnCompleteAction } from "@/lib/types/workflow-actions";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import { useWorkflowSaveActions, useWorkflowStepActions } from "./workflow-card-actions";
import { useWorkflowMutationGuard } from "./workflow-mutation-guard";

vi.mock("@/app/actions/workspaces", () => ({
  createWorkflowAction: vi.fn(),
  createWorkflowStepAction: vi.fn(),
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

function step(
  id: string,
  name: string,
  position: number,
  isStartStep: boolean,
  options: { autoStart?: boolean; onTurnComplete?: OnTurnCompleteAction[] } = {},
): WorkflowStep {
  return {
    id,
    workflow_id: workflow.id,
    name,
    position,
    color: "bg-slate-500",
    allow_manual_move: true,
    is_start_step: isStartStep,
    events: {
      on_enter: options.autoStart ? [{ type: "auto_start_agent" }] : [],
      on_turn_complete: options.onTurnComplete,
    },
    created_at: "",
    updated_at: "",
  };
}

function renderNewWorkflowStepActions(initialSteps: WorkflowStep[]) {
  let steps = initialSteps;
  const setWorkflowSteps = vi.fn(
    (updater: ((prev: WorkflowStep[]) => WorkflowStep[]) | WorkflowStep[]) => {
      steps = typeof updater === "function" ? updater(steps) : updater;
    },
  );
  const view = renderHook(() => {
    const mutationGuard = useWorkflowMutationGuard(steps);
    return useWorkflowStepActions({
      workflow,
      isNewWorkflow: true,
      workflowSteps: steps,
      setWorkflowSteps,
      refreshWorkflowSteps: vi.fn(),
      setStepToDelete: vi.fn(),
      setStepTaskCount: vi.fn(),
      setTargetStepForMigration: vi.fn(),
      setStepDeleteOpen: vi.fn(),
      toast: vi.fn(),
      mutationGuard,
    });
  });
  return { ...view, getSteps: () => steps };
}

function renderPersistedWorkflowStepActions(steps: WorkflowStep[]) {
  const setWorkflowSteps = vi.fn();
  const refreshWorkflowSteps = vi.fn().mockResolvedValue(undefined);
  const setStepDeleteOpen = vi.fn();
  const view = renderHook(() => {
    const mutationGuard = useWorkflowMutationGuard(steps);
    const actions = useWorkflowStepActions({
      workflow,
      isNewWorkflow: false,
      workflowSteps: steps,
      setWorkflowSteps,
      refreshWorkflowSteps,
      setStepToDelete: vi.fn(),
      setStepTaskCount: vi.fn(),
      setTargetStepForMigration: vi.fn(),
      setStepDeleteOpen,
      toast: vi.fn(),
      mutationGuard,
    });
    return { actions, mutationGuard };
  });
  return { ...view, setWorkflowSteps, refreshWorkflowSteps, setStepDeleteOpen };
}

function deferredPromise() {
  let resolve!: () => void;
  const promise = new Promise<void>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

it("keeps one start step while editing a new workflow locally", async () => {
  const { result, getSteps } = renderNewWorkflowStepActions([
    step("step-1", "Todo", 0, true),
    step("step-2", "Plan", 1, false),
  ]);

  await act(async () => {
    await result.current.handleUpdateWorkflowStep("step-2", { is_start_step: true });
  });

  expect(
    getSteps()
      .filter((s) => s.is_start_step)
      .map((s) => s.id),
  ).toEqual(["step-2"]);
});

it("does not update a persisted step when the proposal introduces a blocking cycle", async () => {
  const steps = [
    step("build", "Build", 0, true, {
      autoStart: true,
      onTurnComplete: [{ type: "move_to_next" }],
    }),
    step("review", "Review", 1, false, { autoStart: true }),
  ];
  const { result } = renderHook(() => {
    const mutationGuard = useWorkflowMutationGuard(steps);
    return useWorkflowStepActions({
      workflow,
      isNewWorkflow: false,
      workflowSteps: steps,
      setWorkflowSteps: vi.fn(),
      refreshWorkflowSteps: vi.fn(),
      setStepToDelete: vi.fn(),
      setStepTaskCount: vi.fn(),
      setTargetStepForMigration: vi.fn(),
      setStepDeleteOpen: vi.fn(),
      toast: vi.fn(),
      mutationGuard,
    });
  });

  await act(async () => {
    await result.current.handleUpdateWorkflowStep("review", {
      events: {
        ...steps[1].events,
        on_turn_complete: [{ type: "move_to_previous" }],
      },
    });
  });

  expect(updateWorkflowStepAction).not.toHaveBeenCalled();
});

it("does not persist a second topology edit before the first refresh completes", async () => {
  const refresh = deferredPromise();
  const steps = [
    step("work", "Work", 0, true, { autoStart: true }),
    step("review", "Review", 1, false),
  ];
  const { result, refreshWorkflowSteps } = renderPersistedWorkflowStepActions(steps);
  refreshWorkflowSteps.mockReturnValueOnce(refresh.promise);

  let firstEdit!: Promise<void>;
  await act(async () => {
    firstEdit = result.current.actions.handleUpdateWorkflowStep("work", {
      events: { on_turn_complete: [{ type: "move_to_next" }] },
    });
    await vi.waitFor(() => expect(refreshWorkflowSteps).toHaveBeenCalledTimes(1));
    await result.current.actions.handleUpdateWorkflowStep("review", {
      events: { on_turn_complete: [{ type: "move_to_previous" }] },
    });
  });

  expect(updateWorkflowStepAction).toHaveBeenCalledTimes(1);
  expect(result.current.mutationGuard.isMutationPending).toBe(true);

  refresh.resolve();
  await act(async () => {
    await firstEdit;
  });
  expect(result.current.mutationGuard.isMutationPending).toBe(false);
});

it("does not replace a pending warning proposal with a second edit", async () => {
  const steps = [
    step("work", "Work", 0, true, {
      autoStart: true,
      onTurnComplete: [{ type: "move_to_next" }],
    }),
    step("review", "Review", 1, false),
  ];
  const { result } = renderPersistedWorkflowStepActions(steps);

  await act(async () => {
    await result.current.actions.handleUpdateWorkflowStep("review", {
      events: {
        ...steps[1].events,
        on_turn_complete: [{ type: "move_to_previous" }],
      },
    });
  });
  const proposal = result.current.mutationGuard.proposal;

  await act(async () => {
    await result.current.actions.handleUpdateWorkflowStep("work", { name: "Building" });
  });

  expect(result.current.mutationGuard.proposal).toBe(proposal);
  expect(updateWorkflowStepAction).not.toHaveBeenCalled();
});

it("does not replace a pending blocking proposal with a second edit", async () => {
  const steps = [
    step("build", "Build", 0, true, {
      autoStart: true,
      onTurnComplete: [{ type: "move_to_next" }],
    }),
    step("review", "Review", 1, false, { autoStart: true }),
  ];
  const { result } = renderPersistedWorkflowStepActions(steps);

  await act(async () => {
    await result.current.actions.handleUpdateWorkflowStep("review", {
      events: { on_turn_complete: [{ type: "move_to_previous" }] },
    });
  });
  const proposal = result.current.mutationGuard.proposal;

  await act(async () => {
    await result.current.actions.handleUpdateWorkflowStep("build", { name: "Building" });
  });

  expect(result.current.mutationGuard.proposal).toBe(proposal);
  expect(updateWorkflowStepAction).not.toHaveBeenCalled();
});

it("releases a blocking proposal if confirmation is invoked defensively", async () => {
  const steps = [
    step("build", "Build", 0, true, {
      autoStart: true,
      onTurnComplete: [{ type: "move_to_next" }],
    }),
    step("review", "Review", 1, false, { autoStart: true }),
  ];
  const { result } = renderPersistedWorkflowStepActions(steps);

  await act(async () => {
    await result.current.actions.handleUpdateWorkflowStep("review", {
      events: {
        ...steps[1].events,
        on_turn_complete: [{ type: "move_to_previous" }],
      },
    });
  });
  expect(result.current.mutationGuard.proposal?.severity).toBe("blocking");

  await act(async () => {
    await result.current.mutationGuard.confirmProposal();
  });

  expect(result.current.mutationGuard.proposal).toBeNull();
  expect(result.current.mutationGuard.isMutationPending).toBe(false);
  expect(updateWorkflowStepAction).not.toHaveBeenCalled();
});

it("holds a warning update until confirmation and executes it exactly once", async () => {
  const steps = [
    step("work", "Work", 0, true, {
      autoStart: true,
      onTurnComplete: [{ type: "move_to_next" }],
    }),
    step("review", "Review", 1, false),
  ];
  const { result } = renderPersistedWorkflowStepActions(steps);

  await act(async () => {
    await result.current.actions.handleUpdateWorkflowStep("review", {
      events: { on_turn_complete: [{ type: "move_to_previous" }] },
    });
  });

  expect(updateWorkflowStepAction).not.toHaveBeenCalled();
  expect(result.current.mutationGuard.proposal).toMatchObject({
    intent: "apply",
    severity: "warning",
  });

  await act(async () => {
    await result.current.mutationGuard.confirmProposal();
    await result.current.mutationGuard.confirmProposal();
  });

  expect(updateWorkflowStepAction).toHaveBeenCalledTimes(1);
});

it("discards a warning update when confirmation is cancelled", async () => {
  const steps = [
    step("work", "Work", 0, true, {
      autoStart: true,
      onTurnComplete: [{ type: "move_to_next" }],
    }),
    step("review", "Review", 1, false),
  ];
  const { result } = renderPersistedWorkflowStepActions(steps);

  await act(async () => {
    await result.current.actions.handleUpdateWorkflowStep("review", {
      events: { on_turn_complete: [{ type: "move_to_previous" }] },
    });
    result.current.mutationGuard.cancelProposal();
  });

  expect(updateWorkflowStepAction).not.toHaveBeenCalled();
  expect(result.current.mutationGuard.proposal).toBeNull();
});

it("does not gate an unchanged diagnostic identity or an edit that removes a cycle", async () => {
  const steps = [
    step("work", "Work", 0, true, {
      autoStart: true,
      onTurnComplete: [{ type: "move_to_next" }],
    }),
    step("review", "Review", 1, false, {
      onTurnComplete: [{ type: "move_to_previous" }],
    }),
  ];
  const { result } = renderPersistedWorkflowStepActions(steps);

  await act(async () => {
    await result.current.actions.handleUpdateWorkflowStep("work", { prompt: "New prompt" });
    await result.current.actions.handleUpdateWorkflowStep("review", {
      events: { on_turn_complete: [] },
    });
  });

  expect(updateWorkflowStepAction).toHaveBeenCalledTimes(2);
  expect(result.current.mutationGuard.proposal).toBeNull();
});

it("preflights delete and reorder shapes before any side effect", async () => {
  const work = step("work", "Work", 0, true, {
    autoStart: true,
    onTurnComplete: [{ type: "move_to_next" }],
  });
  const middle = step("middle", "Middle", 1, false);
  const review = step("review", "Review", 2, false, {
    autoStart: true,
    onTurnComplete: [{ type: "move_to_previous" }],
  });
  const { result, setWorkflowSteps } = renderPersistedWorkflowStepActions([work, middle, review]);

  await act(async () => {
    await result.current.actions.handleRemoveWorkflowStep("middle");
    await result.current.actions.handleReorderWorkflowSteps([
      work,
      { ...review, position: 1 },
      { ...middle, position: 2 },
    ]);
  });

  expect(getStepTaskCount).not.toHaveBeenCalled();
  expect(deleteWorkflowStepAction).not.toHaveBeenCalled();
  expect(reorderWorkflowStepsAction).not.toHaveBeenCalled();
  expect(setWorkflowSteps).not.toHaveBeenCalled();
});

it("holds the task migration delete path until a warning is confirmed", async () => {
  vi.mocked(getStepTaskCount).mockResolvedValue({ task_count: 2 });
  const work = step("work", "Work", 0, true, {
    autoStart: true,
    onTurnComplete: [{ type: "move_to_next" }],
  });
  const middle = step("middle", "Middle", 1, false);
  const review = step("review", "Review", 2, false, {
    onTurnComplete: [{ type: "move_to_previous" }],
  });
  const { result, setStepDeleteOpen } = renderPersistedWorkflowStepActions([work, middle, review]);

  await act(async () => {
    await result.current.actions.handleRemoveWorkflowStep("middle");
  });
  expect(getStepTaskCount).not.toHaveBeenCalled();
  expect(result.current.mutationGuard.proposal?.severity).toBe("warning");

  await act(async () => {
    await result.current.mutationGuard.confirmProposal();
  });

  expect(getStepTaskCount).toHaveBeenCalledTimes(1);
  expect(setStepDeleteOpen).toHaveBeenCalledWith(true);
  expect(deleteWorkflowStepAction).not.toHaveBeenCalled();
});

it("adds a step without reconfirming an existing diagnostic", async () => {
  const steps = [
    step("work", "Work", 0, true, {
      autoStart: true,
      onTurnComplete: [{ type: "move_to_step", config: { step_id: "work" } }],
    }),
  ];
  const { result } = renderPersistedWorkflowStepActions(steps);

  await act(async () => {
    await result.current.actions.handleAddWorkflowStep();
  });

  expect(createWorkflowStepAction).toHaveBeenCalledTimes(1);
  expect(result.current.mutationGuard.proposal).toBeNull();
});

it("does not create a draft workflow with a blocking cycle", async () => {
  const draftSteps = [
    step("build", "Build", 0, true, {
      autoStart: true,
      onTurnComplete: [{ type: "move_to_next" }],
    }),
    step("review", "Review", 1, false, {
      autoStart: true,
      onTurnComplete: [{ type: "move_to_previous" }],
    }),
  ];
  const { result } = renderHook(() => {
    const mutationGuard = useWorkflowMutationGuard(draftSteps);
    return useWorkflowSaveActions({
      workflow: { ...workflow, id: "temp-workflow" as Workflow["id"] },
      isNewWorkflow: true,
      workflowSteps: draftSteps,
      templateStepCount: 0,
      onSaveWorkflow: vi.fn(),
      toast: vi.fn(),
      mutationGuard,
    });
  });

  await act(async () => {
    await result.current.handleSaveWorkflow();
  });

  expect(createWorkflowAction).not.toHaveBeenCalled();
  expect(result.current.mutationGuard.proposal?.severity).toBe("blocking");
});

it("cancels a warning draft with no requests and creates it once after confirmation", async () => {
  const createdWorkflow = { ...workflow, id: "created-workflow" as Workflow["id"] };
  vi.mocked(createWorkflowAction).mockResolvedValue(createdWorkflow);
  vi.mocked(createWorkflowStepAction).mockImplementation(async (payload) => ({
    ...step(`created-${payload.position}`, payload.name, payload.position, false),
    workflow_id: createdWorkflow.id,
  }));
  const draftSteps = [
    step("work", "Work", 0, true, {
      autoStart: true,
      onTurnComplete: [{ type: "move_to_next" }],
    }),
    step("review", "Review", 1, false, {
      onTurnComplete: [{ type: "move_to_previous" }],
    }),
  ];
  const { result } = renderHook(() => {
    const mutationGuard = useWorkflowMutationGuard(draftSteps);
    return useWorkflowSaveActions({
      workflow: { ...workflow, id: "temp-workflow" as Workflow["id"] },
      isNewWorkflow: true,
      workflowSteps: draftSteps,
      templateStepCount: 0,
      onSaveWorkflow: vi.fn(),
      toast: vi.fn(),
      mutationGuard,
    });
  });

  await act(async () => {
    await result.current.handleSaveWorkflow();
    result.current.mutationGuard.cancelProposal();
  });
  expect(createWorkflowAction).not.toHaveBeenCalled();
  expect(createWorkflowStepAction).not.toHaveBeenCalled();

  await act(async () => {
    await result.current.handleSaveWorkflow();
    await result.current.mutationGuard.confirmProposal();
    await result.current.mutationGuard.confirmProposal();
  });

  expect(createWorkflowAction).toHaveBeenCalledTimes(1);
  expect(createWorkflowStepAction).toHaveBeenCalledTimes(2);
});

it("remaps template workflow pull sources between backend-created and added steps", async () => {
  const createdWorkflowId = "wf-created" as Workflow["id"];
  const templateName = "Template Step";
  const addedName = "Added Step";
  const backendTemplateId = "backend-template";
  const backendAddedId = "backend-added";
  const draftTemplateId = "draft-template";

  vi.mocked(createWorkflowAction).mockResolvedValue({
    ...workflow,
    id: createdWorkflowId,
    workflow_template_id: "template-1",
  });
  vi.mocked(listWorkflowStepsAction).mockResolvedValue({
    steps: [
      {
        ...step(backendTemplateId, templateName, 0, true),
        id: backendTemplateId,
        workflow_id: createdWorkflowId,
      },
    ],
    total: 1,
  });
  vi.mocked(createWorkflowStepAction).mockResolvedValue({
    ...step(backendAddedId, addedName, 1, false),
    id: backendAddedId,
    workflow_id: createdWorkflowId,
  });
  vi.mocked(updateWorkflowStepAction).mockResolvedValue(step(backendAddedId, addedName, 1, false));

  const templateStep = {
    ...step(draftTemplateId, templateName, 0, true),
    pull_from_step_id: "draft-added",
  };
  const addedStep = {
    ...step("draft-added", addedName, 1, false),
    pull_from_step_id: draftTemplateId,
  };
  const workflowSteps = [templateStep, addedStep];
  const { result } = renderHook(() => {
    const mutationGuard = useWorkflowMutationGuard(workflowSteps);
    return useWorkflowSaveActions({
      workflow: { ...workflow, workflow_template_id: "template-1" },
      isNewWorkflow: true,
      workflowSteps,
      templateStepCount: 1,
      onSaveWorkflow: vi.fn(),
      onWorkflowCreated: vi.fn(),
      toast: vi.fn(),
      mutationGuard,
    });
  });

  await act(async () => {
    await result.current.handleSaveWorkflow();
  });

  expect(createWorkflowStepAction).toHaveBeenCalledWith(
    expect.objectContaining({ pull_from_step_id: "" }),
  );
  expect(updateWorkflowStepAction).toHaveBeenCalledWith(backendAddedId, {
    pull_from_step_id: backendTemplateId,
  });
  expect(updateWorkflowStepAction).toHaveBeenCalledWith(backendTemplateId, {
    pull_from_step_id: backendAddedId,
  });
});
