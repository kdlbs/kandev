import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { CapabilityStatusMessage, RefreshCapabilitiesButton } from "./profile-capability-status";

afterEach(cleanup);

describe("RefreshCapabilitiesButton", () => {
  it("renders compact accessible icon button and invokes onRefresh on click", () => {
    const onRefresh = vi.fn();
    render(
      <TooltipProvider>
        <RefreshCapabilitiesButton onRefresh={onRefresh} isLoading={false} error={null} />
      </TooltipProvider>,
    );

    const button = screen.getByTestId("profile-refresh-capabilities");
    expect(button.getAttribute("aria-label")).toBe("Refresh models");
    expect(button).not.toHaveProperty("disabled", true);

    fireEvent.click(button);
    expect(onRefresh).toHaveBeenCalledTimes(1);
  });

  it("disables button and shows spinner while loading", () => {
    render(
      <TooltipProvider>
        <RefreshCapabilitiesButton onRefresh={vi.fn()} isLoading={true} error={null} />
      </TooltipProvider>,
    );

    const button = screen.getByTestId("profile-refresh-capabilities");
    expect(button).toHaveProperty("disabled", true);
  });
});

describe("CapabilityStatusMessage", () => {
  it("renders status messages for known statuses", () => {
    const { rerender } = render(<CapabilityStatusMessage status="ready" />);
    expect(screen.getByTestId("profile-capability-status").textContent).toBe(
      "Models match the current launch settings.",
    );

    rerender(<CapabilityStatusMessage status="stale" />);
    expect(screen.getByTestId("profile-capability-status").textContent).toBe(
      "Launch settings changed. Refresh models.",
    );
  });
});
