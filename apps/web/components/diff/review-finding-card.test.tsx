import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { TaskReviewFinding } from "@/lib/types/review";
import { ReviewFindingCard } from "./review-finding-card";

function finding(overrides: Partial<TaskReviewFinding> = {}): TaskReviewFinding {
  return {
    id: "f1",
    run_id: "r1",
    task_id: "t1",
    repository_id: "",
    repository_name: "",
    file_path: "apps/web/a.ts",
    start_line: 12,
    end_line: 12,
    side: "additions",
    severity: "blocker",
    category: "correctness",
    title: "Nil dereference",
    body: "`x` can be nil here.",
    suggestion: "",
    anchor_text: "",
    file_diff_hash: "h",
    status: "open",
    created_at: "2026-07-24T10:00:00Z",
    updated_at: "2026-07-24T10:00:00Z",
    ...overrides,
  };
}

afterEach(cleanup);

describe("ReviewFindingCard", () => {
  it("renders the severity, category, title, and body", () => {
    render(<ReviewFindingCard finding={finding()} />);
    expect(screen.getByTestId("review-finding-severity-blocker")).toBeTruthy();
    expect(screen.getByTestId("review-finding-title").textContent).toContain("Nil dereference");
    expect(screen.getByTestId("review-finding-body").textContent).toContain("can be nil here");
    expect(screen.getByText("correctness")).toBeTruthy();
  });

  it("offers resolve, dismiss, and send-to-agent for an open finding", () => {
    const onResolve = vi.fn();
    const onDismiss = vi.fn();
    const onSendToAgent = vi.fn();
    const target = finding();
    render(
      <ReviewFindingCard
        finding={target}
        onResolve={onResolve}
        onDismiss={onDismiss}
        onSendToAgent={onSendToAgent}
      />,
    );

    fireEvent.click(screen.getByTestId("review-finding-resolve"));
    fireEvent.click(screen.getByTestId("review-finding-dismiss"));
    fireEvent.click(screen.getByTestId("review-finding-send-to-agent"));

    expect(onResolve).toHaveBeenCalledWith(target);
    expect(onDismiss).toHaveBeenCalledWith(target);
    expect(onSendToAgent).toHaveBeenCalledWith(target);
  });

  it("collapses a resolved finding and offers undo", () => {
    const onReopen = vi.fn();
    const resolved = finding({ status: "resolved" });
    render(<ReviewFindingCard finding={resolved} onReopen={onReopen} />);

    expect(screen.getByTestId("review-finding-card").getAttribute("data-finding-status")).toBe(
      "resolved",
    );
    // The collapsed form drops the body to keep a reviewed diff readable.
    expect(screen.queryByTestId("review-finding-body")).toBeNull();
    fireEvent.click(screen.getByTestId("review-finding-reopen"));
    expect(onReopen).toHaveBeenCalledWith(resolved);
  });

  it("collapses a dismissed finding too", () => {
    render(<ReviewFindingCard finding={finding({ status: "dismissed" })} />);
    expect(screen.getByTestId("review-finding-card").getAttribute("data-finding-status")).toBe(
      "dismissed",
    );
  });

  it("marks a suggestion as not applied automatically", () => {
    render(<ReviewFindingCard finding={finding({ suggestion: "if (x) {" })} />);
    expect(screen.getByText(/not applied automatically/i)).toBeTruthy();
    expect(screen.getByText("if (x) {")).toBeTruthy();
  });

  it("shows a stale badge and reason when the anchor moved", () => {
    render(
      <ReviewFindingCard finding={finding()} staleReason="The diff changed after this review" />,
    );
    expect(screen.getByTestId("review-finding-stale")).toBeTruthy();
    expect(screen.getByText(/The diff changed after this review/)).toBeTruthy();
  });

  it("shows the location only when asked", () => {
    const { rerender } = render(<ReviewFindingCard finding={finding()} />);
    expect(screen.queryByText("apps/web/a.ts:12")).toBeNull();
    rerender(<ReviewFindingCard finding={finding()} showLocation />);
    expect(screen.getByText("apps/web/a.ts:12")).toBeTruthy();
  });

  it("omits actions that were not supplied", () => {
    render(<ReviewFindingCard finding={finding()} />);
    expect(screen.queryByTestId("review-finding-resolve")).toBeNull();
    expect(screen.queryByTestId("review-finding-send-to-agent")).toBeNull();
  });
});
