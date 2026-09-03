import { buildRestoreWorkspaceRequest } from "./session-launch-helpers";
import { launchSession } from "./session-launch-service";
import { getWebSocketClient } from "@/lib/ws/connection";
import { WebSocketRequestError, type WebSocketRequestErrorDetails } from "@/lib/ws/client";

export type SessionRecoveryAction =
  | "resume"
  | "resume_new_branch"
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

export type SessionRecoveryGuardDetails = WebSocketRequestErrorDetails & {
  kind: "session_recovery_in_progress" | "session_recovery_unstoppable";
  retryable: boolean;
  session_id?: string;
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

/** Returns the startup recovery guard's refusal reason, if the launch/recover was refused because of it. */
export function sessionRecoveryGuardDetails(error: unknown): SessionRecoveryGuardDetails | null {
  if (!(error instanceof WebSocketRequestError) || !isRecord(error.details)) return null;
  if (
    error.details.kind !== "session_recovery_in_progress" &&
    error.details.kind !== "session_recovery_unstoppable"
  ) {
    return null;
  }
  return error.details as SessionRecoveryGuardDetails;
}

/** Minimal shape both `useTranslation()`'s `t` and the module-level `t` satisfy. */
type Translator = (key: string, options?: Record<string, unknown>) => string;

/** Translates the guard's stable `kind` into the copy this session view shows. */
export function sessionRecoveryGuardMessage(
  details: SessionRecoveryGuardDetails,
  t: Translator,
): string {
  if (details.kind === "session_recovery_in_progress") {
    return t("task:sessionRecoveryGuardInProgress");
  }
  return t("task:sessionRecoveryGuardUnstoppable");
}

/** Resolves a thrown request failure to a user-facing message, preferring the
 *  recovery guard's translated reason over the raw backend/transport text. */
export function resolveRequestErrorMessage(
  error: unknown,
  t: Translator,
  fallback = t("common:unknownError"),
): string {
  const guard = sessionRecoveryGuardDetails(error);
  if (guard) return sessionRecoveryGuardMessage(guard, t);
  if (error instanceof Error) return error.message;
  return fallback;
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
