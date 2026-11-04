import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useTaskCreateWorkflowSteps } from "./use-task-create-workflow-steps";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";

const readMocks = vi.hoisted(() => ({
  readJourneyWorkflowSteps: vi.fn(),
}));
const scopeMocks = vi.hoisted(() => ({ value: "scope-a" }));

vi.mock("@/hooks/journey-metadata-resources", () => ({
  readJourneyWorkflowSteps: readMocks.readJourneyWorkflowSteps,
}));

vi.mock("@/components/state-provider", () => ({
  useOptionalAppStore: () => scopeMocks.value,
  useOptionalAppStoreApi: () => undefined,
}));

const WORKFLOW_A = "workflow-a";
const WORKFLOW_B = "workflow-b";
const WORKSPACE_A = "workspace-1";
const WORKSPACE_B = "workspace-2";
const RECOVERED_B = "recovered-b";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((nextResolve, nextReject) => {
    resolve = nextResolve;
    reject = nextReject;
  });
  return { promise, resolve, reject };
}

function snapshot(workflowId: string, steps: WorkflowSnapshotData["steps"]): WorkflowSnapshotData {
  return {
    workflowId,
    workflowName: workflowId,
    steps,
    tasks: [],
  };
}

const baseArgs = {
  open: true,
  workspaceId: WORKSPACE_A,
  workflowId: WORKFLOW_B,
  openCycle: 1,
  loadedSnapshot: null,
};

type ReadResponse = {
  steps: Array<{
    id: string;
    workflow_id: string;
    name: string;
    position: number;
    is_start_step?: boolean;
  }>;
  total: number;
};

function readResponse(id: string, name: string): ReadResponse {
  return {
    steps: [{ id, workflow_id: WORKFLOW_B, name, position: 0, is_start_step: true }],
    total: 1,
  };
}

beforeEach(() => {
  readMocks.readJourneyWorkflowSteps.mockReset();
  scopeMocks.value = "scope-a";
});

describe("useTaskCreateWorkflowSteps", () => {
  it("loads selected workflow metadata without a snapshot or pending board read", async () => {
    readMocks.readJourneyWorkflowSteps.mockResolvedValue({
      steps: [
        {
          id: "implement",
          workflow_id: WORKFLOW_B,
          name: "Implement",
          color: "bg-blue-500",
          position: 1,
          is_start_step: false,
          prompt: "Implement {{task_prompt}}",
          events: { on_enter: [{ type: "auto_start_agent" }] },
          agent_profile_id: "profile-a",
          session_target: null,
        },
        {
          id: "analysis",
          workflow_id: WORKFLOW_B,
          name: "Analysis",
          color: "bg-green-500",
          position: 0,
          is_start_step: true,
          session_target: { kind: "initial" },
        },
      ],
      total: 2,
    });

    const { result } = renderHook(() => useTaskCreateWorkflowSteps(baseArgs));

    expect(result.current.status).toBe("loading");
    expect(readMocks.readJourneyWorkflowSteps).toHaveBeenCalledTimes(1);
    expect(readMocks.readJourneyWorkflowSteps).toHaveBeenCalledWith(
      undefined,
      WORKFLOW_B,
      expect.objectContaining({ signal: expect.any(AbortSignal), refresh: false }),
    );

    await waitFor(() => expect(result.current.status).toBe("success"));
    expect(result.current.steps).toEqual([
      expect.objectContaining({
        id: "analysis",
        title: "Analysis",
        workflowId: WORKFLOW_B,
        color: "bg-green-500",
        position: 0,
        is_start_step: true,
        session_target: { kind: "initial" },
      }),
      expect.objectContaining({
        id: "implement",
        title: "Implement",
        workflowId: WORKFLOW_B,
        prompt: "Implement {{task_prompt}}",
        events: { on_enter: [{ type: "auto_start_agent" }] },
        agent_profile_id: "profile-a",
      }),
    ]);
  });
});

describe("useTaskCreateWorkflowSteps snapshot seeds", () => {
  it("uses a complete loaded snapshot and follows live snapshot updates", () => {
    const loadedSnapshot = snapshot(WORKFLOW_B, [
      {
        id: "implement",
        title: "Implement",
        color: "blue",
        position: 1,
        agent_profile_id: "profile-a",
      },
      {
        id: "analysis",
        title: "Analysis",
        color: "green",
        position: 0,
        is_start_step: true,
      },
    ]);

    const { result, rerender } = renderHook(
      (props: { loadedSnapshot: WorkflowSnapshotData | null }) =>
        useTaskCreateWorkflowSteps({ ...baseArgs, ...props }),
      { initialProps: { loadedSnapshot } },
    );

    expect(result.current.status).toBe("success");
    expect(result.current.steps.map((step) => step.id)).toEqual(["analysis", "implement"]);
    expect(readMocks.readJourneyWorkflowSteps).not.toHaveBeenCalled();

    rerender({
      loadedSnapshot: snapshot(WORKFLOW_B, [
        {
          id: "edited",
          title: "Edited",
          color: "purple",
          position: 0,
          is_start_step: true,
        },
      ]),
    });

    expect(result.current.steps).toEqual([
      expect.objectContaining({ id: "edited", title: "Edited", workflowId: WORKFLOW_B }),
    ]);
    expect(readMocks.readJourneyWorkflowSteps).not.toHaveBeenCalled();
  });
});

