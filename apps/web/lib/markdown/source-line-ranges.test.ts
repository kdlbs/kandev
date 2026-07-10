import { describe, expect, it } from "vitest";
import {
  findLineRangeForSelectedText,
  findNearestSourceElement,
  readSourceLineRange,
  resolveMarkdownDomSelection,
  SOURCE_END_ATTR,
  SOURCE_START_ATTR,
} from "./source-line-ranges";

describe("markdown preview source line ranges", () => {
  it("reads a source line range from markdown preview attributes", () => {
    const element = document.createElement("p");
    element.setAttribute(SOURCE_START_ATTR, "4");
    element.setAttribute(SOURCE_END_ATTR, "6");

    expect(readSourceLineRange(element)).toEqual({ startLine: 4, endLine: 6 });
  });

  it("finds the nearest source element for a nested selected text node", () => {
    const root = document.createElement("div");
    const paragraph = document.createElement("p");
    const strong = document.createElement("strong");
    strong.textContent = "selected";
    paragraph.setAttribute(SOURCE_START_ATTR, "8");
    paragraph.setAttribute(SOURCE_END_ATTR, "8");
    paragraph.appendChild(strong);
    root.appendChild(paragraph);

    expect(findNearestSourceElement(root, strong.firstChild)).toBe(paragraph);
  });

  it("falls back to raw markdown lookup for rendered text spanning source lines", () => {
    const content = [
      "# Title",
      "",
      "First paragraph",
      "continues here",
      "",
      "Second paragraph",
    ].join("\r\n");

    expect(findLineRangeForSelectedText(content, "First paragraph continues here")).toEqual({
      startLine: 3,
      endLine: 4,
    });
  });

  it("resolves a DOM selection to the combined source line range", () => {
    const root = document.createElement("div");
    const first = document.createElement("p");
    first.setAttribute(SOURCE_START_ATTR, "3");
    first.setAttribute(SOURCE_END_ATTR, "3");
    first.textContent = "Alpha paragraph";
    const second = document.createElement("p");
    second.setAttribute(SOURCE_START_ATTR, "5");
    second.setAttribute(SOURCE_END_ATTR, "5");
    second.textContent = "Beta paragraph";
    root.append(first, second);
    document.body.appendChild(root);

    const range = document.createRange();
    range.setStart(first.firstChild!, 0);
    range.setEnd(second.firstChild!, 4);
    const selection = window.getSelection()!;
    selection.removeAllRanges();
    selection.addRange(range);

    expect(
      resolveMarkdownDomSelection(root, "# Title\n\nAlpha paragraph\n\nBeta paragraph", selection),
    ).toMatchObject({
      selectedText: "Alpha paragraphBeta",
      startLine: 3,
      endLine: 5,
    });

    root.remove();
    selection.removeAllRanges();
  });
});
