import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";

const workflowApiMocks = vi.hoisted(() => ({ listWorkflowSteps: vi.fn() }));
const WORKSPACE_A = "workspace-a";
const WORKFLOW_A = "workflow-a";

vi.mock("@/lib/api/domains/workflow-api", () => workflowApiMocks);

import { useWorkflowOptionPreviews } from "./use-workflow-option-previews";

type StepResponse = {
  steps: Array<{
    id: string;
    name: string;
    position: number;
    color: string;
    is_start_step?: boolean;
    agent_profile_id?: string;
  }>;
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function steps(names: string[]): StepResponse {
  return {
    steps: names.map((name, position) => ({
      id: `${name}-${position}`,
      name,
      position,
      color: "#123456",
    })),
  };
}

beforeEach(() => {
  workflowApiMocks.listWorkflowSteps.mockReset();
});

// @covers AC-TASKS-CREATE-WORKFLOW-STEPS-001.1 and AC-TASKS-CREATE-WORKFLOW-STEPS-001.2
it("distinguishes loading from successful empty results and orders step metadata", async () => {
  const populated = deferred<StepResponse>();
  const empty = deferred<StepResponse>();
  const failed = deferred<StepResponse>();
  workflowApiMocks.listWorkflowSteps.mockImplementation((workflowId: string) => {
    if (workflowId === "populated") return populated.promise;
    if (workflowId === "empty") return empty.promise;
    return failed.promise;
  });

  const { result } = renderHook(() =>
    useWorkflowOptionPreviews(WORKSPACE_A, true, ["populated", "empty", "failed"]),
  );

  expect(result.current.previews).toMatchObject({
    populated: { status: "loading" },
    empty: { status: "loading" },
    failed: { status: "loading" },
  });
  expect(workflowApiMocks.listWorkflowSteps).toHaveBeenCalledTimes(3);
  expect(workflowApiMocks.listWorkflowSteps).toHaveBeenCalledWith("populated", {
    cache: "no-store",
  });

  await act(async () => {
    populated.resolve({
      steps: [
        {
          id: "later",
          name: "Later",
          position: 2,
          color: "#abcdef",
          is_start_step: false,
        },
        {
          id: "first",
          name: "First",
          position: 0,
          color: "#123456",
          is_start_step: true,
          agent_profile_id: "agent-profile-1",
        },
      ],
    });
    empty.resolve({ steps: [] });
    failed.reject(new Error("private server detail"));
  });

  await waitFor(() => {
    expect(result.current.previews.populated?.status).toBe("success");
    expect(result.current.previews.empty?.status).toBe("success");
    expect(result.current.previews.failed?.status).toBe("error");
  });
  expect(result.current.previews.populated).toEqual({
    status: "success",
    steps: [
      {
        id: "first",
        title: "First",
        position: 0,
        color: "#123456",
        is_start_step: true,
        agent_profile_id: "agent-profile-1",
      },
      {
        id: "later",
        title: "Later",
        position: 2,
        color: "#abcdef",
        is_start_step: false,
        agent_profile_id: undefined,
      },
    ],
  });
  expect(result.current.previews.empty).toEqual({ status: "success", steps: [] });
  expect(result.current.previews.failed).toEqual({ status: "error" });
});

// @covers AC-TASKS-CREATE-WORKFLOW-STEPS-001.3
it("retains successful rows when one request fails and retries only that row", async () => {
  workflowApiMocks.listWorkflowSteps
    .mockResolvedValueOnce(steps(["Feature analysis"]))
    .mockRejectedValueOnce(new Error("do not expose this"))
    .mockResolvedValueOnce(steps(["Recovered review"]));

  const { result } = renderHook(() =>
    useWorkflowOptionPreviews("workspace-a", true, ["feature", "review"]),
  );

  await waitFor(() => expect(result.current.previews.review?.status).toBe("error"));
  expect(result.current.previews.feature).toMatchObject({
    status: "success",
    steps: [{ title: "Feature analysis" }],
  });

  act(() => result.current.retry("review"));
  expect(result.current.previews.review?.status).toBe("loading");
  await waitFor(() => expect(result.current.previews.review?.status).toBe("success"));

  expect(workflowApiMocks.listWorkflowSteps).toHaveBeenCalledTimes(3);
  expect(workflowApiMocks.listWorkflowSteps.mock.calls.map(([id]) => id)).toEqual([
    "feature",
    "review",
    "review",
  ]);
  expect(result.current.previews.review).toMatchObject({
    status: "success",
    steps: [{ title: "Recovered review" }],
  });
});

// @covers AC-TASKS-CREATE-WORKFLOW-STEPS-001.4
it("masks old scopes synchronously and ignores their late responses", async () => {
  const oldRequest = deferred<StepResponse>();
  const currentRequest = deferred<StepResponse>();
  workflowApiMocks.listWorkflowSteps.mockImplementation((workflowId: string) =>
    workflowId === "old-workflow" ? oldRequest.promise : currentRequest.promise,
  );

  const { result, rerender } = renderHook(
    ({ workspaceId, open, workflowIds }) =>
      useWorkflowOptionPreviews(workspaceId, open, workflowIds),
    {
      initialProps: {
        workspaceId: "workspace-a" as string | null,
        open: true,
        workflowIds: ["old-workflow"],
      },
    },
  );

  rerender({
    workspaceId: "workspace-b",
    open: true,
    workflowIds: ["current-workflow"],
  });
  expect(result.current.previews).toEqual({ "current-workflow": { status: "loading" } });

  await act(async () => {
    oldRequest.resolve(steps(["Old workspace result"]));
    currentRequest.resolve(steps(["Current workspace result"]));
  });

  await waitFor(() => expect(result.current.previews["current-workflow"]?.status).toBe("success"));
  expect(result.current.previews).not.toHaveProperty("old-workflow");
  expect(result.current.previews["current-workflow"]).toMatchObject({
    status: "success",
    steps: [{ title: "Current workspace result" }],
  });

  rerender({ workspaceId: "workspace-b", open: false, workflowIds: ["current-workflow"] });
  expect(result.current.previews).toEqual({});
});

// @covers AC-TASKS-CREATE-WORKFLOW-STEPS-001.4
it("refreshes removed steps when the selector reopens", async () => {
  workflowApiMocks.listWorkflowSteps
    .mockResolvedValueOnce(steps(["Old step"]))
    .mockResolvedValueOnce(steps(["Current step"]));

  const { result, rerender } = renderHook(
    ({ open }) => useWorkflowOptionPreviews(WORKSPACE_A, open, [WORKFLOW_A]),
    { initialProps: { open: true } },
  );
  await waitFor(() => expect(result.current.previews[WORKFLOW_A]?.status).toBe("success"));
  expect(result.current.previews[WORKFLOW_A]).toMatchObject({
    steps: [{ title: "Old step" }],
  });

  rerender({ open: false });
  expect(result.current.previews).toEqual({});
  rerender({ open: true });
  expect(result.current.previews[WORKFLOW_A]).toEqual({ status: "loading" });
  await waitFor(() => expect(result.current.previews[WORKFLOW_A]?.status).toBe("success"));
  expect(result.current.previews[WORKFLOW_A]).toMatchObject({
    steps: [{ title: "Current step" }],
  });
  expect(workflowApiMocks.listWorkflowSteps).toHaveBeenCalledTimes(2);
});

// @covers AC-TASKS-CREATE-WORKFLOW-STEPS-001.4
it("ignores results for workflows removed from the open selector", async () => {
  const removed = deferred<StepResponse>();
  workflowApiMocks.listWorkflowSteps.mockImplementation((workflowId: string) =>
    workflowId === "removed" ? removed.promise : Promise.resolve(steps(["Kept step"])),
  );

  const { result, rerender } = renderHook(
    ({ workflowIds }) => useWorkflowOptionPreviews(WORKSPACE_A, true, workflowIds),
    { initialProps: { workflowIds: ["kept", "removed"] } },
  );
  rerender({ workflowIds: ["kept"] });
  await waitFor(() => expect(result.current.previews.kept?.status).toBe("success"));
  await act(async () => {
    removed.resolve(steps(["Removed step"]));
  });

  expect(result.current.previews).not.toHaveProperty("removed");
  expect(result.current.previews.kept).toMatchObject({
    steps: [{ title: "Kept step" }],
  });
});

// @covers AC-TASKS-CREATE-WORKFLOW-STEPS-001.1 and AC-TASKS-CREATE-WORKFLOW-STEPS-001.4
it("does not duplicate requests after ordinary rerenders", () => {
  workflowApiMocks.listWorkflowSteps.mockReturnValue(new Promise(() => {}));
  const { rerender } = renderHook(
    ({ workflowIds }) => useWorkflowOptionPreviews(WORKSPACE_A, true, workflowIds),
    { initialProps: { workflowIds: ["workflow-b", WORKFLOW_A, WORKFLOW_A] } },
  );

  rerender({ workflowIds: [WORKFLOW_A, "workflow-b"] });

  expect(workflowApiMocks.listWorkflowSteps).toHaveBeenCalledTimes(2);
  expect(workflowApiMocks.listWorkflowSteps.mock.calls.map(([id]) => id)).toEqual([
    WORKFLOW_A,
    "workflow-b",
  ]);
});

// @covers AC-UI-LIST-STEP-GROUPING-001.4
it("refreshes an open workflow and rejects its previous in-flight response", async () => {
  const oldRequest = deferred<StepResponse>();
  workflowApiMocks.listWorkflowSteps
    .mockReturnValueOnce(oldRequest.promise)
    .mockResolvedValueOnce(steps(["Renamed step"]));
  const { result, rerender } = renderHook(
    ({ revision }) => useWorkflowOptionPreviews(WORKSPACE_A, true, [WORKFLOW_A], revision),
    { initialProps: { revision: 0 } },
  );
  rerender({ revision: 1 });
  await waitFor(() =>
    expect(result.current.previews[WORKFLOW_A]).toMatchObject({
      status: "success",
      steps: [{ title: "Renamed step" }],
    }),
  );
  await act(async () => oldRequest.resolve(steps(["Stale step"])));
  expect(result.current.previews[WORKFLOW_A]).toMatchObject({ steps: [{ title: "Renamed step" }] });
});
