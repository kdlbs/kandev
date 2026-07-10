import { describe, it, expect, vi, beforeEach } from "vitest";
import type { DockviewApi } from "dockview-react";

vi.mock("@/lib/local-storage", () => ({
  setEnvLayout: vi.fn(),
  getEnvLayout: vi.fn(() => null),
  getEnvMaximizeState: vi.fn(() => null),
  setEnvMaximizeState: vi.fn(),
  removeEnvMaximizeState: vi.fn(),
  getGlobalSidebarWidth: vi.fn(() => null),
  setGlobalSidebarWidth: vi.fn(),
  clearGlobalSidebarWidth: vi.fn(),
}));

vi.mock("@/lib/layout/panel-portal-manager", () => ({
  panelPortalManager: { releaseByEnv: vi.fn(), reconcile: vi.fn() },
}));

vi.mock("./dockview-scroll-preserve", () => ({
  preserveChatScrollDuringLayout: vi.fn(),
}));

vi.mock("./dockview-measure", () => ({
  measureDockviewContainer: vi.fn(() => ({ width: 800, height: 600 })),
}));

vi.mock("./dockview-pinned-enforce", () => ({
  enforcePinnedTargets: vi.fn(),
}));

vi.mock("./dockview-layout-builders", () => ({
  applyLayoutFixups: vi.fn(() => ({
    sidebarGroupId: "g-sidebar",
    centerGroupId: "g-center",
    rightTopGroupId: "g-right-top",
    rightBottomGroupId: "g-right-bottom",
  })),
  focusOrAddPanel: vi.fn(),
}));

vi.mock("./layout-manager", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./layout-manager")>();
  return {
    ...actual,
    SIDEBAR_GROUP: "sidebar",
    CENTER_GROUP: "center",
    RIGHT_TOP_GROUP: "right-top",
    RIGHT_BOTTOM_GROUP: "right-bottom",
    TERMINAL_DEFAULT_ID: "terminal",
    LAYOUT_SIDEBAR_RATIO: 0.2,
    LAYOUT_RIGHT_RATIO: 0.25,
    LAYOUT_PINNED_MIN_PX: 200,
    computeSidebarMaxPx: vi.fn(() => 350),
    computeRightMaxPx: vi.fn(() => 500),
    getPresetLayout: vi.fn(() => ({ columns: [] })),
    applyLayout: vi.fn(() => ({
      sidebarGroupId: "g-sidebar",
      centerGroupId: "g-center",
      rightTopGroupId: "g-right-top",
      rightBottomGroupId: "g-right-bottom",
    })),
    getPinnedWidth: vi.fn(() => 350),
    getRootSplitview: vi.fn(() => null),
    fromDockviewApi: vi.fn(() => ({ columns: [] })),
    filterEphemeral: vi.fn((s: unknown) => s),
    defaultLayout: vi.fn(() => ({ columns: [] })),
    mergeCurrentPanelsIntoPreset: vi.fn((_api: unknown, preset: unknown) => preset),
    toSerializedDockview: vi.fn((s: unknown) => s),
    injectIntentPanels: vi.fn(),
    applyActivePanelOverrides: vi.fn(),
    resolveNamedIntent: vi.fn(),
    setPinnedTarget: vi.fn(),
    clearPinnedTarget: vi.fn(),
    getPinnedTarget: vi.fn(() => undefined),
    layoutStructuresMatch: vi.fn(() => false),
    savedLayoutMatchesLive: vi.fn(() => false),
  };
});

import { setEnvLayout } from "@/lib/local-storage";
import { persistEnvLayoutNow, useDockviewStore } from "./dockview-store";
import { applyLayout } from "./layout-manager";

function makeApi(snapshot: object = { columns: [] }): DockviewApi {
  return {
    toJSON: vi.fn(() => snapshot),
  } as unknown as DockviewApi;
}

function makeStoreApi(): DockviewApi {
  return {
    width: 800,
    height: 600,
    panels: [],
    groups: [],
    layout: vi.fn(),
    fromJSON: vi.fn(),
    toJSON: vi.fn(() => ({ columns: [{ id: "center" }] })),
    getPanel: vi.fn(() => null),
    addPanel: vi.fn(),
    activeGroup: null,
    hasMaximizedGroup: vi.fn(() => false),
    exitMaximizedGroup: vi.fn(),
    onDidActivePanelChange: vi.fn(() => ({ dispose: vi.fn() })),
  } as unknown as DockviewApi;
}

function flushRaf(): Promise<void> {
  return new Promise((resolve) => requestAnimationFrame(() => resolve()));
}

