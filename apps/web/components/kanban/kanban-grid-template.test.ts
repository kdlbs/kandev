import { describe, expect, it } from "vitest";
import { getKanbanColumnGridTemplate } from "./kanban-grid-template";
import { KANBAN_DRAG_END_PADDING } from "./adaptive-desktop-kanban";

describe("getKanbanColumnGridTemplate", () => {
  it("keeps every desktop column readable", () => {
    expect(getKanbanColumnGridTemplate(4)).toBe("repeat(4, minmax(280px, 1fr))");
  });

  it("uses the same readable minimum for a single desktop lane", () => {
    expect(getKanbanColumnGridTemplate(1)).toBe("repeat(1, minmax(280px, 1fr))");
  });

  it("returns a valid empty grid for zero lanes", () => {
    expect(getKanbanColumnGridTemplate(0)).toBe("repeat(0, minmax(280px, 1fr))");
  });
});

describe("KANBAN_DRAG_END_PADDING", () => {
  it("reserves one viewport minus one readable column for drag anchoring", () => {
    expect(KANBAN_DRAG_END_PADDING).toBe("max(0px, calc(100% - 280px))");
  });
});
