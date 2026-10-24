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
  setPinnedTarget,
  setReusableRightTarget,
  toSerializedDockview,
  type LayoutPanel,
  type LayoutState,
} from "./layout-manager";
import { disposeDockview, mountDockview } from "./layout-manager/dockview-test-host.test-helpers";
import { useDockviewStore } from "./dockview-store";
import {
  setupContainerResizeSync,
  setupSashDragCapToggle,
  syncDockviewContainerSize,
} from "@/components/task/dockview-layout-setup";
import { splitPanel } from "@/components/settings/layouts/layout-editor-actions";
import { validateReusableLayout } from "@/lib/layout/layout-profiles";
import { enforcePinnedTargets } from "./dockview-pinned-enforce";

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

  it.each([
    { manual: null, dockviewResizedFirst: false },
    { manual: null, dockviewResizedFirst: true },
    { manual: 310, dockviewResizedFirst: false },
  ])(
    "resolves container-only resize (manual=$manual, Dockview first=$dockviewResizedFirst)",
    async ({ manual, dockviewResizedFirst }) => {
      const viewportWidth = window.innerWidth;
      startIn("env-notes", horizontalLayout({ bottom: [NOTES] }), custom);
      if (manual !== null) setManualRightWidth("env-fresh", manual);
      await switchEnv("env-notes", "env-fresh");
      stopMounted.push(setupSashDragCapToggle(api), setupContainerResizeSync(api));
      const container = document.createElement("div");
      let width = WIDTH;
      Object.defineProperties(container, {
        clientWidth: { get: () => width },
        clientHeight: { value: HEIGHT },
      });

      for (const nextWidth of [1500, 1000]) {
        width = nextWidth;
        if (dockviewResizedFirst) api.layout(width, HEIGHT);
        syncDockviewContainerSize(api, container);
        await settle();

        const right = manual ?? nextWidth * 0.2;
        expect(window.innerWidth).toBe(viewportWidth);
        expect(rootSizes(api)).toEqual([nextWidth - right, right]);
        expect(getPinnedTarget("right")).toBe(right);
        expect(getManualRightWidth("env-fresh")).toBe(manual);
      }
    },
  );

  it("does not replace a non-owned saved target with the stock ratio on container-only resize", async () => {
    startIn("env-saved", horizontalLayout({ bottom: [PROMPTS] }), null);
    setPinnedTarget("right", 240);
    getRootSplitview(api).resizeView(1, 240);
    stopMounted.push(setupSashDragCapToggle(api), setupContainerResizeSync(api));
    const container = document.createElement("div");
    Object.defineProperties(container, {
      clientWidth: { value: 1500 },
      clientHeight: { value: HEIGHT },
    });

    syncDockviewContainerSize(api, container);
    await settle();

    expect(rootSizes(api)).toEqual([1260, 240]);
    expect(getPinnedTarget("right")).toBe(240);
    expect(getManualRightWidth("env-saved")).toBeNull();
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

/** @covers AC-UI-TASK-LAYOUT-PROFILES-001.10, AC-UI-TASK-LAYOUT-PROFILES-002.6 */
describe.each(["slow", "fast", "explicit"])("unclamped reusable source geometry (%s)", (mode) => {
  it.each([
    { center: 600, right: 600, finalWidth: 1600, initialRight: 320, finalRight: 800 },
    { center: 950, right: 50, finalWidth: 4000, initialRight: 180, finalRight: 200 },
    { center: undefined, right: 200, finalWidth: 1600, initialRight: 200, finalRight: 200 },
  ])(
    "restores source right=$right after initial constraints and wide/narrow/wide resizing",
    async (geometry) => {
      const layout = horizontalLayout({ bottom: [mode === "fast" ? TERMINAL : PROMPTS] });
      if (geometry.center === undefined) delete layout.columns[0].width;
      else layout.columns[0].width = geometry.center;
      layout.columns[1].width = geometry.right;
      startIn(
        "env-old",
        horizontalLayout({ bottom: [mode === "fast" ? TERMINAL : NOTES] }),
        layout,
      );
      api.layout(800, HEIGHT);
      if (mode === "explicit") {
        useDockviewStore.getState().applyCustomLayout({
          id: "source-profile",
          name: "Source profile",
          isDefault: false,
          createdAt: "2026-10-24T00:00:00Z",
          layout,
        });
        await settle();
      } else {
        await switchEnv("env-old", "env-fresh");
      }
      expect(rootSizes(api)[1]).toBe(geometry.initialRight);
      const container = document.createElement("div");
      let width = 800;
      Object.defineProperties(container, {
        clientWidth: { get: () => width },
        clientHeight: { value: HEIGHT },
      });
      for (const next of [geometry.finalWidth, 800, geometry.finalWidth]) {
        width = next;
        syncDockviewContainerSize(api, container);
        await settle();
        expect(rootSizes(api)[1]).toBe(next === 800 ? geometry.initialRight : geometry.finalRight);
      }
    },
  );
});

/** @covers AC-UI-TASK-LAYOUT-PROFILES-001.10, AC-UI-TASK-LAYOUT-PROFILES-002.6 */
describe.each(["reveal", "unmaximize"])(
  "source geometry on %s after an absent-pane resize",
  (action) => {
    it.each([
      { envId: "env-fresh", manual: null, owned: true, width: 1500, expected: 300 },
      { envId: null, manual: null, owned: true, width: 1500, expected: 300 },
      { envId: "env-fresh", manual: 310, owned: true, width: 1500, expected: 310 },
      { envId: "env-fresh", manual: null, owned: true, width: 600, expected: 180 },
      { envId: "env-fresh", manual: 310, owned: true, width: 600, expected: 180 },
      { envId: "env-saved", manual: null, owned: false, width: 1500, expected: 240 },
    ])(
      "restores current geometry for $envId at $width (manual=$manual, owned=$owned)",
      async (scenario) => {
        const layout = horizontalLayout({ bottom: [PROMPTS] });
        startIn("env-fresh", layout, layout);
        useDockviewStore.setState({ currentLayoutEnvId: scenario.envId });
        if (scenario.manual !== null) setManualRightWidth(scenario.envId, scenario.manual);
        if (scenario.owned) {
          useDockviewStore.getState().buildDefaultLayout(api);
          await settle();
        } else {
          setPinnedTarget("right", 240);
          getRootSplitview(api).resizeView(1, 240);
          useDockviewStore.setState({ pinnedWidths: new Map([["right", 240]]) });
        }
        stopMounted.push(setupSashDragCapToggle(api), setupContainerResizeSync(api));
        const before = shape(api);
        if (action === "reveal") useDockviewStore.getState().toggleRightPanels();
        else useDockviewStore.getState().maximizeGroup(CENTER_GROUP);
        await settle();
        expect(api.getPanel(PROMPTS.id)).toBeUndefined();
        const container = document.createElement("div");
        Object.defineProperties(container, {
          clientWidth: { value: scenario.width },
          clientHeight: { value: HEIGHT },
        });
        syncDockviewContainerSize(api, container);
        await settle();

        if (action === "reveal") useDockviewStore.getState().toggleRightPanels();
        else useDockviewStore.getState().exitMaximizedLayout();
        await settle();

        expect(rootSizes(api)).toEqual([scenario.width - scenario.expected, scenario.expected]);
        expect(shape(api)).toEqual(before);
        expect(getManualRightWidth(scenario.envId)).toBe(scenario.manual);
      },
    );
  },
);

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

  it.each(["fresh", "saved"])(
    "restores incoming plugin-free VERTICAL row extents for a %s environment",
    async (target) => {
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
      getRootSplitview(api).resizeView(1, 200);
      expect(rootSizes(api)).toEqual([500, 200]);
      const incoming = {
        ...plain,
        columns: plain.columns.map((column, index) => ({
          ...column,
          width: index === 0 ? 450 : 250,
        })),
      };
      if (target === "saved") saveEnvironment("env-b", incoming);

      await switchEnv("env-a", "env-b");

      expect(rootSizes(api)).toEqual(target === "saved" ? [450, 250] : [300, 400]);
      expect(getPinnedTarget("right")).toBeUndefined();
    },
  );
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
const GENERATED_ENV = "env-generated";
const WORKBENCH_CAPTURE = "workbench capture";

function reusableGeneratedGroups(mode: "settings split" | typeof WORKBENCH_CAPTURE): LayoutState {
  const editor = mountDockview(1600, HEIGHT);
  const source = horizontalLayout({ bottom: [PROMPTS], centerWidth: 800, rightWidth: 800 });
  if (mode === "settings split") {
    source.columns[1].groups[0].panels.push(PROMPTS);
    source.columns[1].groups.pop();
  }
  try {
    applyLayout(editor, source, new Map([["right", 800]]), 1600, HEIGHT);
    if (mode === "settings split") {
      expect(splitPanel(editor, PROMPTS.id, "below")).toBe(true);
      return fromDockviewApi(editor);
    }
    useDockviewStore.setState({ api: editor, preMaximizeLayout: null });
    const captured = useDockviewStore.getState().captureCurrentLayout() as LayoutState;
    expect(captured.columns[1].groups.every((group) => group.id === undefined)).toBe(true);
    return captured;
  } finally {
    disposeDockview(editor);
    clearAllPinnedTargets();
  }
}

async function buildGeneratedDefault(source: LayoutState, envId: string | null = GENERATED_ENV) {
  expect(validateReusableLayout(source).valid).toBe(true);
  startIn(GENERATED_ENV, horizontalLayout({ bottom: [NOTES] }), source);
  api.layout(800, HEIGHT);
  useDockviewStore.setState({ currentLayoutEnvId: envId });
  useDockviewStore.getState().buildDefaultLayout(api);
  await settle();
}

function resizeGeneratedDefault(width: number, boundary: "mounted" | "state"): void {
  if (boundary === "state") {
    api.layout(width, HEIGHT);
    enforcePinnedTargets(api, {
      sidebarVisible: false,
      rightPanelsVisible: true,
      maximized: false,
      envId: useDockviewStore.getState().currentLayoutEnvId,
    });
    return;
  }
  const container = document.createElement("div");
  Object.defineProperties(container, {
    clientWidth: { value: width },
    clientHeight: { value: HEIGHT },
  });
  syncDockviewContainerSize(api, container);
}

/** @covers AC-UI-TASK-LAYOUT-PROFILES-002.3, AC-UI-TASK-LAYOUT-PROFILES-002.6, AC-UI-TASK-LAYOUT-PROFILES-001.9, AC-UI-TASK-LAYOUT-PROFILES-001.10 */
describe.each(["settings split", WORKBENCH_CAPTURE] as const)(
  "generated right groups from %s",
  (mode) => {
    it.each(["mounted", "state"] as const)(
      "refreshes every child cap through the %s boundary",
      async (boundary) => {
        const source = reusableGeneratedGroups(mode);
        await buildGeneratedDefault(source, mode === WORKBENCH_CAPTURE ? null : GENERATED_ENV);
        if (boundary === "mounted") stopMounted.push(setupSashDragCapToggle(api));
        expect(rootSizes(api)).toEqual([480, 320]);
        for (const width of [1600, 800, 1600]) {
          resizeGeneratedDefault(width, boundary);
          await settle();
          const right = width === 800 ? 320 : 800;
          expect(rootSizes(api)).toEqual([width - right, right]);
          for (const id of ["files", PROMPTS.id]) {
            const group = api.getPanel(id)!.group;
            expect(group.width).toBe(right);
            expect(group.maximumWidth).toBe(width === 800 ? 320 : 1120);
          }
        }
        expect(getManualRightWidth(useDockviewStore.getState().currentLayoutEnvId)).toBeNull();
      },
    );

    it("keeps real manual pixels ahead of the reusable ratio and releases the narrow cap", async () => {
      const source = reusableGeneratedGroups(mode);
      setManualRightWidth(GENERATED_ENV, 700);
      await buildGeneratedDefault(source);
      stopMounted.push(setupSashDragCapToggle(api));
      expect(rootSizes(api)).toEqual([480, 320]);
      resizeGeneratedDefault(1600, "mounted");
      await settle();
      expect(rootSizes(api)).toEqual([900, 700]);
      expect(getManualRightWidth(GENERATED_ENV)).toBe(700);
    });

    it("persists a genuine sash width for generated right groups across displays", async () => {
      await buildGeneratedDefault(reusableGeneratedGroups(mode));
      stopMounted.push(setupSashDragCapToggle(api));
      resizeGeneratedDefault(1600, "mounted");
      await settle();
      const sv = getRootSplitview(api);
      const sash: HTMLElement = sv.sashes[0].container;
      sash.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, button: 0 }));
      sv.resizeView(1, 660);
      sash.dispatchEvent(new MouseEvent("mouseup", { bubbles: true, button: 0 }));
      await settle();
      expect(getManualRightWidth(GENERATED_ENV)).toBe(660);
      for (const width of [800, 1600]) {
        resizeGeneratedDefault(width, "mounted");
        await settle();
        expect(rootSizes(api)[1]).toBe(width === 800 ? 320 : 660);
      }
    });

    it.each(["reveal", "unmaximize"])(
      "restores generated child caps on %s after an absent resize",
      async (action) => {
        await buildGeneratedDefault(reusableGeneratedGroups(mode));
        stopMounted.push(setupSashDragCapToggle(api));
        if (action === "reveal") useDockviewStore.getState().toggleRightPanels();
        else useDockviewStore.getState().maximizeGroup(CENTER_GROUP);
        await settle();
        resizeGeneratedDefault(1600, "mounted");
        await settle();
        if (action === "reveal") useDockviewStore.getState().toggleRightPanels();
        else useDockviewStore.getState().exitMaximizedLayout();
        await settle();
        expect(rootSizes(api)).toEqual([800, 800]);
        expect(api.getPanel(PROMPTS.id)!.group.maximumWidth).toBe(1120);
      },
    );
  },
);

