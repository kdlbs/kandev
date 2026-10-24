/* eslint-disable max-lines -- One real-Dockview fixture shared by every environment-switch scenario. */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import { setEnvLayout, getManualRightWidth, setManualRightWidth } from "@/lib/local-storage";
import {
  CENTER_GROUP,
  RIGHT_BOTTOM_GROUP,
  RIGHT_TOP_GROUP,
  applyLayout,
  clearAllPinnedTargets,
  fromDockviewApi,
  getPinnedTarget,
  getRootSplitview,
  toSerializedDockview,
  type LayoutPanel,
  type LayoutState,
} from "./layout-manager";
import { disposeDockview, mountDockview } from "./layout-manager/dockview-test-host.test-helpers";
import { useDockviewStore } from "./dockview-store";
import {
  setupContainerResizeSync,
  setupSashDragCapToggle,
} from "@/components/task/dockview-layout-setup";

vi.mock("@/lib/layout/panel-portal-manager", () => ({
  panelPortalManager: { releaseByEnv: vi.fn(), reconcile: vi.fn() },
}));

const WIDTH = 1200;
const HEIGHT = 700;
const PROFILE = { kind: "custom", id: "layout-plugin" } as const;

const AGENT: LayoutPanel = { id: "chat", component: "chat", title: "Agent" };
const FILES: LayoutPanel = { id: "files", component: "files", title: "Files" };
const CHANGES: LayoutPanel = { id: "changes", component: "changes", title: "Changes" };
const TERMINAL: LayoutPanel = {
  id: "terminal-default",
  component: "terminal",
  title: "Terminal",
  tabComponent: "terminalTab",
};

function pluginPanel(pluginId: string, key: string, title: string): LayoutPanel {
  return {
    id: `plugin:${pluginId}:${key}`,
    component: "plugin-panel",
    title,
    tabComponent: "pluginPanelTab",
    params: { pluginId, panelKey: key },
  };
}

const NOTES = pluginPanel("kandev-plugin-e2e", "notes", "Notes");
const PROMPTS = pluginPanel("kandev-plugin-e2e", "prompt-history-plugin", "Prompt History");

type HorizontalOptions = {
  bottom: LayoutPanel[];
  bottomActive?: string;
  centerWidth?: number;
  rightWidth?: number;
};

/** Agent centre column and a canonical right column: Files/Changes over `bottom`. */
function horizontalLayout({
  bottom,
  bottomActive,
  centerWidth = 960,
  rightWidth = 240,
}: HorizontalOptions): LayoutState {
  return {
    columns: [
      {
        id: "center",
        width: centerWidth,
        groups: [{ id: CENTER_GROUP, panels: [AGENT], activePanel: "chat" }],
      },
      {
        id: "right",
        pinned: true,
        width: rightWidth,
        groups: [
          { id: RIGHT_TOP_GROUP, panels: [FILES, CHANGES], activePanel: "files" },
          { id: RIGHT_BOTTOM_GROUP, panels: bottom, activePanel: bottomActive ?? bottom[0].id },
        ],
      },
    ],
  };
}

/** Structure of a live layout with the Agent's placeholder/session identity erased. */
function shape(dockview: DockviewApi) {
  const captured = fromDockviewApi(dockview);
  return {
    orientation: captured.rootOrientation,
    columns: captured.columns.map((column) => ({
      id: column.id,
      groups: column.groups.map((group) => ({
        id: group.id,
        panels: group.panels.map((item) => item.id),
        active: group.activePanel,
      })),
    })),
  };
}

function expectedShape(layout: LayoutState) {
  return {
    orientation: layout.rootOrientation ?? "HORIZONTAL",
    columns: layout.columns.map((column) => ({
      id: column.id,
      groups: column.groups.map((group) => ({
        id: group.id,
        panels: group.panels.map((item) => item.id),
        active: group.activePanel,
      })),
    })),
  };
}

function nextFrame(): Promise<void> {
  return new Promise((resolve) => requestAnimationFrame(() => resolve()));
}

async function settle(): Promise<void> {
  for (let pass = 0; pass < 3; pass += 1) {
    await new Promise((resolve) => setTimeout(resolve, 0));
    await nextFrame();
  }
}

function rootSizes(dockview: DockviewApi): number[] {
  const sv = getRootSplitview(dockview);
  return Array.from({ length: sv.length }, (_, index) => sv.getViewSize(index));
}

let api: DockviewApi;
let stopMounted: Array<() => void> = [];

