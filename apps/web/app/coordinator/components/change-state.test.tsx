import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ChangeView } from "@/lib/api/domains/coordinator-merge-api";
import { ChangeState } from "./change-state";

afterEach(cleanup);

function change(over: Partial<ChangeView> = {}): ChangeView {
  return {
    key: "github|h|r1|7",
    provider: "github",
    host: "h",
    repository_id: "r1",
    number: 7,
    target_branch: "main",
    head_sha: "0123456789abcdef",
    state: "open",
    pipeline: "none",
    mergeable: "met",
    conflict: "met",
    approvals: "met",
    draft: "met",
    unresolved_threads: 0,
    behind: 2,
    unavailable: false,
    observed_at: "2026-05-01T10:00:00Z",
    ...over,
  };
}

describe("ChangeState", () => {
  it("shows every field and labels a green-less change as No checks", () => {
    render(<ChangeState changes={[change()]} status="ready" onRetry={vi.fn()} />);
    const card = screen.getByTestId("queue-change-7");
    expect(card.textContent).toContain("main");
    expect(card.textContent).toContain("0123456");
    expect(card.textContent).toContain("No checks");
    expect(card.textContent).toContain("2 commits behind");
    expect(screen.queryByTestId("queue-change-failed-7")).toBeNull();
  });

  it("says unavailable with Retry for a change that was never read", () => {
    const onRetry = vi.fn();
    render(
      <ChangeState
        changes={[
          change({
            unavailable: true,
            observed_at: "0001-01-01T00:00:00Z",
            target_branch: "",
            head_sha: "",
            behind: null,
            unresolved_threads: null,
            pipeline: "unavailable",
          }),
        ]}
        status="ready"
        onRetry={onRetry}
      />,
    );
    const card = screen.getByTestId("queue-change-7");
    expect(card.textContent).toContain("Unavailable");
    expect(card.textContent).not.toContain("0 commits");
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it("keeps the last good values when the latest read failed", () => {
    render(
      <ChangeState changes={[change({ unavailable: true })]} status="ready" onRetry={vi.fn()} />,
    );
    const card = screen.getByTestId("queue-change-7");
    expect(card.textContent).toContain("main");
    expect(screen.getByTestId("queue-change-failed-7")).toBeTruthy();
  });

  it("offers Retry when nothing could be read", () => {
    const onRetry = vi.fn();
    render(<ChangeState changes={null} status="error" onRetry={onRetry} />);
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(onRetry).toHaveBeenCalled();
  });
});
