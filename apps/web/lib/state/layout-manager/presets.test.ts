import { describe, expect, it } from "vitest";
import { compactLayout, defaultLayout } from "./presets";

describe("layout presets", () => {
  it("keeps the compact workbench on Dockview while prioritizing the center panel", () => {
    const compact = compactLayout();
    const compactSidebar = compact.columns.find((column) => column.id === "sidebar");
    const defaultSidebarMaxWidth = defaultLayout().columns[0].maxWidth ?? Number.POSITIVE_INFINITY;

    expect(compact.columns.map((column) => column.id)).toEqual(["sidebar", "center"]);
    expect(compactSidebar?.width).toBeLessThan(defaultSidebarMaxWidth);
    expect(compactSidebar?.maxWidth).toBeLessThan(defaultSidebarMaxWidth);
    expect(compact.columns.find((column) => column.id === "center")?.groups[0].panels[0].id).toBe(
      "chat",
    );
  });
});
