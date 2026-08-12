import { createElement, type ReactNode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { TaskLspLanguageSnapshot, TaskLspSnapshot } from "@/lib/types/http-lsp";

const api = vi.hoisted(() => ({
  get: vi.fn(),
  start: vi.fn(),
  stop: vi.fn(),
  restart: vi.fn(),
  policy: vi.fn(),
}));
const websocket = vi.hoisted(() => ({
  subscribe: vi.fn(),
  unsubscribe: vi.fn(),
}));

vi.mock("@/lib/api/domains/lsp-api", () => ({
  getTaskLsp: api.get,
  startTaskLsp: api.start,
  stopTaskLsp: api.stop,
  restartTaskLsp: api.restart,
  setTaskLspPolicy: api.policy,
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ subscribe: websocket.subscribe }),
}));

import { useTaskLsp } from "./use-task-lsp";

const NOW = "2026-08-05T10:00:00Z";
const STOP_FAILED = "stop failed";

function language(revision: number, phase: TaskLspLanguageSnapshot["phase"] = "off") {
  return {
    task_id: "task-1",
    language: "kotlin",
    policy: "inherit",
    detected: true,
    detection_state: "complete",
    detection_truncated: false,
    phase,
    generation: phase === "off" ? 0 : 1,
    revision,
    last_transition_at: NOW,
    last_action: "",
    last_initiator: "automatic",
    restart_required: false,
    created_at: NOW,
    updated_at: NOW,
    effective_policy: "inherit",
    activity: "idle",
    progress: [],
  } satisfies TaskLspLanguageSnapshot;
}

function snapshot(item = language(1)): TaskLspSnapshot {
  return {
    task_id: "task-1",
    languages: [item],
    capacity: { active: 0, queued: 0, limit: 4 },
  };
}

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

function cachedConnectedWrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, {
    children,
    initialState: {
      connection: { status: "connected", error: null, issueSeverity: "none" },
      taskLsp: {
        byTaskId: {
          "task-1": {
            languages: { kotlin: language(4, "ready") },
            capacity: { active: 1, queued: 0, limit: 4 },
            loaded: true,
            loading: false,
            error: null,
          },
        },
        pendingByKey: {},
      },
    },
  });
}

function subject(taskId: string | null) {
  return { lsp: useTaskLsp(taskId), store: useAppStoreApi() };
}

beforeEach(() => {
  for (const mock of Object.values(api)) mock.mockReset();
  websocket.subscribe.mockReset();
  websocket.unsubscribe.mockReset();
  websocket.subscribe.mockReturnValue(websocket.unsubscribe);
  api.get.mockResolvedValue(snapshot());
});

afterEach(cleanup);

describe("useTaskLsp", () => {
  it("loads one task snapshot and exposes languages independent of active files", async () => {
    const view = renderHook(() => subject("task-1"), { wrapper });
    await waitFor(() => expect(view.result.current.lsp.loaded).toBe(true));
    expect(websocket.subscribe).toHaveBeenCalledWith("task-1");
    expect(api.get).toHaveBeenCalledWith(
      "task-1",
      expect.objectContaining({ init: expect.objectContaining({ signal: expect.anything() }) }),
    );
    expect(view.result.current.lsp.languages.map((item) => item.language)).toEqual(["kotlin"]);
    view.unmount();
    expect(websocket.unsubscribe).toHaveBeenCalledOnce();
  });

  it("does not let a late fetch rewind a newer live revision", async () => {
    let resolve!: (value: TaskLspSnapshot) => void;
    api.get.mockReturnValueOnce(new Promise<TaskLspSnapshot>((done) => (resolve = done)));
    const view = renderHook(() => subject("task-1"), { wrapper });
    act(() => view.result.current.store.getState().mergeTaskLspLanguage(language(8, "ready")));
    await act(async () => resolve(snapshot(language(7, "starting"))));
    expect(view.result.current.lsp.byLanguage.kotlin?.phase).toBe("ready");
  });
});

