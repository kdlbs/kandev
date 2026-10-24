import { afterEach, beforeEach, expect, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import { splitPanel } from "@/components/settings/layouts/layout-editor-actions";
import {
  setupSashDragCapToggle,
  syncDockviewContainerSize,
} from "@/components/task/dockview-layout-setup";
import { validateReusableLayout } from "@/lib/layout/layout-profiles";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { enforcePinnedTargets } from "@/lib/state/dockview-pinned-enforce";
import {
  applyLayout,
  clearAllPinnedTargets,
  fromDockviewApi,
  getRootSplitview,
  type LayoutState,
} from "@/lib/state/layout-manager";
import {
  disposeDockview,
  mountDockview,
} from "@/lib/state/layout-manager/dockview-test-host.test-helpers";
const ENV = "env-root-proportions";
const HEIGHT = 700;
// i18n-exempt: Canonical title used only by the real Dockview test fixture.
const CHAT = { id: "chat", component: "chat", title: "Agent" };
// i18n-exempt: Canonical title used only by the real Dockview test fixture.
const FILES = { id: "files", component: "files", title: "Files" };
// i18n-exempt: Canonical title used only by the real Dockview test fixture.
const REVIEW = { id: "pr-detail", component: "pr-detail", title: "PR Details" };
const PLUGIN = {
  id: "plugin:kandev-plugin-e2e:notes",
  component: "plugin-panel",
  // i18n-exempt: Canonical title used only by the real Dockview test fixture.
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

function expectGeometry(api: DockviewApi, expected: readonly number[]) {
  expect(sizes(api)).toEqual(expected);
  const root = api.toJSON().grid.root;
  expect(root.type).toBe("branch");
  if (root.type !== "branch" || !Array.isArray(root.data)) throw new Error("Expected root branch");
  expect(root.data.map((child) => child.size)).toEqual(expected);
}

function sourceFromSettings(widths: number[], capture: boolean, review = false): LayoutState {
  const width = widths.reduce((sum, size) => sum + size, 0);
  const editor = mount(width);
  applyLayout(
    editor,
    {
      columns: [
        {
          id: "center",
          width: width / 2,
          groups: [{ id: "group-center", panels: review ? [CHAT, REVIEW] : [CHAT] }],
        },
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
      rightPanelsVisible: useDockviewStore.getState().rightPanelsVisible,
      maximized: useDockviewStore.getState().preMaximizeLayout !== null,
      envId: useDockviewStore.getState().currentLayoutEnvId,
      hiddenRightPane: useDockviewStore.getState().hiddenRightPane,
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

export function setupRootGeometryTests(): void {
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
}

export {
  ENV,
  HEIGHT,
  CHAT,
  FILES,
  REVIEW,
  PLUGIN,
  stops,
  mount,
  settle,
  sizes,
  expectGeometry,
  sourceFromSettings,
  build,
  resize,
};