// Regression: applyBuiltInPreset and applyCustomLayout used to mutate the live
// layout in memory but never wrote it to env-keyed sessionStorage. The auto-save
// in setupLayoutPersistence is gated by isRestoringLayout=true (which both
// actions hold for the entire rAF window in which they emit layout-change
// events), so the debounced save never fired. A page refresh after a preset
// switch restored the pre-preset layout from sessionStorage, losing the user's
// intent. persistEnvLayoutNow runs after isRestoringLayout flips back to false
// at the end of those rAF callbacks.
describe("persistEnvLayoutNow", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("writes the live api.toJSON() to env storage when envId is set", () => {
    const snapshot = { columns: [{ id: "center" }] };
    const api = makeApi(snapshot);

    persistEnvLayoutNow(api, "env-1", null);

    expect(setEnvLayout).toHaveBeenCalledTimes(1);
    expect(setEnvLayout).toHaveBeenCalledWith("env-1", snapshot);
  });

  it("is a no-op when envId is null (no env adopted yet)", () => {
    const api = makeApi();

    persistEnvLayoutNow(api, null, null);

    expect(setEnvLayout).not.toHaveBeenCalled();
    expect(api.toJSON).not.toHaveBeenCalled();
  });

  it("is a no-op while maximized (toJSON would be the 2-column overlay)", () => {
    // saveOutgoingEnv owns the maximize slot. Writing the overlay as the env's
    // regular layout would resurface a truncated layout on reload.
    const api = makeApi();
    const preMaximizeLayout = { columns: [] };

    persistEnvLayoutNow(api, "env-1", preMaximizeLayout);

    expect(setEnvLayout).not.toHaveBeenCalled();
    expect(api.toJSON).not.toHaveBeenCalled();
  });

  it("swallows serialization/storage errors so callers do not crash mid-rAF", () => {
    const api = {
      toJSON: vi.fn(() => {
        throw new Error("dockview serialize failed");
      }),
    } as unknown as DockviewApi;

    expect(() => persistEnvLayoutNow(api, "env-1", null)).not.toThrow();
    expect(setEnvLayout).not.toHaveBeenCalled();
  });
});

// Integration coverage: assert the real preset/custom-layout actions invoke
// persistEnvLayoutNow at the end of their rAF callback. The helper-only tests
// above cover the contract, but the bug we fixed was at the call sites — they
// previously held isRestoringLayout=true for the whole rAF and never wrote.
function resetStoreForIntegration() {
  vi.clearAllMocks();
  useDockviewStore.setState({
    api: null,
    currentLayoutEnvId: null,
    preMaximizeLayout: null,
    maximizedGroupId: null,
    isRestoringLayout: false,
  });
}

const customLayout = {
  id: "custom-1",
  name: "custom",
  layout: { columns: [{ id: "center", views: [], activeView: null }] },
};
type ApplyCustomLayoutArg = Parameters<
  ReturnType<typeof useDockviewStore.getState>["applyCustomLayout"]
>[0];

describe("applyBuiltInPreset — persistence at call site", () => {
  beforeEach(resetStoreForIntegration);

  it("persists the env layout after isRestoringLayout clears", async () => {
    const api = makeStoreApi();
    useDockviewStore.setState({ api, currentLayoutEnvId: "env-preset" });

    useDockviewStore.getState().applyBuiltInPreset("default");

    expect(useDockviewStore.getState().isRestoringLayout).toBe(true);
    await flushRaf();

    expect(useDockviewStore.getState().isRestoringLayout).toBe(false);
    expect(setEnvLayout).toHaveBeenCalledTimes(1);
    expect(setEnvLayout).toHaveBeenCalledWith("env-preset", expect.any(Object));
  });

  it("does not persist when no env is adopted yet", async () => {
    const api = makeStoreApi();
    useDockviewStore.setState({ api, currentLayoutEnvId: null });

    useDockviewStore.getState().applyBuiltInPreset("default");
    await flushRaf();

    expect(setEnvLayout).not.toHaveBeenCalled();
  });

  it("skips persistence while maximized to avoid stomping the regular layout", async () => {
    const api = makeStoreApi();
    type StoreState = ReturnType<typeof useDockviewStore.getState>;
    useDockviewStore.setState({
      api,
      currentLayoutEnvId: "env-maxed",
      preMaximizeLayout: { columns: [] } as unknown as StoreState["preMaximizeLayout"],
    });

    useDockviewStore.getState().applyBuiltInPreset("default");
    await flushRaf();

    expect(setEnvLayout).not.toHaveBeenCalled();
  });

  it("does not persist if the env changes between scheduling and rAF", async () => {
    const api = makeStoreApi();
    useDockviewStore.setState({ api, currentLayoutEnvId: "env-before" });

    useDockviewStore.getState().applyBuiltInPreset("default");
    // Simulate the user navigating to a different task in the ~16ms before
    // the rAF callback fires. The rAF should detect the env switch and skip
    // the write so the old layout is not stored under the new env's key.
    useDockviewStore.setState({ currentLayoutEnvId: "env-after" });
    await flushRaf();

    expect(setEnvLayout).not.toHaveBeenCalled();
  });
});

