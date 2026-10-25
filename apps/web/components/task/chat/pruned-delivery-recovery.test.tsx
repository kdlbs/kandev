import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { SessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      agentProfiles: { items: [{ id: "profile" }] },
      taskSessions: { items: { session: { agent_profile_id: "profile" } } },
    }),
}));
vi.mock("@/components/task/new-session-dialog", () => ({ NewSessionDialog: () => null }));
vi.mock("@/hooks/domains/session/use-session-actions", () => ({
  useSessionActions: () => ({ stop: vi.fn() }),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
import { SessionStoppedBanner } from "./session-stopped-banner";
afterEach(cleanup);
describe("SessionStoppedBanner pruned delivery gap", () => {
  // @covers AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.7
  it("offers same-conversation Resume for a verified pruned gap, with Stop still available", () => {
    const handleRecover = vi.fn();
    const actions = {
      busyAction: null,
      guardDetails: null,
      recoveryError: null,
      handleRecover,
      deliveryHistoryGap: {
        firstSequence: 5495,
        lastSequence: 5499,
        receivedSequence: 5494,
        projectedSequence: 5494,
      },
    } as unknown as SessionRecoveryActions;
    render(
      <TooltipProvider>
        <SessionStoppedBanner
          mode="recoverable"
          taskId="task"
          sessionId="session"
          showDialog={false}
          onShowDialog={vi.fn()}
          uncertainDelivery
          recoveryActions={actions}
        />
      </TooltipProvider>,
    );
    expect(screen.getByText("task:deliveryHistoryGapTitle")).toBeTruthy();
    expect(screen.queryByTestId("recovery-fresh-button")).toBeNull();
    expect(screen.getByTestId("recovery-stop-button")).toBeTruthy();
    fireEvent.click(screen.getByTestId("recovery-resume-button"));
    expect(handleRecover).toHaveBeenCalledWith("resume");
    expect(screen.queryByTestId("recovery-retry-connection-button")).toBeNull();
  });
});