describe("useTaskLsp reconnection", () => {
  it("keeps the newest authoritative response when an older unseen epoch settles last", async () => {
    const initial = snapshot(language(4, "initializing"));
    initial.capacity = { active: 1, queued: 0, limit: 4, epoch: "epoch-a", revision: 4 };
    api.get.mockResolvedValueOnce(initial);
    const view = renderHook(() => subject("task-1"), { wrapper });
    await waitFor(() => expect(view.result.current.lsp.capacity.epoch).toBe("epoch-a"));

    let resolveOlder!: (value: TaskLspSnapshot) => void;
    api.get.mockReturnValueOnce(
      new Promise<TaskLspSnapshot>((resolve) => {
        resolveOlder = resolve;
      }),
    );
    const older = view.result.current.lsp.refetch();

    const newest = snapshot(language(6, "ready"));
    newest.capacity = { active: 3, queued: 0, limit: 4, epoch: "epoch-c", revision: 6 };
    api.get.mockResolvedValueOnce(newest);
    await act(async () => view.result.current.lsp.refetch());

    const stale = snapshot(language(5, "starting"));
    stale.capacity = { active: 2, queued: 0, limit: 4, epoch: "epoch-b", revision: 5 };
    await act(async () => {
      resolveOlder(stale);
      await older;
    });

    expect(view.result.current.lsp.capacity).toEqual(newest.capacity);
  });

  it("retries a failed initial load when the subscription reconnects", async () => {
    api.get.mockRejectedValueOnce(new Error("initial load failed"));
    const view = renderHook(() => subject("task-1"), { wrapper });
    await waitFor(() => expect(view.result.current.lsp.error).toBe("initial load failed"));
    const callsBeforeReconnect = api.get.mock.calls.length;

    act(() => view.result.current.store.getState().setConnectionStatus("disconnected"));
    act(() => view.result.current.store.getState().setConnectionStatus("connected"));

    await waitFor(() => expect(view.result.current.lsp.loaded).toBe(true));
    expect(api.get.mock.calls.length).toBeGreaterThan(callsBeforeReconnect);
  });

  it("refreshes stable task state after the WebSocket subscription reconnects", async () => {
    let serverSnapshot = snapshot(language(4, "ready"));
    api.get.mockImplementation(async () => serverSnapshot);
    const view = renderHook(() => subject("task-1"), { wrapper });
    await waitFor(() => expect(view.result.current.lsp.byLanguage.kotlin?.phase).toBe("ready"));

    act(() => view.result.current.store.getState().setConnectionStatus("disconnected"));
    serverSnapshot = snapshot(language(5, "off"));
    act(() => view.result.current.store.getState().setConnectionStatus("connected"));

    await waitFor(() => expect(view.result.current.lsp.byLanguage.kotlin?.phase).toBe("off"));
    expect(websocket.subscribe).toHaveBeenLastCalledWith("task-1");
  });

  it("refreshes cached stable state when a connected subscription is established", async () => {
    api.get.mockResolvedValue(snapshot(language(5, "off")));
    const view = renderHook(() => subject("task-1"), { wrapper: cachedConnectedWrapper });

    await waitFor(() => expect(view.result.current.lsp.byLanguage.kotlin?.phase).toBe("off"));
    expect(api.get).toHaveBeenCalled();
  });

  it("does not let a delayed reconnect refresh rewind a newer live event", async () => {
    let resolveReconnect!: (value: TaskLspSnapshot) => void;
    api.get.mockResolvedValueOnce(snapshot(language(4, "ready")));
    const view = renderHook(() => subject("task-1"), { wrapper });
    await waitFor(() => expect(view.result.current.lsp.loaded).toBe(true));
    act(() => view.result.current.store.getState().setConnectionStatus("disconnected"));
    const callsBeforeReconnect = api.get.mock.calls.length;
    api.get.mockReturnValueOnce(
      new Promise<TaskLspSnapshot>((resolve) => {
        resolveReconnect = resolve;
      }),
    );

    act(() => view.result.current.store.getState().setConnectionStatus("connected"));
    await waitFor(() => expect(api.get.mock.calls.length).toBeGreaterThan(callsBeforeReconnect));
    act(() => view.result.current.store.getState().mergeTaskLspLanguage(language(6, "error")));
    await act(async () => resolveReconnect(snapshot(language(5, "off"))));

    expect(view.result.current.lsp.byLanguage.kotlin?.phase).toBe("error");
    expect(view.result.current.lsp.byLanguage.kotlin?.revision).toBe(6);
  });
});

