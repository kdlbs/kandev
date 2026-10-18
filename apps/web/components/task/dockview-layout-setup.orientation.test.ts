import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import {
  RIGHT_BOTTOM_GROUP,
  RIGHT_TOP_GROUP,
  CENTER_GROUP,
  applyLayout,
  clearAllPinnedTargets,
  getPinnedTarget,
  getRootSplitview,
  setPinnedTarget,
  type LayoutState,
} from "@/lib/state/layout-manager";
import {
  disposeDockview,
  mountDockview,
} from "@/lib/state/layout-manager/dockview-test-host.test-helpers";
import { getManualRightWidth, setManualRightWidth } from "@/lib/local-storage";
import { setupGroupTracking, setupSashDragCapToggle } from "./dockview-layout-setup";

const ENV_ID = "env-orientation";
const UNCONSTRAINED = 1_000_000;

const storeState = vi.hoisted(() => ({
  isRestoringLayout: false,
  preMaximizeLayout: null,
  sidebarVisible: false,
  currentLayoutEnvId: "env-orientation",
  pinnedWidths: new Map<string, number>(),
  setPinnedWidth: vi.fn(),
  refreshRightPaneState: vi.fn(),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: { getState: () => storeState, setState: vi.fn() },
}));

let api: DockviewApi | null = null;
let cleanups: Array<() => void> = [];

beforeEach(() => {
  clearAllPinnedTargets();
  localStorage.clear();
  sessionStorage.clear();
  storeState.pinnedWidths = new Map();
  storeState.setPinnedWidth.mockClear();
});

afterEach(() => {
  for (const cleanup of cleanups) cleanup();
  cleanups = [];
  if (api) disposeDockview(api);
  api = null;
  document.body.innerHTML = "";
});

/** Agent row above a canonical Files/Changes/plugin row carrying historical pin metadata. */
function layoutOf(rootOrientation: LayoutState["rootOrientation"]): LayoutState {
  return {
    rootOrientation,
    columns: [
      {
        id: "center",
        width: 300,
        groups: [{ id: CENTER_GROUP, panels: [{ id: "chat", component: "chat", title: "Agent" }] }],
      },
      {
        id: "right",
        pinned: true,
        width: 400,
        groups: [
          { id: RIGHT_TOP_GROUP, panels: [{ id: "files", component: "files", title: "Files" }] },
          {
            id: RIGHT_BOTTOM_GROUP,
            panels: [
              {
                id: "plugin:kandev-plugin-prompt-history:prompt-history",
                component: "plugin-panel",
                title: "Prompt History",
                tabComponent: "pluginPanelTab",
                params: { pluginId: "kandev-plugin-prompt-history", panelKey: "prompt-history" },
              },
            ],
          },
        ],
      },
    ],
  };
}

function mountLayout(rootOrientation: LayoutState["rootOrientation"]): DockviewApi {
  const dockview = mountDockview(1400, 700);
  applyLayout(dockview, layoutOf(rootOrientation), new Map(), 1400, 700);
  api = dockview;
  return dockview;
}

function rootSizes(dockview: DockviewApi): number[] {
  const sv = getRootSplitview(dockview);
  return [0, 1].map((index) => sv.getViewSize(index));
}

function nextFrame(): Promise<void> {
  return new Promise((resolve) => requestAnimationFrame(() => resolve()));
}

/** Dockview coalesces `onDidLayoutChange` asynchronously; wait for it and any frame work. */
async function settle(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 0));
  await nextFrame();
}

let changeCount = 0;

/** Dockview emits `onDidLayoutChange` for panel changes but not for splitview resizes in jsdom. */
async function emitLayoutChange(dockview: DockviewApi): Promise<void> {
  changeCount += 1;
  dockview.getPanel("chat")!.api.setTitle(`Agent ${changeCount}`);
  await settle();
}

/** Press and release the primary button on the last root sash after moving it. */
async function dragLastRootSash(dockview: DockviewApi, resizeTo: number): Promise<void> {
  const sv = getRootSplitview(dockview);
  const sash = sv.sashes[sv.sashes.length - 1].container as HTMLElement;
  sash.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, button: 0 }));
  sv.resizeView(1, resizeTo);
  sash.dispatchEvent(new MouseEvent("mouseup", { bubbles: true, button: 0 }));
  await settle();
}

/** @covers AC-UI-TASK-LAYOUT-PROFILES-002.6 */
describe("mounted setup with a VERTICAL root", () => {
  it("does not enforce a right-width target on layout changes or cap the right groups", async () => {
    const dockview = mountLayout("VERTICAL");
    setPinnedTarget("right", 250);
    cleanups.push(setupSashDragCapToggle(dockview));

    getRootSplitview(dockview).resizeView(1, 380);
    await emitLayoutChange(dockview);

    expect(rootSizes(dockview)).toEqual([320, 380]);
    expect(dockview.getPanel("files")!.group.maximumWidth).toBeGreaterThan(UNCONSTRAINED);
  });

  it("keeps the manual right width and pinned target when a vertical sash is dragged", async () => {
    const dockview = mountLayout("VERTICAL");
    cleanups.push(setupSashDragCapToggle(dockview));

    await dragLastRootSash(dockview, 380);

    expect(getManualRightWidth(ENV_ID)).toBeNull();
    expect(getPinnedTarget("right")).toBeUndefined();
    expect(rootSizes(dockview)[1]).toBe(380);
  });

  it("does not record a row extent as the tracked right width", async () => {
    setManualRightWidth(ENV_ID, 240);
    const dockview = mountLayout("VERTICAL");

    cleanups.push(setupGroupTracking(dockview));
    getRootSplitview(dockview).resizeView(1, 380);
    await emitLayoutChange(dockview);

    expect(storeState.setPinnedWidth).not.toHaveBeenCalled();
  });

  it("does not turn a window resize into a stock right-width target", async () => {
    const dockview = mountLayout("VERTICAL");
    cleanups.push(setupSashDragCapToggle(dockview));
    const { setupContainerResizeSync } = await import("./dockview-layout-setup");
    cleanups.push(setupContainerResizeSync(dockview));

    window.dispatchEvent(new Event("resize"));
    await settle();

    expect(getPinnedTarget("right")).toBeUndefined();
  });
});

/** @covers AC-UI-TASK-LAYOUT-PROFILES-001.10 */
describe.each([["HORIZONTAL" as const], [undefined]])(
  "mounted setup with a %s root",
  (rootOrientation) => {
    it("still enforces the right target and persists a dragged right width", async () => {
      const dockview = mountLayout(rootOrientation);
      setPinnedTarget("right", 250);
      cleanups.push(setupSashDragCapToggle(dockview));
      cleanups.push(setupGroupTracking(dockview));

      getRootSplitview(dockview).resizeView(1, 400);
      await emitLayoutChange(dockview);
      expect(rootSizes(dockview)[1]).toBe(250);

      await dragLastRootSash(dockview, 300);

      expect(getManualRightWidth(ENV_ID)).toBe(300);
      expect(getPinnedTarget("right")).toBe(300);
      expect(dockview.getPanel("files")!.group.maximumWidth).toBeLessThan(UNCONSTRAINED);
    });
  },
);
