import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import { splitPanel } from "@/components/settings/layouts/layout-editor-actions";
import {
  setupSashDragCapToggle,
  syncDockviewContainerSize,
} from "@/components/task/dockview-layout-setup";
import { validateReusableLayout } from "@/lib/layout/layout-profiles";
import { getManualRightWidth, setManualRightWidth } from "@/lib/local-storage";
import { resolveCustomLayoutPinnedWidths, useDockviewStore } from "./dockview-store";
import { enforcePinnedTargets } from "./dockview-pinned-enforce";
import {
  applyLayout,
  clearAllPinnedTargets,
  fromDockviewApi,
  getRootSplitview,
  setReusableRightTarget,
  type LayoutState,
} from "./layout-manager";
import { disposeDockview, mountDockview } from "./layout-manager/dockview-test-host.test-helpers";

vi.mock("@/lib/layout/panel-portal-manager", () => ({
  panelPortalManager: { releaseByEnv: vi.fn(), reconcile: vi.fn() },
}));

const ENV = "env-root-proportions";
const HEIGHT = 700;
const CHAT = { id: "chat", component: "chat", title: "Agent" };
const FILES = { id: "files", component: "files", title: "Files" };
const PLUGIN = {
  id: "plugin:kandev-plugin-e2e:notes",
  component: "plugin-panel",
  title: "Notes",
  params: { pluginId: "kandev-plugin-e2e", panelKey: "notes" },
  tabComponent: "pluginPanelTab",
};
const apis: DockviewApi[] = [];
const stops: Array<() => void> = [];
type Boundary = "mounted" | "state";

function mount(width: number) {
  const api = mountDockview(width, HEIGHT);
  apis.push(api);
  return api;
}

async function settle() {
  await vi.advanceTimersByTimeAsync(100);
}

function sizes(api: DockviewApi): number[] {
  const sv = getRootSplitview(api);
  return Array.from({ length: sv.length }, (_, index) => sv.getViewSize(index));
}

function expectGeometry(api: DockviewApi, expected: number[]) {
  expect(sizes(api)).toEqual(expected);
  const root = api.toJSON().grid.root;
  expect(root.type).toBe("branch");
  if (root.type !== "branch" || !Array.isArray(root.data)) throw new Error("Expected root branch");
  expect(root.data.map((child) => child.size)).toEqual(expected);
}

function sourceFromSettings(widths: number[], capture: boolean): LayoutState {
  const width = widths.reduce((sum, size) => sum + size, 0);
  const editor = mount(width);
  applyLayout(
    editor,
    {
      columns: [
        { id: "center", width: width / 2, groups: [{ id: "group-center", panels: [CHAT] }] },
        {
          id: "right",
          pinned: true,
          width: width / 2,
          groups: [{ id: "group-right-top", panels: [FILES, PLUGIN] }],
        },
      ],
    },
    new Map([["right", width / 2]]),
    width,
    HEIGHT,
  );
  if (widths.length === 3) expect(splitPanel(editor, PLUGIN.id, "right")).toBe(true);
  const sv = getRootSplitview(editor);
  widths.forEach((size, index) => sv.resizeView(index, size));
  expect(sizes(editor)).toEqual(widths);
  useDockviewStore.setState({ api: editor, preMaximizeLayout: null });
  const source = capture
    ? (useDockviewStore.getState().captureCurrentLayout() as LayoutState)
    : fromDockviewApi(editor);
  expect(validateReusableLayout(source).valid).toBe(true);
  if (capture)
    expect(source.columns.flatMap((column) => column.groups).every((group) => !group.id)).toBe(
      true,
    );
  clearAllPinnedTargets();
  return source;
}

async function build(source: LayoutState, boundary: Boundary, envId: string | null = ENV) {
  const api = mount(800);
  useDockviewStore.setState({
    api,
    currentLayoutEnvId: envId,
    userDefaultLayout: source,
    userDefaultLayoutProfile: { kind: "custom", id: "root-proportions" },
    pinnedWidths: new Map(),
    preMaximizeLayout: null,
    hiddenRightPane: null,
    isRestoringLayout: false,
    sidebarVisible: false,
  });
  useDockviewStore.getState().buildDefaultLayout(api);
  await settle();
  if (boundary === "mounted") stops.push(setupSashDragCapToggle(api));
  return api;
}

