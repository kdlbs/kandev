import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it } from "vitest";

import { ExecutionIndicator } from "./execution-indicator";

afterEach(cleanup);

function renderIndicator(status: string) {
  return render(
    <TooltipProvider>
      <ExecutionIndicator status={status} />
    </TooltipProvider>,
  );
}

describe("ExecutionIndicator", () => {
  it("pulses a static icon through an HTML compositor target while live", () => {
    const { container } = renderIndicator("IN_PROGRESS");

    const pulse = container.querySelector<HTMLElement>("[data-compositor-pulse]");
    expect(pulse).toBeTruthy();
    expect(pulse?.className).toContain("animate-pulse");
    expect(pulse?.querySelector("svg")?.getAttribute("class")).not.toContain("animate-pulse");
    expect(screen.getByText("Live")).toBeTruthy();
  });

  it("keeps the review-ready state static", () => {
    const { container } = renderIndicator("REVIEW");

    expect(container.querySelector("[data-compositor-pulse]")).toBeNull();
    expect(screen.getByText("Ready")).toBeTruthy();
  });
});
