import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { isQuestionReviewAvailable, useQuestionReview } from "./use-question-review";

describe("isQuestionReviewAvailable", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-001.1
  it("allows review only for pending, answerable image bundles", () => {
    const base = { hasImages: true, lateInteraction: false, submitState: "idle" as const };
    expect(isQuestionReviewAvailable(base)).toBe(true);
    expect(isQuestionReviewAvailable({ ...base, submitState: "error" })).toBe(true);
    expect(isQuestionReviewAvailable({ ...base, hasImages: false })).toBe(false);
    expect(isQuestionReviewAvailable({ ...base, lateInteraction: true })).toBe(false);
    expect(isQuestionReviewAvailable({ ...base, submitState: "ok" })).toBe(false);
    expect(isQuestionReviewAvailable({ ...base, submitState: "expired" })).toBe(false);
  });
});

describe("useQuestionReview", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-001.2
  it("stays closed until opened explicitly", () => {
    const { result } = renderHook(() => useQuestionReview({ pendingId: "p1", available: true }));
    expect(result.current.open).toBe(false);
    act(() => result.current.openReview());
    expect(result.current.open).toBe(true);
    act(() => result.current.closeReview());
    expect(result.current.open).toBe(false);
  });

  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-001.5
  it("closes when the bundle is replaced or becomes unavailable, and does not reopen", () => {
    const { result, rerender } = renderHook(
      (props: { pendingId: string | null; available: boolean }) => useQuestionReview(props),
      { initialProps: { pendingId: "p1", available: true } },
    );
    act(() => result.current.openReview());
    rerender({ pendingId: "p2", available: true });
    expect(result.current.open).toBe(false);
    act(() => result.current.openReview());
    rerender({ pendingId: "p2", available: false });
    expect(result.current.open).toBe(false);
    rerender({ pendingId: "p2", available: true });
    expect(result.current.open).toBe(false);
  });

  it("ignores open requests while unavailable", () => {
    const { result } = renderHook(() => useQuestionReview({ pendingId: "p1", available: false }));
    act(() => result.current.openReview());
    expect(result.current.open).toBe(false);
  });
});
