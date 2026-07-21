import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { deleteWorkflowAction } from "@/app/actions/workspaces";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import { SettingsSaveCancelledError, type SettingsSaveContributor } from "./settings-save-provider";
import { persistWorkflowDraft } from "./workflow-card-actions";
import { useWorkflowDraftContributor } from "./use-workflow-draft-contributor";

const captured = vi.hoisted(() => ({ contributor: null as SettingsSaveContributor | null }));

vi.mock("@/app/actions/workspaces", () => ({
  deleteWorkflowAction: vi.fn(),
}));

vi.mock("./settings-save-provider", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./settings-save-provider")>();
  return {
    ...actual,
    useSettingsSaveContributor: (contributor: SettingsSaveContributor) => {
      captured.contributor = contributor;
    },
  };
});

vi.mock("./workflow-card-actions", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./workflow-card-actions")>();
  return { ...actual, persistWorkflowDraft: vi.fn() };
});

const savedWorkflow = {
  id: "workflow-1",
  workspace_id: "workspace-1",
  name: "Workflow",
  created_at: "",
  updated_at: "",
} as Workflow;

function workflow(id: string = savedWorkflow.id): Workflow {
  return { ...savedWorkflow, id } as Workflow;
}

function step(id: string, workflowId: string): WorkflowStep {
  return {
    id,
    workflow_id: workflowId,
    name: "Backlog",
    position: 0,
    color: "bg-slate-500",
    allow_manual_move: true,
    is_start_step: true,
    created_at: "",
    updated_at: "",
  } as WorkflowStep;
}

function contributor(): SettingsSaveContributor {
  if (!captured.contributor) throw new Error("Contributor was not registered");
  return captured.contributor;
}

function renderContributor({
  draftWorkflow = savedWorkflow,
  workflowSteps = [step("step-1", draftWorkflow.id)],
  savedWorkflowSteps = workflowSteps,
  guardMutation = vi.fn(async ({ operation }: { operation: () => Promise<void> }) => operation()),
  onDeleteWorkflow = vi.fn(async () => undefined),
}: {
  draftWorkflow?: Workflow;
  workflowSteps?: WorkflowStep[];
  savedWorkflowSteps?: WorkflowStep[];
  guardMutation?: ReturnType<typeof vi.fn>;
  onDeleteWorkflow?: () => Promise<unknown>;
} = {}) {
  const setWorkflowSteps = vi.fn();
  const setSavedWorkflowSteps = vi.fn();
  const onDiscardWorkflow = vi.fn();
  const view = renderHook(() =>
    useWorkflowDraftContributor({
      workflow: draftWorkflow,
      isWorkflowDirty: true,
      workflowSteps,
      savedWorkflowSteps,
      setWorkflowSteps,
      setSavedWorkflowSteps,
      mutationGuard: { guardMutation } as never,
      toast: vi.fn(),
      onWorkflowSaved: vi.fn(),
      onDiscardWorkflow,
      onDeleteWorkflow,
    }),
  );
  return { ...view, setWorkflowSteps, onDiscardWorkflow, onDeleteWorkflow };
}

function mockPersistedDraft(steps: WorkflowStep[]) {
  vi.mocked(persistWorkflowDraft).mockImplementationOnce(async (input) => {
    input.progress.workflow = savedWorkflow;
    return { workflow: savedWorkflow, steps };
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  captured.contributor = null;
});

describe("useWorkflowDraftContributor", () => {
  it("deletes a partially persisted temporary workflow when discarding", async () => {
    const draftWorkflow = workflow("temp-workflow-1");
    const steps = [step("temp-step-1", draftWorkflow.id)];
    mockPersistedDraft(steps);
    const view = renderContributor({ draftWorkflow, workflowSteps: steps, savedWorkflowSteps: [] });

    await act(async () => contributor().save(contributor().revision));
    await act(async () => contributor().discard());

    expect(deleteWorkflowAction).toHaveBeenCalledWith(savedWorkflow.id);
    expect(view.setWorkflowSteps).toHaveBeenCalledWith([]);
    expect(view.onDiscardWorkflow).toHaveBeenCalledOnce();
  });

  it("blocks discard after a failed partial save", async () => {
    vi.mocked(persistWorkflowDraft).mockRejectedValueOnce(new Error("step create failed"));
    renderContributor();

    await expect(contributor().save(contributor().revision)).rejects.toThrow("step create failed");
    await expect(contributor().discard()).rejects.toThrow(
      "Retry the partial workflow save before leaving",
    );
  });

  it("reports guard cancellation as a cancelled coordinated save", async () => {
    const draftWorkflow = workflow("temp-workflow-1");
    const guardMutation = vi.fn(async () => undefined);
    renderContributor({ draftWorkflow, guardMutation });

    await expect(contributor().save(contributor().revision)).rejects.toBeInstanceOf(
      SettingsSaveCancelledError,
    );
    expect(persistWorkflowDraft).not.toHaveBeenCalled();
  });

  it("coalesces concurrent removal requests for a persisted temporary draft", async () => {
    const draftWorkflow = workflow("temp-workflow-1");
    const steps = [step("temp-step-1", draftWorkflow.id)];
    mockPersistedDraft(steps);
    let finishDelete: (() => void) | undefined;
    vi.mocked(deleteWorkflowAction).mockImplementationOnce(
      () => new Promise<void>((resolve) => (finishDelete = resolve)),
    );
    const view = renderContributor({ draftWorkflow, workflowSteps: steps, savedWorkflowSteps: [] });
    await act(async () => contributor().save(contributor().revision));

    let firstRemoval: Promise<void> | undefined;
    await act(async () => {
      firstRemoval = view.result.current.removeDraftWorkflow();
      await view.result.current.removeDraftWorkflow();
    });

    expect(deleteWorkflowAction).toHaveBeenCalledOnce();
    expect(view.onDeleteWorkflow).not.toHaveBeenCalled();
    await act(async () => {
      finishDelete?.();
      await firstRemoval;
    });
    expect(view.onDeleteWorkflow).toHaveBeenCalledOnce();
    expect(view.result.current.isRemovingDraft).toBe(false);
  });
});
