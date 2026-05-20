import { describe, expect, it } from "vitest";
import { compactLayout, defaultLayout, getPresetSidebarColumn } from "./presets";

describe("layout presets", () => {
  it("keeps the compact workbench on Dockview while prioritizing the center panel", () => {
    const compact = compactLayout();
    const compactSidebar = compact.columns.find((column) => column.id === "sidebar");
    const defaultSidebarMaxWidth =
      defaultLayout().columns.find((column) => column.id === "sidebar")?.maxWidth ??
      Number.POSITIVE_INFINITY;

    expect(compact.columns.map((column) => column.id)).toEqual(["sidebar", "center"]);
    const compactSidebarWidth = compactSidebar?.width ?? Number.POSITIVE_INFINITY;
    const compactSidebarMaxWidth = compactSidebar?.maxWidth ?? Number.POSITIVE_INFINITY;
    expect(compactSidebarWidth).toBeLessThan(defaultSidebarMaxWidth);
    expect(compactSidebarMaxWidth).toBeLessThan(defaultSidebarMaxWidth);
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
