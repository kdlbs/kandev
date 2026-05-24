import { describe, expect, it } from "vitest";
import { compactLayout, defaultLayout, getPresetSidebarColumn } from "./presets";
import { computeSidebarMaxPx } from "./caps";

describe("layout presets", () => {
  it("keeps the compact workbench on Dockview while prioritizing the center panel", () => {
    const compact = compactLayout();
    const compactSidebar = compact.columns.find((column) => column.id === "sidebar");
    // Default sidebar inherits the runtime cap (no per-column maxWidth);
    // compact pins itself tighter.
    const defaultSidebarCap =
      defaultLayout().columns.find((column) => column.id === "sidebar")?.maxWidth ??
      computeSidebarMaxPx();

    expect(compact.columns.map((column) => column.id)).toEqual(["sidebar", "center"]);
    const compactSidebarWidth = compactSidebar?.width ?? Number.POSITIVE_INFINITY;
    const compactSidebarMaxWidth = compactSidebar?.maxWidth ?? Number.POSITIVE_INFINITY;
    expect(compactSidebarWidth).toBeLessThan(defaultSidebarCap);
    expect(compactSidebarMaxWidth).toBeLessThan(defaultSidebarCap);
    expect(compact.columns.find((column) => column.id === "center")?.groups[0].panels[0].id).toBe(
      "chat",
    );
  });

  it("returns compact sidebar sizing for compact preset restoration", () => {
    const compactSidebar = compactLayout().columns.find((column) => column.id === "sidebar");

    expect(getPresetSidebarColumn("compact")).toEqual(compactSidebar);
    expect(getPresetSidebarColumn("compact").width).toBe(220);
    expect(getPresetSidebarColumn("compact").maxWidth).toBe(260);
  });
});
