import { fireEvent, render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";
import type { SessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";
import { SessionRecoveryCard } from "./session-recovery-card";

vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));

const executionId = "650e8400-e29b-41d4-a716-446655440000";
const actions = {
  busyAction: null,
  recoveryError: null,
  guardDetails: null,
  branchDetails: null,
  recoveryNotice: null,
  managedCloneRecoveryStamp: null,
  handleRecover: vi.fn(),
  handleManagedCloneRelocation: vi.fn().mockResolvedValue(true),
} as unknown as SessionRecoveryActions;
const session = {
  id: "session",
  task_id: "task",
  state: "FAILED",
  agent_profile_id: "profile",
  metadata: {},
} as unknown as TaskSession;

it("keeps bootstrap phase, time, and production host references in mounted details", () => {
  render(
    <StateProvider
      initialState={
        {
          taskSessions: { items: { session } },
          agentProfiles: { items: [{ id: "profile" }] },
        } as unknown as Partial<AppState>
      }
    >
      <SessionRecoveryCard
        model={{
          sessionId: "session",
          stamp: "typed-bootstrap-failure",
          kind: "generic",
          details: "token=malicious-private-value",
          error: {
            message: "The agent could not start.",
            occurredAt: "2026-09-30T10:00:00Z",
            phase: "bootstrap",
            attemptId: "resume-1",
            executionId,
            causes: [{ operation: "resume", code: "permission_denied" }],
          },
        }}
        actions={actions}
        onNewSession={vi.fn()}
      />
    </StateProvider>,
  );

  fireEvent.click(screen.getByText("Technical details"));
  const details = document.querySelector("pre")?.textContent ?? "";
  expect(details).toContain("Phase: bootstrap");
  expect(details).toContain("Occurred: 2026-09-30T10:00:00Z");
  expect(details).toContain("Attempt: resume-1");
  expect(details).toContain(`Execution: ${executionId}`);
  expect(details).not.toContain("malicious-private-value");
});
