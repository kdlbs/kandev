import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { reconcileReviews, ReviewsSection } from "./pr-reviews-section";
import type { PRReview, RequestedReviewer } from "@/lib/types/github";

function review(author: string, state: string, created_at = "2026-07-23T10:00:00Z"): PRReview {
  return { id: 1, author, author_avatar: "", state, body: "", created_at };
}

function requested(login: string): RequestedReviewer {
  return { login, type: "user" };
}

function renderReviews(ui: React.ReactElement) {
  return render(<TooltipProvider>{ui}</TooltipProvider>);
}

afterEach(() => cleanup());

describe("reconcileReviews", () => {
  it("keeps only latest review per author and enables its dismissed re-request action", () => {
    const result = reconcileReviews(
      [review("octocat", "APPROVED", "2026-07-22T10:00:00Z"), review("octocat", "DISMISSED")],
      [],
    );

    expect(result.reviews).toEqual([review("octocat", "DISMISSED")]);
    expect(result.pendingReviewers).toEqual([]);
  });

  it("makes current requests win over dismissed history case-insensitively", () => {
    const result = reconcileReviews([review("OctoCat", "DISMISSED")], [requested("octocat")]);

    expect(result.reviews).toEqual([]);
    expect(result.pendingReviewers).toEqual([requested("octocat")]);
  });

  it("deduplicates current requested reviewers case-insensitively", () => {
    const result = reconcileReviews([], [requested("OctoCat"), requested("octocat")]);

    expect(result.pendingReviewers).toEqual([requested("OctoCat")]);
  });
});

describe("ReviewsSection re-request action", () => {
  it("names dismissed reviewer and blocks repeated submission while busy", () => {
    const onReRequest = vi.fn();
    renderReviews(
      <ReviewsSection
        reviews={[review("octocat", "DISMISSED")]}
        requestedReviewers={[]}
        prUrl="https://github.com/acme/site/pull/1"
        reviewState="pending"
        pendingReviewCount={0}
        onAddAsContext={vi.fn()}
        canReRequest
        requestingReviewers={["OCTOCAT"]}
        onReRequest={onReRequest}
      />,
    );

    const action = screen.getByRole("button", { name: "Re-request review from octocat" });
    expect(action).toHaveProperty("disabled", true);
    expect(action.className).toContain("min-h-11");
    fireEvent.click(action);
    expect(onReRequest).not.toHaveBeenCalled();
  });

  it("wraps a long reviewer row so the 44px action stays on-screen on phones", () => {
    const longLogin = "reviewer-with-a-near-maximum-practical-github-login";
    renderReviews(
      <ReviewsSection
        reviews={[review(longLogin, "DISMISSED")]}
        requestedReviewers={[]}
        prUrl="https://github.com/acme/site/pull/1"
        reviewState="pending"
        pendingReviewCount={0}
        onAddAsContext={vi.fn()}
        canReRequest
        onReRequest={vi.fn()}
      />,
    );

    const action = screen.getByRole("button", {
      name: `Re-request review from ${longLogin}`,
    });
    const author = screen.getByRole("link", { name: longLogin });
    expect(action.className).toContain("min-h-11");
    expect(action.parentElement?.className).toContain("basis-full");
    expect(author.className).toContain("inline-block");
    expect(author.className).toContain("max-w-full");
  });

  it("does not show action for a closed PR or a currently requested reviewer", () => {
    const { rerender } = renderReviews(
      <ReviewsSection
        reviews={[review("octocat", "DISMISSED")]}
        requestedReviewers={[]}
        prUrl="https://github.com/acme/site/pull/1"
        reviewState="pending"
        pendingReviewCount={0}
        onAddAsContext={vi.fn()}
      />,
    );
    expect(screen.queryByRole("button", { name: "Re-request review from octocat" })).toBeNull();

    rerender(
      <TooltipProvider>
        <ReviewsSection
          reviews={[review("octocat", "DISMISSED")]}
          requestedReviewers={[requested("OctoCat")]}
          prUrl="https://github.com/acme/site/pull/1"
          reviewState="pending"
          pendingReviewCount={1}
          onAddAsContext={vi.fn()}
          canReRequest
          onReRequest={vi.fn()}
        />
        ,
      </TooltipProvider>,
    );
    expect(screen.queryByRole("button", { name: "Re-request review from octocat" })).toBeNull();
    expect(screen.getByText("OctoCat")).not.toBeNull();
  });
});
