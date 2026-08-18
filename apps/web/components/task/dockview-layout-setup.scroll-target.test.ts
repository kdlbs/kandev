import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";

const SESSION_ONE_PANEL_ID = "session:session-1";

const { mockClearScrollTargetForOwner, mockRelease, dockviewState } = vi.hoisted(() => ({
  mockClearScrollTargetForOwner: vi.fn((sessionId: string, hostPanelId: string) => {
    if (
      dockviewState.scrollTarget?.sessionId === sessionId &&
      dockviewState.scrollTarget.hostPanelId === hostPanelId
    ) {
      dockviewState.scrollTarget = null;
    }
  }),
  mockRelease: vi.fn(),
  dockviewState: {
    isRestoringLayout: false,
    scrollTarget: null as null | { sessionId: string; token: number; hostPanelId: string },
  },
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: Object.assign(() => ({}), {
    getState: () => ({
      clearScrollTargetForOwner: mockClearScrollTargetForOwner,
      isRestoringLayout: dockviewState.isRestoringLayout,
      // Non-maximized so handleMaximizeExitOnLastClose early-returns without
      // touching the maximize helpers this test does not mock.
      preMaximizeLayout: null,
    }),
  }),
}));

vi.mock("@/lib/layout/panel-portal-manager", () => ({
  panelPortalManager: {
    get: vi.fn(() => undefined),
    release: mockRelease,
  },
}));

import { setupPortalCleanup } from "./dockview-layout-setup";

type RemoveHandler = (panel: { id: string }) => void;

/** Builds a fake dockview API that captures removal handlers and exposes fireRemoval. */
function makeApi() {
  const handlers: RemoveHandler[] = [];
  return {
    onDidRemovePanel: (handler: RemoveHandler) => {
      handlers.push(handler);
      return { dispose: () => {} };
    },
    panels: [] as Array<{ id: string }>,
    hasMaximizedGroup: () => false,
    /** Fires every captured onDidRemovePanel handler with the given panel. */
    fireRemoval(panel: { id: string }) {
      for (const handler of handlers) handler(panel);
    },
  };
}

/** Builds a minimal zustand store whose state exposes the given active session id. */
function makeAppStore(activeSessionId: string): StoreApi<AppState> {
  return {
    getState: () => ({ tasks: { activeSessionId } }) as unknown as AppState,
  } as StoreApi<AppState>;
}

beforeEach(() => {
  vi.clearAllMocks();
  dockviewState.isRestoringLayout = false;
  dockviewState.scrollTarget = null;
});

