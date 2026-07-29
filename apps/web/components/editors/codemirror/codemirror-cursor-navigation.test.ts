import { Text } from "@codemirror/state";
import { describe, expect, it } from "vitest";
import { getCodeMirrorCursorOffset } from "./codemirror-cursor-navigation";

const DOCUMENT = Text.of(["alpha", "bravo", "charlie"]);

describe("getCodeMirrorCursorOffset", () => {
  it("translates a 1-based line and column into a document offset", () => {
    expect(getCodeMirrorCursorOffset(DOCUMENT, 2, 3)).toBe(8);
  });

  it("clamps a position before the document to its start", () => {
    expect(getCodeMirrorCursorOffset(DOCUMENT, -4, 0)).toBe(0);
  });

  it("clamps a position after the document to its end", () => {
    expect(getCodeMirrorCursorOffset(DOCUMENT, 99, 99)).toBe(DOCUMENT.length);
  });
});
