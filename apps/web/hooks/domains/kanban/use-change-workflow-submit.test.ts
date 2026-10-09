import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkflowChangePayload } from "@/lib/api/domains/kanban-api";
import type { Task, Workflow } from "@/lib/types/http";
import { ApiError } from "@/lib/api/client";
import { useChangeWorkflowSubmit } from "./use-change-workflow-submit";

const SERVER_ERROR_MESSAGE = "server error";

const mocks = vi.hoisted(() => ({
  moveTask: vi.fn(),
  toast: vi.fn(),
}));

vi.mock("@/lib/api", () => ({ moveTask: mocks.moveTask }));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: mocks.toast }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

afterEach(() => cleanup());
beforeEach(() => {
  mocks.moveTask.mockReset();
  mocks.toast.mockReset();
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function matchingTask(): Task {
  return {
    id: "task-1",
    workflow_id: "workflow-target",
    workflow_step_id: "step-pr",
    workflow_agent_overrides: {
      workflow_id: "workflow-target",
      steps: [{ source_profile_id: "profile-a", replacement_profile_id: "profile-b" }],
    },
  } as Task;
}

function input(overrides: Partial<Parameters<typeof useChangeWorkflowSubmit>[0]> = {}) {
  return {
    open: true,
    canSubmit: true,
    task: { id: "task-1", workflow_id: "workflow-source", workflow_step_id: "step-source" } as Task,
    selectedWorkflow: { id: "workflow-target", name: "Target" } as Workflow,
    selectedStepId: "step-pr",
    workflowChange: {
      expected_workflow_id: "workflow-source",
      expected_step_id: "step-source",
      expected_updated_at: "2026-09-23T12:00:00Z",
      agent_overrides: { "profile-a": "profile-b" },
    } as WorkflowChangePayload,
    refreshTask: vi.fn().mockResolvedValue(null),
    onOpenChange: vi.fn(),
    onSuccess: vi.fn(),
    ...overrides,
  };
}

describe("useChangeWorkflowSubmit", () => {
  it("refreshes and reports a source conflict without treating it as uncertain", async () => {
    const params = input();
    mocks.moveTask.mockRejectedValueOnce(
      new ApiError("stale source", 409, { code: "workflow_change_conflict" }),
    );
    const { result } = renderHook(() => useChangeWorkflowSubmit(params));

    await act(async () => expect(result.current.submit()).resolves.toBe(false));

    expect(params.refreshTask).toHaveBeenCalledOnce();
    expect(result.current.submitError).toEqual({ code: "workflow_change_conflict" });
    expect(result.current.uncertainResult).toBe(false);
    expect(result.current.isSubmitting).toBe(false);
  });

  it("keeps a validation error local and does not refresh on a client error", async () => {
    const params = input();
    mocks.moveTask.mockRejectedValueOnce(
      new ApiError("profile invalid", 400, {
        code: "workflow_agent_unavailable",
        source_profile_id: "profile-a",
      }),
    );
    const { result } = renderHook(() => useChangeWorkflowSubmit(params));

    await act(async () => expect(result.current.submit()).resolves.toBe(false));

    expect(params.refreshTask).not.toHaveBeenCalled();
    expect(result.current.submitError).toEqual({
      code: "workflow_agent_unavailable",
      source_profile_id: "profile-a",
    });
    expect(result.current.sourceProfileErrorId).toBe("profile-a");
    expect(result.current.isSubmitting).toBe(false);
  });

  it("recognizes a server-side commit after an uncertain response", async () => {
    const params = input({ refreshTask: vi.fn().mockResolvedValue(matchingTask()) });
    mocks.moveTask.mockRejectedValueOnce(new ApiError(SERVER_ERROR_MESSAGE, 503, null));
    const { result } = renderHook(() => useChangeWorkflowSubmit(params));

    await act(async () => expect(result.current.submit()).resolves.toBe(true));

    expect(mocks.toast).toHaveBeenCalledWith({
      title: "task:changeWorkflowObservedSuccess",
      variant: "success",
    });
    expect(params.onSuccess).toHaveBeenCalledOnce();
    expect(params.onOpenChange).toHaveBeenCalledWith(false);
    expect(result.current.uncertainResult).toBe(false);
  });

  it("leaves a nonmatching server-error result gated for refresh", async () => {
    const params = input();
    mocks.moveTask.mockRejectedValueOnce(new ApiError(SERVER_ERROR_MESSAGE, 503, null));
    const { result } = renderHook(() => useChangeWorkflowSubmit(params));

    await act(async () => expect(result.current.submit()).resolves.toBe(false));

    expect(result.current.uncertainResult).toBe(true);
    expect(result.current.submitError).toEqual({ code: "workflow_change_uncertain" });
    expect(params.onOpenChange).not.toHaveBeenCalled();
  });
});

describe("useChangeWorkflowSubmit explicit refresh", () => {
  it("treats a matching assignment found by the explicit refresh as success", async () => {
    const params = input({
      refreshTask: vi.fn().mockResolvedValueOnce(null).mockResolvedValueOnce(matchingTask()),
    });
    mocks.moveTask.mockRejectedValueOnce(new ApiError(SERVER_ERROR_MESSAGE, 503, null));
    const { result } = renderHook(() => useChangeWorkflowSubmit(params));
    await act(async () => expect(result.current.submit()).resolves.toBe(false));

    await act(async () => result.current.retryAfterRefresh());

    expect(mocks.toast).toHaveBeenLastCalledWith({
      title: "task:changeWorkflowObservedSuccess",
      variant: "success",
    });
    expect(params.onSuccess).toHaveBeenCalledOnce();
    expect(params.onOpenChange).toHaveBeenCalledWith(false);
    expect(result.current.uncertainResult).toBe(false);
    expect(result.current.submitError).toBeNull();
  });

  it("prevents concurrent submissions and clears the busy state after failure", async () => {
    const params = input();
    const pending = deferred<void>();
    mocks.moveTask.mockReturnValueOnce(pending.promise);
    const { result } = renderHook(() => useChangeWorkflowSubmit(params));
    let firstSubmit!: Promise<boolean>;
    let secondSubmit!: Promise<boolean>;
    act(() => {
      firstSubmit = result.current.submit();
      secondSubmit = result.current.submit();
    });

    await expect(secondSubmit).resolves.toBe(false);
    expect(mocks.moveTask).toHaveBeenCalledOnce();
    expect(result.current.isSubmitting).toBe(true);
    await act(async () => {
      pending.reject(new ApiError("invalid", 400, { code: "invalid_workflow_change" }));
      await expect(firstSubmit).resolves.toBe(false);
    });

    expect(result.current.isSubmitting).toBe(false);
    expect(result.current.submitError).toEqual({ code: "invalid_workflow_change" });
  });

  it("keeps uncertain state when refresh cannot load the task", async () => {
    const params = input({ refreshTask: vi.fn().mockResolvedValue(null) });
    mocks.moveTask.mockRejectedValueOnce(new ApiError(SERVER_ERROR_MESSAGE, 503, null));
    const { result } = renderHook(() => useChangeWorkflowSubmit(params));
    await act(async () => expect(result.current.submit()).resolves.toBe(false));

    await act(async () => result.current.retryAfterRefresh());

    expect(result.current.uncertainResult).toBe(true);
    expect(result.current.submitError).toEqual({ code: "workflow_change_uncertain" });
  });
});
