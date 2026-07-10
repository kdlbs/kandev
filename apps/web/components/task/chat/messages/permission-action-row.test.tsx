import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { PermissionActionRow } from "./permission-action-row";

describe("PermissionActionRow", () => {
  // This vitest config does not auto-clean the DOM between tests, so unmount
  // explicitly to avoid stale buttons leaking duplicate test ids.
  afterEach(cleanup);

  it("renders only Approve and Deny when onAllowAlways is not provided", () => {
    render(<PermissionActionRow onApprove={vi.fn()} onReject={vi.fn()} />);

    expect(screen.getByTestId("permission-approve")).toBeTruthy();
    expect(screen.getByTestId("permission-reject")).toBeTruthy();
    // The "Always allow" button is hidden for agents that don't offer it.
    expect(screen.queryByTestId("permission-allow-always")).toBeNull();
  });

  it("renders the Always allow button and fires its callback when offered", () => {
    const onAllowAlways = vi.fn();
    render(
      <PermissionActionRow onApprove={vi.fn()} onReject={vi.fn()} onAllowAlways={onAllowAlways} />,
    );

    const button = screen.getByTestId("permission-allow-always");
    expect(button.textContent).toContain("Always allow");

    fireEvent.click(button);
    expect(onAllowAlways).toHaveBeenCalledOnce();
  });

  it("wires Approve and Deny to their handlers", () => {
    const onApprove = vi.fn();
    const onReject = vi.fn();
    render(
      <PermissionActionRow onApprove={onApprove} onReject={onReject} onAllowAlways={vi.fn()} />,
    );

    fireEvent.click(screen.getByTestId("permission-approve"));
    fireEvent.click(screen.getByTestId("permission-reject"));
    expect(onApprove).toHaveBeenCalledOnce();
    expect(onReject).toHaveBeenCalledOnce();
  });

  it("disables every action while a response is in flight", () => {
    render(
      <PermissionActionRow
        onApprove={vi.fn()}
        onReject={vi.fn()}
        onAllowAlways={vi.fn()}
        isResponding
      />,
    );

    expect(screen.getByTestId<HTMLButtonElement>("permission-approve").disabled).toBe(true);
    expect(screen.getByTestId<HTMLButtonElement>("permission-reject").disabled).toBe(true);
    expect(screen.getByTestId<HTMLButtonElement>("permission-allow-always").disabled).toBe(true);
  });
});
