import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Canvas } from "@/lib/api/domains/canvas-api";
import { invalidateCanvasLifecycle } from "@/lib/canvas-lifecycle";
import type { Task } from "@/lib/types/http";

const listTaskCanvasesMock = vi.hoisted(() => vi.fn());

const TASK_ID = "task-1";
const WORKSPACE_ID = "workspace-1";

vi.mock("@/lib/api/domains/canvas-api", () => ({
  listTaskCanvases: listTaskCanvasesMock,
}));

import { useTaskCanvases, useTaskCanvasesForTask, useTaskCanvasesState } from "./use-task-canvases";

const canvas: Canvas = {
  id: "canvas-1",
  plugin_instance_id: "instance-1",
  plugin_id: "plugin-1",
  workspace_id: WORKSPACE_ID,
  task_id: TASK_ID,
  scope_kind: "task",
  title: "Task canvas",
  status: "active",
};

const connectionState = { status: "connected" };
const authState = { mode: "disabled", user: null as { id: string } | null };

vi.mock("@/components/state-provider", () => ({
  useAppStore: (
    selector: (state: { connection: typeof connectionState; auth: typeof authState }) => unknown,
  ) => selector({ connection: connectionState, auth: authState }),
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

beforeEach(() => {
  listTaskCanvasesMock.mockReset().mockResolvedValue({ canvases: [canvas] });
  connectionState.status = "connected";
  authState.mode = "disabled";
  authState.user = null;
});

afterEach(cleanup);

describe("useTaskCanvases loading", () => {
  it("loads canvases for the task and workspace", async () => {
    const { result } = renderHook(() => useTaskCanvases(TASK_ID, WORKSPACE_ID));

    await waitFor(() => expect(result.current).toEqual([canvas]));
    expect(listTaskCanvasesMock).toHaveBeenCalledWith(TASK_ID, {
      workspaceId: WORKSPACE_ID,
      cache: "no-store",
    });
  });

  it("does not request canvases when disabled", () => {
    const { result } = renderHook(() => useTaskCanvases(TASK_ID, WORKSPACE_ID, false));

    expect(result.current).toEqual([]);
    expect(listTaskCanvasesMock).not.toHaveBeenCalled();
  });

  it("keeps an empty successful inventory distinct from a failed request", async () => {
    listTaskCanvasesMock.mockResolvedValueOnce({ canvases: [] });
    const successful = renderHook(() => useTaskCanvasesState(TASK_ID, WORKSPACE_ID));

    await waitFor(() => expect(successful.result.current.status).toBe("success"));
    expect(successful.result.current.canvases).toEqual([]);
    successful.unmount();

    listTaskCanvasesMock.mockRejectedValueOnce(new Error("inventory unavailable"));
    const failed = renderHook(() => useTaskCanvasesState(TASK_ID, WORKSPACE_ID));

    await waitFor(() => expect(failed.result.current.status).toBe("error"));
    expect(failed.result.current.canvases).toEqual([]);
  });

  it("loads the task inventory for desktop task entry", async () => {
    const { result } = renderHook(() =>
      useTaskCanvasesForTask(
        {
          id: TASK_ID as Task["id"],
          workspace_id: WORKSPACE_ID as Task["workspace_id"],
        },
        false,
        true,
      ),
    );

    await waitFor(() => expect(result.current).toEqual([canvas]));
    expect(listTaskCanvasesMock).toHaveBeenCalledWith(TASK_ID, {
      workspaceId: WORKSPACE_ID,
      cache: "no-store",
    });
  });

  it("ignores a stale request after the task changes", async () => {
    const first = deferred<{ canvases: Canvas[] }>();
    const second = deferred<{ canvases: Canvas[] }>();
    const nextCanvas = { ...canvas, id: "canvas-2", task_id: "task-2" };
    listTaskCanvasesMock.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);

    const { result, rerender } = renderHook(
      ({ taskId }: { taskId: string }) => useTaskCanvases(taskId, WORKSPACE_ID),
      { initialProps: { taskId: TASK_ID } },
    );

    rerender({ taskId: "task-2" });
    await waitFor(() => expect(listTaskCanvasesMock).toHaveBeenCalledTimes(2));

    second.resolve({ canvases: [nextCanvas] });
    await waitFor(() => expect(result.current).toEqual([nextCanvas]));

    await act(async () => {
      first.resolve({ canvases: [canvas] });
      await first.promise;
    });
    expect(result.current).toEqual([nextCanvas]);
    expect(listTaskCanvasesMock).toHaveBeenNthCalledWith(2, "task-2", {
      workspaceId: WORKSPACE_ID,
      cache: "no-store",
    });
  });
});

describe("useTaskCanvases request lifecycle", () => {
  it("starts a fresh shared request after lifecycle invalidation", async () => {
    const first = deferred<{ canvases: Canvas[] }>();
    const second = deferred<{ canvases: Canvas[] }>();
    listTaskCanvasesMock.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);

    const firstHook = renderHook(() => useTaskCanvasesState(TASK_ID, WORKSPACE_ID));
    const secondHook = renderHook(() => useTaskCanvasesState(TASK_ID, WORKSPACE_ID));
    await waitFor(() => expect(listTaskCanvasesMock).toHaveBeenCalledTimes(1));

    act(() => invalidateCanvasLifecycle());
    await waitFor(() => expect(listTaskCanvasesMock).toHaveBeenCalledTimes(2));

    second.resolve({ canvases: [canvas] });
    await waitFor(() => expect(firstHook.result.current.canvases).toEqual([canvas]));
    expect(secondHook.result.current.canvases).toEqual([canvas]);

    first.resolve({ canvases: [] });
    await first.promise;
    expect(firstHook.result.current.canvases).toEqual([canvas]);
  });

  it("does not reuse a connected request after reconnect invalidates it", async () => {
    const beforeDisconnect = deferred<{ canvases: Canvas[] }>();
    const duringDisconnect = deferred<{ canvases: Canvas[] }>();
    const afterReconnect = deferred<{ canvases: Canvas[] }>();
    listTaskCanvasesMock
      .mockReturnValueOnce(beforeDisconnect.promise)
      .mockReturnValueOnce(duringDisconnect.promise)
      .mockReturnValueOnce(afterReconnect.promise);

    const hook = renderHook(() => useTaskCanvasesState(TASK_ID, WORKSPACE_ID));
    await waitFor(() => expect(listTaskCanvasesMock).toHaveBeenCalledTimes(1));

    connectionState.status = "disconnected";
    hook.rerender();
    await waitFor(() => expect(listTaskCanvasesMock).toHaveBeenCalledTimes(2));
    duringDisconnect.resolve({ canvases: [] });
    await duringDisconnect.promise;

    connectionState.status = "connected";
    hook.rerender();
    await waitFor(() => expect(listTaskCanvasesMock).toHaveBeenCalledTimes(3));

    afterReconnect.resolve({ canvases: [canvas] });
    await waitFor(() => expect(hook.result.current.canvases).toEqual([canvas]));
    beforeDisconnect.resolve({ canvases: [] });
    await beforeDisconnect.promise;
  });

  it("does not share a pending inventory across authenticated identities", async () => {
    const firstIdentity = deferred<{ canvases: Canvas[] }>();
    const secondIdentity = deferred<{ canvases: Canvas[] }>();
    listTaskCanvasesMock
      .mockReturnValueOnce(firstIdentity.promise)
      .mockReturnValueOnce(secondIdentity.promise);

    const hook = renderHook(() => useTaskCanvasesState(TASK_ID, WORKSPACE_ID));
    await waitFor(() => expect(listTaskCanvasesMock).toHaveBeenCalledTimes(1));

    authState.mode = "enabled";
    authState.user = { id: "user-2" };
    hook.rerender();
    await waitFor(() => expect(listTaskCanvasesMock).toHaveBeenCalledTimes(2));

    secondIdentity.resolve({ canvases: [canvas] });
    await waitFor(() => expect(hook.result.current.canvases).toEqual([canvas]));
    firstIdentity.resolve({ canvases: [] });
    await firstIdentity.promise;
    expect(hook.result.current.canvases).toEqual([canvas]);
  });
});
