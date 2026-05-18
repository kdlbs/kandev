import { describe, expect, it } from "vitest";
import { compactLayout, defaultLayout } from "./presets";

describe("layout presets", () => {
  it("keeps the compact workbench on Dockview while prioritizing the center panel", () => {
    const compact = compactLayout();

    expect(compact.columns.map((column) => column.id)).toEqual(["sidebar", "center"]);
    expect(compact.columns.find((column) => column.id === "sidebar")?.width).toBeLessThan(
      defaultLayout().columns[0].maxWidth ?? Number.POSITIVE_INFINITY,
    );
    expect(compact.columns.find((column) => column.id === "center")?.groups[0].panels[0].id).toBe(
      "chat",
    );
  });
});
