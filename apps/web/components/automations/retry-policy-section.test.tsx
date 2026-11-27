import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { RetryPolicy } from "@/lib/types/automation";
import { RetryPolicySection } from "./retry-policy-section";

afterEach(cleanup);

function policy(overrides: Partial<RetryPolicy> = {}): RetryPolicy {
  return {
    mode: "disabled",
    max_retries: "0",
    delay_seconds: "0",
    backoff: "fixed",
    history_mode: "attempts",
    ...overrides,
  };
}

function renderSection(current: RetryPolicy) {
  const updateField = vi.fn();
  render(
    <TooltipProvider>
      <RetryPolicySection policy={current} savedPolicy={current} updateField={updateField} />
    </TooltipProvider>,
  );
  return updateField;
}

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-002.6, AC-OFFICE-AUTOMATION-RETRIES-001.8
describe("RetryPolicySection", () => {
  it("offers the retry modes as one labelled choice", () => {
    renderSection(policy());
    expect(screen.getByRole("radiogroup", { name: "Automatic retries" })).toBeTruthy();
    expect(screen.getByRole("radio", { name: "Don't retry" }).getAttribute("aria-checked")).toBe(
      "true",
    );
    expect(screen.getByText("Record the failure and stop after the first attempt.")).toBeTruthy();
  });

  it("hides attempt and history settings until retries are on", () => {
    renderSection(policy());
    expect(screen.queryByLabelText("Additional attempts")).toBeNull();
    expect(screen.queryByText("History view")).toBeNull();
    cleanup();

    renderSection(policy({ mode: "finite", max_retries: "2" }));
    expect(screen.getByLabelText("Additional attempts")).toBeTruthy();
    expect(screen.getByText("History view")).toBeTruthy();
  });

  it("switches modes through the retry policy rules", () => {
    const updateField = renderSection(policy());
    fireEvent.click(screen.getByRole("radio", { name: "Fixed number of times" }));
    expect(updateField).toHaveBeenCalledWith(
      "retryPolicy",
      expect.objectContaining({ mode: "finite" }),
    );
  });
});