describe("setupPortalCleanup — scroll-target teardown", () => {
  it("clears the removed session's target when a session:<id> panel is removed", () => {
    const api = makeApi();
    setupPortalCleanup(api as never, makeAppStore("session-1"));

    api.fireRemoval({ id: "session:session-7" });

    expect(mockClearScrollTargetForOwner).toHaveBeenCalledWith("session-7", "session:session-7");
  });

  it("clears by the active session when the canonical chat panel is removed", () => {
    const api = makeApi();
    setupPortalCleanup(api as never, makeAppStore("session-9"));

    api.fireRemoval({ id: "chat" });

    expect(mockClearScrollTargetForOwner).toHaveBeenCalledWith("session-9", "chat");
  });

  it("leaves targets intact when an unrelated panel is removed", () => {
    const api = makeApi();
    setupPortalCleanup(api as never, makeAppStore("session-1"));

    api.fireRemoval({ id: "files" });

    expect(mockClearScrollTargetForOwner).not.toHaveBeenCalled();
  });

  it("clears the removed session only, never the other sessions' targets", () => {
    const api = makeApi();
    setupPortalCleanup(api as never, makeAppStore("session-active"));

    api.fireRemoval({ id: "session:session-a" });

    expect(mockClearScrollTargetForOwner).toHaveBeenCalledTimes(1);
    expect(mockClearScrollTargetForOwner).toHaveBeenCalledWith("session-a", "session:session-a");
  });

  it("clears the latest target when its panel is removed", () => {
    // Task 03 teardown policy: removal clears by session and owner (no token),
    // so the latest target is invalidated before a stale consumer can use it.
    const api = makeApi();
    setupPortalCleanup(api as never, makeAppStore("session-1"));
    dockviewState.scrollTarget = {
      sessionId: "session-1",
      token: 2,
      hostPanelId: SESSION_ONE_PANEL_ID,
    };

    api.fireRemoval({ id: SESSION_ONE_PANEL_ID });

    expect(mockClearScrollTargetForOwner).toHaveBeenCalledWith("session-1", SESSION_ONE_PANEL_ID);
    expect(dockviewState.scrollTarget).toBeNull();
  });

  it("leaves the latest target intact when an unrelated session's panel is removed", () => {
    const api = makeApi();
    setupPortalCleanup(api as never, makeAppStore("session-1"));
    dockviewState.scrollTarget = {
      sessionId: "session-1",
      token: 2,
      hostPanelId: SESSION_ONE_PANEL_ID,
    };

    api.fireRemoval({ id: "session:session-9" });

    expect(dockviewState.scrollTarget).toEqual({
      sessionId: "session-1",
      token: 2,
      hostPanelId: SESSION_ONE_PANEL_ID,
    });
  });

  it("runs the clear even while a layout restore is in progress (before the restore guard)", () => {
    dockviewState.isRestoringLayout = true;
    const api = makeApi();
    setupPortalCleanup(api as never, makeAppStore("session-1"));

    api.fireRemoval({ id: "session:session-3" });
    api.fireRemoval({ id: "chat" });

    expect(mockClearScrollTargetForOwner).toHaveBeenCalledWith("session-3", "session:session-3");
    expect(mockClearScrollTargetForOwner).toHaveBeenCalledWith("session-1", "chat");
  });

  it("clears the target during a restore detach that keeps the host portal alive", () => {
    // Task 03 persistent-portal restore: `fromJSON` DETACHES the canonical
    // chat slot but deliberately does NOT release its portal, so the
    // TaskChatPanel host stays MOUNTED and its unmount cleanup never runs.
    // The removal-path clear (before the restore guard) is the only cleanup —
    // assert both halves: the target IS cleared and the portal is NOT
    // released (host stays mounted).
    dockviewState.isRestoringLayout = true;
    const api = makeApi();
    setupPortalCleanup(api as never, makeAppStore("session-1"));

    api.fireRemoval({ id: "chat" });

    expect(mockClearScrollTargetForOwner).toHaveBeenCalledWith("session-1", "chat");
    expect(mockRelease).not.toHaveBeenCalled();
  });

  it("integration: restore detach keeps the REAL portal host mounted while the removal path clears", async () => {
    // Exercise the actual PanelPortalManager lifecycle instead of only the
    // mocked removal handler: the canonical chat host acquires a portal
    // (stays mounted), the restore detaches the slot and fires the removal,
    // and the clear must run while the portal entry survives.
    const { panelPortalManager: realManager } = await vi.importActual<
      typeof import("@/lib/layout/panel-portal-manager")
    >("@/lib/layout/panel-portal-manager");
    dockviewState.isRestoringLayout = true;
    const api = makeApi();
    setupPortalCleanup(api as never, makeAppStore("session-1"));

    realManager.acquire("chat", "chat", {}, { component: "chat" } as never);
    expect(realManager.has("chat")).toBe(true);

    api.fireRemoval({ id: "chat" });

    expect(mockClearScrollTargetForOwner).toHaveBeenCalledWith("session-1", "chat");
    // Detach without release: the portal entry (host) is retained.
    expect(mockRelease).not.toHaveBeenCalled();
    expect(realManager.has("chat")).toBe(true);
    realManager.release("chat");
  });

  it("does not clear without an active session when the canonical chat is removed", () => {
    const api = makeApi();
    setupPortalCleanup(api as never, makeAppStore(""));

    api.fireRemoval({ id: "chat" });

    expect(mockClearScrollTargetForOwner).not.toHaveBeenCalled();
  });
});
