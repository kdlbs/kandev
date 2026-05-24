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
