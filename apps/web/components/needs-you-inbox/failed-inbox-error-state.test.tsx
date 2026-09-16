import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { FailedInboxErrorState } from "./failed-inbox-error-state";

afterEach(() => cleanup());

describe("FailedInboxErrorState", () => {
  it("renders an alert distinct from the empty state and does not claim nothing failed (AC .22)", () => {
    render(<FailedInboxErrorState />);
    const alert = screen.getByTestId("failed-inbox-error");
    expect(alert.getAttribute("role")).toBe("alert");
    expect(alert.textContent).not.toContain("Nothing has failed");
  });

  // design-01#Failure-and-recovery: recovery needs no operator action, so the
  // error state carries no retry affordance -- the periodic and foreground
  // triggers already re-read within the same bound the rows are.
  it("renders no retry control", () => {
    render(<FailedInboxErrorState />);
    expect(screen.queryByRole("button")).toBeNull();
  });
});
