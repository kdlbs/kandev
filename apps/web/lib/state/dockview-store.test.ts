import { describe, it, expect, beforeEach, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import {
  useDockviewStore,
  resolvePresetPinnedWidths,
  collectPinnedWidthUpdates,
} from "./dockview-store";
import { getGlobalSidebarWidth, setGlobalSidebarWidth } from "@/lib/local-storage";
import { getPinnedTarget, setPinnedTarget, clearPinnedTarget } from "./layout-manager";

type ActivePanelEvent = { id: string };
type CapturedHandlers = {
  active: ((e?: ActivePanelEvent) => void) | null;
};

type ParamsPanel = { id: string; params: Record<string, unknown> };

function makeApi(panels: ParamsPanel[] = []): { api: DockviewApi; captured: CapturedHandlers } {
  const captured: CapturedHandlers = { active: null };
  const api = {
    onDidActivePanelChange: (cb: (e?: ActivePanelEvent) => void) => {
      captured.active = cb;
      return { dispose: vi.fn() };
    },
    onDidAddPanel: () => ({ dispose: vi.fn() }),
    onDidRemovePanel: () => ({ dispose: vi.fn() }),
    getPanel: (id: string) => panels.find((p) => p.id === id),
    hasMaximizedGroup: () => false,
  } as unknown as DockviewApi;
  return { api, captured };
}

describe("dockview-store resolveFilePath (via onDidActivePanelChange)", () => {
  beforeEach(() => {
    useDockviewStore.getState().setApi(null);
  });

  it("resolves pinned file: panel id to its path", () => {
    const { api, captured } = makeApi();
    useDockviewStore.getState().setApi(api);

    captured.active?.({ id: "file:src/foo.ts" });

    expect(useDockviewStore.getState().activeFilePath).toBe("src/foo.ts");
    expect(useDockviewStore.getState().activeFileRepo).toBeNull();
  });

  it("resolves pinned diff:file: panel id to its path", () => {
    const { api, captured } = makeApi();
    useDockviewStore.getState().setApi(api);

    captured.active?.({ id: "diff:file:src/bar.ts" });

    expect(useDockviewStore.getState().activeFilePath).toBe("src/bar.ts");
    expect(useDockviewStore.getState().activeFileRepo).toBeNull();
  });

  it("resolves preview:file-editor panel via params.path", () => {
    const { api, captured } = makeApi([
      { id: "preview:file-editor", params: { path: "src/baz.ts", repo: "backend" } },
    ]);
    useDockviewStore.getState().setApi(api);

    captured.active?.({ id: "preview:file-editor" });

    expect(useDockviewStore.getState().activeFilePath).toBe("src/baz.ts");
    expect(useDockviewStore.getState().activeFileRepo).toBe("backend");
  });

  it("resolves preview:file-diff panel via params.path", () => {
    const { api, captured } = makeApi([
      {
        id: "preview:file-diff",
        params: { path: "src/diff.ts", repositoryName: "frontend" },
      },
    ]);
    useDockviewStore.getState().setApi(api);

    captured.active?.({ id: "preview:file-diff" });

    expect(useDockviewStore.getState().activeFilePath).toBe("src/diff.ts");
    expect(useDockviewStore.getState().activeFileRepo).toBe("frontend");
  });

  it("clears activeFilePath when a non-file panel becomes active", () => {
    const { api, captured } = makeApi();
    useDockviewStore.getState().setApi(api);

    captured.active?.({ id: "file:src/foo.ts" });
    expect(useDockviewStore.getState().activeFilePath).toBe("src/foo.ts");

    captured.active?.({ id: "chat" });
    expect(useDockviewStore.getState().activeFilePath).toBeNull();
    expect(useDockviewStore.getState().activeFileRepo).toBeNull();
  });

  it("clears activeFilePath when active-panel-change fires with no panel", () => {
    const { api, captured } = makeApi();
    useDockviewStore.getState().setApi(api);

    captured.active?.({ id: "diff:file:src/bar.ts" });
    expect(useDockviewStore.getState().activeFilePath).toBe("src/bar.ts");

    captured.active?.(undefined);
    expect(useDockviewStore.getState().activeFilePath).toBeNull();
    expect(useDockviewStore.getState().activeFileRepo).toBeNull();
  });
});

describe("resolvePresetPinnedWidths", () => {
  // sidebar + center + right, with the legacy initial caps (sidebar 350, right
  // 450). At totalWidth 1600 the sidebar ratio clamps to 350, while the right
  // ratio clamps to 450.
  const cols = [
    { id: "sidebar", pinned: true, groups: [] },
    { id: "center", groups: [] },
    { id: "right", pinned: true, groups: [] },
  ] as unknown as Parameters<typeof resolvePresetPinnedWidths>[1];

  it("returns each pinned column's default width when resetWidths is true", () => {
    // Explicit layout pick: drop the carried-over live widths and pass the
    // preset's computed defaults as explicit overrides (NOT an empty map, which
    // would let applyLayout capture a transient post-fromJSON live size).
    const live = new Map([
      ["sidebar", 519],
      ["right", 900],
    ]);

    const result = resolvePresetPinnedWidths(live, cols, 1600, true);

    expect(result.get("sidebar")).toBe(350); // clamped to legacy sidebar cap
    expect(result.get("right")).toBe(450); // clamped to legacy right cap
    expect(result.has("center")).toBe(false); // not pinned
  });

  it("keeps right live widths for columns in the target layout when not resetting", () => {
    const live = new Map([
      ["sidebar", 519],
      ["right", 900],
    ]);

    const result = resolvePresetPinnedWidths(live, cols, 1600, false);

    expect(result.has("sidebar")).toBe(false);
    expect(result.get("right")).toBe(900);
  });

  it("does not carry a live sidebar override on non-reset switches", () => {
    setGlobalSidebarWidth(520);
    const live = new Map([
      ["sidebar", 350],
      ["right", 900],
    ]);

    const result = resolvePresetPinnedWidths(live, cols, 1600, false);

    expect(result.has("sidebar")).toBe(false);
    expect(result.get("right")).toBe(900);
    expect(getGlobalSidebarWidth()).toBe(520);
  });

  it("drops live overrides for columns absent from the target layout", () => {
    // e.g. switching to a layout without a "right" column must not leak the
    // old right width into the new layout.
    const live = new Map([
      ["sidebar", 300],
      ["right", 900],
    ]);
    const noRight = [
      { id: "sidebar", pinned: true, groups: [] },
      { id: "center", groups: [] },
    ] as unknown as Parameters<typeof resolvePresetPinnedWidths>[1];

    const result = resolvePresetPinnedWidths(live, noRight, 1600, false);

    expect(result.has("sidebar")).toBe(false);
    expect(result.has("right")).toBe(false);
  });

  it("does not mutate the input map", () => {
    const live = new Map([["sidebar", 300]]);
    resolvePresetPinnedWidths(live, cols, 1600, false);
    expect(live.get("sidebar")).toBe(300);
  });

  describe("Default layout reset clears the global sidebar width", () => {
    beforeEach(() => {
      window.localStorage.clear();
      clearPinnedTarget("sidebar");
    });

    it("clears the global pref + runtime target and uses the ratio default", () => {
      setGlobalSidebarWidth(520);
      setPinnedTarget("sidebar", 520);

      const result = resolvePresetPinnedWidths(new Map(), cols, 1600, true);

      // pref cleared → ratio default (1600*0.25=400, clamped to sidebar cap 350)
      expect(result.get("sidebar")).toBe(350);
      expect(getGlobalSidebarWidth()).toBeNull();
      expect(getPinnedTarget("sidebar")).toBeUndefined();
    });

    it("does NOT clear the pref/target on a non-reset (programmatic) switch", () => {
      setGlobalSidebarWidth(520);
      setPinnedTarget("sidebar", 520);

      resolvePresetPinnedWidths(new Map([["sidebar", 480]]), cols, 1600, false);

      expect(getGlobalSidebarWidth()).toBe(520);
      expect(getPinnedTarget("sidebar")).toBe(520);
    });
  });
});

describe("collectPinnedWidthUpdates", () => {
  const size = (i: number) => [350, 560, 560][i]; // sidebar, center, last

  it("tracks right but not sidebar when both are visible", () => {
    const columns = [{ id: "sidebar" }, { id: "center" }, { id: "right" }];

    const updates = collectPinnedWidthUpdates(columns, size, {
      rightPanelsVisible: true,
    });

    expect(updates.has("sidebar")).toBe(false);
    expect(updates.get("right")).toBe(560);
  });

  it("does NOT track right when rightPanelsVisible is false (plan/preview/vscode)", () => {
    // Regression: in plan mode the side column inherits files/changes panels
    // and fromDockviewApi labels it "right". With the right column hidden, its
    // width must NOT be captured, or it leaks into the default layout on
    // toggle-back.
    const columns = [{ id: "sidebar" }, { id: "center" }, { id: "right" }];

    const updates = collectPinnedWidthUpdates(columns, size, {
      rightPanelsVisible: false,
    });

    expect(updates.has("right")).toBe(false);
    expect(updates.has("sidebar")).toBe(false);
  });

  it("skips collapsed/transient widths <= 50px", () => {
    const columns = [{ id: "sidebar" }, { id: "right" }];

    const updates = collectPinnedWidthUpdates(columns, () => 40, {
      rightPanelsVisible: true,
    });

    expect(updates.size).toBe(0);
  });
});
