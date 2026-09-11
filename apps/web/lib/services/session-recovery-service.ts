import { buildRestoreWorkspaceRequest } from "./session-launch-helpers";
import { launchSession } from "./session-launch-service";
import { getWebSocketClient } from "@/lib/ws/connection";
import { WebSocketRequestError, type WebSocketRequestErrorDetails } from "@/lib/ws/client";

export type SessionRecoveryAction =
  | "resume"
  | "resume_new_branch"
  | "continue_from_history"
  | "fresh_start"
  | "runtime_retry";

export type BranchRecoveryDetails = WebSocketRequestErrorDetails & {
  kind: "branch_unrecoverable";
  recovery_action: "resume_new_branch";
  original_branch?: string;
  base_branch?: string;
  repository_id?: string;
  session_id?: string;
};

export type ContextContinuationDetails = WebSocketRequestErrorDetails & {
  kind: "session_restore_required";
  recovery_action: "continue_from_history";
  reason?: string;
  session_id?: string;
  generation?: number;
};

type RecoveryResponse = { success?: boolean; error?: string };

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function responseFailure(response: unknown, fallback: string): Error | null {
  if (!isRecord(response) || response.success !== false) return null;
  const message = typeof response.error === "string" && response.error ? response.error : fallback;
  return new Error(message);
}

/** Returns the structured branch-loss context that authorizes replacement. */
export function branchRecoveryDetails(error: unknown): BranchRecoveryDetails | null {
  if (!(error instanceof WebSocketRequestError) || !isRecord(error.details)) return null;
  if (
    error.details.kind !== "branch_unrecoverable" ||
    error.details.recovery_action !== "resume_new_branch"
  ) {
    return null;
  }
  return error.details as BranchRecoveryDetails;
}

/** Returns the structured native-state loss context that authorizes history continuation. */
export function contextContinuationDetails(error: unknown): ContextContinuationDetails | null {
  if (!(error instanceof WebSocketRequestError) || !isRecord(error.details)) return null;
  if (
    error.details.kind !== "session_restore_required" ||
    error.details.recovery_action !== "continue_from_history"
  ) {
    return null;
  }
  return error.details as ContextContinuationDetails;
}

/** Converts unknown request failures into an Error for an inline recovery alert. */
export function asRecoveryError(error: unknown, fallback: string): Error {
  return error instanceof Error ? error : new Error(fallback);
}

/** Send one of the explicit session.recover actions. */
export async function requestSessionRecover(
  taskId: string,
  sessionId: string,
  action: SessionRecoveryAction,
  failureMessage: string,
): Promise<void> {
  const client = getWebSocketClient();
  if (!client) throw new Error(failureMessage);
  const response = await client.request<RecoveryResponse>(
    "session.recover",
    {
      task_id: taskId,
      session_id: sessionId,
      action,
    },
    30_000,
  );
  const failure = responseFailure(response, failureMessage);
  if (failure) throw failure;
}

/** Restore the existing task workspace without starting the provider. */
export async function restoreSessionWorkspace(
  taskId: string,
  sessionId: string,
  failureMessage: string,
): Promise<void> {
  const { request } = buildRestoreWorkspaceRequest(taskId, sessionId);
  const response = await launchSession(request);
  const failure = responseFailure(response, failureMessage);
  if (failure) throw failure;
}
