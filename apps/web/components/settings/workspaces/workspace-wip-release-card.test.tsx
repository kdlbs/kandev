import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WorkspaceWipReleaseCard } from "./workspace-wip-release-card";

afterEach(cleanup);

describe("WorkspaceWipReleaseCard", () => {
  it("is off by default and enables the grace input with the policy", () => {
    const onEnabledChange = vi.fn();
    const onGraceChange = vi.fn();
    const props = {
      canManage: true,
      enabled: false,
      graceMinutes: "10",
      graceValid: true,
      enabledIsDirty: false,
      graceIsDirty: false,
      onEnabledChange,
      onGraceChange,
    };

    const { rerender } = render(<WorkspaceWipReleaseCard {...props} />);
    const grace = screen.getByTestId("workspace-wip-release-grace-input") as HTMLInputElement;
    expect(grace.value).toBe("10");
    expect(grace.disabled).toBe(true);
    fireEvent.click(screen.getByTestId("workspace-wip-release-switch"));
    expect(onEnabledChange).toHaveBeenCalledWith(true);

    rerender(<WorkspaceWipReleaseCard {...props} enabled />);
    expect(grace.disabled).toBe(false);
    fireEvent.change(grace, { target: { value: "20" } });
    expect(onGraceChange).toHaveBeenCalledWith("20");
  });

  it("shows an inline error for an invalid grace and blocks non-managers", () => {
    render(
      <WorkspaceWipReleaseCard
        canManage={false}
        enabled
        graceMinutes="0"
        graceValid={false}
        enabledIsDirty
        graceIsDirty
        onEnabledChange={vi.fn()}
        onGraceChange={vi.fn()}
      />,
    );

    expect(screen.getByTestId("workspace-wip-release-grace-error").getAttribute("role")).toBe(
      "alert",
    );
    expect((screen.getByTestId("workspace-wip-release-switch") as HTMLButtonElement).disabled).toBe(
      true,
    );
  });
});
