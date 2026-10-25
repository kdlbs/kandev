import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { SessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";
const sessionFixture = vi.hoisted(() => ({ archivedAt: null as string | null }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      agentProfiles: { items: [{ id: "profile" }] },
      taskSessions: {
        items: { session: { agent_profile_id: "profile", archived_at: sessionFixture.archivedAt } },
      },
    }),
}));
vi.mock("@/components/task/new-session-dialog", () => ({ NewSessionDialog: () => null }));
vi.mock("@/hooks/domains/session/use-session-actions", () => ({
  useSessionActions: () => ({ stop: vi.fn() }),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
import { SessionStoppedBanner } from "./session-stopped-banner";
afterEach(cleanup);
beforeEach(() => {
  sessionFixture.archivedAt = null;
});
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

  it("keeps archived conversation history read-only while allowing a task-scoped new agent", () => {
    sessionFixture.archivedAt = "2026-10-25T04:00:00.000Z";
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
    const onShowDialog = vi.fn();
    render(
      <TooltipProvider>
        <SessionStoppedBanner
          mode="recoverable"
          taskId="task"
          sessionId="session"
          showDialog={false}
          onShowDialog={onShowDialog}
          uncertainDelivery
          recoveryActions={actions}
        />
      </TooltipProvider>,
    );

    expect(screen.queryByTestId("recovery-resume-button")).toBeNull();
    expect(screen.queryByTestId("recovery-stop-button")).toBeNull();
    fireEvent.click(screen.getByTestId("archived-session-new-agent-button"));
    expect(onShowDialog).toHaveBeenCalledWith(true);
    expect(handleRecover).not.toHaveBeenCalled();
  });
});
