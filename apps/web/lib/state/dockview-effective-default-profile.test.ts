import { describe, expect, it, vi, beforeEach } from "vitest";
import type { DockviewApi } from "dockview-react";
import { getBuiltInLayoutOverrideId, getLayoutProfileIdentity } from "@/lib/layout/layout-profiles";
import type { LayoutState } from "./layout-manager";

vi.mock("@/lib/local-storage", () => ({
  setEnvLayout: vi.fn(),
  getEnvLayout: vi.fn(() => null),
  getEnvLayoutProfile: vi.fn(() => null),
  setEnvLayoutProfile: vi.fn(),
  getEnvMaximizeState: vi.fn(() => null),
  setEnvMaximizeState: vi.fn(),
  removeEnvMaximizeState: vi.fn(),
  getGlobalSidebarWidth: vi.fn(() => null),
  setGlobalSidebarWidth: vi.fn(),
  clearGlobalSidebarWidth: vi.fn(),
  getManualRightWidth: vi.fn(() => null),
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

import { useDockviewStore } from "./dockview-store";
import { applyLayout } from "./layout-manager";

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

function resetStore(): void {
  vi.clearAllMocks();
  useDockviewStore.setState({
    api: null,
    currentLayoutEnvId: null,
    preMaximizeLayout: null,
    maximizedGroupId: null,
    isRestoringLayout: false,
    pinnedWidths: new Map(),
    userDefaultLayout: null,
    userDefaultLayoutProfile: { kind: "built-in", id: "default" },
  });
}

describe("effective default layout profile identity", () => {
  beforeEach(resetStore);

  // @covers AC-UI-TASK-LAYOUT-PROFILES-001.11
  it("preserves built-in Default through a fresh build and Reset Layout", async () => {
    const api = makeStoreApi();
    const userDefaultLayout = {
      columns: [
        {
          id: "center",
          groups: [{ panels: [{ id: "chat", component: "chat", title: "Agent" }] }],
        },
      ],
    } as LayoutState;
    const reservedDefaultProfile = getLayoutProfileIdentity({
      id: getBuiltInLayoutOverrideId("default"),
    });

    useDockviewStore.setState({ api, currentLayoutEnvId: "env-default" });
    useDockviewStore.getState().setUserDefaultLayout(userDefaultLayout, reservedDefaultProfile);

    useDockviewStore.getState().buildDefaultLayout(api);
    expect(useDockviewStore.getState().activeLayoutProfile).toEqual({
      kind: "built-in",
      id: "default",
    });
    expect(applyLayout).toHaveBeenCalledWith(api, userDefaultLayout, expect.any(Map), 800, 600);
    await flushRaf();

    useDockviewStore.getState().resetLayout();
    expect(useDockviewStore.getState().activeLayoutProfile).toEqual({
      kind: "built-in",
      id: "default",
    });
    expect(applyLayout).toHaveBeenLastCalledWith(api, userDefaultLayout, expect.any(Map), 800, 600);
    await flushRaf();
  });

  it("keeps an arbitrary saved default custom", () => {
    const api = makeStoreApi();
    const userDefaultLayout = { columns: [{ id: "center", groups: [] }] } as LayoutState;

    useDockviewStore.setState({ api, currentLayoutEnvId: "env-custom" });
    useDockviewStore
      .getState()
      .setUserDefaultLayout(
        userDefaultLayout,
        getLayoutProfileIdentity({ id: "layout-copied-default" }),
      );

    useDockviewStore.getState().buildDefaultLayout(api);

    expect(useDockviewStore.getState().activeLayoutProfile).toEqual({
      kind: "custom",
      id: "layout-copied-default",
    });
  });
});