function resize(api: DockviewApi, width: number, boundary: Boundary) {
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
  vi.stubGlobal("innerWidth", 3120);
  localStorage.clear();
  sessionStorage.clear();
  clearAllPinnedTargets();
});
afterEach(() => {
  stops.splice(0).forEach((stop) => stop());
  apis.splice(0).forEach(disposeDockview);
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

/** @covers AC-UI-TASK-LAYOUT-PROFILES-001.10, AC-UI-TASK-LAYOUT-PROFILES-002.6 */
describe.each(["mounted", "state"] as const)(
  "all-root source proportions through %s enforcement",
  (boundary) => {
    it.each(
      [false, true].flatMap((capture) => [
        { widths: [400, 1000, 200], narrow: [320, 320, 160], capture },
        { widths: [800, 1200, 800], narrow: [240, 320, 240], capture },
        { widths: [800, 600, 200], narrow: [400, 300, 100], capture },
      ]),
    )(
      "restores every live and serialized root extent for $widths (capture=$capture)",
      async ({ widths, narrow, capture }) => {
        const source = sourceFromSettings(widths, capture);
        const immutable = structuredClone(source);
        const width = widths.reduce((sum, size) => sum + size, 0);
        const direct = mount(width);
        applyLayout(
          direct,
          source,
          resolveCustomLayoutPinnedWidths(source.columns, width, source.rootOrientation),
          width,
          HEIGHT,
        );
        expectGeometry(direct, widths);
        clearAllPinnedTargets();
        const api = await build(source, boundary);
        for (const current of [800, width, 800, width]) {
          resize(api, current, boundary);
          await settle();
          expectGeometry(api, current === 800 ? narrow : widths);
        }
        expect(source).toEqual(immutable);
        expect(getManualRightWidth(ENV)).toBeNull();
      },
    );

    it("retains the canonical two-root cap-release control", async () => {
      const source = sourceFromSettings([600, 1000], false);
      const api = await build(source, boundary);
      expectGeometry(api, [480, 320]);
      for (const width of [1600, 800, 1600]) {
        resize(api, width, boundary);
        await settle();
        expectGeometry(api, width === 800 ? [480, 320] : [600, 1000]);
      }
    });

    it("keeps actual manual pixels ahead of automatic root redistribution", async () => {
      const source = sourceFromSettings([400, 1000, 200], true);
      setManualRightWidth(ENV, 800);
      const api = await build(source, boundary);
      for (const width of [1600, 800, 1600]) {
        resize(api, width, boundary);
        await settle();
        expect(api.getPanel("files")!.group.width).toBe(width === 800 ? 320 : 800);
        expect(getManualRightWidth(ENV)).toBe(800);
      }
    });

    it("restores a genuinely null-owned source without writing task preferences", async () => {
      const api = await build(sourceFromSettings([400, 1000, 200], true), boundary, null);
      resize(api, 1600, boundary);
      await settle();
      expectGeometry(api, [400, 1000, 200]);
      expect(getManualRightWidth(null)).toBeNull();
    });

    it.each(["env-foreign", null])("does not borrow the foreign %s source", async (owner) => {
      const source = sourceFromSettings([400, 1000, 200], true);
      const api = await build(source, boundary);
      setReusableRightTarget(owner, source);
      resize(api, 1600, boundary);
      await settle();
      expect(api.getPanel("files")!.group.width).toBe(320);
      expect(getManualRightWidth(ENV)).toBeNull();
    });

    it("retains incomplete source absolute-right-width policy", async () => {
      const source = sourceFromSettings([400, 1000, 200], true);
      delete source.columns[0].width;
      const api = await build(source, boundary);
      resize(api, 1600, boundary);
      await settle();
      expect(api.getPanel("files")!.group.width).toBe(1000);
      expect(source.columns[0].width).toBeUndefined();
      expect(getManualRightWidth(ENV)).toBeNull();
    });
  },
);

/** @covers AC-UI-TASK-LAYOUT-PROFILES-001.10, AC-UI-TASK-LAYOUT-PROFILES-002.6 */
describe.each(["mounted", "state"] as const)(
  "root geometry lifecycle through %s enforcement",
  (boundary) => {
    it("keeps vertical row extents outside horizontal redistribution", async () => {
      const source = sourceFromSettings([400, 1000, 200], true);
      source.rootOrientation = "VERTICAL";
      const api = await build(source, boundary);
      const before = sizes(api);
      resize(api, 1600, boundary);
      await settle();
      expectGeometry(api, before);
      expect(getManualRightWidth(ENV)).toBeNull();
    });

    it("uses actual pinned sidebar pixels when distributing the remaining source weights", async () => {
      const source = sourceFromSettings([400, 1000, 200], true);
      source.columns.unshift({
        id: "sidebar",
        pinned: true,
        width: 300,
        groups: [{ panels: [{ id: "sidebar", component: "sidebar", title: "Sidebar" }] }],
      });
      const api = await build(source, boundary);
      expectGeometry(api, [180, 293, 180, 147]);
      resize(api, 1900, boundary);
      await settle();
      const sidebar = api.getPanel("sidebar")!.group.width;
      const center = Math.round(((1900 - sidebar - 1000) * 2) / 3);
      expectGeometry(api, [sidebar, center, 1000, 1900 - sidebar - 1000 - center]);
      expect(api.getPanel("files")!.group.maximumWidth).toBe(Math.min(1330, 1900 - sidebar - 480));
    });

    it("keeps root weights when chat identity or an extra tab changes without a root edit", async () => {
      const api = await build(sourceFromSettings([400, 1000, 200], true), boundary);
      api.addPanel({ id: "todos", component: "todos", position: { referencePanel: "chat" } });
      api.getPanel("chat")!.api.close();
      api.addPanel({
        id: "session:geometry-session",
        component: "chat",
        position: { referencePanel: "todos" },
      });
      api.getPanel("session:geometry-session")!.api.setTitle("Agent after session update");
      await settle();
      resize(api, 1600, boundary);
      await settle();
      expectGeometry(api, [400, 1000, 200]);
    });

    it("does not revive old all-root allocation after a structural edit is undone", async () => {
      const api = await build(sourceFromSettings([400, 1000, 200], true), boundary);
      api.addPanel({
        id: "todos",
        component: "todos",
        position: { referencePanel: PLUGIN.id, direction: "right" },
      });
      const sv = getRootSplitview(api);
      [280, 320, 100, 100].forEach((size, index) => sv.resizeView(index, size));
      resize(api, 800, boundary);
      await settle();
      api.getPanel("todos")!.api.close();
      await settle();
      getRootSplitview(api).resizeView(0, 280);
      api.getPanel("chat")!.api.setTitle("Agent after structural edit");
      await settle();
      expect(api.getPanel("chat")!.group.width).toBe(280);
      resize(api, 1600, boundary);
      await settle();
      expect(api.getPanel("chat")!.group.width).not.toBe(400);
      expect(api.getPanel("files")!.group.width).toBe(1000);
    });

    it.each([201, 250])(
      "keeps a genuine unrelated root sash edit to %s pixels on layout events",
      async (editedWidth) => {
        const source = sourceFromSettings([400, 1000, 200], true);
        source.columns.push({
          id: "todos",
          width: 200,
          groups: [{ panels: [{ id: "todos", component: "todos", title: "Todos" }] }],
        });
        const api = await build(source, boundary);
        if (boundary === "state") stops.push(setupSashDragCapToggle(api));
        resize(api, 1800, boundary);
        await settle();
        expectGeometry(api, [400, 1000, 200, 200]);
        const sv = getRootSplitview(api);
        const sash: HTMLElement = sv.sashes[2].container;
        sash.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, button: 0 }));
        sv.resizeView(2, editedWidth);
        sash.dispatchEvent(new MouseEvent("mouseup", { bubbles: true, button: 0 }));
        await settle();
        api.getPanel("chat")!.api.setTitle("Agent after unrelated root width edit");
        await settle();
        expectGeometry(api, [400, 1000, editedWidth, 400 - editedWidth]);
        expect(getManualRightWidth(ENV)).toBeNull();
      },
    );
  },
);
