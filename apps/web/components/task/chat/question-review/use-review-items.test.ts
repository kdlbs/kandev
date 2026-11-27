import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { buildReviewSegments, type ReviewSegment } from "@/lib/question-review/segments";
import type { ClarificationQuestion, QuestionImageGroup } from "@/lib/types/http";
import { useReviewItems } from "./use-review-items";

const image = (id: string) => ({ id, path: `${id}.png`, alt: id });

function segment(ids: string[], groups?: QuestionImageGroup[]): ReviewSegment {
  const question: ClarificationQuestion = {
    id: "q1",
    title: "",
    prompt: "Approve?",
    options: [],
    images: ids.map(image),
    image_groups: groups,
  };
  return buildReviewSegments([{ questionId: "q1", question }])[0];
}

const mixed = () =>
  segment(
    ["before", "after", "billing"],
    [
      { kind: "pair", image_ids: ["before", "after"] },
      { kind: "single", image_ids: ["billing"] },
    ],
  );

describe("useReviewItems", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-003.3
  it("steps between items without wrapping and handles PageUp and PageDown", () => {
    const { result } = renderHook(() => useReviewItems(mixed()));
    expect(result.current.index).toBe(0);
    expect(result.current.item?.kind).toBe("pair");
    expect(result.current.canPrev).toBe(false);
    act(() => result.current.step(1));
    expect(result.current.item?.kind).toBe("single");
    expect(result.current.canNext).toBe(false);
    act(() => result.current.step(1));
    expect(result.current.index).toBe(1);
    let handled = false;
    act(() => {
      handled = result.current.handleKey({ key: "PageUp" });
    });
    expect(handled).toBe(true);
    expect(result.current.index).toBe(0);
    expect(result.current.handleKey({ key: "x" })).toBe(false);
  });

  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-003.4
  it("starts an operator comparison from a gallery image and returns to A's item", () => {
    const { result } = renderHook(() => useReviewItems(segment(["one", "two", "three"])));
    expect(result.current.canCompare).toBe(true);
    act(() => result.current.step(1));
    act(() => result.current.startCompare());
    expect(result.current.compare).toEqual({ aId: "two", bId: "three" });
    act(() => result.current.backToGallery("three"));
    expect(result.current.compare).toBeNull();
    expect(result.current.index).toBe(2);
    act(() => result.current.startCompare());
    expect(result.current.compare).toEqual({ aId: "three", bId: "two" });
  });

  it("offers comparison only for ungrouped segments with two or more images", () => {
    expect(renderHook(() => useReviewItems(mixed())).result.current.canCompare).toBe(false);
    expect(renderHook(() => useReviewItems(segment(["one"]))).result.current.canCompare).toBe(
      false,
    );
  });

  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-003.5
  it("moves to the item that shows a selected region's image", () => {
    const { result } = renderHook(() => useReviewItems(mixed()));
    act(() => result.current.showImage("billing"));
    expect(result.current.index).toBe(1);
    act(() => result.current.showImage("after"));
    expect(result.current.index).toBe(0);
    act(() => result.current.showImage("missing"));
    expect(result.current.index).toBe(0);
  });

  it("keeps an operator comparison when its region image is among the compared images", () => {
    const { result } = renderHook(() => useReviewItems(segment(["one", "two", "three"])));
    act(() => result.current.startCompare());
    act(() => result.current.showImage("three"));
    expect(result.current.compare).toEqual({ aId: "one", bId: "two" });
  });
});