/** Mount Dockview showing `live` for `envId`, with `defaultLayout` as the user's effective default. */
function startIn(envId: string, live: LayoutState, defaultLayout: LayoutState | null): void {
  api = mountDockview(WIDTH, HEIGHT);
  applyLayout(api, live, new Map(), WIDTH, HEIGHT);
  useDockviewStore.setState({
    api,
    currentLayoutEnvId: envId,
    userDefaultLayout: defaultLayout,
    userDefaultLayoutProfile: defaultLayout ? PROFILE : { kind: "built-in", id: "default" },
    pinnedWidths: new Map(),
    isRestoringLayout: false,
    preMaximizeLayout: null,
    hiddenRightPane: null,
  });
}

/** An environment that already has a saved layout, as left behind when its task was last open. */
function saveEnvironment(envId: string, layout: LayoutState): void {
  setEnvLayout(envId, toSerializedDockview(layout, WIDTH, HEIGHT, new Map()) as unknown as object);
}

async function switchEnv(from: string, to: string): Promise<void> {
  useDockviewStore.getState().switchEnvLayout(from, to, null, []);
  await settle();
}

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  clearAllPinnedTargets();
});

afterEach(() => {
  for (const stop of stopMounted) stop();
  stopMounted = [];
  if (api) disposeDockview(api);
  document.body.innerHTML = "";
});

/** @covers AC-UI-TASK-LAYOUT-PROFILES-002.5, AC-UI-TASK-LAYOUT-PROFILES-002.6, AC-UI-TASK-LAYOUT-PROFILES-001.4 */
describe("environment switches with generic plugin panels", () => {
  it("restores the incoming default's plugin instead of reusing an equal component skeleton", async () => {
    const notes = horizontalLayout({ bottom: [NOTES] });
    const prompts = horizontalLayout({ bottom: [PROMPTS] });
    startIn("env-notes", notes, prompts);

    await switchEnv("env-notes", "env-fresh");

    expect(shape(api)).toEqual(expectedShape(prompts));
    expect(api.getPanel(PROMPTS.id)?.params).toEqual(PROMPTS.params);
    expect(api.getPanel(NOTES.id)).toBeUndefined();
  });

  it("returns to the saved environment's own plugin and leaves portable storage untouched", async () => {
    const notes = horizontalLayout({ bottom: [NOTES] });
    const prompts = horizontalLayout({ bottom: [PROMPTS] });
    startIn("env-notes", notes, prompts);
    const savedLayoutsBefore = localStorage.getItem("saved_layouts");

    await switchEnv("env-notes", "env-fresh");
    await switchEnv("env-fresh", "env-notes");

    expect(shape(api)).toEqual(expectedShape(notes));
    expect(api.getPanel(NOTES.id)?.params).toEqual(NOTES.params);
    expect(api.getPanel(PROMPTS.id)).toBeUndefined();
    expect(localStorage.getItem("saved_layouts")).toBe(savedLayoutsBefore);
  });

  it("restores a saved environment with the same plugin IDs but different structure and selection", async () => {
    const saved: LayoutState = {
      columns: [
        {
          id: "center",
          width: 600,
          groups: [{ id: CENTER_GROUP, panels: [AGENT, PROMPTS, NOTES], activePanel: NOTES.id }],
        },
        {
          id: "right",
          pinned: true,
          width: 600,
          groups: [{ id: RIGHT_TOP_GROUP, panels: [CHANGES, FILES], activePanel: "changes" }],
        },
      ],
    };
    const live = horizontalLayout({ bottom: [PROMPTS, NOTES], bottomActive: PROMPTS.id });
    saveEnvironment("env-saved", saved);
    startIn("env-live", live, null);

    await switchEnv("env-live", "env-saved");

    expect(shape(api)).toEqual(expectedShape(saved));
    expect(api.getPanel(NOTES.id)?.group.panels.map((item) => item.id)).toEqual([
      "chat",
      PROMPTS.id,
      NOTES.id,
    ]);
  });

  it.each([
    ["a plugin-free task to a plugin default", horizontalLayout({ bottom: [TERMINAL] }), true],
    ["a plugin task to a plugin-free default", horizontalLayout({ bottom: [PROMPTS] }), false],
  ])("restores the target when switching from %s", async (_name, live, targetHasPlugin) => {
    const target = horizontalLayout({ bottom: targetHasPlugin ? [PROMPTS] : [TERMINAL] });
    startIn("env-live", live, target);

    await switchEnv("env-live", "env-fresh");

    expect(shape(api)).toEqual(expectedShape(target));
    expect(Boolean(api.getPanel(PROMPTS.id))).toBe(targetHasPlugin);
  });

  it("keeps the fast path when neither side has a plugin panel", async () => {
    const plain = horizontalLayout({ bottom: [TERMINAL] });
    startIn("env-a", plain, plain);
    const fromJSON = vi.spyOn(api, "fromJSON");

    await switchEnv("env-a", "env-b");

    expect(fromJSON).not.toHaveBeenCalled();
    expect(shape(api)).toEqual(expectedShape(plain));
  });
});

