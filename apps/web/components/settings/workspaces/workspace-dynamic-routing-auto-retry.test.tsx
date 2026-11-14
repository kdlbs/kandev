import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  WorkspaceDynamicRoutingCard,
  type FailedRouteAutoRetryControls,
} from "./workspace-dynamic-routing-card";

afterEach(cleanup);

function controls(overrides: Partial<FailedRouteAutoRetryControls> = {}) {
  return {
    enabled: false,
    max: "1",
    maxValid: true,
    enabledIsDirty: false,
    maxIsDirty: false,
    onEnabledChange: vi.fn(),
    onMaxChange: vi.fn(),
    ...overrides,
  } satisfies FailedRouteAutoRetryControls;
}

function renderCard(autoRetry: FailedRouteAutoRetryControls, canManage = true) {
  render(
    <WorkspaceDynamicRoutingCard
      canManage={canManage}
      enabled={false}
      isDirty={false}
      onEnabledChange={vi.fn()}
      autoRetry={autoRetry}
    />,
  );
}

describe("WorkspaceDynamicRoutingCard failed-route retry", () => {
  it("renders the toggle off with a disabled limit and reports a change", () => {
    const autoRetry = controls();
    renderCard(autoRetry);
    const toggle = screen.getByTestId("workspace-failed-route-auto-retry-switch");
    const max = screen.getByTestId("workspace-failed-route-auto-retry-max") as HTMLInputElement;
    expect(toggle.getAttribute("aria-checked")).toBe("false");
    expect(max.value).toBe("1");
    expect(max.disabled).toBe(true);
    fireEvent.click(toggle);
    expect(autoRetry.onEnabledChange).toHaveBeenCalledWith(true);
  });

  it("edits the limit while enabled and flags an out-of-range value", () => {
    const autoRetry = controls({ enabled: true, max: "4", maxValid: false, maxIsDirty: true });
    renderCard(autoRetry);
    const max = screen.getByTestId("workspace-failed-route-auto-retry-max") as HTMLInputElement;
    expect(max.disabled).toBe(false);
    expect(max.getAttribute("aria-invalid")).toBe("true");
    expect(screen.getByTestId("workspace-failed-route-auto-retry-max-error")).toBeTruthy();
    fireEvent.change(max, { target: { value: "2" } });
    expect(autoRetry.onMaxChange).toHaveBeenCalledWith("2");
    expect(screen.getByTestId("workspace-dynamic-routing-card").textContent).toContain(
      "Enter a whole number from 1 to 3.",
    );
  });

  it("keeps both toggles independent and read-only for non-managers", () => {
    renderCard(controls({ enabled: true, enabledIsDirty: true }), false);
    const midTurn = screen.getByTestId("workspace-midturn-failover-switch") as HTMLButtonElement;
    const autoRetry = screen.getByTestId(
      "workspace-failed-route-auto-retry-switch",
    ) as HTMLButtonElement;
    expect(midTurn.getAttribute("aria-checked")).toBe("false");
    expect(autoRetry.getAttribute("aria-checked")).toBe("true");
    expect(autoRetry.getAttribute("data-settings-dirty")).toBe("true");
    expect(autoRetry.disabled).toBe(true);
    expect(midTurn.disabled).toBe(true);
  });
});
