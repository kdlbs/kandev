import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import { splitPanel } from "@/components/settings/layouts/layout-editor-actions";
import {
  setupSashDragCapToggle,
  syncDockviewContainerSize,
} from "@/components/task/dockview-layout-setup";
import { validateReusableLayout } from "@/lib/layout/layout-profiles";
import { getManualRightWidth, setManualRightWidth } from "@/lib/local-storage";
import { useDockviewStore } from "./dockview-store";
import { enforcePinnedTargets } from "./dockview-pinned-enforce";
import {
  applyLayout,
  clearAllPinnedTargets,
  fromDockviewApi,
  getPinnedTarget,
  getRootSplitview,
  setReusableRightTarget,
  type LayoutState,
} from "./layout-manager";
import { disposeDockview, mountDockview } from "./layout-manager/dockview-test-host.test-helpers";

vi.mock("@/lib/layout/panel-portal-manager", () => ({
  panelPortalManager: { releaseByEnv: vi.fn(), reconcile: vi.fn() },
}));

const ENV = "env-three-root";
const HEIGHT = 700;
const PLUGIN = {
  id: "plugin:kandev-plugin-e2e:notes",
  component: "plugin-panel",
  tabComponent: "pluginPanelTab",
  title: "Notes",
  params: { pluginId: "kandev-plugin-e2e", panelKey: "notes" },
};
const CHAT = { id: "chat", component: "chat", title: "Agent" };
const FILES = { id: "files", component: "files", title: "Files" };
const CHANGES = { id: "changes", component: "changes", title: "Changes" };
const TERMINAL = { id: "terminal-default", component: "terminal", title: "Terminal" };
const apis: DockviewApi[] = [];
const stops: Array<() => void> = [];

function mount(width: number): DockviewApi {
  const api = mountDockview(width, HEIGHT);
  apis.push(api);
  return api;
}

async function settle(): Promise<void> {
  await vi.advanceTimersByTimeAsync(100);
}

/** Use the shipped settings split, then optionally the task's portable capture path. */
function splitRightSource(capture: boolean): LayoutState {
  const editor = mount(1600);
  applyLayout(
    editor,
    {
      columns: [
        { id: "center", width: 800, groups: [{ panels: [CHAT] }] },
        { id: "right", pinned: true, width: 800, groups: [{ panels: [FILES, CHANGES, PLUGIN] }] },
      ],
    },
    new Map([["right", 800]]),
    1600,
    HEIGHT,
  );
  expect(splitPanel(editor, PLUGIN.id, "right")).toBe(true);
  useDockviewStore.setState({ api: editor, preMaximizeLayout: null });
  const source = capture
    ? (useDockviewStore.getState().captureCurrentLayout() as LayoutState)
    : fromDockviewApi(editor);
  expect(validateReusableLayout(source).valid).toBe(true);
  expect(source.columns.map((column) => column.width)).toEqual([533, 533, 534]);
  if (capture) {
    expect(source.columns.flatMap((column) => column.groups).every((group) => !group.id)).toBe(
      true,
    );
  }
  clearAllPinnedTargets();
  return source;
}

async function build(source: LayoutState, width = 1200, envId: string | null = ENV) {
  const api = mount(width);
  useDockviewStore.setState({
    api,
    currentLayoutEnvId: envId,
    userDefaultLayout: source,
    userDefaultLayoutProfile: { kind: "custom", id: "three-root" },
    pinnedWidths: new Map(),
    preMaximizeLayout: null,
    hiddenRightPane: null,
    isRestoringLayout: false,
    sidebarVisible: false,
  });
  useDockviewStore.getState().buildDefaultLayout(api);
  await settle();
  return api;
}

function sync(api: DockviewApi, width: number, boundary: "mounted" | "state") {
  if (boundary === "state") {
    api.layout(width, HEIGHT);
    enforcePinnedTargets(api, {
      sidebarVisible: useDockviewStore.getState().sidebarVisible,
      rightPanelsVisible: true,
      maximized: false,
      envId: useDockviewStore.getState().currentLayoutEnvId,
    });
  } else {
    const container = document.createElement("div");
    Object.defineProperties(container, {
      clientWidth: { value: width },
      clientHeight: { value: HEIGHT },
    });
    syncDockviewContainerSize(api, container);
  }
}

beforeEach(() => {
  vi.useFakeTimers({
    toFake: ["setTimeout", "clearTimeout", "requestAnimationFrame", "cancelAnimationFrame"],
  });
  localStorage.clear();
  sessionStorage.clear();
  clearAllPinnedTargets();
});
afterEach(() => {
  stops.splice(0).forEach((stop) => stop());
  apis.splice(0).forEach(disposeDockview);
  document.body.innerHTML = "";
  vi.useRealTimers();
});

