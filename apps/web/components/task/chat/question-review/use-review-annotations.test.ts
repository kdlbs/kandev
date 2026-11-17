import { act, renderHook } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it } from "vitest";
import { useReviewAnnotations, type ReviewAnnotations } from "./use-review-annotations";

const rect = { x: 1, y: 2, width: 3, height: 4 };
const image = { width: 10, height: 10 };

function useHarness() {
  const [annotations, setAnnotations] = useState<ReviewAnnotations>({});
  return { annotations, controller: useReviewAnnotations(annotations, setAnnotations) };
}

describe("region comments", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-007.1
  it("opens the comment for a new region and keeps it once commented", () => {
    const { result } = renderHook(useHarness);
    act(() => result.current.controller.add("q1", "after", rect, image));
    expect(result.current.controller.commenting).toEqual({
      questionId: "q1",
      index: 0,
      isNew: true,
    });
    act(() => {
      result.current.controller.saveComment("Too tight");
      result.current.controller.closeComment();
    });
    expect(result.current.controller.commenting).toBeNull();
    expect(result.current.annotations.q1).toEqual([
      expect.objectContaining({ image_id: "after", comment: "Too tight" }),
    ]);
  });

  it("discards a new region when its comment is cancelled", () => {
    const { result } = renderHook(useHarness);
    act(() => result.current.controller.add("q1", "after", rect, image));
    act(() => result.current.controller.closeComment());
    expect(result.current.annotations.q1).toBeUndefined();
    expect(result.current.controller.commenting).toBeNull();
  });

  it("keeps an existing region when editing is cancelled", () => {
    const { result } = renderHook(useHarness);
    act(() => result.current.controller.add("q1", "after", rect, image));
    act(() => {
      result.current.controller.saveComment("Keep");
      result.current.controller.closeComment();
    });
    act(() => result.current.controller.editComment("q1", 0));
    expect(result.current.controller.commenting).toEqual({
      questionId: "q1",
      index: 0,
      isNew: false,
    });
    act(() => result.current.controller.closeComment());
    expect(result.current.annotations.q1?.[0].comment).toBe("Keep");
  });
});
