import { describe, expect, it } from "vitest";
import { EditorSelection, EditorState } from "@codemirror/state";
import { readCodeMirrorSelection } from "./codemirror-selection";

function stateWith(doc: string, from: number, to: number) {
  return EditorState.create({ doc, selection: EditorSelection.single(from, to) });
}

describe("readCodeMirrorSelection", () => {
  it("returns the selected text and line range", () => {
    expect(readCodeMirrorSelection(stateWith("one\ntwo\nthree", 4, 11))).toEqual({
      text: "two\nthr",
      startLine: 2,
      endLine: 3,
    });
  });

  it("ignores empty and whitespace-only selections", () => {
    expect(readCodeMirrorSelection(stateWith("one two", 3, 3))).toBeNull();
    expect(readCodeMirrorSelection(stateWith("one   two", 3, 6))).toBeNull();
  });
});
