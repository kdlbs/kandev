import { describe, it, expect, beforeEach, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import {
  useDockviewStore,
  resolvePresetPinnedWidths,
  collectPinnedWidthUpdates,
} from "./dockview-store";

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
  });

  it("resolves pinned diff:file: panel id to its path", () => {
    const { api, captured } = makeApi();
    useDockviewStore.getState().setApi(api);

    captured.active?.({ id: "diff:file:src/bar.ts" });

    expect(useDockviewStore.getState().activeFilePath).toBe("src/bar.ts");
  });

  it("resolves preview:file-editor panel via params.path", () => {
    const { api, captured } = makeApi([
      { id: "preview:file-editor", params: { path: "src/baz.ts" } },
    ]);
    useDockviewStore.getState().setApi(api);

    captured.active?.({ id: "preview:file-editor" });

    expect(useDockviewStore.getState().activeFilePath).toBe("src/baz.ts");
  });

  it("resolves preview:file-diff panel via params.path", () => {
    const { api, captured } = makeApi([
      { id: "preview:file-diff", params: { path: "src/diff.ts" } },
    ]);
    useDockviewStore.getState().setApi(api);

    captured.active?.({ id: "preview:file-diff" });

    expect(useDockviewStore.getState().activeFilePath).toBe("src/diff.ts");
  });

  it("clears activeFilePath when a non-file panel becomes active", () => {
    const { api, captured } = makeApi();
    useDockviewStore.getState().setApi(api);

    captured.active?.({ id: "file:src/foo.ts" });
    expect(useDockviewStore.getState().activeFilePath).toBe("src/foo.ts");

    captured.active?.({ id: "chat" });
    expect(useDockviewStore.getState().activeFilePath).toBeNull();
  });

  it("clears activeFilePath when active-panel-change fires with no panel", () => {
    const { api, captured } = makeApi();
    useDockviewStore.getState().setApi(api);

    captured.active?.({ id: "diff:file:src/bar.ts" });
    expect(useDockviewStore.getState().activeFilePath).toBe("src/bar.ts");

    captured.active?.(undefined);
    expect(useDockviewStore.getState().activeFilePath).toBeNull();
  });
});

describe("resolvePresetPinnedWidths", () => {
  // sidebar + center + right, with the legacy initial caps (sidebar 350, right
  // 450). At totalWidth 1600 the ratio (0.25) is 400 → sidebar clamps to 350,
  // right stays 400.
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
    expect(result.get("right")).toBe(400); // ratio 0.25 * 1600
    expect(result.has("center")).toBe(false); // not pinned
  });

  it("keeps live widths for columns in the target layout when not resetting", () => {
    const live = new Map([
      ["sidebar", 519],
      ["right", 900],
    ]);

    const result = resolvePresetPinnedWidths(live, cols, 1600, false);

    expect(result.get("sidebar")).toBe(519);
    expect(result.get("right")).toBe(900);
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

    expect(result.get("sidebar")).toBe(300);
    expect(result.has("right")).toBe(false);
  });

  it("does not mutate the input map", () => {
    const live = new Map([["sidebar", 300]]);
    resolvePresetPinnedWidths(live, cols, 1600, false);
    expect(live.get("sidebar")).toBe(300);
  });
});

describe("collectPinnedWidthUpdates", () => {
  const size = (i: number) => [350, 560, 560][i]; // sidebar, center, last

  it("tracks sidebar + right when both are visible", () => {
    const columns = [{ id: "sidebar" }, { id: "center" }, { id: "right" }];

    const updates = collectPinnedWidthUpdates(columns, size, {
      sidebarVisible: true,
      rightPanelsVisible: true,
    });

    expect(updates.get("sidebar")).toBe(350);
    expect(updates.get("right")).toBe(560);
  });

  it("does NOT track right when rightPanelsVisible is false (plan/preview/vscode)", () => {
    // Regression: in plan mode the side column inherits files/changes panels
    // and fromDockviewApi labels it "right". With the right column hidden, its
    // width must NOT be captured, or it leaks into the default layout on
    // toggle-back.
    const columns = [{ id: "sidebar" }, { id: "center" }, { id: "right" }];

    const updates = collectPinnedWidthUpdates(columns, size, {
      sidebarVisible: true,
      rightPanelsVisible: false,
    });

    expect(updates.has("right")).toBe(false);
    expect(updates.get("sidebar")).toBe(350);
  });

  it("skips collapsed/transient widths <= 50px", () => {
    const columns = [{ id: "sidebar" }, { id: "right" }];

    const updates = collectPinnedWidthUpdates(columns, () => 40, {
      sidebarVisible: true,
      rightPanelsVisible: true,
    });

    expect(updates.size).toBe(0);
  });
});
