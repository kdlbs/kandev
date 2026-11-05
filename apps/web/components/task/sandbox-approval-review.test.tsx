import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { Dialog } from "@kandev/ui/dialog";
import { SandboxApprovalReview } from "./sandbox-approval-review";

const mocks = vi.hoisted(() => ({
  model: {
    approvals: [
      {
        request_id: "request",
        attempt_id: "attempt",
        session_id: "session",
        version: 3,
        state: "pending",
        access: "read",
        target: "/work/file",
        policy_digest: "digest",
        expires_at: "2099-01-01T00:00:00Z",
        can_decide: true,
      },
    ],
    isPending: false,
    isError: false,
    isBusy: false,
    isAvailable: true,
    decisionFailed: false,
    failedRequestId: undefined,
    canDecide: vi.fn(),
    decide: vi.fn(),
  },
}));
vi.mock("@/hooks/domains/session/use-sandbox-approvals", () => ({
  useSandboxApprovals: () => mocks.model,
}));
beforeEach(() => {
  mocks.model.canDecide.mockReturnValue(true);
  mocks.model.decide.mockReset();
});
afterEach(cleanup);
function view() {
  return (
    <Dialog open>
      <SandboxApprovalReview sessionId="session" requestId="request" />
    </Dialog>
  );
}
it("requires an explicit one-operation decision and sends the displayed record", () => {
  render(view());
  expect(screen.getByRole("dialog")).toBeTruthy();
  expect(screen.getByText("/work/file")).toBeTruthy();
  expect(mocks.model.decide).not.toHaveBeenCalled();
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Enter" });
  expect(mocks.model.decide).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Allow once" }));
  expect(mocks.model.decide).toHaveBeenCalledWith(mocks.model.approvals[0], "allow_once");
  fireEvent.click(screen.getByRole("button", { name: /^Deny$/ }));
  expect(mocks.model.decide).toHaveBeenLastCalledWith(mocks.model.approvals[0], "deny");
});
it("disables decisions when the live model removes authority", () => {
  const { rerender } = render(view());
  mocks.model.canDecide.mockReturnValue(false);
  rerender(view());
  expect(screen.getByRole("button", { name: "Allow once" }).hasAttribute("disabled")).toBe(true);
  expect(screen.getByRole("button", { name: /^Deny$/ }).hasAttribute("disabled")).toBe(true);
});
