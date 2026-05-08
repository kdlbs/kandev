import { describe, it, expect, vi, beforeEach } from "vitest";
import type { DockviewApi } from "dockview-react";
import { useDockviewStore } from "./dockview-store";

vi.mock("@/lib/local-storage", () => ({
  getEnvLayout: vi.fn(() => null),
  setEnvLayout: vi.fn(),
  getEnvMaximizeState: vi.fn(() => null),
  setEnvMaximizeState: vi.fn(),
  removeEnvMaximizeState: vi.fn(),
}));

vi.mock("@/lib/layout/panel-portal-manager", () => ({
  panelPortalManager: {
    releaseByEnv: vi.fn(),
    reconcile: vi.fn(),
  },
}));

import { setEnvLayout, getEnvMaximizeState } from "@/lib/local-storage";
import { panelPortalManager } from "@/lib/layout/panel-portal-manager";

function makeMockApi(): DockviewApi {
  return {
    width: 800,
    height: 600,
    panels: [],
    groups: [],
    fromJSON: vi.fn(),
    toJSON: vi.fn(() => ({})),
    layout: vi.fn(),
    activeGroup: null,
    onDidActivePanelChange: vi.fn(() => ({ dispose: vi.fn() })),
    getPanel: vi.fn(() => null),
    addPanel: vi.fn(),
    hasMaximizedGroup: vi.fn(() => false),
  } as unknown as DockviewApi;
}

describe("switchEnvLayout — root fix for terminal/layout swapping", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useDockviewStore.setState({
      api: null,
      currentLayoutEnvId: null,
      preMaximizeLayout: null,
      maximizedGroupId: null,
      isRestoringLayout: false,
    });
  });

  it("no-ops when switching between sessions of the same env", () => {
    const api = makeMockApi();
    useDockviewStore.setState({ api, currentLayoutEnvId: "env-shared" });

    useDockviewStore.getState().switchEnvLayout("env-shared", "env-shared", "session-B");

    // Same env = no layout rebuild + no portal release. This is the entire
    // point of env-keyed layouts: terminals + panels stay put.
    expect(api.fromJSON).not.toHaveBeenCalled();
    expect(panelPortalManager.releaseByEnv).not.toHaveBeenCalled();
    expect(setEnvLayout).not.toHaveBeenCalled();
  });

  it("saves outgoing env + releases its portals when switching to a new env", () => {
    const api = makeMockApi();
    useDockviewStore.setState({ api, currentLayoutEnvId: "env-old" });

    useDockviewStore.getState().switchEnvLayout("env-old", "env-new", "session-X");

    expect(setEnvLayout).toHaveBeenCalledWith("env-old", expect.anything());
    expect(panelPortalManager.releaseByEnv).toHaveBeenCalledWith("env-old");
    expect(useDockviewStore.getState().currentLayoutEnvId).toBe("env-new");
  });

  it("first adoption (no previous env) just records the new env", () => {
    const api = makeMockApi();
    useDockviewStore.setState({ api, currentLayoutEnvId: null });

    useDockviewStore.getState().switchEnvLayout(null, "env-first", "session-Y");

    // First adoption keeps existing layout, no portal release on the
    // (empty) outgoing env.
    expect(panelPortalManager.releaseByEnv).not.toHaveBeenCalled();
    expect(useDockviewStore.getState().currentLayoutEnvId).toBe("env-first");
  });

  it("does nothing when api is unset", () => {
    useDockviewStore.setState({ api: null });
    useDockviewStore.getState().switchEnvLayout("env-a", "env-b", null);
    expect(setEnvLayout).not.toHaveBeenCalled();
  });
});

/**
 * Regression suite for the "maximize Task A → click Task B in sidebar →
 * click Task A again → centre group is shrunk" bug. The root cause is two
 * separate state-management slips during a maximize-then-env-switch sequence:
 *
 *   1. `saveOutgoingEnv` wrote `api.toJSON()` (the 2-column maximize overlay)
 *      into the env's regular layout slot. If we ever fall back to that slot
 *      (maximize state cleared, slow-path switch, refresh after dropping max
 *      state, etc.) the user sees the truncated 2-column layout instead of
 *      their real one.
 *   2. `restoreMaximizeFromStorage` only writes `preMaximizeLayout` and
 *      forgets `maximizedGroupId` — leaving the store in an inconsistent
 *      half-maximized state on the way back.
 */
describe("switchEnvLayout — maximize+sidebar-switch regression", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useDockviewStore.setState({
      api: null,
      currentLayoutEnvId: null,
      preMaximizeLayout: null,
      maximizedGroupId: null,
      isRestoringLayout: false,
    });
  });

  it("persists pre-max (not the 2-col overlay) as the env's regular layout when maximized", () => {
    const api = makeMockApi();
    const preMaxLayout = {
      columns: [
        { id: "sidebar", pinned: true, width: 200, groups: [] },
        { id: "center", width: 400, groups: [] },
        { id: "right", pinned: true, width: 200, groups: [] },
      ],
    };
    useDockviewStore.setState({
      api,
      currentLayoutEnvId: "env-a",
      preMaximizeLayout: preMaxLayout as never,
      maximizedGroupId: "g-center",
    });

    useDockviewStore.getState().switchEnvLayout("env-a", "env-b", "session-b");

    expect(setEnvLayout).toHaveBeenCalled();
    const [savedEnvId, savedLayout] = vi.mocked(setEnvLayout).mock.calls[0];
    expect(savedEnvId).toBe("env-a");
    // Structural marker: the persisted layout must be the 3-column user
    // layout, not the 2-column maximize overlay from api.toJSON().
    const grid = (savedLayout as { grid?: { root?: { data?: unknown[] } } }).grid;
    const rootChildren = grid?.root?.data;
    expect(Array.isArray(rootChildren)).toBe(true);
    expect((rootChildren as unknown[]).length).toBe(3);
  });

  it("restores maximizedGroupId when switching back to an env with saved maximize", () => {
    const api = makeMockApi();
    const savedMaximizedJson = {
      grid: {
        root: {
          type: "branch",
          size: 600,
          data: [
            { type: "leaf", size: 200, data: { id: "g-sidebar", views: ["sidebar"] } },
            { type: "leaf", size: 600, data: { id: "g-center", views: ["chat"] } },
          ],
        },
        height: 600,
        width: 800,
        orientation: "HORIZONTAL",
      },
      panels: {
        sidebar: { id: "sidebar", contentComponent: "sidebar" },
        chat: { id: "chat", contentComponent: "chat" },
      },
      activeGroup: "g-center",
    };
    vi.mocked(getEnvMaximizeState).mockImplementation((envId) =>
      envId === "env-a"
        ? { preMaximizeLayout: { columns: [] }, maximizedDockviewJson: savedMaximizedJson }
        : null,
    );

    useDockviewStore.setState({ api, currentLayoutEnvId: "env-b" });
    useDockviewStore.getState().switchEnvLayout("env-b", "env-a", "session-a");

    const state = useDockviewStore.getState();
    expect(state.preMaximizeLayout).not.toBeNull();
    expect(state.maximizedGroupId).toBeTruthy();
  });
});
