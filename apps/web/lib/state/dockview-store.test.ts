import { describe, it, expect, beforeEach, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import { useDockviewStore } from "./dockview-store";

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

describe("dockview-store minimize state", () => {
  beforeEach(() => {
    useDockviewStore.setState({ minimizedGroupIds: new Set<string>() });
  });

  it("toggleGroupMinimized adds then removes a group ID", () => {
    const { toggleGroupMinimized } = useDockviewStore.getState();
    toggleGroupMinimized("g1");
    expect(useDockviewStore.getState().minimizedGroupIds.has("g1")).toBe(true);
    toggleGroupMinimized("g1");
    expect(useDockviewStore.getState().minimizedGroupIds.has("g1")).toBe(false);
  });

  it("toggleGroupMinimized swaps the Set instance so selectors re-fire", () => {
    const before = useDockviewStore.getState().minimizedGroupIds;
    useDockviewStore.getState().toggleGroupMinimized("g1");
    const after = useDockviewStore.getState().minimizedGroupIds;
    expect(after).not.toBe(before);
  });

  it("toggleGroupMinimized treats distinct IDs independently", () => {
    const { toggleGroupMinimized } = useDockviewStore.getState();
    toggleGroupMinimized("g1");
    toggleGroupMinimized("g2");
    const ids = useDockviewStore.getState().minimizedGroupIds;
    expect(ids.has("g1")).toBe(true);
    expect(ids.has("g2")).toBe(true);
    toggleGroupMinimized("g1");
    const ids2 = useDockviewStore.getState().minimizedGroupIds;
    expect(ids2.has("g1")).toBe(false);
    expect(ids2.has("g2")).toBe(true);
  });

  it("clearMinimizedGroups empties the Set", () => {
    useDockviewStore.getState().toggleGroupMinimized("g1");
    useDockviewStore.getState().toggleGroupMinimized("g2");
    expect(useDockviewStore.getState().minimizedGroupIds.size).toBe(2);
    useDockviewStore.getState().clearMinimizedGroups();
    expect(useDockviewStore.getState().minimizedGroupIds.size).toBe(0);
  });

  it("clearMinimizedGroups is a no-op when already empty (preserves identity)", () => {
    const before = useDockviewStore.getState().minimizedGroupIds;
    useDockviewStore.getState().clearMinimizedGroups();
    const after = useDockviewStore.getState().minimizedGroupIds;
    expect(after).toBe(before);
  });
});
