import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { defaultFeatureFlags } from "@/lib/state/slices/features/types";
import { ApiError } from "@/lib/api/client";
import type { HydrationState } from "@/lib/state/store";

const listClarificationInboxMock = vi.fn();
vi.mock("@/lib/api/domains/clarification-inbox-api", () => ({
  listClarificationInbox: (...args: unknown[]) => listClarificationInboxMock(...args),
}));

const SESSION_STATE_CHANGED = "session.state_changed";

type WsHandler = () => void;
const wsMocks = vi.hoisted(() => ({
  handlers: new Map<string, Set<WsHandler>>(),
}));

function emitWsEvent(type: string) {
  for (const handler of wsMocks.handlers.get(type) ?? []) handler();
}

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({
    on: (type: string, handler: WsHandler) => {
      const handlers = wsMocks.handlers.get(type) ?? new Set<WsHandler>();
      handlers.add(handler);
      wsMocks.handlers.set(type, handlers);
      return () => handlers.delete(handler);
    },
  }),
}));

const readBootPayloadMock = vi.fn();
vi.mock("@/src/boot-payload", () => ({
  readBootPayload: () => readBootPayloadMock(),
}));

import { useNeedsYouInboxController } from "./use-needs-you-inbox-controller";

const WORKSPACE_ID = "w1";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((innerResolve) => {
    resolve = innerResolve;
  });
  return { promise, resolve };
}

function page(overrides: Partial<Awaited<ReturnType<typeof listClarificationInboxMock>>> = {}) {
  return {
    bundles: [],
    count: 0,
    hidden_count: 0,
    next_snooze_expiry: null,
    ...overrides,
  };
}

function initialState(enabled: boolean): HydrationState {
  return {
    features: { ...defaultFeatureFlags, needsYouInbox: enabled },
    workspaces: { activeId: WORKSPACE_ID, byId: {}, allIds: [] },
  } as unknown as HydrationState;
}

function renderController(enabled = true) {
  return renderHook(
    () => {
      useNeedsYouInboxController();
      return useAppStoreApi();
    },
    {
      wrapper: ({ children }) => (
        <StateProvider initialState={initialState(enabled)}>{children}</StateProvider>
      ),
    },
  );
}