/** @covers AC-UI-TASK-LAYOUT-PROFILES-001.10, AC-UI-TASK-LAYOUT-PROFILES-002.6 */
describe("generated right group policy controls", () => {
  it("retains the all-canonical positive control", async () => {
    await buildGeneratedDefault(
      horizontalLayout({ bottom: [PROMPTS], centerWidth: 800, rightWidth: 800 }),
    );
    stopMounted.push(setupSashDragCapToggle(api));
    expect(rootSizes(api)).toEqual([480, 320]);
    resizeGeneratedDefault(1600, "mounted");
    await settle();
    expect(rootSizes(api)).toEqual([800, 800]);
  });

  it("keeps generated VERTICAL groups outside horizontal width policy", async () => {
    const source = reusableGeneratedGroups(WORKBENCH_CAPTURE);
    source.rootOrientation = "VERTICAL";
    setManualRightWidth(GENERATED_ENV, 700);
    await buildGeneratedDefault(source);
    stopMounted.push(setupSashDragCapToggle(api));
    const before = rootSizes(api);
    resizeGeneratedDefault(1600, "mounted");
    await settle();
    expect(rootSizes(api)).toEqual(before);
    for (const id of ["files", PROMPTS.id]) {
      expect(api.getPanel(id)!.group.maximumWidth).toBeGreaterThan(1_000_000);
    }
    expect(getManualRightWidth(GENERATED_ENV)).toBe(700);
  });

  it.each(["env-other", null])(
    "does not adopt %s geometry for a non-owned generated saved layout",
    async (owner) => {
      const source = reusableGeneratedGroups(WORKBENCH_CAPTURE);
      startIn("env-saved-generated", source, null);
      api.layout(800, HEIGHT);
      applyLayout(api, source, new Map([["right", 320]]), 800, HEIGHT);
      setReusableRightTarget(owner, source);
      stopMounted.push(setupSashDragCapToggle(api));
      resizeGeneratedDefault(1600, "mounted");
      await settle();
      expect(rootSizes(api)).toEqual([1280, 320]);
      expect(getPinnedTarget("right")).toBe(320);
      expect(getManualRightWidth("env-saved-generated")).toBeNull();
      const sv = getRootSplitview(api);
      const sash: HTMLElement = sv.sashes[0].container;
      sash.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, button: 0 }));
      sv.resizeView(1, 660);
      sash.dispatchEvent(new MouseEvent("mouseup", { bubbles: true, button: 0 }));
      await settle();
      expect(rootSizes(api)).toEqual([940, 660]);
      expect(getManualRightWidth("env-saved-generated")).toBe(660);
      expect(getManualRightWidth(owner)).toBeNull();
    },
  );
});
