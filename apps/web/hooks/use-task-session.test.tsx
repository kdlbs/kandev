import { type ReactNode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { sessionId, taskId, type TaskSession } from "@/lib/types/http";
import { useTaskSession } from "./use-task-session";

const ALPHA_SESSION = "alpha-session";
const BETA_SESSION = "beta-session";

const transport = vi.hoisted(() => ({ available: true, request: vi.fn() }));
vi.mock("@/lib/ws/connection", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/ws/connection")>()),
  getWebSocketClient: () => (transport.available ? { request: transport.request } : null),
}));

type SessionList = { sessions: Array<{ id: string; is_primary?: boolean }> };
function deferred() {
  let resolve!: (value: SessionList) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<SessionList>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

function storedSession(id: string, owner: string, primary = false): TaskSession {
  return {
    id: sessionId(id),
    task_id: taskId(owner),
    state: "RUNNING",
    is_primary: primary,
    started_at: "2026-10-06T00:00:00Z",
    updated_at: "2026-10-06T00:00:00Z",
  };
}

function fixture() {
  let store!: ReturnType<typeof useAppStoreApi>;
  function Capture({ children }: { children: ReactNode }) {
    store = useAppStoreApi();
    return children;
  }
  function wrapper({ children }: { children: ReactNode }) {
    return (
      <StateProvider>
        <Capture>{children}</Capture>
      </StateProvider>
    );
  }
  return { wrapper, getStore: () => store };
}

function mount(task: string | null = "alpha") {
  const state = fixture();
  const renders: Array<{ task: string | null } & ReturnType<typeof useTaskSession>> = [];
  const view = renderHook(
    ({ task }) => {
      const value = useTaskSession(task);
      renders.push({ task, ...value });
      return value;
    },
    { initialProps: { task }, wrapper: state.wrapper },
  );
  return { ...view, ...state, renders };
}

beforeEach(() => {
  transport.available = true;
  transport.request.mockReset();
});
afterEach(() => cleanup());

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.8 through .11
// eslint-disable-next-line max-lines-per-function -- one resolver contract with deferred transport cases.
describe("task session fallback ownership", () => {
  it.each([false, true])(
    "hides settled Alpha on the first Beta render (close first: %s)",
    async (closeFirst) => {
      const beta = deferred();
      transport.request.mockImplementation((_method, args) =>
        args.task_id === "alpha"
          ? Promise.resolve({ sessions: [{ id: ALPHA_SESSION }] })
          : beta.promise,
      );
      const view = mount();
      await waitFor(() => expect(view.result.current.sessionId).toBe(ALPHA_SESSION));
      try {
        if (closeFirst) {
          view.rerender({ task: null });
          expect(view.result.current.sessionId).toBeNull();
          expect(view.result.current.hasSession).toBe(false);
          expect(view.result.current.isLoading).toBeFalsy();
        }
        view.rerender({ task: "beta" });
        expect(view.renders.find((render) => render.task === "beta")?.sessionId).toBeNull();
        expect(view.result.current).toEqual({
          sessionId: null,
          hasSession: false,
          isLoading: true,
        });
        await act(async () => beta.resolve({ sessions: [{ id: BETA_SESSION }] }));
        expect(view.result.current).toEqual({
          sessionId: BETA_SESSION,
          hasSession: true,
          isLoading: false,
        });
      } finally {
        await act(async () => beta.resolve({ sessions: [] }));
      }
    },
  );

  it.each(["success", "failure"])(
    "ignores old %s without clearing Beta loading",
    async (outcome) => {
      const alpha = deferred();
      const beta = deferred();
      transport.request.mockImplementation((_method, args) =>
        args.task_id === "alpha" ? alpha.promise : beta.promise,
      );
      const view = mount();
      view.rerender({ task: "beta" });
      try {
        await act(async () => {
          if (outcome === "success") alpha.resolve({ sessions: [{ id: "old-alpha" }] });
          else alpha.reject(new Error("old read failed"));
        });
        expect(view.result.current).toEqual({
          sessionId: null,
          hasSession: false,
          isLoading: true,
        });
        await act(async () => beta.resolve({ sessions: [{ id: BETA_SESSION }] }));
        expect(view.result.current.sessionId).toBe(BETA_SESSION);
      } finally {
        await act(async () => {
          alpha.resolve({ sessions: [] });
          beta.resolve({ sessions: [] });
        });
      }
    },
  );

  it.each([true, false])(
    "prefers current store primary-or-first (primary: %s)",
    async (hasPrimary) => {
      transport.request.mockResolvedValue({ sessions: [{ id: ALPHA_SESSION }] });
      const view = mount();
      await waitFor(() => expect(view.result.current.sessionId).toBe(ALPHA_SESSION));
      act(() =>
        view
          .getStore()
          .getState()
          .setTaskSessionsForTask(
            "beta",
            [
              storedSession("beta-first", "beta"),
              storedSession("beta-primary", "beta", hasPrimary),
            ],
            {},
          ),
      );
      view.rerender({ task: "beta" });
      expect(view.result.current).toEqual({
        sessionId: hasPrimary ? "beta-primary" : "beta-first",
        hasSession: true,
        isLoading: false,
      });
      expect(transport.request).toHaveBeenCalledTimes(1);
    },
  );

  it("keeps store takeover when the abandoned fallback settles", async () => {
    const pending = deferred();
    transport.request.mockReturnValue(pending.promise);
    const view = mount();
    act(() =>
      view
        .getStore()
        .getState()
        .setTaskSessionsForTask("alpha", [storedSession("store-primary", "alpha", true)], {}),
    );
    await act(async () => pending.resolve({ sessions: [{ id: "old-fallback" }] }));
    expect(view.result.current).toEqual({
      sessionId: "store-primary",
      hasSession: true,
      isLoading: false,
    });
  });

  it("keeps first-list fallback and the transport contract", async () => {
    transport.request.mockResolvedValue({
      sessions: [{ id: "first" }, { id: "primary", is_primary: true }],
    });
    const view = mount();
    await waitFor(() => expect(view.result.current.sessionId).toBe("first"));
    expect(view.result.current.hasSession).toBe(true);
    expect(view.result.current.isLoading).toBe(false);
    expect(transport.request).toHaveBeenCalledWith(
      "task.session.list",
      { task_id: "alpha" },
      10000,
    );
  });

  it.each(["empty", "failure", "no-client"])(
    "settles a current %s read without a session",
    async (outcome) => {
      transport.available = outcome !== "no-client";
      if (outcome === "failure")
        transport.request.mockRejectedValue(new Error("current read failed"));
      else transport.request.mockResolvedValue({ sessions: [] });
      const view = mount();
      await waitFor(() => expect(view.result.current.isLoading).toBe(false));
      expect(view.result.current.sessionId).toBeNull();
      expect(view.result.current.hasSession).toBe(false);
      if (outcome === "no-client") expect(transport.request).not.toHaveBeenCalled();
    },
  );

  it("hides loading while closed and ignores settlement after unmount", async () => {
    const pending = deferred();
    transport.request.mockReturnValue(pending.promise);
    const view = mount();
    expect(view.result.current.isLoading).toBe(true);
    view.rerender({ task: null });
    expect(view.result.current.sessionId).toBeNull();
    expect(view.result.current.isLoading).toBeFalsy();
    const count = view.renders.length;
    view.unmount();
    await act(async () => pending.resolve({ sessions: [{ id: "late" }] }));
    expect(view.renders).toHaveLength(count);
  });

  it("retains the existing same-task fallback while a reopened read is pending", async () => {
    const reopened = deferred();
    transport.request
      .mockResolvedValueOnce({ sessions: [{ id: "retained-alpha" }] })
      .mockReturnValue(reopened.promise);
    const view = mount();
    await waitFor(() => expect(view.result.current.sessionId).toBe("retained-alpha"));
    view.rerender({ task: null });
    view.rerender({ task: "alpha" });
    expect(view.result.current).toEqual({
      sessionId: "retained-alpha",
      hasSession: true,
      isLoading: true,
    });
    await act(async () => reopened.resolve({ sessions: [] }));
    expect(view.result.current.sessionId).toBeNull();
  });

  it("keeps two mounted instances independent in the same real store", async () => {
    const alpha = deferred();
    const beta = deferred();
    transport.request.mockImplementation((_method, args) =>
      args.task_id === "alpha" ? alpha.promise : beta.promise,
    );
    const state = fixture();
    const view = renderHook(
      ({ left }) => ({ left: useTaskSession(left), right: useTaskSession("beta") }),
      { initialProps: { left: "alpha" as string | null }, wrapper: state.wrapper },
    );
    await act(async () => alpha.resolve({ sessions: [{ id: ALPHA_SESSION }] }));
    expect(view.result.current.right).toEqual({
      sessionId: null,
      hasSession: false,
      isLoading: true,
    });
    view.rerender({ left: null });
    await act(async () => beta.resolve({ sessions: [{ id: BETA_SESSION }] }));
    expect(view.result.current.left.sessionId).toBeNull();
    expect(view.result.current.right).toEqual({
      sessionId: BETA_SESSION,
      hasSession: true,
      isLoading: false,
    });
  });
});
