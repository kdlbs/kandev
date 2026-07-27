import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { TaskReviewFinding } from "@/lib/types/review";
import { ReviewFindingsButton } from "./review-findings-button";

const { navigateToFinding } = vi.hoisted(() => ({
  navigateToFinding: vi.fn((..._args: unknown[]) => Promise.resolve(true)),
}));
vi.mock("@/lib/review/navigation", () => ({ navigateToFinding }));

function finding(overrides: Partial<TaskReviewFinding> = {}): TaskReviewFinding {
  return {
    id: "f1",
    run_id: "r1",
    task_id: "t1",
    repository_id: "",
    repository_name: "",
    file_path: "a.ts",
    start_line: 5,
    end_line: 5,
    side: "additions",
    severity: "blocker",
    category: "correctness",
    title: "A finding",
    body: "body",
    suggestion: "",
    anchor_text: "",
    file_diff_hash: "h",
    status: "open",
    created_at: "2026-07-24T10:00:00Z",
    updated_at: "2026-07-24T10:00:00Z",
    ...overrides,
  };
}

function renderButton(findings: TaskReviewFinding[], onSelectFile = vi.fn()) {
  return render(
    <TooltipProvider>
      <ReviewFindingsButton findings={findings} onSelectFile={onSelectFile} />
    </TooltipProvider>,
  );
}

afterEach(() => {
  cleanup();
  navigateToFinding.mockClear();
});

describe("ReviewFindingsButton", () => {
  it("renders nothing when there are no open findings", () => {
    const { container } = renderButton([finding({ status: "dismissed" })]);
    expect(container.firstChild).toBeNull();
    expect(screen.queryByTestId("review-open-count")).toBeNull();
  });

  it("shows the open-finding count", () => {
    renderButton([finding({ id: "a" }), finding({ id: "b", status: "resolved" })]);
    expect(screen.getByTestId("review-open-count").textContent).toContain("1 finding");
  });

  it("opens the navigator and jumps to the clicked finding", async () => {
    const onSelectFile = vi.fn();
    const target = finding({ id: "clickme", title: "Click me" });
    renderButton([target], onSelectFile);

    fireEvent.click(screen.getByTestId("review-open-count"));
    const item = await screen.findByText("Click me");
    fireEvent.click(item);

    expect(navigateToFinding).toHaveBeenCalledTimes(1);
    expect(navigateToFinding.mock.calls[0][0]).toBe(target);
    expect(navigateToFinding.mock.calls[0][1]).toBe(onSelectFile);
    // The popover closes on navigate.
    await waitFor(() => expect(screen.queryByTestId("review-findings-popover")).toBeNull());
  });
});