describe("useTaskLsp overlapping refreshes", () => {
  it("does not retain an older refresh error after a newer refresh succeeds", async () => {
    const view = renderHook(() => subject("task-1"), { wrapper });
    await waitFor(() => expect(view.result.current.lsp.loaded).toBe(true));

    let rejectOlder!: (reason: Error) => void;
    api.get.mockReturnValueOnce(
      new Promise<TaskLspSnapshot>((_, reject) => {
        rejectOlder = reject;
      }),
    );
    const older = view.result.current.lsp.refetch();

    let resolveNewer!: (value: TaskLspSnapshot) => void;
    api.get.mockReturnValueOnce(
      new Promise<TaskLspSnapshot>((resolve) => {
        resolveNewer = resolve;
      }),
    );
    const newer = view.result.current.lsp.refetch();

    await act(async () => {
      rejectOlder(new Error("stale refresh failed"));
      await expect(older).rejects.toThrow("stale refresh failed");
    });
    expect(view.result.current.lsp.error).toBe("stale refresh failed");
    await act(async () => {
      resolveNewer(snapshot(language(5, "ready")));
      await newer;
    });

    expect(view.result.current.lsp.error).toBeNull();
    expect(view.result.current.lsp.byLanguage.kotlin?.revision).toBe(5);
  });
});

describe("useTaskLsp controls", () => {
  it("runs controls through one API seam and merges the authoritative result", async () => {
    api.start.mockResolvedValue(language(2, "starting"));
    const view = renderHook(() => subject("task-1"), { wrapper });
    await waitFor(() => expect(view.result.current.lsp.loaded).toBe(true));
    await act(async () => view.result.current.lsp.start("kotlin"));
    expect(api.start).toHaveBeenCalledWith("task-1", "kotlin");
    expect(view.result.current.lsp.byLanguage.kotlin?.phase).toBe("starting");
    expect(view.result.current.lsp.pending.kotlin).toBeUndefined();
  });

  it("clears a failed control error after a successful retry", async () => {
    api.start.mockRejectedValueOnce(new Error("start failed"));
    const view = renderHook(() => subject("task-1"), { wrapper });
    await waitFor(() => expect(view.result.current.lsp.loaded).toBe(true));

    await act(async () => {
      await expect(view.result.current.lsp.start("kotlin")).rejects.toThrow("start failed");
    });
    expect(view.result.current.lsp.error).toBe("start failed");

    api.start.mockResolvedValueOnce(language(2, "starting"));
    await act(async () => view.result.current.lsp.start("kotlin"));
    expect(view.result.current.lsp.error).toBeNull();
  });

  it("keeps a control failure when an older refetch settles afterward", async () => {
    const view = renderHook(() => subject("task-1"), { wrapper });
    await waitFor(() => expect(view.result.current.lsp.loaded).toBe(true));

    let resolveRefetch!: (value: TaskLspSnapshot) => void;
    api.get.mockReturnValueOnce(
      new Promise<TaskLspSnapshot>((resolve) => {
        resolveRefetch = resolve;
      }),
    );
    const refetch = view.result.current.lsp.refetch();
    api.stop.mockRejectedValueOnce(new Error(STOP_FAILED));
    await act(async () => {
      await expect(view.result.current.lsp.stop("kotlin")).rejects.toThrow(STOP_FAILED);
    });
    expect(view.result.current.lsp.error).toBe(STOP_FAILED);

    await act(async () => {
      resolveRefetch(snapshot(language(1, "ready")));
      await refetch;
    });
    expect(view.result.current.lsp.error).toBe(STOP_FAILED);
  });

  it("refetches transient state so a dropped ready event cannot leave stale progress", async () => {
    const view = renderHook(() => subject("task-1"), { wrapper });
    await waitFor(() => expect(view.result.current.lsp.loaded).toBe(true));
    api.get.mockResolvedValueOnce(snapshot(language(3, "ready")));

    vi.useFakeTimers();
    try {
      act(() =>
        view.result.current.store.getState().mergeTaskLspLanguage(language(2, "initializing")),
      );
      await act(async () => vi.advanceTimersByTimeAsync(2_000));
      expect(view.result.current.lsp.byLanguage.kotlin?.phase).toBe("ready");
    } finally {
      view.unmount();
      vi.useRealTimers();
    }
  });

  it("does not load or control without a task", async () => {
    const view = renderHook(() => subject(null), { wrapper });
    expect(api.get).not.toHaveBeenCalled();
    expect(websocket.subscribe).not.toHaveBeenCalled();
    await expect(view.result.current.lsp.start("kotlin")).rejects.toThrow();
  });
});
