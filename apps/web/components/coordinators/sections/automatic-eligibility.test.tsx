import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ClassEligibility } from "@/lib/api/domains/coordinator-automatic-api";

let userId: string | null = "u1";
vi.mock("@/components/state-provider", () => ({
  useAppStore: (sel: (s: unknown) => unknown) =>
    sel({ auth: { user: userId ? { id: userId } : null } }),
}));

import { AutomaticEligibility, RaisedRecord } from "./automatic-eligibility";

const eligibility = (over: Partial<ClassEligibility> = {}): ClassEligibility => ({
  eligible: false,
  conditions: [
    { name: "history_30d", met: true, value: "2026-08-01T00:00:00Z" },
    { name: "volume", met: false, value: 19 },
    { name: "unedited_rate", met: false, value: 85 },
    { name: "no_undo", met: true, value: 0 },
    { name: "reviewed_7d", met: false, value: null },
  ],
  setting: "requires_approval",
  changed_by: "",
  changed_at: null,
  ...over,
});

const props = {
  status: "ready" as const,
  canManage: true,
  reviewOpened: false,
  reviewFailed: false,
  onMarkReviewed: vi.fn(),
  onRetry: vi.fn(),
};

const NOT_MET = "Not met";
const MARK_REVIEWED = "automatic-mark-reviewed";

afterEach(() => {
  cleanup();
  userId = "u1";
});

describe("AutomaticEligibility", () => {
  it("lists every condition as Met or Not met with its value", () => {
    render(<AutomaticEligibility {...props} eligibility={eligibility()} />);
    expect(screen.getByTestId("automatic-condition-volume").textContent).toContain(NOT_MET);
    expect(screen.getByTestId("automatic-condition-volume").textContent).toContain("19");
    expect(screen.getByTestId("automatic-condition-unedited_rate").textContent).toContain("85%");
    expect(screen.getByTestId("automatic-condition-no_undo").getAttribute("data-met")).toBe("true");
    expect(screen.getByTestId("automatic-condition-reviewed_7d").textContent).toContain(NOT_MET);
  });

  it("keeps Mark as reviewed disabled until the log was opened", () => {
    const onMarkReviewed = vi.fn();
    const { rerender } = render(
      <AutomaticEligibility
        {...props}
        onMarkReviewed={onMarkReviewed}
        eligibility={eligibility()}
      />,
    );
    const button = screen.getByTestId(MARK_REVIEWED) as HTMLButtonElement;
    expect(button.disabled).toBe(true);
    rerender(
      <AutomaticEligibility
        {...props}
        reviewOpened
        onMarkReviewed={onMarkReviewed}
        eligibility={eligibility()}
      />,
    );
    expect((screen.getByTestId(MARK_REVIEWED) as HTMLButtonElement).disabled).toBe(false);
    fireEvent.click(screen.getByTestId(MARK_REVIEWED));
    expect(onMarkReviewed).toHaveBeenCalledTimes(1);
  });

  it("offers no review control to a reader", () => {
    render(<AutomaticEligibility {...props} canManage={false} eligibility={eligibility()} />);
    expect(screen.queryByTestId(MARK_REVIEWED)).toBeNull();
  });

  it("shows a retry when the read failed and a review failure alert", () => {
    const onRetry = vi.fn();
    const { rerender } = render(
      <AutomaticEligibility {...props} status="error" onRetry={onRetry} eligibility={null} />,
    );
    fireEvent.click(screen.getByRole("button"));
    expect(onRetry).toHaveBeenCalled();
    rerender(<AutomaticEligibility {...props} reviewFailed eligibility={eligibility()} />);
    expect(screen.getByTestId("automatic-review-failed")).toBeTruthy();
  });
});

describe("RaisedRecord", () => {
  const raised = eligibility({
    setting: "automatic",
    changed_by: "u1",
    changed_at: "2026-09-01T10:00:00Z",
  });

  it("names the viewer when they raised it and another manager otherwise", () => {
    const { unmount } = render(<RaisedRecord eligibility={raised} />);
    expect(screen.getByTestId("automatic-raised-record").textContent).toContain("you");
    unmount();
    userId = "u2";
    render(<RaisedRecord eligibility={raised} />);
    expect(screen.getByTestId("automatic-raised-record").textContent).not.toContain("you");
  });

  it("renders nothing while the class is not automatic", () => {
    render(<RaisedRecord eligibility={eligibility({ changed_at: "2026-09-01T10:00:00Z" })} />);
    expect(screen.queryByTestId("automatic-raised-record")).toBeNull();
  });
});
