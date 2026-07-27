import { describe, expect, it } from "vitest";
import {
  getKanbanColumnGridTemplate,
  getLeadingKanbanColumnIndex,
  shouldUseWindowedKanban,
} from "./kanban-grid-template";

describe("getKanbanColumnGridTemplate", () => {
  it("keeps every desktop column readable", () => {
    expect(getKanbanColumnGridTemplate(4)).toBe("repeat(4, minmax(280px, 1fr))");
  });

  it("uses the same readable minimum for a single desktop lane", () => {
    expect(getKanbanColumnGridTemplate(1)).toBe("repeat(1, minmax(280px, 1fr))");
  });
});

describe("shouldUseWindowedKanban", () => {
  it("only windows multi-step boards when the measured board cannot fit them", () => {
    expect(shouldUseWindowedKanban(1_119, 4)).toBe(true);
    expect(shouldUseWindowedKanban(1_120, 4)).toBe(false);
  });

  it("keeps an unmeasured or single-step board in its stable grid fallback", () => {
    expect(shouldUseWindowedKanban(0, 4)).toBe(false);
    expect(shouldUseWindowedKanban(500, 1)).toBe(false);
  });
});

describe("getLeadingKanbanColumnIndex", () => {
  it("resolves the leading snap-aligned column and keeps midpoint ties stable", () => {
    expect(getLeadingKanbanColumnIndex(290, [0, 280, 560, 840])).toBe(1);
    expect(getLeadingKanbanColumnIndex(420, [0, 280, 560, 840])).toBe(1);
  });

  it("falls back to the first column without measured offsets", () => {
    expect(getLeadingKanbanColumnIndex(320, [])).toBe(0);
  });
});
