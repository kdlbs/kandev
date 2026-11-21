import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { AuthorExclusionStepsCard, authorExclusionStepKey } from "./author-exclusion-steps-card";
import type { AuthorExclusionStep } from "@/lib/types/author-exclusion";

const review: AuthorExclusionStep = {
  workflow_id: "wf-1",
  workflow_name: "Office Default",
  step_id: "step-review",
  step_name: "Review",
  role: "reviewer",
  enabled: false,
};
const approval: AuthorExclusionStep = {
  ...review,
  step_id: "step-approval",
  step_name: "Approval",
  role: "approver",
  enabled: true,
};

describe("AuthorExclusionStepsCard", () => {
  afterEach(cleanup);

  it("lists each seat step with its workflow, step, role, and switch state", () => {
    render(<AuthorExclusionStepsCard steps={[review, approval]} draft={{}} onChange={vi.fn()} />);
    expect(screen.getByText("Review on a different provider")).toBeTruthy();
    expect(screen.getByText("Office Default workflow, Review step")).toBeTruthy();
    expect(screen.getByText("Office Default workflow, Approval step")).toBeTruthy();
    expect(screen.getByText("Reviewer")).toBeTruthy();
    expect(screen.getByText("Approver")).toBeTruthy();
    const reviewSwitch = screen.getByRole("switch", {
      name: "Review on a different provider for Office Default workflow, Review step (Reviewer)",
    });
    expect(reviewSwitch.getAttribute("aria-checked")).toBe("false");
    const approvalSwitch = screen.getByRole("switch", {
      name: "Review on a different provider for Office Default workflow, Approval step (Approver)",
    });
    expect(approvalSwitch.getAttribute("aria-checked")).toBe("true");
  });

  it("shows the draft value and reports changes by step and role", () => {
    const onChange = vi.fn();
    const key = authorExclusionStepKey(review);
    render(
      <AuthorExclusionStepsCard steps={[review]} draft={{ [key]: true }} onChange={onChange} />,
    );
    const sw = screen.getByRole("switch");
    expect(sw.getAttribute("aria-checked")).toBe("true");
    fireEvent.click(sw);
    expect(onChange).toHaveBeenCalledWith(key, false);
  });

  it("says when no workflow has a reviewer or approver step", () => {
    render(<AuthorExclusionStepsCard steps={[]} draft={{}} onChange={vi.fn()} />);
    expect(
      screen.getByText("No workflow in this workspace has a reviewer or approver step."),
    ).toBeTruthy();
    expect(screen.queryByRole("switch")).toBeNull();
  });
});
