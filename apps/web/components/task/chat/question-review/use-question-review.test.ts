import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { isQuestionReviewAvailable, useQuestionReview } from "./use-question-review";

describe("isQuestionReviewAvailable", () => {
  // @covers AC-TASKS-QUESTION-IMAGE-REVIEW-001.1
  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-001.1
  it("allows review for editable image bundles, including late-answer forms", () => {
    const base = { hasImages: true, lateStatus: "idle" as const, submitState: "idle" as const };
    expect(isQuestionReviewAvailable(base)).toBe(true);
    expect(isQuestionReviewAvailable({ ...base, submitState: "error" })).toBe(true);
    expect(isQuestionReviewAvailable({ ...base, lateStatus: "sending" })).toBe(true);
    expect(isQuestionReviewAvailable({ ...base, lateStatus: "error" })).toBe(true);
    expect(isQuestionReviewAvailable({ ...base, hasImages: false })).toBe(false);
    expect(isQuestionReviewAvailable({ ...base, lateStatus: "sent" })).toBe(false);
    expect(isQuestionReviewAvailable({ ...base, lateStatus: "queued" })).toBe(false);
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

  // @covers AC-TASKS-QUESTION-IMAGE-ITEMS-004.4
  it("keeps the Wipe handle height for the question round only", () => {
    const { result, rerender } = renderHook(
      (props: { pendingId: string | null; available: boolean }) => useQuestionReview(props),
      { initialProps: { pendingId: "p1", available: true } },
    );
    expect(result.current.wipeLift.value).toBeUndefined();
    act(() => result.current.wipeLift.set(180));
    act(() => result.current.openReview());
    act(() => result.current.closeReview());
    expect(result.current.wipeLift.value).toBe(180);
    rerender({ pendingId: "p2", available: true });
    expect(result.current.wipeLift.value).toBeUndefined();
    rerender({ pendingId: "p1", available: true });
    expect(result.current.wipeLift.value).toBeUndefined();
  });
});
