import { describe, expect, it } from "vitest";
import { getKanbanColumnGridTemplate } from "./kanban-grid-template";

describe("getKanbanColumnGridTemplate", () => {
  it("keeps full desktop columns fluid", () => {
    expect(getKanbanColumnGridTemplate(4, false)).toBe("repeat(4, minmax(0, 1fr))");
  });

  it("gives compact desktop columns a usable minimum width", () => {
    expect(getKanbanColumnGridTemplate(4, true)).toBe("repeat(4, minmax(260px, 1fr))");
  });
});
