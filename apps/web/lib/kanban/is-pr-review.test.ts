import { describe, it, expect } from "vitest";
import { isPRReviewFromMetadata } from "./is-pr-review";

describe("isPRReviewFromMetadata", () => {
  it("returns false for null/undefined/non-object", () => {
    expect(isPRReviewFromMetadata(null)).toBe(false);
    expect(isPRReviewFromMetadata(undefined)).toBe(false);
    expect(isPRReviewFromMetadata("string")).toBe(false);
    expect(isPRReviewFromMetadata(42)).toBe(false);
  });

  it("returns false when review_watch_id missing or empty", () => {
    expect(isPRReviewFromMetadata({})).toBe(false);
    expect(isPRReviewFromMetadata({ review_watch_id: "" })).toBe(false);
    expect(isPRReviewFromMetadata({ review_watch_id: 123 })).toBe(false);
  });

  it("returns true for non-empty string review_watch_id", () => {
    expect(isPRReviewFromMetadata({ review_watch_id: "watch-abc" })).toBe(true);
  });
});