beforeEach(() => {
  listClarificationInboxMock.mockReset();
  listClarificationInboxMock.mockResolvedValue(page());
  readBootPayloadMock.mockReset();
  readBootPayloadMock.mockReturnValue({ initialState: {} });
  wsMocks.handlers.clear();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("useNeedsYouInboxController", () => {
  it("reads on mount for the active workspace when the flag is enabled", async () => {
    renderController(true);

    await waitFor(() =>
      expect(listClarificationInboxMock).toHaveBeenCalledWith(WORKSPACE_ID, expect.anything()),
    );
  });

  it("never reads when the flag is disabled", async () => {
    renderController(false);

    await act(async () => {
      await Promise.resolve();
    });
    expect(listClarificationInboxMock).not.toHaveBeenCalled();
  });

  it("applies a successful read into the count slice", async () => {
    listClarificationInboxMock.mockResolvedValue(
      page({ count: 2, bundles: [], hidden_count: 1, next_snooze_expiry: null }),
    );
    const { result } = renderController(true);

    await waitFor(() => {
      const state = result.current.getState().needsYouInbox.byWorkspaceId[WORKSPACE_ID];
      expect(state?.status).toBe("ready");
      expect(state?.count).toBe(2);
      expect(state?.hiddenCount).toBe(1);
    });
  });

  it("re-reads when the WS refresh trigger tick is bumped", async () => {
    vi.useFakeTimers();
    const { result } = renderController(true);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(1);

    act(() => {
      result.current.getState().bumpNeedsYouInboxRefreshTick();
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1_000);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(2);
  });

  it("shares mixed triggers during a read and runs at most one trailing refresh", async () => {
    vi.useFakeTimers();
    const firstRead = deferred<ReturnType<typeof page>>();
    listClarificationInboxMock.mockImplementationOnce(() => firstRead.promise);
    listClarificationInboxMock.mockResolvedValue(page({ count: 4 }));
    const { result } = renderController(true);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(1);

    act(() => {
      emitWsEvent(SESSION_STATE_CHANGED);
      emitWsEvent("session.pending_action_changed");
      result.current.getState().bumpNeedsYouInboxRefreshTick();
      window.dispatchEvent(new Event("focus"));
      window.dispatchEvent(new Event("online"));
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(250);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      firstRead.resolve(page());
      await firstRead.promise;
      await vi.advanceTimersByTimeAsync(750);
    });

    expect(listClarificationInboxMock).toHaveBeenCalledTimes(2);
    await act(async () => {
      await Promise.resolve();
    });
    expect(result.current.getState().needsYouInbox.byWorkspaceId[WORKSPACE_ID]?.count).toBe(4);
  });
});

describe("useNeedsYouInboxController temporary-failure cooldowns", () => {
  it("keeps WS refreshes inside the temporary-failure cooldown and honors Retry-After", async () => {
    vi.useFakeTimers();
    listClarificationInboxMock
      .mockRejectedValueOnce(new ApiError("busy", 503, null, 3))
      .mockResolvedValueOnce(page())
      .mockRejectedValueOnce(new ApiError("busy", 503, null))
      .mockResolvedValue(page());
    const { result } = renderController(true);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(1);

    act(() => emitWsEvent(SESSION_STATE_CHANGED));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    for (let i = 0; i < 5; i += 1) {
      act(() => emitWsEvent(SESSION_STATE_CHANGED));
      await act(async () => {
        await vi.advanceTimersByTimeAsync(500);
      });
    }
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(499);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(1);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(2);

    act(() => emitWsEvent(SESSION_STATE_CHANGED));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(2);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1_000);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(3);

    act(() => result.current.getState().bumpNeedsYouInboxRefreshTick());
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1_999);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(3);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(4);
  });

  it("backs off successive temporary failures by 2, 5, 15, and 30 seconds", async () => {
    vi.useFakeTimers();
    listClarificationInboxMock.mockRejectedValue(new ApiError("busy", 503, null));
    const { result } = renderController(true);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(1);

    const delays = [2_000, 5_000, 15_000, 30_000];
    for (const [index, delay] of delays.entries()) {
      act(() => result.current.getState().bumpNeedsYouInboxRefreshTick());
      await act(async () => {
        await vi.advanceTimersByTimeAsync(delay - 1);
      });
      expect(listClarificationInboxMock).toHaveBeenCalledTimes(index + 1);
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1);
      });
      expect(listClarificationInboxMock).toHaveBeenCalledTimes(index + 2);
    }
  });
});