describe("applyCustomLayout — persistence at call site", () => {
  beforeEach(resetStoreForIntegration);

  it("retargets saved session chat panels to the active session", async () => {
    const api = makeStoreApi();
    useDockviewStore.setState({ api, currentLayoutEnvId: "env-custom" });

    const layoutWithOldSession = {
      id: "simple",
      name: "Simple",
      layout: {
        columns: [
          {
            id: "center",
            groups: [
              {
                panels: [
                  {
                    id: "session:s-old",
                    component: "chat",
                    title: "Agent",
                    tabComponent: "sessionTab",
                    params: { sessionId: "s-old" },
                  },
                ],
                activePanel: "session:s-old",
              },
            ],
          },
        ],
      },
    };

    (
      useDockviewStore.getState().applyCustomLayout as (
        layout: ApplyCustomLayoutArg,
        opts: { activeSessionId: string },
      ) => void
    )(layoutWithOldSession as unknown as ApplyCustomLayoutArg, { activeSessionId: "s-new" });

    await flushRaf();

    const appliedState = vi.mocked(applyLayout).mock.calls.at(-1)?.[1];
    const panel = appliedState?.columns[0]?.groups[0]?.panels[0];
    expect(panel).toMatchObject({
      id: "session:s-new",
      component: "chat",
      tabComponent: "sessionTab",
      params: { sessionId: "s-new" },
    });
    expect(appliedState?.columns[0]?.groups[0]?.activePanel).toBe("session:s-new");
  });

  it("persists the env layout after isRestoringLayout clears", async () => {
    const api = makeStoreApi();
    useDockviewStore.setState({ api, currentLayoutEnvId: "env-custom" });

    useDockviewStore.getState().applyCustomLayout(customLayout as unknown as ApplyCustomLayoutArg);
    await flushRaf();

    expect(setEnvLayout).toHaveBeenCalledTimes(1);
    expect(setEnvLayout).toHaveBeenCalledWith("env-custom", expect.any(Object));
  });

  it("does not persist when no env is adopted yet", async () => {
    const api = makeStoreApi();
    useDockviewStore.setState({ api, currentLayoutEnvId: null });

    useDockviewStore.getState().applyCustomLayout(customLayout as unknown as ApplyCustomLayoutArg);
    await flushRaf();

    expect(setEnvLayout).not.toHaveBeenCalled();
  });

  it("skips persistence while maximized", async () => {
    const api = makeStoreApi();
    type StoreState = ReturnType<typeof useDockviewStore.getState>;
    useDockviewStore.setState({
      api,
      currentLayoutEnvId: "env-maxed-custom",
      preMaximizeLayout: { columns: [] } as unknown as StoreState["preMaximizeLayout"],
    });

    useDockviewStore.getState().applyCustomLayout(customLayout as unknown as ApplyCustomLayoutArg);
    await flushRaf();

    expect(setEnvLayout).not.toHaveBeenCalled();
  });

  it("does not persist if the env changes between scheduling and rAF", async () => {
    const api = makeStoreApi();
    useDockviewStore.setState({ api, currentLayoutEnvId: "env-before-custom" });

    useDockviewStore.getState().applyCustomLayout(customLayout as unknown as ApplyCustomLayoutArg);
    useDockviewStore.setState({ currentLayoutEnvId: "env-after-custom" });
    await flushRaf();

    expect(setEnvLayout).not.toHaveBeenCalled();
  });

  it("does not persist when legacy fromJSON restore throws", async () => {
    const api = makeStoreApi();
    // Force the old-format path by passing a layout without `columns`, and
    // make fromJSON throw so the API may be in a partial state. Persisting
    // that partial snapshot would propagate corruption to the next load.
    (api.fromJSON as ReturnType<typeof vi.fn>).mockImplementation(() => {
      throw new Error("dockview fromJSON failed");
    });
    useDockviewStore.setState({ api, currentLayoutEnvId: "env-legacy" });

    const legacyLayout = { id: "legacy", name: "legacy", layout: { grid: {} } };
    useDockviewStore.getState().applyCustomLayout(legacyLayout as unknown as ApplyCustomLayoutArg);
    await flushRaf();

    expect(setEnvLayout).not.toHaveBeenCalled();
  });
});
