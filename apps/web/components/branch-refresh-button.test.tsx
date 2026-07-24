import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { BranchRefreshButton } from "./branch-refresh-button";

function renderRefreshButton() {
  const onRefresh = vi.fn();
  render(
    <TooltipProvider delayDuration={0}>
      <BranchRefreshButton
        onRefresh={onRefresh}
        label="repositories"
        testId="repo-refresh-button"
        touchTarget
      />
    </TooltipProvider>,
  );
  return onRefresh;
}

afterEach(cleanup);

describe("BranchRefreshButton", () => {
  it("renders a repository refresh target with the supplied test id and touch target", () => {
    renderRefreshButton();

    const button = screen.getByTestId("repo-refresh-button");
    expect(button.getAttribute("aria-label")).toBe("Refresh repositories");
    expect(button.className).toContain("h-12");
    expect(button.className).toContain("w-12");
  });

  it("refreshes on click", () => {
    const onRefresh = renderRefreshButton();
    const button = screen.getByTestId("repo-refresh-button");

    fireEvent.click(button);
    expect(onRefresh).toHaveBeenCalledOnce();
  });
});
