import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WorkspaceDynamicRoutingCard } from "./workspace-dynamic-routing-card";

afterEach(cleanup);

describe("WorkspaceDynamicRoutingCard", () => {
  it("renders the mid-turn failover toggle off and reports a change", () => {
    const onEnabledChange = vi.fn();
    render(
      <WorkspaceDynamicRoutingCard
        canManage
        enabled={false}
        isDirty={false}
        onEnabledChange={onEnabledChange}
      />,
    );
    const toggle = screen.getByTestId("workspace-midturn-failover-switch");
    expect(toggle.getAttribute("aria-checked")).toBe("false");
    fireEvent.click(toggle);
    expect(onEnabledChange).toHaveBeenCalledWith(true);
  });

  it("marks a changed toggle dirty and blocks editing for non-managers", () => {
    render(
      <WorkspaceDynamicRoutingCard canManage={false} enabled isDirty onEnabledChange={vi.fn()} />,
    );
    const toggle = screen.getByTestId("workspace-midturn-failover-switch") as HTMLButtonElement;
    expect(toggle.getAttribute("aria-checked")).toBe("true");
    expect(toggle.getAttribute("data-settings-dirty")).toBe("true");
    expect(toggle.disabled).toBe(true);
  });
});
