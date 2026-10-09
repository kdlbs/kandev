import { afterEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { PermissionActionRow } from "./permission-action-row";

const allowAlwaysTestId = "permission-allow-always";
const allowAlwaysLabel = "Always allow";
const permissionApproveTestId = "permission-approve";
const permissionRejectTestId = "permission-reject";

describe("PermissionActionRow", () => {
  // This vitest config does not auto-clean the DOM between tests, so unmount
  // explicitly to avoid stale buttons leaking duplicate test ids.
  afterEach(cleanup);

  it("renders only Approve and Deny when onAllowAlways is not provided", () => {
    render(<PermissionActionRow onApprove={vi.fn()} onReject={vi.fn()} />);

    expect(screen.getByTestId(permissionApproveTestId)).toBeTruthy();
    expect(screen.getByTestId(permissionRejectTestId)).toBeTruthy();
    // The session-wide action is hidden for agents that don't offer it.
    expect(screen.queryByTestId(allowAlwaysTestId)).toBeNull();
  });

  it(`renders the ${allowAlwaysLabel} button and fires its callback when offered`, () => {
    const onAllowAlways = vi.fn();
    render(
      <PermissionActionRow onApprove={vi.fn()} onReject={vi.fn()} onAllowAlways={onAllowAlways} />,
    );

    const button = screen.getByTestId(allowAlwaysTestId);
    expect(button.textContent).toContain(allowAlwaysLabel);

    fireEvent.click(button);
    expect(onAllowAlways).toHaveBeenCalledOnce();
  });

  it("wires Approve and Deny to their handlers", () => {
    const onApprove = vi.fn();
    const onReject = vi.fn();
    render(
      <PermissionActionRow onApprove={onApprove} onReject={onReject} onAllowAlways={vi.fn()} />,
    );

    fireEvent.click(screen.getByTestId(permissionApproveTestId));
    fireEvent.click(screen.getByTestId(permissionRejectTestId));
    expect(onApprove).toHaveBeenCalledOnce();
    expect(onReject).toHaveBeenCalledOnce();
  });

  it("renders each provider-offered choice and returns its option ID", () => {
    const onChooseOfferedChoice = vi.fn();
    render(
      <PermissionActionRow
        onApprove={vi.fn()}
        onReject={vi.fn()}
        offeredChoices={[
          { option_id: "decision-0", label: "Approve", kind: "allow_once" },
          { option_id: "decision-1", label: "Approve with command rule", kind: "allow_always" },
          { option_id: "decision-2", label: "Deny", kind: "reject_once" },
        ]}
        onChooseOfferedChoice={onChooseOfferedChoice}
      />,
    );

    expect(screen.getByRole("button", { name: "Approve" })).toBeTruthy();
    const policyChoice = screen.getByRole("button", { name: "Approve with command rule" });
    expect(screen.getByRole("button", { name: "Deny" })).toBeTruthy();
    fireEvent.click(policyChoice);
    expect(onChooseOfferedChoice).toHaveBeenCalledOnce();
    expect(onChooseOfferedChoice).toHaveBeenCalledWith("decision-1");
    expect(screen.queryByTestId(permissionApproveTestId)).toBeNull();
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

    expect(screen.getByTestId<HTMLButtonElement>(permissionApproveTestId).disabled).toBe(true);
    expect(screen.getByTestId<HTMLButtonElement>(permissionRejectTestId).disabled).toBe(true);
    expect(screen.getByTestId<HTMLButtonElement>(allowAlwaysTestId).disabled).toBe(true);
  });
});
