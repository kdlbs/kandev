import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { SessionRecoveryFeedback } from "./ensure-session-error";

afterEach(cleanup);
const feedback = {
  error: "Session recovery failed",
  notice: null,
  onRetry: vi.fn(),
  recoveryFailure: {
    outcome: "recovery_failed" as const,
    resumeError: "Resume transport failed",
    restoreError: "Restore failed",
  },
};
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.1
it("does not render a second recovery surface when Chat owns automatic feedback", () => {
  render(<SessionRecoveryFeedback {...feedback} ownedByChat />);
  expect(screen.queryByTestId("session-recovery-error")).toBeNull();
});
it("keeps the fallback when no composer owns recovery", () => {
  render(<SessionRecoveryFeedback {...feedback} ownedByChat={false} />);
  expect(screen.getByTestId("session-recovery-error")).toBeTruthy();
});
it("keeps status checking separate from a stopped agent", () => {
  render(
    <SessionRecoveryFeedback
      {...feedback}
      error={null}
      ownedByChat
      recoveryFailure={{ outcome: "status_unavailable", kind: "request", statusError: "Offline" }}
    />,
  );
  expect(screen.getByTestId("session-status-unavailable")).toBeTruthy();
});
