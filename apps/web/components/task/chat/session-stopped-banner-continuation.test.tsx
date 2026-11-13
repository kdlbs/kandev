import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { SessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";

const SESSION_ID = "session-1";
const CONTINUATION_BUTTON_ID = "recovery-continue-from-history-button";
const CONTINUATION_DISCLOSURE =
  "Saved history starts a new native conversation. The interrupted prompt outcome remains unknown.";
const CONTINUATION_PENDING = "Continuing from saved history...";

const mocks = vi.hoisted(() => ({
  agentProfiles: [{ id: "profile-1" }],
  stop: vi.fn().mockResolvedValue(true),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      agentProfiles: { items: mocks.agentProfiles },
      taskSessions: { items: { [SESSION_ID]: { agent_profile_id: "profile-1" } } },
    }),
}));

vi.mock("@/hooks/domains/session/use-session-actions", () => ({
  useSessionActions: () => ({ stop: mocks.stop }),
}));

vi.mock("@/components/task/new-session-dialog", () => ({
  NewSessionDialog: () => null,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) =>
      ({
        "task:durableDeliveryUncertain": "Delivery is uncertain.",
        "task:continueFromHistory": "Continue from saved history",
        "task:continueFromHistoryDisclosure": CONTINUATION_DISCLOSURE,
        "task:continuingFromHistory": CONTINUATION_PENDING,
        "task:resume": "Resume",
        "task:retryConnection": "Retry connection",
        "task:stop": "Stop",
        "task:stopping": "Stopping...",
        "task:failedToResumeSession": "Resume failed",
        "task:recoveryMoreOptions": "More options",
        "task:deliveryStopOutcomeUnconfirmed": "Stop outcome is unknown.",
        "task:deliveryStopFailed": "Stop failed.",
        "task:agentProfileNoLongerExists": "Agent profile is unavailable.",
      })[key] ?? key,
  }),
}));

import { SessionStoppedBanner } from "./session-stopped-banner";

beforeEach(() => {
  mocks.agentProfiles.splice(0, mocks.agentProfiles.length, { id: "profile-1" });
  mocks.stop.mockClear().mockResolvedValue(true);
});

afterEach(cleanup);

it("retains continuation guidance when delivery is uncertain", () => {
  const actions = continuationRecoveryActions();
  renderUncertainBanner(actions);

  const continuation = screen.getByTestId(CONTINUATION_BUTTON_ID);
  const resume = screen.getByTestId("recovery-resume-button");
  expect(screen.getByTestId("provider-restored-resume-disclosure").textContent).toBe(
    CONTINUATION_DISCLOSURE,
  );
  expect(
    continuation.compareDocumentPosition(resume) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).toBeTruthy();
  expect(actions.handleContinueFromHistory).not.toHaveBeenCalled();
  expect(screen.queryByTestId("recovery-fresh-button")).toBeNull();
  expect(screen.queryByTestId("recovery-restore-workspace-button")).toBeNull();
  expect(screen.queryByTestId("managed-clone-relocate-button")).toBeNull();

  fireEvent.click(continuation);
  expect(actions.handleContinueFromHistory).toHaveBeenCalledTimes(1);
});

it.each(["missing profile", "non-retryable guard", "workspace relocation", "inspection busy"])(
  "keeps continuation unavailable when %s blocks recovery",
  (restriction) => {
    const actions = continuationRecoveryActions();
    if (restriction === "missing profile") {
      mocks.agentProfiles.splice(0, mocks.agentProfiles.length);
    } else if (restriction === "non-retryable guard") {
      actions.guardDetails = { kind: "session_recovery_unstoppable", retryable: false };
    } else if (restriction === "workspace relocation") {
      actions.managedCloneRecoveryStamp = "relocation-1";
    } else {
      actions.recoveryNoticeKind = "inspection_busy";
      actions.deliveryResumeAvailable = false;
    }

    renderUncertainBanner(actions);

    if (restriction === "missing profile") {
      expect(screen.getByTestId(CONTINUATION_BUTTON_ID)).toHaveProperty("disabled", true);
    } else {
      expect(screen.queryByTestId(CONTINUATION_BUTTON_ID)).toBeNull();
    }
    expect(screen.getByTestId("recovery-stop-button")).toBeTruthy();
  },
);

it("shows pending and failed continuation feedback while retaining the explicit action", () => {
  const pending = continuationRecoveryActions();
  pending.busyAction = "continue_from_history";
  const { rerender } = renderUncertainBanner(pending);
  expect(screen.getByTestId(CONTINUATION_BUTTON_ID)).toHaveProperty("disabled", true);
  expect(screen.getByRole("status").textContent).toContain(CONTINUATION_PENDING);

  const failed = continuationRecoveryActions();
  failed.recoveryError = new Error("native continuation bootstrap failed");
  failed.lastFailedAction = "continue_from_history";
  rerender(renderBannerTree(failed));
  expect(screen.getByTestId("session-recovery-error").textContent).toBe("Resume failed");
  expect(screen.getByTestId(CONTINUATION_BUTTON_ID)).toBeTruthy();
});

function renderUncertainBanner(actions: SessionRecoveryActions) {
  return render(renderBannerTree(actions));
}

function renderBannerTree(actions: SessionRecoveryActions) {
  return (
    <TooltipProvider>
      <SessionStoppedBanner
        mode="recoverable"
        showDialog={false}
        onShowDialog={() => undefined}
        taskId="task-1"
        sessionId={SESSION_ID}
        uncertainDelivery
        recoveryActions={actions}
      />
    </TooltipProvider>
  );
}

function continuationRecoveryActions(): SessionRecoveryActions {
  return {
    busyAction: null,
    recoveryError: null,
    branchDetails: null,
    guardDetails: null,
    managedCloneRecoveryStamp: null,
    continuationDetails: {
      kind: "session_restore_required",
      recovery_action: "continue_from_history",
      reason: "unresolved_durable_work",
      session_id: SESSION_ID,
      generation: 2,
    },
    manualRecoveryFailure: null,
    recoveryNotice: null,
    recoveryNoticeKind: null,
    deliveryRecoveryNotice: null,
    deliveryHistoryGap: undefined,
    deliveryResumeAvailable: true,
    workspaceRecovery: null,
    workspaceRecoveryMatchesCurrentFailure: false,
    workspaceRecoveryRepositoryName: null,
    workspaceRecoveryStatusCheck: "idle",
    providerRestoredResumeEligible: false,
    lastFailedAction: null,
    checkWorkspaceRecoveryStatus: vi.fn(),
    handleRecover: vi.fn().mockResolvedValue(false),
    handleRestore: vi.fn().mockResolvedValue(undefined),
    handleRetry: vi.fn().mockResolvedValue(false),
    handleNewBranch: vi.fn().mockResolvedValue(false),
    handleManagedCloneRelocation: vi.fn().mockResolvedValue(false),
    handleContinueFromHistory: vi.fn().mockResolvedValue(true),
    clearInspectionContentionNotice: vi.fn(),
  };
}