/** @covers AC-UI-TASK-LAYOUT-PROFILES-001.9, AC-UI-TASK-LAYOUT-PROFILES-001.10, AC-UI-TASK-LAYOUT-PROFILES-002.6 */
describe("fresh default geometry after a plugin-bearing environment switch", () => {
  const custom = horizontalLayout({ bottom: [PROMPTS], centerWidth: 960, rightWidth: 240 });

  it("settles the right column at the saved in-cap ratio, not the stock ratio", async () => {
    startIn("env-notes", horizontalLayout({ bottom: [NOTES] }), custom);

    await switchEnv("env-notes", "env-fresh");

    expect(rootSizes(api)).toEqual([960, 240]);
    expect(getPinnedTarget("right")).toBe(240);
    expect(getManualRightWidth("env-fresh")).toBeNull();
  });

  it("keeps the saved ratio through the mounted automatic refresh and a container resize", async () => {
    startIn("env-notes", horizontalLayout({ bottom: [NOTES] }), custom);
    await switchEnv("env-notes", "env-fresh");
    stopMounted.push(setupSashDragCapToggle(api), setupContainerResizeSync(api));

    window.dispatchEvent(new Event("resize"));
    await settle();
    expect(getPinnedTarget("right")).toBe(240);

    api.layout(1000, HEIGHT);
    window.dispatchEvent(new Event("resize"));
    await settle();

    expect(getPinnedTarget("right")).toBe(200);
    expect(getManualRightWidth("env-fresh")).toBeNull();
  });

  it("lets an actual manual right width win over the saved ratio", async () => {
    startIn("env-notes", horizontalLayout({ bottom: [NOTES] }), custom);
    setManualRightWidth("env-fresh", 300);

    await switchEnv("env-notes", "env-fresh");

    expect(rootSizes(api)).toEqual([900, 300]);
  });

  it("falls back to the stock ratio when the default carries no reusable geometry", async () => {
    const stock = horizontalLayout({ bottom: [PROMPTS] });
    for (const column of stock.columns) delete column.width;
    startIn("env-notes", horizontalLayout({ bottom: [NOTES] }), stock);

    await switchEnv("env-notes", "env-fresh");

    expect(rootSizes(api)).toEqual([840, 360]);
  });

  it("does not leak the previous default's geometry into a saved environment", async () => {
    startIn("env-notes", horizontalLayout({ bottom: [NOTES] }), custom);
    saveEnvironment(
      "env-saved",
      horizontalLayout({ bottom: [PROMPTS], rightWidth: 500, centerWidth: 700 }),
    );
    await switchEnv("env-notes", "env-fresh");

    await switchEnv("env-fresh", "env-saved");

    expect(rootSizes(api)[1]).toBe(360);
  });
});