/** @covers AC-UI-TASK-LAYOUT-PROFILES-002.3, AC-UI-TASK-LAYOUT-PROFILES-002.6, AC-UI-TASK-LAYOUT-PROFILES-001.9 */
describe.each([false, true])("three root groups, task capture=%s", (capture) => {
  it("applies the right cap without reserving the Agent as a sidebar", () => {
    const source = splitRightSource(capture);
    const api = mount(1200);
    applyLayout(api, source, new Map([["right", 400]]), 1200, HEIGHT);
    expect(api.getPanel("files")!.group.maximumWidth).toBe(720);
    expect(api.getPanel("files")!.group.width).toBe(400);
  });

  it.each(["mounted", "state"] as const)(
    "preserves the owned share through %s same-size and display sync",
    async (boundary) => {
      const api = await build(splitRightSource(capture));
      expect(useDockviewStore.getState().sidebarVisible).toBe(false);
      expect(["chat", "files", PLUGIN.id].map((id) => api.getPanel(id)!.group.width)).toEqual([
        400, 400, 400,
      ]);
      if (boundary === "mounted") stops.push(setupSashDragCapToggle(api));
      for (const width of [1200, 1600, 1200, 1600]) {
        sync(api, width, boundary);
        await settle();
        expect(api.getPanel("files")!.group.width).toBe(width === 1200 ? 400 : 533);
        expect(api.getPanel("files")!.group.maximumWidth).toBe(width === 1200 ? 720 : 1120);
        expect(getPinnedTarget("right")).toBe(width === 1200 ? 400 : 533);
      }
      expect(getManualRightWidth(ENV)).toBeNull();
    },
  );
});

it("retains unequal three-root proportions instead of charging the center as sidebar", async () => {
  const source = splitRightSource(true);
  [800, 600, 200].forEach((width, index) => {
    source.columns[index].width = width;
  });
  const api = await build(source, 1600);
  stops.push(setupSashDragCapToggle(api));
  for (const width of [1600, 1200, 1600]) {
    sync(api, width, "mounted");
    await settle();
    expect(api.getPanel("files")!.group.width).toBe(width === 1600 ? 600 : 450);
  }
});

it("keeps raw manual pixels ahead of the three-root ratio without a fictitious cap", async () => {
  const source = splitRightSource(true);
  setManualRightWidth(ENV, 500);
  const api = await build(source);
  stops.push(setupSashDragCapToggle(api));
  for (const width of [1200, 1600, 1200]) {
    sync(api, width, "mounted");
    await settle();
    expect(api.getPanel("files")!.group.width).toBe(500);
  }
  expect(getManualRightWidth(ENV)).toBe(500);
});

it.each(["mounted", "state"] as const)(
  "reserves a real root-owned sidebar through %s enforcement",
  async (boundary) => {
    const source = splitRightSource(true);
    source.columns.forEach((column) => {
      column.width = 400;
    });
    source.columns.unshift({
      id: "sidebar",
      pinned: true,
      width: 400,
      groups: [{ panels: [{ id: "sidebar", component: "sidebar", title: "Sidebar" }] }],
    });
    const api = await build(source, 1200);
    expect(useDockviewStore.getState().sidebarVisible).toBe(true);
    if (boundary === "mounted") stops.push(setupSashDragCapToggle(api));
    sync(api, 1200, boundary);
    await settle();
    const sidebar = api.getPanel("sidebar")!.group.width;
    expect(sidebar).toBe(300);
    expect(api.getPanel("files")!.group.maximumWidth).toBe(420);
    expect(api.getPanel("files")!.group.width).toBe(300);
  },
);

async function dragSash(sash: HTMLElement, resize: () => void): Promise<void> {
  sash.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, button: 0 }));
  resize();
  sash.dispatchEvent(new MouseEvent("mouseup", { bubbles: true, button: 0 }));
  await settle();
}

/** @covers AC-UI-TASK-LAYOUT-PROFILES-001.10, AC-UI-TASK-LAYOUT-PROFILES-002.6 */
describe("middle owned column sash", () => {
  it.each([0, 1])(
    "persists boundary %s as raw pixels through title events and display changes",
    async (edge) => {
      const api = await build(splitRightSource(true), 1600);
      stops.push(setupSashDragCapToggle(api));
      const sv = getRootSplitview(api);
      await dragSash(sv.sashes[edge].container, () => sv.resizeView(1, 500));
      expect(getManualRightWidth(ENV)).toBe(500);
      expect(getPinnedTarget("right")).toBe(500);
      api.getPanel("chat")!.api.setTitle("Agent after sash drag");
      await settle();
      expect(api.getPanel("files")!.group.width).toBe(500);
      for (const width of [1200, 1600]) {
        sync(api, width, "mounted");
        await settle();
        expect(api.getPanel("files")!.group.width).toBe(500);
        expect(getManualRightWidth(ENV)).toBe(500);
      }
    },
  );
});

