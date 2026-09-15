import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { defaultFeatureFlags } from "@/lib/state/slices/features/types";
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

    await waitFor(() => expect(listClarificationInboxMock).toHaveBeenCalledWith(WORKSPACE_ID));
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
    const { result } = renderController(true);
    await waitFor(() => expect(listClarificationInboxMock).toHaveBeenCalledTimes(1));

    act(() => {
      result.current.getState().bumpNeedsYouInboxRefreshTick();
    });

    await waitFor(() => expect(listClarificationInboxMock).toHaveBeenCalledTimes(2));
  });

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

    await waitFor(() => expect(listClarificationInboxMock).toHaveBeenCalledWith(WORKSPACE_ID));
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
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(2);

    // Still inside the window: no second read has fired yet, but one is
    // armed for the remainder of it.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(249);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(2);

    // The window elapses: the trailing read fires and its result is applied,
    // so nothing from the burst is silently lost.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(3);
    expect(result.current.getState().needsYouInbox.byWorkspaceId[WORKSPACE_ID]?.count).toBe(3);

    // Well past the window: a later, separate event still causes its own
    // leading read.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(250);
    });
    act(() => {
      emitWsEvent(SESSION_STATE_CHANGED);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(4);
  });

  it("clears a pending trailing read on unmount", async () => {
    vi.useFakeTimers();
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
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(2);

    unmount();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(250);
    });
    expect(listClarificationInboxMock).toHaveBeenCalledTimes(2);
  });
});
