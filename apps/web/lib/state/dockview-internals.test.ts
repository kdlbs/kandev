import { describe, expect, it, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import { readDockviewGrid, readResizableRootSplitview } from "./dockview-internals";

describe("Dockview root compatibility", () => {
  it("preserves the real splitview receiver and recursive children", () => {
    const splitview = { length: 2, getViewSize: vi.fn(() => 300), resizeView: vi.fn() };
    const children = [{ id: "center" }];
    const api = {
      component: { gridview: { root: { splitview, children } } },
    } as unknown as DockviewApi;
    const result = readDockviewGrid(api);
    expect(result.splitview).toBe(splitview);
    expect(result.children).toBe(children);
    readResizableRootSplitview(api)?.resizeView(1, 320);
    expect(splitview.resizeView).toHaveBeenCalledWith(1, 320);
  });
  it("reads a layout without requiring resize support", () => {
    const splitview = { getViewSize: vi.fn(() => 300) };
    const api = {
      component: { gridview: { root: { splitview, children: [] } } },
    } as unknown as DockviewApi;
    expect(readDockviewGrid(api).splitview).toBe(splitview);
    expect(readResizableRootSplitview(api)).toBeNull();
  });
  it.each([
    {},
    { component: {} },
    { component: { gridview: { root: { splitview: {}, children: "invalid" } } } },
  ])("rejects an unavailable or unsupported grid", (shape) => {
    expect(readDockviewGrid(shape as unknown as DockviewApi)).toEqual({
      splitview: null,
      orientation: undefined,
      children: undefined,
    });
  });
});
