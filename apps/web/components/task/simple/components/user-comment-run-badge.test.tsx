import { describe, it, expect, afterEach, vi } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import type { ReactNode } from "react";

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children, ...rest }: { children: ReactNode } & Record<string, unknown>) => (
    <div {...rest}>{children}</div>
  ),
}));

import { UserCommentRunBadge } from "./user-comment-run-badge";

afterEach(() => cleanup());

const BADGE_TESTID = "user-comment-run-badge";
const TOOLTIP_TESTID = "user-comment-run-badge-tooltip";
const DATA_STATUS = "data-status";

describe("UserCommentRunBadge", () => {
  it("renders 'Queued' for status=queued", () => {
    render(<UserCommentRunBadge status="queued" />);
    const badge = screen.getByTestId(BADGE_TESTID);
    expect(badge.getAttribute(DATA_STATUS)).toBe("queued");
    expect(badge.textContent).toContain("Queued");
  });

  it("renders 'Working…' with a spinner for status=claimed", () => {
    const { container } = render(<UserCommentRunBadge status="claimed" />);
    const badge = screen.getByTestId(BADGE_TESTID);
    expect(badge.getAttribute(DATA_STATUS)).toBe("claimed");
    expect(badge.textContent).toContain("Working");
    // Spinner uses the animate-spin utility class.
    expect(container.querySelector(".animate-spin")).toBeTruthy();
  });

  it("renders nothing for status=finished", () => {
    const { container } = render(<UserCommentRunBadge status="finished" />);
    expect(container.firstChild).toBeNull();
    expect(screen.queryByTestId(BADGE_TESTID)).toBeNull();
  });

  it("renders 'Failed' with the errorMessage in tooltip for status=failed", () => {
    render(<UserCommentRunBadge status="failed" errorMessage="boom: agent crashed" />);
    const badge = screen.getByTestId(BADGE_TESTID);
    expect(badge.getAttribute(DATA_STATUS)).toBe("failed");
    expect(badge.textContent).toContain("Failed");
    const tip = screen.getByTestId(TOOLTIP_TESTID);
    expect(tip.textContent).toContain("boom: agent crashed");
  });

  it("renders 'Failed' without a tooltip when no errorMessage is supplied", () => {
    render(<UserCommentRunBadge status="failed" />);
    const badge = screen.getByTestId(BADGE_TESTID);
    expect(badge.getAttribute(DATA_STATUS)).toBe("failed");
    expect(screen.queryByTestId(TOOLTIP_TESTID)).toBeNull();
  });

  it("renders 'Cancelled' for status=cancelled", () => {
    render(<UserCommentRunBadge status="cancelled" />);
    const badge = screen.getByTestId(BADGE_TESTID);
    expect(badge.getAttribute(DATA_STATUS)).toBe("cancelled");
    expect(badge.textContent).toContain("Cancelled");
  });
});
