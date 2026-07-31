import { describe, expect, it } from "vitest";
import { compactLayout, defaultLayout, planLayout, previewLayout, vscodeLayout } from "./presets";

describe("layout presets", () => {
  it("default preset has center + right columns (no embedded sidebar)", () => {
    const layout = defaultLayout();
    expect(layout.columns.map((c) => c.id)).toEqual(["center", "right"]);
  });

  it("places PR Details after Files and Changes in the top-right group", () => {
    const layout = defaultLayout();
    const rightTop = layout.columns.find((column) => column.id === "right")?.groups[0];

    expect(rightTop?.panels.map((panel) => panel.id)).toEqual(["files", "changes", "pr-detail"]);
  });

  it("compact preset is a single center column with everything tabbed", () => {
    const layout = compactLayout();
    expect(layout.columns.map((c) => c.id)).toEqual(["center"]);
    expect(layout.columns[0].groups[0].panels.map((p) => p.id)).toEqual([
      "chat",
      "files",
      "changes",
      "pr-detail",
      "terminal-default",
    ]);
  });

  it("plan/preview/vscode presets drop the legacy sidebar column", () => {
    for (const preset of [planLayout(), previewLayout(), vscodeLayout()]) {
      expect(preset.columns.some((c) => c.id === "sidebar")).toBe(false);
      expect(preset.columns.some((c) => c.id === "center")).toBe(true);
    }
  });
});
