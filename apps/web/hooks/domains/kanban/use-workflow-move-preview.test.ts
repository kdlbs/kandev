import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkflowMovePreviewResponse } from "@/lib/api";
import type { AppState } from "@/lib/state/store";
import { getWorkflowMovePreviewRevision } from "./use-workflow-move-preview-revision";
import {
  MAX_PREVIEW_CONCURRENT_REQUESTS,
  useWorkflowMovePreview,
} from "./use-workflow-move-preview";

const TASK_ID = "task-1";
const WORKFLOW_ID = "workflow-1";
const FIRST_STEP_ID = "step-2";
const SECOND_STEP_ID = "step-3";

const { previewWorkflowMoveMock } = vi.hoisted(() => ({
  previewWorkflowMoveMock: vi.fn(),
}));

vi.mock("@/lib/api", () => ({
  previewWorkflowMove: previewWorkflowMoveMock,
}));

function makePreview(stepId: string): WorkflowMovePreviewResponse {
  return {
    task_id: "task-1",
    workflow_step_id: stepId,
    evaluated_at: "2026-09-14T00:00:00Z",
    outcome: "create_new",
    recipient: { profile_name: "Luna" },
    model: {
      before: { known: false },
      after: { known: true, label: "gpt-5.6-luna" },
    },
    context_reset: false,
    context_reset_state: "unchanged",
    source_disposition: "park",
    dispatch: "prompt",
  };
}

function makeRevisionState() {
  return {
    connection: { status: "connected" },
    workspaceContextGeneration: 1,
    kanban: {
      workflowId: WORKFLOW_ID,
      steps: [{ id: FIRST_STEP_ID, title: "Implement", position: 0 }],
      tasks: [
        {
          id: TASK_ID,
          workflowId: WORKFLOW_ID,
          workflowStepId: FIRST_STEP_ID,
          title: "Task",
          description: "Initial description",
          position: 0,
        },
      ],
    },
    kanbanMulti: { snapshots: {} },
    workflows: { items: [{ id: WORKFLOW_ID }], activeId: WORKFLOW_ID },
    taskSessions: { items: {} },
    taskSessionsByTask: {
      itemsByTaskId: {},
      loadingByTaskId: {},
      loadedByTaskId: {},
      errorByTaskId: {},
    },
    agentProfiles: { items: [], version: 1 },
    sessionModels: { bySessionId: {} },
  } as unknown as AppState;
}

function useThreePreviews() {
  const first = useWorkflowMovePreview({
    taskId: TASK_ID,
    workflowId: WORKFLOW_ID,
    workflowStepId: "step-2",
    enabled: true,
  });
  const second = useWorkflowMovePreview({
    taskId: TASK_ID,
    workflowId: WORKFLOW_ID,
    workflowStepId: "step-3",
    enabled: true,
  });
  const third = useWorkflowMovePreview({
    taskId: TASK_ID,
    workflowId: WORKFLOW_ID,
    workflowStepId: "step-4",
    enabled: true,
  });
  return [first, second, third];
}

