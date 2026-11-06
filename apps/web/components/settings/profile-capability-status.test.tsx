import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { CapabilityStatusMessage, RefreshCapabilitiesButton } from "./profile-capability-status";

const statusTestId = "profile-capability-status";

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
    expect(screen.getByTestId(statusTestId).textContent).toBe(
      "Models match the current launch settings.",
    );

    rerender(<CapabilityStatusMessage status="stale" />);
    expect(screen.getByTestId(statusTestId).textContent).toBe(
      "Launch settings changed. Refresh models.",
    );
  });

  it("uses policy-specific recovery and keeps unknown reasons on the generic path", () => {
    const { rerender } = render(
      <CapabilityStatusMessage status="failed" reasonCode="policy_unverified" />,
    );
    expect(screen.getByTestId(statusTestId).textContent).toBe(
      "Model discovery is blocked by this plugin selection. Uncheck plugins, Save, then refresh models.",
    );

    rerender(<CapabilityStatusMessage status="failed" reasonCode="native_output" />);
    expect(screen.getByTestId(statusTestId).textContent).toBe(
      "Probe failed. Check agent logs for details.",
    );
  });
});
