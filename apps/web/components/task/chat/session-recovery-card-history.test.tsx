import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { SessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";
import { SessionRecoveryCard } from "./session-recovery-card";

afterEach(cleanup);
const HISTORY_BUTTON = "recovery-continue-from-history-button";

function recoveryView(actions: SessionRecoveryActions) {
  return (
    <StateProvider
      initialState={
        {
          taskSessions: { items: { session: { id: "session", agent_profile_id: "profile" } } },
          agentProfiles: { items: [{ id: "profile" }] },
        } as unknown as Partial<AppState>
      }
    >
      <SessionRecoveryCard
        model={{ sessionId: "session", stamp: "failure", kind: "generic" }}
        actions={actions}
        onNewSession={vi.fn()}
      />
    </StateProvider>
  );
}

// @covers AC-AGENTS-HARNESS-SESSION-CONTINUITY-006.2
describe("history continuation in the composer recovery card", () => {
  it.each(["unresolved_durable_work", "native_state_missing"])(
    "offers explicit continuation after Resume reports %s",
    (reason) => {
      const continueFromHistory = vi.fn().mockResolvedValue(true);
      const actions = {
        busyAction: null,
        recoveryError: null,
        guardDetails: null,
        branchDetails: null,
        continuationDetails: null,
        handleRecover: vi.fn(),
        handleContinueFromHistory: continueFromHistory,
      } as unknown as SessionRecoveryActions;
      const { rerender } = render(recoveryView(actions));
      expect(screen.queryByTestId(HISTORY_BUTTON)).toBeNull();
      rerender(
        recoveryView({
          ...actions,
          recoveryError: new Error("Session continuity requires explicit history continuation."),
          continuationDetails: {
            kind: "session_restore_required",
            recovery_action: "continue_from_history",
            session_id: "session",
            reason,
          },
        }),
      );
      const button = screen.getByTestId(HISTORY_BUTTON);
      expect(button.textContent).toBe("Continue from saved history");
      expect(continueFromHistory).not.toHaveBeenCalled();
      fireEvent.click(button);
      expect(continueFromHistory).toHaveBeenCalledOnce();
    },
  );

  it("keeps managed workspace relocation ahead of history continuation", () => {
    const continueFromHistory = vi.fn();
    render(
      recoveryView({
        busyAction: null,
        guardDetails: null,
        managedCloneRecoveryStamp: "failure",
        continuationDetails: {
          kind: "session_restore_required",
          recovery_action: "continue_from_history",
          session_id: "session",
        },
        handleContinueFromHistory: continueFromHistory,
      } as unknown as SessionRecoveryActions),
    );
    expect(screen.getByTestId("managed-clone-relocate-button")).toBeTruthy();
    expect(screen.queryByTestId(HISTORY_BUTTON)).toBeNull();
    expect(continueFromHistory).not.toHaveBeenCalled();
  });
});