describe("useTaskCreateWorkflowSteps snapshot retirement", () => {
  it("starts loading when a snapshot-backed result retires for the same workflow", async () => {
    const refreshed = deferred<ReadResponse>();
    readMocks.readJourneyWorkflowSteps.mockReturnValueOnce(refreshed.promise);
    const loadedSnapshot = snapshot(WORKFLOW_B, [
      {
        id: "snapshot-b",
        title: "Snapshot B",
        color: "green",
        position: 0,
        is_start_step: true,
      },
    ]);

    const { result, rerender } = renderHook(
      (props: { loadedSnapshot: WorkflowSnapshotData | null }) =>
        useTaskCreateWorkflowSteps({ ...baseArgs, ...props }),
      { initialProps: { loadedSnapshot: loadedSnapshot as WorkflowSnapshotData | null } },
    );

    expect(result.current.status).toBe("success");
    expect(result.current.steps.map((step) => step.id)).toEqual(["snapshot-b"]);
    expect(readMocks.readJourneyWorkflowSteps).not.toHaveBeenCalled();

    rerender({ loadedSnapshot: null });
    expect(result.current.status).toBe("loading");
    expect(result.current.steps).toEqual([]);
    await waitFor(() => expect(readMocks.readJourneyWorkflowSteps).toHaveBeenCalledTimes(1));

    await act(async () => {
      refreshed.resolve(readResponse("refreshed-b", "Refreshed B"));
      await Promise.resolve();
    });

    expect(result.current.status).toBe("success");
    expect(result.current.steps.map((step) => step.id)).toEqual(["refreshed-b"]);
  });

  it("rejects a failed snapshot that retains stale steps", async () => {
    readMocks.readJourneyWorkflowSteps.mockResolvedValue(readResponse(RECOVERED_B, "Recovered B"));
    const failedSnapshot = {
      ...snapshot(WORKFLOW_B, [
        {
          id: "stale-b",
          title: "Stale B",
          color: "gray",
          position: 0,
          is_start_step: true,
        },
      ]),
      fetchFailed: true,
    };

    const { result } = renderHook(() =>
      useTaskCreateWorkflowSteps({ ...baseArgs, loadedSnapshot: failedSnapshot }),
    );

    expect(result.current.status).toBe("loading");
    expect(result.current.steps).toEqual([]);
    await waitFor(() => expect(result.current.status).toBe("success"));
    expect(result.current.steps.map((step) => step.id)).toEqual([RECOVERED_B]);
    expect(readMocks.readJourneyWorkflowSteps).toHaveBeenCalledTimes(1);
  });

  it("requests metadata for a placeholder snapshot and treats an empty success as authoritative", async () => {
    readMocks.readJourneyWorkflowSteps.mockResolvedValue({ steps: [], total: 0 });
    const placeholder = {
      ...snapshot(WORKFLOW_B, []),
      isPlaceholder: true,
    };

    const { result } = renderHook(() =>
      useTaskCreateWorkflowSteps({ ...baseArgs, loadedSnapshot: placeholder }),
    );

    expect(result.current.status).toBe("loading");
    await waitFor(() => expect(result.current.status).toBe("success"));
    expect(result.current.steps).toEqual([]);
    expect(readMocks.readJourneyWorkflowSteps).toHaveBeenCalledTimes(1);
  });
});

