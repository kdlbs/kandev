import { vi } from "vitest";
import type { SessionRecoveryActions } from "@/hooks/domains/session/use-session-recovery-actions";
import type { SessionStoppedBannerProps } from "./session-stopped-banner";

export function guardRecoveryActions(
  message: string,
  operation: "resume" | "fresh_start" | "restore_workspace",
): NonNullable<SessionStoppedBannerProps["recoveryActions"]> {
  return {
    busyAction: null,
    workspaceRecovery: null,
    workspaceRecoveryRepositoryName: null,
    workspaceRecoveryStatusCheck: "idle",
    checkWorkspaceRecoveryStatus: vi.fn().mockResolvedValue({ resolved: true, projection: null }),
    recoveryError: new Error(message),
    branchDetails: null,
    providerRestoredResumeEligible: false,
    continuationDetails: null,
    guardDetails: { kind: "session_recovery_in_progress", retryable: true },
    recoveryNotice: null,
    managedCloneRecoveryStamp: null,
    lastFailedAction: null,
    manualRecoveryFailure: {
      operation,
      sessionId: "session-1",
      errorStamp: "failure-1",
      requestKey: "task-1\u0000session-1\u0000failure-1",
      operationId: 1,
    },
    handleRecover: vi.fn().mockResolvedValue(false),
    handleRestore: vi.fn().mockResolvedValue(undefined),
    handleRetry: vi.fn().mockResolvedValue(false),
    handleNewBranch: vi.fn().mockResolvedValue(false),
    handleManagedCloneRelocation: vi.fn().mockResolvedValue(false),
    handleContinueFromHistory: vi.fn().mockResolvedValue(false),
  } as unknown as SessionRecoveryActions;
}
