import { act, cleanup, fireEvent, render, renderHook, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TaskPR } from "@/lib/types/github";
import { ReviewPRDiffBoundary, useReviewDialogTransientState } from "./review-dialog-pr-state";

afterEach(cleanup);

const selectedPR = {
  repo: "widgets",
  pr_number: 42,
} as TaskPR;

describe("ReviewPRDiffBoundary", () => {
  it("retries a failed selected PR without rendering stale children", () => {
    const onRetry = vi.fn();
    render(
      <ReviewPRDiffBoundary
        selectedPR={selectedPR}
        loading={false}
        error="Could not load PR changes"
        onRetry={onRetry}
      >
        <span>stale diff</span>
      </ReviewPRDiffBoundary>,
    );

    expect(screen.queryByText("stale diff")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it("ignores PR fetch state for a local-only review source", () => {
    render(
      <ReviewPRDiffBoundary selectedPR={null} loading error="Could not load PR changes">
        <span>local diff</span>
      </ReviewPRDiffBoundary>,
    );

    expect(screen.getByText("local diff")).toBeTruthy();
  });
});

describe("useReviewDialogTransientState", () => {
  it("derives cleared state for a new source and keeps subsequent edits on that source", () => {
    const { result, rerender } = renderHook(
      ({ sourceKey }) => useReviewDialogTransientState(sourceKey),
      { initialProps: { sourceKey: "session-a:pr-1" } },
    );

    act(() => {
      result.current.setSelectedFile("src/old.ts");
      result.current.setFilter("old");
    });
    rerender({ sourceKey: "session-b:pr-1" });

    expect(result.current.selectedFile).toBeNull();
    expect(result.current.filter).toBe("");

    act(() => result.current.setFilter("new"));
    expect(result.current.filter).toBe("new");
  });
});