describe("useTaskCreateWorkflowSteps request states", () => {
  it("exposes failure and retries only the selected workflow with shared refresh", async () => {
    readMocks.readJourneyWorkflowSteps
      .mockRejectedValueOnce(new Error("unavailable"))
      .mockResolvedValueOnce({
        steps: [
          {
            id: "analysis",
            workflow_id: WORKFLOW_B,
            name: "Analysis",
            position: 0,
            is_start_step: true,
          },
        ],
        total: 1,
      });

    const { result } = renderHook(() => useTaskCreateWorkflowSteps(baseArgs));

    await waitFor(() => expect(result.current.status).toBe("error"));
    expect(result.current.steps).toEqual([]);

    act(() => result.current.retry());

    await waitFor(() => expect(result.current.status).toBe("success"));
    expect(readMocks.readJourneyWorkflowSteps).toHaveBeenCalledTimes(2);
    expect(readMocks.readJourneyWorkflowSteps).toHaveBeenLastCalledWith(
      undefined,
      WORKFLOW_B,
      expect.objectContaining({ refresh: true }),
    );
    expect(result.current.steps.map((step) => step.id)).toEqual(["analysis"]);
  });

  it("does not establish demand outside task creation", () => {
    const { result } = renderHook(() =>
      useTaskCreateWorkflowSteps({ ...baseArgs, enabled: false }),
    );

    expect(result.current.status).toBe("idle");
    expect(result.current.steps).toEqual([]);
    expect(readMocks.readJourneyWorkflowSteps).not.toHaveBeenCalled();
  });

  it("does not repeat demand on stable rerenders", async () => {
    readMocks.readJourneyWorkflowSteps.mockResolvedValue({ steps: [], total: 0 });
    const { result, rerender } = renderHook(() => useTaskCreateWorkflowSteps(baseArgs));

    await waitFor(() => expect(result.current.status).toBe("success"));
    rerender();
    rerender();

    expect(readMocks.readJourneyWorkflowSteps).toHaveBeenCalledTimes(1);
  });

  it("rejects a delayed response from the previous workflow", async () => {
    const first = deferred<{ steps: never[]; total: number }>();
    const second = deferred<{ steps: never[]; total: number }>();
    readMocks.readJourneyWorkflowSteps.mockReturnValueOnce(first.promise);
    readMocks.readJourneyWorkflowSteps.mockReturnValueOnce(second.promise);

    const { result, rerender } = renderHook(
      (props: { workflowId: string }) => useTaskCreateWorkflowSteps({ ...baseArgs, ...props }),
      { initialProps: { workflowId: WORKFLOW_A } },
    );

    rerender({ workflowId: WORKFLOW_B });
    expect(result.current.status).toBe("loading");

    await act(async () => {
      first.resolve({ steps: [], total: 0 });
      await Promise.resolve();
    });
    expect(result.current.status).toBe("loading");
    expect(result.current.steps).toEqual([]);

    await act(async () => {
      second.resolve({ steps: [], total: 0 });
      await Promise.resolve();
    });
    expect(result.current.status).toBe("success");
  });
});

describe("useTaskCreateWorkflowSteps activations", () => {
  const snapshotA = snapshot(WORKFLOW_A, [
    {
      id: "snapshot-a",
      title: "Snapshot A",
      color: "green",
      position: 0,
      is_start_step: true,
    },
  ]);

  it("does not resurrect metadata from an earlier visit to the same workflow", async () => {
    const second = deferred<ReadResponse>();
    readMocks.readJourneyWorkflowSteps
      .mockResolvedValueOnce(readResponse("first-b", "First B"))
      .mockReturnValueOnce(second.promise);

    const { result, rerender } = renderHook(
      (props: { workflowId: string; loadedSnapshot: WorkflowSnapshotData | null }) =>
        useTaskCreateWorkflowSteps({ ...baseArgs, ...props }),
      {
        initialProps: {
          workflowId: WORKFLOW_B,
          loadedSnapshot: null as WorkflowSnapshotData | null,
        },
      },
    );

    await waitFor(() => expect(result.current.status).toBe("success"));
    expect(result.current.steps.map((step) => step.id)).toEqual(["first-b"]);

    rerender({ workflowId: WORKFLOW_A, loadedSnapshot: snapshotA });
    expect(result.current.status).toBe("success");
    expect(result.current.steps.map((step) => step.id)).toEqual(["snapshot-a"]);
    expect(readMocks.readJourneyWorkflowSteps).toHaveBeenCalledTimes(1);

    rerender({ workflowId: WORKFLOW_B, loadedSnapshot: null });
    expect(result.current.status).toBe("loading");
    expect(result.current.steps).toEqual([]);
    await waitFor(() => expect(readMocks.readJourneyWorkflowSteps).toHaveBeenCalledTimes(2));
    expect(result.current.status).toBe("loading");
    expect(result.current.steps).toEqual([]);

    await act(async () => {
      second.resolve(readResponse("second-b", "Second B"));
      await Promise.resolve();
    });

    expect(result.current.status).toBe("success");
    expect(result.current.steps.map((step) => step.id)).toEqual(["second-b"]);
  });

  it("clears a retired error when the same workflow is selected again", async () => {
    const second = deferred<ReadResponse>();
    readMocks.readJourneyWorkflowSteps
      .mockRejectedValueOnce(new Error("unavailable"))
      .mockReturnValueOnce(second.promise);

    const { result, rerender } = renderHook(
      (props: { workflowId: string; loadedSnapshot: WorkflowSnapshotData | null }) =>
        useTaskCreateWorkflowSteps({ ...baseArgs, ...props }),
      {
        initialProps: {
          workflowId: WORKFLOW_B,
          loadedSnapshot: null as WorkflowSnapshotData | null,
        },
      },
    );

    await waitFor(() => expect(result.current.status).toBe("error"));
    rerender({ workflowId: WORKFLOW_A, loadedSnapshot: snapshotA });
    expect(result.current.status).toBe("success");

    rerender({ workflowId: WORKFLOW_B, loadedSnapshot: null });
    expect(result.current.status).toBe("loading");
    expect(result.current.steps).toEqual([]);
    await waitFor(() => expect(readMocks.readJourneyWorkflowSteps).toHaveBeenCalledTimes(2));
    expect(result.current.status).toBe("loading");

    await act(async () => {
      second.resolve(readResponse(RECOVERED_B, "Recovered B"));
      await Promise.resolve();
    });

    expect(result.current.status).toBe("success");
    expect(result.current.steps.map((step) => step.id)).toEqual([RECOVERED_B]);
  });
});

