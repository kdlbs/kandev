import { describe, expect, it } from "vitest";
import { canResizeColumnBoundary, resizeAdjacentColumns } from "./table-resize";

describe("canResizeColumnBoundary", () => {
  it("allows a boundary when both adjacent columns meet the minimum", () => {
    expect(canResizeColumnBoundary([64, 64, 240], 0)).toBe(true);
  });

  it("rejects a boundary when either adjacent column is below the minimum", () => {
    expect(canResizeColumnBoundary([63, 65, 240], 0)).toBe(false);
    expect(canResizeColumnBoundary([65, 63, 240], 0)).toBe(false);
  });
});

describe("resizeAdjacentColumns", () => {
  it("moves one boundary without changing the table width", () => {
    expect(resizeAdjacentColumns([120, 180, 240], 0, 32)).toEqual([152, 148, 240]);
  });

  it("clamps a shrinking right column to the minimum width", () => {
    expect(resizeAdjacentColumns([120, 80, 240], 0, 40)).toEqual([136, 64, 240]);
  });

  it("clamps a shrinking left column to the minimum width", () => {
    expect(resizeAdjacentColumns([80, 120, 240], 0, -40)).toEqual([64, 136, 240]);
  });

  it("leaves widths unchanged when the boundary is invalid", () => {
    expect(resizeAdjacentColumns([120, 180], 1, 20)).toEqual([120, 180]);
  });

  it("leaves a sub-128-pixel pair unchanged when both minimums cannot fit", () => {
    expect(resizeAdjacentColumns([63, 64, 240], 0, 20)).toEqual([63, 64, 240]);
  });
});
