import { afterEach, expect, it, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import { registerDockviewRoot, unregisterDockviewRoot } from "@/lib/state/dockview-measure";
import {
  RIGHT_TOP_GROUP,
  setPinnedTarget,
  clearAllPinnedTargets,
} from "@/lib/state/layout-manager";
import { setupSashDragCapToggle } from "./dockview-layout-setup";

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: {
    getState: () => ({
      isRestoringLayout: false,
      preMaximizeLayout: null,
      sidebarVisible: false,
      currentLayoutEnvId: null,
    }),
  },
}));

afterEach(() => {
  document.body.innerHTML = "";
  localStorage.clear();
  clearAllPinnedTargets();
});

it("measures once before layout writes and remeasures on the next resize", () => {
  const root = document.createElement("div");
  const grid = document.createElement("div");
  grid.className = "dv-dockview";
  root.appendChild(grid);
  document.body.appendChild(root);
  let width = 1200;
  const geometryReads: string[] = [];
  Object.defineProperty(grid, "clientWidth", {
    get: () => {
      geometryReads.push("read");
      return width;
    },
  });
  let onLayout = () => {};
  const resizeView = vi.fn();
  const setConstraints = vi.fn((_constraints: { maximumWidth: number; minimumWidth: number }) =>
    geometryReads.push("write"),
  );
  const api = {
    width: 2000,
    hasMaximizedGroup: () => false,
    groups: [{ id: RIGHT_TOP_GROUP, api: { setConstraints } }],
    component: {
      gridview: {
        root: {
          splitview: {
            length: 2,
            getViewSize: () => 300,
            resizeView,
          },
        },
      },
    },
    onDidLayoutChange: (listener: () => void) => {
      onLayout = listener;
      return { dispose: vi.fn() };
    },
  } as unknown as DockviewApi;
  registerDockviewRoot(api, root);
  setPinnedTarget("right", 450);
  const dispose = setupSashDragCapToggle(api);
  try {
    geometryReads.length = 0;
    onLayout();
    expect(geometryReads).toEqual(["read", "write"]);
    expect(resizeView).toHaveBeenLastCalledWith(1, 450);
    expect(setConstraints).toHaveBeenLastCalledWith({ maximumWidth: 720, minimumWidth: 180 });
    width = 500;
    geometryReads.length = 0;
    onLayout();
    expect(geometryReads).toEqual(["read", "write"]);
    expect(setConstraints).toHaveBeenLastCalledWith({ maximumWidth: 180, minimumWidth: 180 });
    expect(resizeView).toHaveBeenLastCalledWith(1, 180);
  } finally {
    dispose();
    unregisterDockviewRoot(api);
  }
});
