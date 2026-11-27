import { describe, expect, it } from "vitest";
import {
  changesTimelineNextIndex,
  isChangesTimelineTextEntry,
} from "./changes-timeline-interaction";

describe("Changes timeline keyboard navigation", () => {
  it("moves through logical rows and supports Home and End", () => {
    expect(changesTimelineNextIndex("ArrowDown", 4, 10)).toBe(5);
    expect(changesTimelineNextIndex("ArrowUp", 4, 10)).toBe(3);
    expect(changesTimelineNextIndex("Home", 4, 10)).toBe(0);
    expect(changesTimelineNextIndex("End", 4, 10)).toBe(9);
    expect(changesTimelineNextIndex("ArrowDown", 9, 10)).toBe(9);
    expect(changesTimelineNextIndex("ArrowUp", 0, 10)).toBe(0);
    expect(changesTimelineNextIndex("End", 0, 0)).toBe(-1);
  });

  it("does not capture navigation keys from editable controls", () => {
    const input = document.createElement("input");
    const button = document.createElement("button");
    const editable = document.createElement("div");
    editable.contentEditable = "true";
    expect(isChangesTimelineTextEntry(input)).toBe(true);
    expect(isChangesTimelineTextEntry(editable)).toBe(true);
    expect(isChangesTimelineTextEntry(button)).toBe(false);
    expect(isChangesTimelineTextEntry(null)).toBe(false);
  });
});
