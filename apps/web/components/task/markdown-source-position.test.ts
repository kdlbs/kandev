import { describe, expect, it } from "vitest";
import {
  clampSourcePosition,
  sourceLineAtOffset,
  sourceOffsetAtLine,
} from "./markdown-source-position";

describe("Markdown mode source positions (AC-UI-MARKDOWN-FILE-EDITING-002.4)", () => {
  it("maps CRLF source lines without changing source offsets", () => {
    const content = "alpha\r\nbravo\r\nend";
    expect(sourceOffsetAtLine(content, 2, 3)).toBe(9);
    expect(sourceLineAtOffset(content, 9)).toBe(2);
    expect(sourceOffsetAtLine(content, 2, 99)).toBe(12);
    expect(sourceOffsetAtLine(content, 99, 99)).toBe(content.length);
  });

  it("clamps a reversed selection after replacement without reversing its direction", () => {
    expect(
      clampSourcePosition("abc\nxyz", {
        selection: { anchor: 99, active: 2 },
        nearestLine: 90,
      }),
    ).toEqual({ selection: { anchor: 7, active: 2 }, nearestLine: 2 });
  });

  it("handles empty source, negative offsets, and a terminal empty line", () => {
    expect(
      clampSourcePosition("", {
        selection: { anchor: 4, active: -2 },
        nearestLine: 4,
      }),
    ).toEqual({ selection: { anchor: 0, active: 0 }, nearestLine: 1 });
    expect(sourceOffsetAtLine("a\n", 2)).toBe(2);
    expect(sourceLineAtOffset("a\n", 2)).toBe(2);
    expect(clampSourcePosition("a\n", { nearestLine: -1 })).toEqual({ nearestLine: 1 });
  });
});