describe("useTaskCreateWorkflowSteps lifecycle", () => {
  it("releases the current interest on close, workspace change, and unmount", async () => {
    const reads: Array<{ signal: AbortSignal }> = [];
    readMocks.readJourneyWorkflowSteps.mockImplementation(
      (
        _store: unknown,
        _workflowId: string,
        options: { signal: AbortSignal },
      ): Promise<{ steps: never[]; total: number }> => {
        reads.push(options);
        return new Promise(() => undefined);
      },
    );

    const { result, rerender, unmount } = renderHook(
      (props: { open?: boolean; workspaceId?: string }) =>
        useTaskCreateWorkflowSteps({ ...baseArgs, ...props }),
    );

    await waitFor(() => expect(reads).toHaveLength(1));

    rerender({ workspaceId: WORKSPACE_B });
    await waitFor(() => expect(reads).toHaveLength(2));
    expect(reads[0]?.signal.aborted).toBe(true);

    rerender({ workspaceId: WORKSPACE_B, open: false });
    expect(result.current.status).toBe("idle");
    await waitFor(() => expect(reads[1]?.signal.aborted).toBe(true));

    rerender({ workspaceId: WORKSPACE_B, open: true });
    await waitFor(() => expect(reads).toHaveLength(3));
    unmount();
    expect(reads[2]?.signal.aborted).toBe(true);
  });

  it("rejects a response from a retired store or auth generation", async () => {
    const retired = deferred<{ steps: never[]; total: number }>();
    readMocks.readJourneyWorkflowSteps.mockReturnValueOnce(retired.promise).mockResolvedValueOnce({
      steps: [
        {
          id: "current",
          workflow_id: WORKFLOW_B,
          name: "Current",
          position: 0,
          is_start_step: true,
        },
      ],
      total: 1,
    });

    const { result, rerender } = renderHook(() => useTaskCreateWorkflowSteps(baseArgs));
    await waitFor(() => expect(readMocks.readJourneyWorkflowSteps).toHaveBeenCalledTimes(1));
    const retiredSignal = readMocks.readJourneyWorkflowSteps.mock.calls[0]?.[2]?.signal as
      | AbortSignal
      | undefined;

    scopeMocks.value = "scope-b";
    rerender();
    await waitFor(() => expect(result.current.status).toBe("success"));
    expect(retiredSignal?.aborted).toBe(true);

    await act(async () => {
      retired.resolve({ steps: [], total: 0 });
      await Promise.resolve();
    });
    expect(result.current.steps.map((step) => step.id)).toEqual(["current"]);
  });

  it("does not repeat the first demand when the form normalizes its initial cycle", async () => {
    readMocks.readJourneyWorkflowSteps.mockResolvedValue({ steps: [], total: 0 });
    const { result, rerender } = renderHook(
      (props: { openCycle: number }) => useTaskCreateWorkflowSteps({ ...baseArgs, ...props }),
      { initialProps: { openCycle: 0 } },
    );

    await waitFor(() => expect(result.current.status).toBe("success"));
    rerender({ openCycle: 1 });

    expect(readMocks.readJourneyWorkflowSteps).toHaveBeenCalledTimes(1);
  });

  it("reloads on a new dialog open cycle", async () => {
    readMocks.readJourneyWorkflowSteps.mockResolvedValue({ steps: [], total: 0 });
    const { result, rerender } = renderHook(
      (props: { openCycle: number }) => useTaskCreateWorkflowSteps({ ...baseArgs, ...props }),
      { initialProps: { openCycle: 1 } },
    );

    await waitFor(() => expect(result.current.status).toBe("success"));
    rerender({ openCycle: 2 });

    expect(result.current.status).toBe("loading");
    await waitFor(() => expect(result.current.status).toBe("success"));
    expect(readMocks.readJourneyWorkflowSteps).toHaveBeenCalledTimes(2);
  });
});
