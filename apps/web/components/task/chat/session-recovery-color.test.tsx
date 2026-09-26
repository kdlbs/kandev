import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { SessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";
import { SessionRecoveryCard } from "./session-recovery-card";

const context = vi.hoisted(() => ({ state: "checking", sessionId: "session" }));
vi.mock("../task-launch-error-context", () => ({
  useTaskLaunchErrorContext: () => ({
    statusSummary: { active_error: { session_id: context.sessionId } },
    automaticRecovery: { resumptionState: context.state },
  }),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
afterEach(cleanup);

function showCard(busyAction: SessionRecoveryActions["busyAction"] = null) {
  render(
    <StateProvider>
      <SessionRecoveryCard
        model={{ sessionId: "session", kind: "generic", summary: "Connection lost" }}
        actions={
          {
            busyAction,
            recoveryError: null,
            guardDetails: null,
            branchDetails: null,
            recoveryNotice: null,
            handleRecover: vi.fn(),
          } as unknown as SessionRecoveryActions
        }
        onNewSession={vi.fn()}
      />
    </StateProvider>,
  );
  return screen.getByTestId("session-recovery-card");
}

it.each([
  ["checking", false],
  ["error", false],
  ["resuming", true],
] as const)("distinguishes automatic %s from an active recovery attempt", (state, recovering) => {
  context.state = state;
  context.sessionId = "session";
  expect(showCard().getAttribute("data-recovering")).toBe(String(recovering));
  expect(screen.getByTestId("recovery-resume-button").getAttribute("aria-label")).toBe(
    "Resume session",
  );
  if (state === "resuming")
    expect(document.querySelector('[role="status"]')?.textContent).toBe("Resuming...");
  if (state === "checking") {
    expect(document.querySelector('[role="status"]')?.textContent).toBe("Checking session...");
    expect(screen.getByTestId("recovery-fresh-button")).toHaveProperty("disabled", true);
    expect(screen.getByTestId("recovery-resume-button").textContent).toBe("Resume session");
  }
});
it("shows manual recovery progress while the automatic status check is pending", () => {
  context.state = "checking";
  context.sessionId = "session";
  expect(showCard("restore").getAttribute("data-recovering")).toBe("true");
});
it("does not borrow progress from another session", () => {
  context.state = "resuming";
  context.sessionId = "other-session";
  expect(showCard().getAttribute("data-recovering")).toBe("false");
});
