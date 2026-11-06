import { useLayoutEffect, type ReactNode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { Task, TaskSession } from "@/lib/types/http";
import { taskId, workspaceId, workflowId } from "@/lib/types/ids";
import { useExistingControlConversation } from "./use-existing-control-conversation";

const api = vi.hoisted(() => ({ task: vi.fn(), sessions: vi.fn() }));
vi.mock("@/lib/api/domains/kanban-api", () => ({ fetchTask: api.task }));
vi.mock("@/lib/api", async (original) => ({
  ...(await original<typeof import("@/lib/api")>()),
  listTaskSessions: api.sessions,
}));

const task: Task = {
  id: taskId("task"),
  workspace_id: workspaceId("beta"),
  workflow_id: workflowId("workflow"),
  workflow_step_id: "step",
  position: 0,
  title: "Original coordinator conversation",
  description: "",
  state: "WAITING_FOR_INPUT",
  priority: "medium",
  created_at: "",
  updated_at: "",
  origin: "coordinator",
  metadata: { coordinator_id: "coordinator" },
};
const session = {
  id: "session",
  task_id: "task",
  state: "WAITING_FOR_INPUT",
  is_primary: true,
  active_subagent_count: 0,
  foreground_activity: null,
  supports_steering: false,
} as TaskSession;
function wrapper({ children }: { children: ReactNode }) {
  return <StateProvider>{children}</StateProvider>;
}
function mount() {
  const commits: Array<ReturnType<typeof useExistingControlConversation>> = [];
  const hook = renderHook(
    ({ workspace, coordinator, id }) => {
      const value = useExistingControlConversation(workspace, coordinator, id);
      const store = useAppStoreApi();
      useLayoutEffect(() => {
        commits.push(value);
      });
      return { ...value, store };
    },
    { wrapper, initialProps: { workspace: "beta", coordinator: "coordinator", id: "task" } },
  );
  return { ...hook, commits };
}

beforeEach(() => {
  api.task.mockReset().mockResolvedValue(task);
  api.sessions.mockReset().mockResolvedValue({ sessions: [session] });
});
afterEach(cleanup);

describe("verified original conversation", () => {
  it("loads sessions only after all task ownership invariants pass", async () => {
    const { result } = mount();
    await waitFor(() => expect(result.current.isLoaded).toBe(true));
    expect(result.current.task).toEqual(task);
    expect(result.current.sessions).toEqual([session]);
    expect(api.task).toHaveBeenCalledWith("task", expect.objectContaining({ cache: "no-store" }));
    expect(api.sessions.mock.calls.every(([id]) => id === "task")).toBe(true);
  });
  it.each([
    { id: "foreign-task" },
    { workspace_id: "alpha" },
    { origin: "manual" },
    { metadata: { coordinator_id: "foreign-coordinator" } },
  ])("rejects mismatched ownership %j before reading sessions", async (mismatch) => {
    api.task.mockResolvedValue({ ...task, ...mismatch });
    const { result } = mount();
    await waitFor(() => expect(result.current.taskError).toBe(true));
    expect(result.current.task).toBeNull();
    expect(api.sessions).not.toHaveBeenCalled();
  });
  it.each(["workspace", "coordinator"] as const)(
    "masks the old task in the first committed %s render",
    async (field) => {
      const { result, rerender, commits } = mount();
      await waitFor(() => expect(result.current.isLoaded).toBe(true));
      const next = Promise.withResolvers<Task>();
      api.task.mockReturnValueOnce(next.promise);
      commits.length = 0;
      rerender({ workspace: "beta", coordinator: "coordinator", id: "task", [field]: "new-owner" });
      expect(commits.every((value) => !value.task && value.sessions.length === 0)).toBe(true);
      await act(async () => next.resolve(task));
      expect(result.current.taskError).toBe(true);
    },
  );
  it("revalidates cached ownership on authenticated identity changes", async () => {
    const { result } = mount();
    await waitFor(() => expect(result.current.isLoaded).toBe(true));
    const next = Promise.withResolvers<Task>();
    api.task.mockReturnValueOnce(next.promise);
    act(() =>
      result.current.store.setState((state) => ({
        workspaceContextGeneration: state.workspaceContextGeneration + 1,
      })),
    );
    expect(result.current.task).toBeUndefined();
    expect(api.task).toHaveBeenCalledTimes(2);
    await act(async () => next.resolve(task));
    expect(result.current.task).toEqual(task);
  });
  it("aborts on unmount and ignores a transport that completes after abort", async () => {
    const pending = Promise.withResolvers<Task>();
    api.task.mockReturnValueOnce(pending.promise);
    const { unmount } = mount();
    const signal = api.task.mock.calls[0][1].init.signal as AbortSignal;
    unmount();
    expect(signal.aborted).toBe(true);
    await act(async () => pending.resolve(task));
    expect(api.sessions).not.toHaveBeenCalled();
  });
  it("keeps an old response from replacing the newly selected conversation", async () => {
    const pending = Promise.withResolvers<Task>();
    api.task.mockReturnValueOnce(pending.promise);
    const { result, rerender } = mount();
    const other = { ...task, id: "other" };
    api.task.mockResolvedValueOnce(other);
    api.sessions.mockResolvedValue({ sessions: [] });
    rerender({ workspace: "beta", coordinator: "coordinator", id: "other" });
    await waitFor(() => expect(result.current.task).toEqual(other));
    await act(async () => pending.resolve(task));
    expect(result.current.task).toEqual(other);
  });
  it("exposes a denied task read without reading or creating sessions", async () => {
    api.task.mockRejectedValue(new Error("denied"));
    const { result } = mount();
    await waitFor(() => expect(result.current.taskError).toBe(true));
    expect(api.sessions).not.toHaveBeenCalled();
  });
});