describe("useNeedsYouInboxController permanent-failure recovery", () => {
  it("suspends automatic work after a permanent error but keeps explicit retry available", async () => {
    vi.useFakeTimers();
    listClarificationInboxMock
      .mockRejectedValueOnce(new ApiError("not found", 404, null))
      .mockResolvedValue(page({ count: 2 }));
    const { result } = renderController(true);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(1);

    act(() => {
      result.current.getState().bumpNeedsYouInboxRefreshTick();
      emitWsEvent(SESSION_STATE_CHANGED);
      window.dispatchEvent(new Event("focus"));
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(120_000);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(1);

    act(() => result.current.getState().requestNeedsYouInboxRetry());
    await act(async () => {
      await Promise.resolve();
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(2);
    expect(result.current.getState().needsYouInbox.byWorkspaceId[WORKSPACE_ID]?.count).toBe(2);
  });
});

describe("useNeedsYouInboxController scope cancellation", () => {
  it("aborts and fences reads when workspace, identity, feature, or lifetime changes", async () => {
    const reads: Array<{
      signal: AbortSignal;
      deferred: ReturnType<typeof deferred<ReturnType<typeof page>>>;
    }> = [];
    listClarificationInboxMock.mockImplementation((_workspaceId, options) => {
      const request = deferred<ReturnType<typeof page>>();
      reads.push({
        signal: options?.init?.signal ?? new AbortController().signal,
        deferred: request,
      });
      return request.promise;
    });
    const { result, unmount } = renderController(true);
    const store = result.current;

    await waitFor(() => expect(reads).toHaveLength(1));
    act(() => store.getState().setActiveWorkspace("w2"));
    await waitFor(() => expect(reads).toHaveLength(2));
    expect(reads[0].signal.aborted).toBe(true);
    await act(async () => {
      reads[0].deferred.resolve(page({ count: 9 }));
      await reads[0].deferred.promise;
    });
    expect(store.getState().needsYouInbox.byWorkspaceId.w1?.count ?? 0).toBe(0);

    act(() =>
      store.getState().setAuthState({
        mode: "enabled",
        authenticated: true,
        user: {
          id: "user-2",
          email: "user@example.test",
          display_name: "User",
          role: "user",
          status: "active",
        },
      }),
    );
    await waitFor(() => expect(reads).toHaveLength(3));
    expect(reads[1].signal.aborted).toBe(true);

    act(() => store.getState().setFeatures({ ...defaultFeatureFlags, needsYouInbox: false }));
    expect(reads[2].signal.aborted).toBe(true);
    await act(async () => {
      reads[2].deferred.resolve(page({ count: 9 }));
      await reads[2].deferred.promise;
    });
    expect(store.getState().needsYouInbox.byWorkspaceId.w2?.count ?? 0).toBe(0);

    act(() => store.getState().setFeatures({ ...defaultFeatureFlags, needsYouInbox: true }));
    await waitFor(() => expect(reads).toHaveLength(4));
    unmount();
    expect(reads[3].signal.aborted).toBe(true);
    await act(async () => {
      reads[3].deferred.resolve(page({ count: 9 }));
      await reads[3].deferred.promise;
    });
    expect(store.getState().needsYouInbox.byWorkspaceId.w2?.count ?? 0).toBe(0);
  });
});

describe("useNeedsYouInboxController periodic and snooze refreshes", () => {
  it("re-reads once at the periodic interval while the tab stays visible", async () => {
    vi.useFakeTimers();
    renderController(true);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(60_000);
    });

    expect(listClarificationInboxMock).toHaveBeenCalledTimes(2);
  });

  it("arms a snooze-expiry timer from the read response and re-reads once it fires", async () => {
    vi.useFakeTimers();
    const soon = new Date(Date.now() + 10_000).toISOString();
    listClarificationInboxMock.mockResolvedValueOnce(page({ next_snooze_expiry: soon }));
    listClarificationInboxMock.mockResolvedValue(page());

    renderController(true);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(1);

    // The reported expiry is whole-second RFC3339; the reschedule buffer
    // (below) pushes the actual timer 1s past it so the re-read always
    // crosses the real, possibly-subsecond boundary.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(11_000);
    });

    expect(listClarificationInboxMock).toHaveBeenCalledTimes(2);
  });

  it("delays the re-read a second past the reported second-precision expiry", async () => {
    vi.useFakeTimers();
    const soon = new Date(Date.now() + 10_000).toISOString();
    listClarificationInboxMock.mockResolvedValueOnce(page({ next_snooze_expiry: soon }));
    listClarificationInboxMock.mockResolvedValue(page());

    renderController(true);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(1);

    // Firing exactly at the reported (truncated) instant would still see the
    // bundle as snoozed on a server storing subsecond precision; the buffer
    // means nothing has re-read yet at this point.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1_000);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(2);
  });
});

describe("useNeedsYouInboxController boot-hydration seed (AC .34, .40, .41)", () => {
  it("seeds the count from the boot payload before the live read resolves", async () => {
    readBootPayloadMock.mockReturnValue({
      initialState: {
        needsYouInboxBoot: {
          workspaceId: WORKSPACE_ID,
          count: 5,
          hasMore: true,
          nextSnoozeExpiry: "2026-01-01T00:00:00Z",
        },
      },
    });

    const { result } = renderController(true);

    // Synchronous, before the mocked fetch's promise has settled: only the
    // layout-effect seed and beginNeedsYouInboxRead's own effect have run.
    const seeded = result.current.getState().needsYouInbox.byWorkspaceId[WORKSPACE_ID];
    expect(seeded?.count).toBe(5);
    expect(seeded?.hasMore).toBe(true);
    expect(seeded?.nextSnoozeExpiry).toBe("2026-01-01T00:00:00Z");

    await waitFor(() => {
      const settled = result.current.getState().needsYouInbox.byWorkspaceId[WORKSPACE_ID];
      expect(settled?.status).toBe("ready");
      expect(settled?.count).toBe(0);
    });
  });

  it("does not seed when the boot payload names a different workspace", async () => {
    readBootPayloadMock.mockReturnValue({
      initialState: {
        needsYouInboxBoot: {
          workspaceId: "some-other-workspace",
          count: 5,
          hasMore: true,
          nextSnoozeExpiry: null,
        },
      },
    });

    const { result } = renderController(true);

    const seeded = result.current.getState().needsYouInbox.byWorkspaceId[WORKSPACE_ID];
    expect(seeded?.count ?? 0).toBe(0);

    await waitFor(() =>
      expect(listClarificationInboxMock).toHaveBeenCalledWith(WORKSPACE_ID, expect.anything()),
    );
  });

  it("does not seed when the flag is disabled", async () => {
    readBootPayloadMock.mockReturnValue({
      initialState: {
        needsYouInboxBoot: {
          workspaceId: WORKSPACE_ID,
          count: 5,
          hasMore: true,
          nextSnoozeExpiry: null,
        },
      },
    });

    const { result } = renderController(false);

    await act(async () => {
      await Promise.resolve();
    });
    expect(result.current.getState().needsYouInbox.byWorkspaceId[WORKSPACE_ID]).toBeUndefined();
  });
});

