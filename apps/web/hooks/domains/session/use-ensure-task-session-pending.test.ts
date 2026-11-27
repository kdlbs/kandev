import { act, renderHook } from "@testing-library/react";
import { createElement, StrictMode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  beginTaskRemoval,
  createTaskRemovalState,
  type TaskRemovalState,
} from "@/lib/state/task-removal";

const ensure = vi.fn();
let loadSessions = vi.fn();
const store = {
  userSettings: { preventAutoStartAgentOnOpen: false },
  kanban: { workflowId: null, steps: [], isLoading: false },
  kanbanMulti: { snapshots: {} },
  taskRemoval: undefined as TaskRemovalState | undefined,
};

vi.mock("@/lib/services/session-launch-service", () => ({
  ensureTaskSession: (...args: unknown[]) => ensure(...args),
}));
vi.mock("@/hooks/use-task-sessions", () => ({
  useTaskSessions: () => ({ sessions: [], isLoaded: true, loadSessions }),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof store) => unknown) => selector(store),
}));

import { useEnsureTaskSession } from "./use-ensure-task-session";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((accept, fail) => {
    resolve = accept;
    reject = fail;
  });
  return { promise, resolve, reject };
}

function blockAdmission(reason: string) {
  if (reason === "removal") {
    store.taskRemoval =
      beginTaskRemoval(createTaskRemovalState(), {
        token: "remove-child",
        action: "archive",
        workspaceId: null,
        taskIds: ["child"],
        requestIds: ["child"],
        departure: null,
      }) ?? undefined;
  }
  return {
    enabled: reason !== "disabled",
    isArchived: reason === "archived",
    archiveStateKnown: reason !== "archive unknown",
  };
}

beforeEach(() => {
  ensure.mockReset();
  loadSessions = vi.fn().mockResolvedValue(undefined);
  store.taskRemoval = undefined;
});

describe("pending session admission", () => {
  // @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.4
  it("shows the rejection after same-task hydration changes the session loader", async () => {
    const request = deferred<unknown>();
    ensure.mockReturnValue(request.promise);
    const { result, rerender } = renderHook(() => useEnsureTaskSession({ id: "child" }));
    expect(result.current.status).toBe("preparing");
    loadSessions = vi.fn().mockResolvedValue(undefined);
    rerender();
    const error = new Error("workspace_reuse_unsupported: use new_workspace");
    await act(async () => request.reject(error));
    expect(result.current.status).toBe("error");
    expect(result.current.error).toBe(error);
    expect(ensure).toHaveBeenCalledTimes(1);
  });

  it("settles a successful request after same-task hydration", async () => {
    const request = deferred<unknown>();
    ensure.mockReturnValue(request.promise);
    const { result, rerender } = renderHook(() => useEnsureTaskSession({ id: "child" }));
    const initialLoader = loadSessions;
    loadSessions = vi.fn().mockResolvedValue(undefined);
    rerender();
    await act(async () => request.resolve({ success: true }));
    expect(result.current.status).toBe("idle");
    expect(result.current.error).toBeNull();
    expect(initialLoader).toHaveBeenCalledWith(true);
    expect(ensure).toHaveBeenCalledTimes(1);
  });

  it("ignores the previous request when navigating away and back to the same task", async () => {
    const old = deferred<unknown>();
    const current = deferred<unknown>();
    ensure
      .mockReturnValueOnce(old.promise)
      .mockResolvedValueOnce({ success: true })
      .mockReturnValueOnce(current.promise);
    const { result, rerender } = renderHook(({ id }) => useEnsureTaskSession({ id }), {
      initialProps: { id: "child" },
    });
    rerender({ id: "other" });
    rerender({ id: "child" });
    await act(async () => old.reject(new Error("stale rejection")));
    expect(result.current.status).toBe("preparing");
    expect(result.current.error).toBeNull();
    const error = new Error("current rejection");
    await act(async () => current.reject(error));
    expect(result.current.error).toBe(error);
  });

  it("does not reload sessions when the request succeeds after unmount", async () => {
    const request = deferred<unknown>();
    ensure.mockReturnValue(request.promise);
    const { unmount } = renderHook(() => useEnsureTaskSession({ id: "child" }));
    unmount();
    await act(async () => request.resolve({ success: true }));
    expect(loadSessions).not.toHaveBeenCalled();
  });

  it("preserves one admitted request through Strict Mode effect replay", async () => {
    const request = deferred<unknown>();
    ensure.mockReturnValue(request.promise);
    const { result } = renderHook(() => useEnsureTaskSession({ id: "child" }), {
      wrapper: ({ children }) => createElement(StrictMode, null, children),
    });
    const error = new Error("workspace_reuse_unsupported");
    await act(async () => request.reject(error));
    expect(result.current.error).toBe(error);
    expect(ensure).toHaveBeenCalledTimes(1);
  });
});

describe("session admission revocation", () => {
  it.each(["disabled", "archived", "archive unknown", "removal"])(
    "ignores pending results after admission becomes %s",
    async (reason) => {
      for (const outcome of ["success", "failure"]) {
        const request = deferred<unknown>();
        ensure.mockReturnValue(request.promise);
        const { result, rerender, unmount } = renderHook(
          ({ enabled, isArchived, archiveStateKnown }) =>
            useEnsureTaskSession({ id: "child", isArchived, archiveStateKnown }, { enabled }),
          { initialProps: { enabled: true, isArchived: false, archiveStateKnown: true } },
        );
        expect(result.current.status).toBe("preparing");
        rerender(blockAdmission(reason));
        await act(async () => {
          if (outcome === "success") request.resolve({ success: true });
          else request.reject(new Error("stale admission rejection"));
        });
        expect(result.current.status).toBe("idle");
        expect(result.current.error).toBeNull();
        expect(loadSessions).not.toHaveBeenCalled();
        unmount();
        store.taskRemoval = undefined;
      }
    },
  );

  it("keeps a fresh request when admission is disabled and re-enabled", async () => {
    const old = deferred<unknown>();
    const current = deferred<unknown>();
    ensure.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise);
    const { result, rerender } = renderHook(
      ({ enabled }) => useEnsureTaskSession({ id: "child" }, { enabled }),
      { initialProps: { enabled: true } },
    );
    rerender({ enabled: false });
    rerender({ enabled: true });
    expect(ensure).toHaveBeenCalledTimes(2);
    await act(async () => old.reject(new Error("obsolete request")));
    expect(result.current.status).toBe("preparing");
    expect(result.current.error).toBeNull();
    const error = new Error("current request");
    await act(async () => current.reject(error));
    expect(result.current.status).toBe("error");
    expect(result.current.error).toBe(error);
  });
});
