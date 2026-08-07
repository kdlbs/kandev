import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { ReviewItemSummary } from "@/lib/plugins/types";
import { reviewItemId, selectReviewItem, useReviewItemSelection } from "./review-selection";

const githubReview: ReviewItemSummary = {
  providerId: "github",
  reviewKey: "owner/repository/12",
  title: "GitHub pull request",
  url: "https://github.test/owner/repository/pull/12",
  repositoryId: "owner/repository",
  state: "OPEN",
};

const bitbucketReview: ReviewItemSummary = {
  providerId: "bitbucket",
  reviewKey: "workspace/repository/42",
  title: "Bitbucket pull request",
  url: "https://bitbucket.test/workspace/repository/pull-requests/42",
  repositoryId: "workspace/repository",
  state: "OPEN",
};

const secondGithubReview: ReviewItemSummary = {
  ...githubReview,
  reviewKey: "owner/repository/13",
  title: "Second GitHub pull request",
  url: "https://github.test/owner/repository/pull/13",
};

describe("selectReviewItem", () => {
  it("requires an explicit choice when built-in and plugin reviews coexist", () => {
    expect(selectReviewItem([githubReview, bitbucketReview], null)).toBeNull();
  });

  it("selects the requested plugin review instead of the first built-in result", () => {
    expect(selectReviewItem([githubReview, bitbucketReview], reviewItemId(bitbucketReview))).toBe(
      bitbucketReview,
    );
  });

  it("keeps delimiter-bearing provider and review identities distinct", () => {
    const first = { ...bitbucketReview, providerId: "a:b", reviewKey: "c" };
    const second = { ...bitbucketReview, providerId: "a", reviewKey: "b:c" };

    expect(reviewItemId(first)).not.toBe(reviewItemId(second));
    expect(selectReviewItem([first, second], reviewItemId(second))).toBe(second);
  });

  it("opens a lone review without an unnecessary chooser", () => {
    expect(selectReviewItem([bitbucketReview], null)).toBe(bitbucketReview);
  });

  it("preserves the provider's primary review when every choice has the same owner", () => {
    expect(selectReviewItem([githubReview, secondGithubReview], null)).toBe(githubReview);
  });
});

describe("useReviewItemSelection", () => {
  it("honors an externally requested review from a same-provider list", () => {
    const { result } = renderHook(() =>
      useReviewItemSelection(
        "task-1",
        [githubReview, secondGithubReview],
        reviewItemId(secondGithubReview),
      ),
    );

    expect(result.current.selectedReview).toBe(secondGithubReview);
  });

  it("keeps an explicit local choice while the external request is unchanged", () => {
    const preferredReviewId = reviewItemId(githubReview);
    const { result, rerender } = renderHook(() =>
      useReviewItemSelection("task-1", [githubReview, bitbucketReview], preferredReviewId),
    );

    act(() => result.current.selectReview(bitbucketReview));
    rerender();

    expect(result.current.selectedReview).toBe(bitbucketReview);
  });

  it("follows a changed external review request", () => {
    const { result, rerender } = renderHook(
      ({ preferredReviewId }) =>
        useReviewItemSelection("task-1", [githubReview, secondGithubReview], preferredReviewId),
      { initialProps: { preferredReviewId: reviewItemId(githubReview) } },
    );

    rerender({ preferredReviewId: reviewItemId(secondGithubReview) });

    expect(result.current.selectedReview).toBe(secondGithubReview);
  });
});
