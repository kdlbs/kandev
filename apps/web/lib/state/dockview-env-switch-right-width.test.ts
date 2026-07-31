import { describe, expect, it } from "vitest";
import type { SerializedDockview } from "dockview-react";
import { savedRightColumnWidth } from "./dockview-env-switch";

const CENTER_GROUP_ID = "group-center";

function makeSaved(children: Array<{ id: string; size: number }>): SerializedDockview {
  return {
    grid: {
      root: {
        type: "branch",
        data: children.map((child) => ({
          type: "leaf",
          data: { id: child.id, views: [] },
          size: child.size,
        })),
      },
      height: 600,
      width: 1600,
      orientation: "HORIZONTAL",
    },
    panels: {},
    activeGroup: undefined,
  } as unknown as SerializedDockview;
}

describe("savedRightColumnWidth", () => {
  it("returns the saved right size for a 3-column layout (sidebar+center+right)", () => {
    const saved = makeSaved([
      { id: "group-sidebar", size: 300 },
      { id: CENTER_GROUP_ID, size: 1000 },
      { id: "group-right-top", size: 300 },
    ]);

    expect(savedRightColumnWidth(saved)).toBe(300);
  });

  it("returns the saved right size for a 2-column layout with sidebar hidden", () => {
    const saved = makeSaved([
      { id: CENTER_GROUP_ID, size: 1380 },
      { id: "group-right-top", size: 220 },
    ]);

    expect(savedRightColumnWidth(saved)).toBe(220);
  });

  it("returns undefined when the last 2-column child is not a right column", () => {
    const saved = makeSaved([
      { id: "group-sidebar", size: 300 },
      { id: CENTER_GROUP_ID, size: 1300 },
    ]);

    expect(savedRightColumnWidth(saved)).toBeUndefined();
  });

  it("returns undefined for null input", () => {
    expect(savedRightColumnWidth(null)).toBeUndefined();
  });
});