describe("useNeedsYouInboxController WS event coalescing", () => {
  it("coalesces a burst into a leading read now and one trailing read for what changed inside the window", async () => {
    vi.useFakeTimers();
    listClarificationInboxMock.mockResolvedValueOnce(page());
    listClarificationInboxMock.mockResolvedValue(page({ count: 3 }));
    const { result } = renderController(true);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(1);

    // Burst of two distinct signals plus a repeat, all inside the window:
    // `session.state_changed` and `session.pending_action_changed` are
    // distinct signals from distinct sessions, not duplicates of one browser
    // event, so every one of them after the leading read must be queued, not
    // dropped.
    act(() => {
      emitWsEvent(SESSION_STATE_CHANGED);
      emitWsEvent("session.pending_action_changed");
      emitWsEvent(SESSION_STATE_CHANGED);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(1);

    // The WS debounce expires at 250ms, while the shared one-second start
    // floor holds its single trailing read.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(249);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(1);

    // Once the start floor elapses, the trailing read includes the burst.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(751);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(2);
    expect(result.current.getState().needsYouInbox.byWorkspaceId[WORKSPACE_ID]?.count).toBe(3);

    // A later event after the read has settled still causes a bounded read.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(250);
    });
    act(() => {
      emitWsEvent(SESSION_STATE_CHANGED);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(2);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(750);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(3);
  });
});

describe("useNeedsYouInboxController WS event cleanup", () => {
  it("aborts a held read and clears pending WS work on unmount", async () => {
    vi.useFakeTimers();
    const firstRead = deferred<ReturnType<typeof page>>();
    listClarificationInboxMock.mockImplementationOnce(() => firstRead.promise);
    const { unmount } = renderController(true);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(1);

    act(() => {
      emitWsEvent(SESSION_STATE_CHANGED);
      emitWsEvent(SESSION_STATE_CHANGED);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(1);

    // The second WS event and held request leave timer work pending.
    expect(vi.getTimerCount()).toBeGreaterThan(0);

    unmount();

    expect(vi.getTimerCount()).toBe(0);
    firstRead.resolve(page());
  });

  // R2-F3: `trailingBumpTimeoutRef` must survive an effect re-run triggered
  // by something other than unmount (here, a connectionStatus change that is
  // not itself a reconnect-to-"connected" edge) -- the effect's own cleanup
  // only tears down the WS listeners it registered, and must not silently
  // drop a still-pending trailing bump.
  it("does not drop a pending trailing read when the WS effect re-runs for an unrelated reason", async () => {
    vi.useFakeTimers();
    listClarificationInboxMock.mockResolvedValueOnce(page());
    const secondRead = deferred<ReturnType<typeof page>>();
    listClarificationInboxMock.mockImplementationOnce(() => secondRead.promise);
    listClarificationInboxMock.mockResolvedValue(page({ count: 3 }));
    const { result } = renderController(true);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1_000);
    });

    act(() => {
      emitWsEvent(SESSION_STATE_CHANGED);
      emitWsEvent(SESSION_STATE_CHANGED);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(2);

    // A connectionStatus change that is not a "disconnected -> connected"
    // edge (so it does not itself trigger a reconnect read) still re-runs the
    // effect the trailing timer lives inside.
    act(() => {
      result.current.getState().setConnectionStatus("reconnecting");
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(250);
    });

    await act(async () => {
      secondRead.resolve(page());
      await secondRead.promise;
      await vi.advanceTimersByTimeAsync(750);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(3);
    expect(result.current.getState().needsYouInbox.byWorkspaceId[WORKSPACE_ID]?.count).toBe(3);
  });
});