beforeEach(() => {
  vi.useFakeTimers();
  previewWorkflowMoveMock.mockReset();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("useWorkflowMovePreview request lifecycle", () => {
  it("debounces a visible request and sends normalized move options", async () => {
    previewWorkflowMoveMock.mockResolvedValueOnce(makePreview("step-2"));
    const { result } = renderHook(() =>
      useWorkflowMovePreview({
        taskId: TASK_ID,
        workflowId: WORKFLOW_ID,
        workflowStepId: FIRST_STEP_ID,
        entryOptions: {
          reset_context: true,
          instructions: "  inspect the destination  ",
        },
        enabled: true,
      }),
    );

    expect(result.current.status).toBe("loading");
    expect(previewWorkflowMoveMock).not.toHaveBeenCalled();

    await act(async () => {
      vi.advanceTimersByTime(150);
      await Promise.resolve();
    });

    expect(previewWorkflowMoveMock).toHaveBeenCalledOnce();
    expect(previewWorkflowMoveMock).toHaveBeenCalledWith(
      TASK_ID,
      {
        workflow_id: WORKFLOW_ID,
        workflow_step_id: FIRST_STEP_ID,
        entry_options: {
          reset_context: true,
          instructions: "inspect the destination",
        },
      },
      { cache: "no-store", init: { signal: expect.any(AbortSignal) } },
    );
    expect(result.current.status).toBe("success");
    expect(result.current.preview?.workflow_step_id).toBe(FIRST_STEP_ID);
  });
});

describe("useWorkflowMovePreview invalidation", () => {
  it("ignores a late response from an earlier destination", async () => {
    let resolveFirst!: (value: WorkflowMovePreviewResponse) => void;
    let resolveSecond!: (value: WorkflowMovePreviewResponse) => void;
    previewWorkflowMoveMock
      .mockImplementationOnce(
        () =>
          new Promise<WorkflowMovePreviewResponse>((resolve) => {
            resolveFirst = resolve;
          }),
      )
      .mockImplementationOnce(
        () =>
          new Promise<WorkflowMovePreviewResponse>((resolve) => {
            resolveSecond = resolve;
          }),
      );

    const { result, rerender } = renderHook(
      ({ workflowStepId }: { workflowStepId: string }) =>
        useWorkflowMovePreview({
          taskId: TASK_ID,
          workflowId: WORKFLOW_ID,
          workflowStepId,
          enabled: true,
        }),
      { initialProps: { workflowStepId: FIRST_STEP_ID } },
    );

    await act(async () => {
      vi.advanceTimersByTime(150);
      await Promise.resolve();
    });
    expect(previewWorkflowMoveMock).toHaveBeenCalledOnce();

    rerender({ workflowStepId: SECOND_STEP_ID });
    await act(async () => {
      vi.advanceTimersByTime(150);
      await Promise.resolve();
    });
    expect(previewWorkflowMoveMock).toHaveBeenCalledTimes(2);

    await act(async () => {
      resolveSecond(makePreview(SECOND_STEP_ID));
      await Promise.resolve();
    });
    expect(result.current.preview?.workflow_step_id).toBe(SECOND_STEP_ID);

    await act(async () => {
      resolveFirst(makePreview(FIRST_STEP_ID));
      await Promise.resolve();
    });
    expect(result.current.preview?.workflow_step_id).toBe(SECOND_STEP_ID);
  });

  it("clears a successful result and fetches again when the invalidation key changes", async () => {
    previewWorkflowMoveMock
      .mockResolvedValueOnce(makePreview(FIRST_STEP_ID))
      .mockResolvedValueOnce(makePreview(SECOND_STEP_ID));
    const { result, rerender } = renderHook(
      ({ invalidationKey }: { invalidationKey: string }) =>
        useWorkflowMovePreview({
          taskId: TASK_ID,
          workflowId: WORKFLOW_ID,
          workflowStepId: FIRST_STEP_ID,
          invalidationKey,
          enabled: true,
        }),
      { initialProps: { invalidationKey: "connected:1" } },
    );

    await act(async () => {
      vi.advanceTimersByTime(150);
      await Promise.resolve();
    });
    expect(result.current.status).toBe("success");

    rerender({ invalidationKey: "reconnecting:2" });
    expect(result.current.status).toBe("loading");
    expect(result.current.preview).toBeNull();

    await act(async () => {
      vi.advanceTimersByTime(150);
      await Promise.resolve();
    });
    expect(previewWorkflowMoveMock).toHaveBeenCalledTimes(2);
    expect(result.current.status).toBe("success");
  });
});

describe("useWorkflowMovePreview stability", () => {
  it("keeps success and request count stable across harmless store updates", async () => {
    previewWorkflowMoveMock.mockResolvedValueOnce(makePreview(FIRST_STEP_ID));
    const revisionState = makeRevisionState();
    const { result, rerender } = renderHook(
      ({ revision }: { revision: string }) =>
        useWorkflowMovePreview({
          taskId: TASK_ID,
          workflowId: WORKFLOW_ID,
          workflowStepId: FIRST_STEP_ID,
          invalidationKey: revision,
          enabled: true,
        }),
      {
        initialProps: {
          revision: getWorkflowMovePreviewRevision(
            revisionState,
            TASK_ID,
            WORKFLOW_ID,
            FIRST_STEP_ID,
          ),
        },
      },
    );

    await act(async () => {
      vi.advanceTimersByTime(150);
      await Promise.resolve();
    });
    expect(result.current.status).toBe("success");

    (revisionState.kanban.tasks[0] as { description?: string }).description = "Updated copy";
    const nextRevision = getWorkflowMovePreviewRevision(
      revisionState,
      TASK_ID,
      WORKFLOW_ID,
      FIRST_STEP_ID,
    );
    rerender({ revision: nextRevision });

    expect(nextRevision).toBe(
      getWorkflowMovePreviewRevision(revisionState, TASK_ID, WORKFLOW_ID, FIRST_STEP_ID),
    );
    expect(result.current.status).toBe("success");
    expect(result.current.preview?.workflow_step_id).toBe(FIRST_STEP_ID);
    expect(previewWorkflowMoveMock).toHaveBeenCalledOnce();
  });
});

describe("useWorkflowMovePreview request queue", () => {
  it("queues disclosed destinations while keeping only two requests active", async () => {
    const resolvers = new Map<string, (preview: WorkflowMovePreviewResponse) => void>();
    previewWorkflowMoveMock.mockImplementation(
      (_taskId: string, payload: { workflow_step_id: string }) =>
        new Promise<WorkflowMovePreviewResponse>((resolve) => {
          resolvers.set(payload.workflow_step_id, resolve);
        }),
    );
    const { result } = renderHook(() => useThreePreviews());

    await act(async () => {
      vi.advanceTimersByTime(150);
      await Promise.resolve();
    });
    expect(MAX_PREVIEW_CONCURRENT_REQUESTS).toBe(2);
    expect(previewWorkflowMoveMock).toHaveBeenCalledTimes(MAX_PREVIEW_CONCURRENT_REQUESTS);
    expect(result.current[2].status).toBe("loading");
    expect(resolvers.has("step-4")).toBe(false);

    await act(async () => {
      resolvers.get("step-2")?.(makePreview("step-2"));
      await Promise.resolve();
    });
    expect(previewWorkflowMoveMock).toHaveBeenCalledTimes(3);
    expect(resolvers.has("step-4")).toBe(true);

    await act(async () => {
      resolvers.get("step-3")?.(makePreview("step-3"));
      resolvers.get("step-4")?.(makePreview("step-4"));
      await Promise.resolve();
    });
    expect(result.current.map((entry) => entry.status)).toEqual(["success", "success", "success"]);
  });
});

describe("useWorkflowMovePreview cleanup", () => {
  it("clears the result and aborts when the disclosure closes", async () => {
    let resolveRequest!: (value: WorkflowMovePreviewResponse) => void;
    previewWorkflowMoveMock.mockImplementationOnce(
      () =>
        new Promise<WorkflowMovePreviewResponse>((resolve) => {
          resolveRequest = resolve;
        }),
    );
    const { result, rerender } = renderHook(
      ({ enabled }: { enabled: boolean }) =>
        useWorkflowMovePreview({
          taskId: TASK_ID,
          workflowId: WORKFLOW_ID,
          workflowStepId: FIRST_STEP_ID,
          enabled,
        }),
      { initialProps: { enabled: true } },
    );

    await act(async () => {
      vi.advanceTimersByTime(150);
      await Promise.resolve();
    });
    const requestOptions = previewWorkflowMoveMock.mock.calls[0][2] as {
      init: { signal: AbortSignal };
    };
    rerender({ enabled: false });

    expect(requestOptions.init.signal.aborted).toBe(true);
    expect(result.current.status).toBe("idle");
    resolveRequest(makePreview(FIRST_STEP_ID));
  });

  it("re-enables and issues a fresh request after disabling with the same key", async () => {
    let resolveFirst!: (value: WorkflowMovePreviewResponse) => void;
    previewWorkflowMoveMock
      .mockImplementationOnce(
        () =>
          new Promise<WorkflowMovePreviewResponse>((resolve) => {
            resolveFirst = resolve;
          }),
      )
      .mockResolvedValueOnce(makePreview(FIRST_STEP_ID));

    const { result, rerender } = renderHook(
      ({ enabled }: { enabled: boolean }) =>
        useWorkflowMovePreview({
          taskId: TASK_ID,
          workflowId: WORKFLOW_ID,
          workflowStepId: FIRST_STEP_ID,
          enabled,
        }),
      { initialProps: { enabled: true } },
    );

    await act(async () => {
      vi.advanceTimersByTime(150);
      await Promise.resolve();
    });
    expect(previewWorkflowMoveMock).toHaveBeenCalledOnce();

    rerender({ enabled: false });
    expect(result.current.status).toBe("idle");

    rerender({ enabled: true });
    await act(async () => {
      vi.advanceTimersByTime(150);
      await Promise.resolve();
    });
    expect(previewWorkflowMoveMock).toHaveBeenCalledTimes(2);
    expect(result.current.status).toBe("success");

    resolveFirst(makePreview(FIRST_STEP_ID));
    await act(async () => {
      await Promise.resolve();
    });
    expect(result.current.status).toBe("success");
  });
});
