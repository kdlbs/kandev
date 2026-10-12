import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WorkspaceIdlePolicyCard } from "./workspace-idle-policy-card";

afterEach(cleanup);

describe("WorkspaceIdlePolicyCard", () => {
  it("keeps the saved timeout visible while disabled and enables editing with the policy", () => {
    const onEnabledChange = vi.fn();
    const onTimeoutChange = vi.fn();
    const props = {
      canManage: true,
      enabled: false,
      timeoutMinutes: "120",
      timeoutValid: true,
      enabledIsDirty: false,
      timeoutIsDirty: false,
      onEnabledChange,
      onTimeoutChange,
    };

    const { rerender } = render(<WorkspaceIdlePolicyCard {...props} />);
    const timeout = screen.getByTestId("workspace-idle-timeout-input");
    expect((timeout as HTMLInputElement).value).toBe("120");
    expect((timeout as HTMLInputElement).disabled).toBe(true);
    fireEvent.click(screen.getByTestId("workspace-idle-suspension-switch"));
    expect(onEnabledChange).toHaveBeenCalledWith(true);

    rerender(<WorkspaceIdlePolicyCard {...props} enabled />);
    expect((timeout as HTMLInputElement).disabled).toBe(false);
    fireEvent.change(timeout, { target: { value: "45" } });
    expect(onTimeoutChange).toHaveBeenCalledWith("45");
  });

  it("shows an inline validation error and blocks editing for non-managers", () => {
    render(
      <WorkspaceIdlePolicyCard
        canManage={false}
        enabled
        timeoutMinutes="0"
        timeoutValid={false}
        enabledIsDirty
        timeoutIsDirty
        onEnabledChange={vi.fn()}
        onTimeoutChange={vi.fn()}
      />,
    );

    expect(screen.getByTestId("workspace-idle-timeout-error").getAttribute("role")).toBe("alert");
    expect(screen.getByTestId("workspace-idle-timeout-input").getAttribute("aria-invalid")).toBe(
      "true",
    );
    expect(
      (screen.getByTestId("workspace-idle-suspension-switch") as HTMLButtonElement).disabled,
    ).toBe(true);
    expect((screen.getByTestId("workspace-idle-timeout-input") as HTMLInputElement).disabled).toBe(
      true,
    );
  });
});
