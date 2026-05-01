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

import { setEnvLayout } from "@/lib/local-storage";
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