it.each([0, 1])(
  "keeps a null-owned boundary %s drag transient without task-scoped storage",
  async (edge) => {
    const api = await build(splitRightSource(true), 1600, null);
    stops.push(setupSashDragCapToggle(api));
    const sv = getRootSplitview(api);
    await dragSash(sv.sashes[edge].container, () => sv.resizeView(1, 500));
    expect(getPinnedTarget("right")).toBe(500);
    expect(getManualRightWidth(null)).toBeNull();
    api.getPanel("chat")!.api.setTitle("Agent after null-owner drag");
    await settle();
    expect(api.getPanel("files")!.group.width).toBe(500);
    sync(api, 1200, "mounted");
    await settle();
    expect(api.getPanel("files")!.group.width).toBe(400);
    expect(getManualRightWidth(null)).toBeNull();
  },
);

it.each(["env-foreign", null])(
  "does not persist a generated root sash for the foreign %s owner",
  async (owner) => {
    const source = splitRightSource(true);
    const api = await build(source, 1600);
    setReusableRightTarget(owner, source);
    stops.push(setupSashDragCapToggle(api));
    const sv = getRootSplitview(api);
    await dragSash(sv.sashes[1].container, () => sv.resizeView(1, 500));
    api.getPanel("chat")!.api.setTitle("Agent after foreign drag");
    await settle();
    expect(getManualRightWidth(ENV)).toBeNull();
    expect(getManualRightWidth(owner)).toBeNull();
    expect(getPinnedTarget("right")).toBe(533);
  },
);

it.each([0, 1])("does not reinterpret VERTICAL boundary %s as a width drag", async (edge) => {
  const source = splitRightSource(true);
  source.rootOrientation = "VERTICAL";
  const api = await build(source, 1600);
  stops.push(setupSashDragCapToggle(api));
  const sv = getRootSplitview(api);
  await dragSash(sv.sashes[edge].container, () => sv.resizeView(1, 200));
  api.getPanel("chat")!.api.setTitle("Agent after row drag");
  await settle();
  expect(sv.getViewSize(1)).toBe(200);
  expect(getManualRightWidth(ENV)).toBeNull();
  expect(getPinnedTarget("right")).toBeUndefined();
});

it("does not record an unrelated root sash as the owned middle child's manual width", async () => {
  const source = splitRightSource(true);
  source.columns.forEach((column) => {
    column.width = 400;
  });
  source.columns.push({
    id: TERMINAL.id,
    width: 400,
    groups: [{ panels: [TERMINAL] }],
  });
  const api = await build(source, 1600);
  stops.push(setupSashDragCapToggle(api));
  const sv = getRootSplitview(api);
  await dragSash(sv.sashes[2].container, () => sv.resizeView(2, 350));
  api.getPanel("chat")!.api.setTitle("Agent after unrelated root drag");
  await settle();
  expect(getManualRightWidth(ENV)).toBeNull();
  expect(getPinnedTarget("right")).toBe(400);
  expect(api.getPanel("files")!.group.width).toBe(400);
});

it("does not record a nested right-group height sash as manual root width", async () => {
  const source = splitRightSource(true);
  source.columns[1].groups.push({
    panels: [TERMINAL],
  });
  const api = await build(source, 1600);
  stops.push(setupSashDragCapToggle(api));
  const rootSashes = getRootSplitview(api).sashes.map(
    (sash: { container: HTMLElement }) => sash.container,
  );
  const nested = [...document.querySelectorAll<HTMLElement>(".dv-sash")].find(
    (sash) => !rootSashes.includes(sash),
  )!;
  const terminal = api.getPanel(TERMINAL.id)!.group;
  const originalHeight = terminal.height;
  await dragSash(nested, () => terminal.api.setSize({ height: 250 }));
  expect(terminal.height).toBe(250);
  expect(terminal.height).not.toBe(originalHeight);
  api.getPanel("chat")!.api.setTitle("Agent after nested drag");
  await settle();
  expect(getManualRightWidth(ENV)).toBeNull();
  expect(getPinnedTarget("right")).toBe(533);
  expect(api.getPanel("files")!.group.width).toBe(533);
});