/** @covers AC-UI-TASK-LAYOUT-PROFILES-002.6 */
describe("VERTICAL root through environment switches and resets", () => {
  const vertical: LayoutState = {
    rootOrientation: "VERTICAL",
    columns: [
      {
        id: "center",
        width: 300,
        groups: [{ id: CENTER_GROUP, panels: [AGENT], activePanel: "chat" }],
      },
      {
        id: "right",
        pinned: true,
        width: 400,
        groups: [
          { id: RIGHT_TOP_GROUP, panels: [FILES, CHANGES], activePanel: "changes" },
          { id: RIGHT_BOTTOM_GROUP, panels: [PROMPTS], activePanel: PROMPTS.id },
        ],
      },
    ],
  };

  it("builds a fresh default with row extents, nested placement, and no right-width state", async () => {
    startIn("env-notes", horizontalLayout({ bottom: [NOTES] }), vertical);

    await switchEnv("env-notes", "env-fresh");

    expect(shape(api)).toEqual(expectedShape(vertical));
    expect(rootSizes(api)).toEqual([300, 400]);
    expect(getPinnedTarget("right")).toBeUndefined();
    expect(getManualRightWidth("env-fresh")).toBeNull();
  });

  it("restores the same rows on Reset Layout after the user moved the sash", async () => {
    startIn("env-notes", horizontalLayout({ bottom: [NOTES] }), vertical);
    await switchEnv("env-notes", "env-fresh");
    getRootSplitview(api).resizeView(1, 500);

    useDockviewStore.getState().resetLayout();
    await settle();

    expect(rootSizes(api)).toEqual([300, 400]);
    expect(shape(api)).toEqual(expectedShape(vertical));
  });

  it("restores a saved VERTICAL environment, scales with the container, and keeps proportions", async () => {
    saveEnvironment("env-saved", vertical);
    startIn("env-live", horizontalLayout({ bottom: [NOTES] }), null);

    await switchEnv("env-live", "env-saved");
    api.layout(WIDTH, 350);
    await settle();

    expect(shape(api)).toEqual(expectedShape(vertical));
    expect(rootSizes(api).reduce((total, size) => total + size, 0)).toBe(350);
    expect(getPinnedTarget("right")).toBeUndefined();
  });

  it("keeps row extents on the plugin-free fast path", async () => {
    const plain: LayoutState = {
      ...vertical,
      columns: [
        vertical.columns[0],
        {
          ...vertical.columns[1],
          groups: [vertical.columns[1].groups[0], { id: RIGHT_BOTTOM_GROUP, panels: [TERMINAL] }],
        },
      ],
    };
    startIn("env-a", plain, plain);
    const fromJSON = vi.spyOn(api, "fromJSON");

    await switchEnv("env-a", "env-b");

    expect(fromJSON).not.toHaveBeenCalled();
    expect(rootSizes(api)).toEqual([300, 400]);
    expect(getPinnedTarget("right")).toBeUndefined();
  });
});

/** @covers AC-UI-TASK-LAYOUT-PROFILES-001.9, AC-UI-TASK-LAYOUT-PROFILES-002.6 */
describe("plugin-free environment switches across root orientations", () => {
  const plainHorizontal = horizontalLayout({ bottom: [TERMINAL] });
  const plainVertical: LayoutState = {
    rootOrientation: "VERTICAL",
    columns: [
      {
        id: "center",
        width: 300,
        groups: [{ id: CENTER_GROUP, panels: [AGENT], activePanel: "chat" }],
      },
      {
        id: "right",
        pinned: true,
        width: 400,
        groups: [
          { id: RIGHT_TOP_GROUP, panels: [FILES, CHANGES], activePanel: "files" },
          { id: RIGHT_BOTTOM_GROUP, panels: [TERMINAL], activePanel: TERMINAL.id },
        ],
      },
    ],
  };

  it("applies a VERTICAL default to a fresh environment opened from a horizontal task", async () => {
    startIn("env-a", plainHorizontal, plainVertical);

    await switchEnv("env-a", "env-fresh");

    expect(shape(api).orientation).toBe("VERTICAL");
    expect(rootSizes(api)).toEqual([300, 400]);
    expect(getPinnedTarget("right")).toBeUndefined();
  });

  it("does not leak a VERTICAL task into a fresh environment whose default is horizontal", async () => {
    startIn("env-a", plainVertical, plainHorizontal);

    await switchEnv("env-a", "env-fresh");

    expect(shape(api).orientation).toBe("HORIZONTAL");
    expect(rootSizes(api)).toEqual([960, 240]);
  });

  it("restores a saved VERTICAL environment from a horizontal task", async () => {
    saveEnvironment("env-saved", plainVertical);
    startIn("env-a", plainHorizontal, null);

    await switchEnv("env-a", "env-saved");

    expect(shape(api).orientation).toBe("VERTICAL");
    expect(rootSizes(api)).toEqual([300, 400]);
  });

  it("keeps a custom default's in-cap right ratio on the plugin-free fast path", async () => {
    startIn("env-a", plainHorizontal, plainHorizontal);
    const fromJSON = vi.spyOn(api, "fromJSON");

    await switchEnv("env-a", "env-fresh");

    expect(fromJSON).not.toHaveBeenCalled();
    expect(rootSizes(api)).toEqual([960, 240]);
    expect(getPinnedTarget("right")).toBe(240);
    expect(getManualRightWidth("env-fresh")).toBeNull();
  });
});
