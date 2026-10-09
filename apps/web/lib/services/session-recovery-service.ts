import { buildRestoreWorkspaceRequest } from "./session-launch-helpers";
import { launchSession, type LaunchSessionResponse } from "./session-launch-service";
import { getWebSocketClient } from "@/lib/ws/connection";
import { WebSocketRequestError, type WebSocketRequestErrorDetails } from "@/lib/ws/client";
import type { WorkspaceRecoveryProjection } from "@/lib/types/http";

export type SessionRecoveryAction =
  | "resume"
  | "resume_new_branch"
  | "continue_from_history"
  | "fresh_start"
  | "runtime_retry"
  | "relocate_and_resume"
  | "retry_connection";

export type SessionDeliveryRecoveryOutcome =
  | "attached"
  | "settled"
  | "uncertain"
  | "blocked"
  | "unavailable"
  | "continued"
  | "restored_blocked";

export type SessionDeliveryRecoveryIdentity = {
  submission_id: string;
  stream_id: string;
  incarnation_id: string;
  harness_generation: number;
  prompt_generation: number;
};

export type SessionDeliveryRecoveryResponse = {
  task_id: string;
  session_id: string;
  outcome: SessionDeliveryRecoveryOutcome;
  reason?: string;
  allowed_actions?: string[];
  recovery_revision: number;
  recovery_identity?: SessionDeliveryRecoveryIdentity;
};

export type SessionRecoverySettingsPolicy = "provider_restored";

export type InterruptedSessionResumeRequest = {
  acknowledge_interruption: boolean;
  recovery_revision: number;
  recovery_identity: SessionDeliveryRecoveryIdentity;
  instruction: string;
  idempotency_key: string;
};

export type SessionRecoveryRequest = {
  taskId: string;
  sessionId: string;
  action: SessionRecoveryAction;
  failureMessage: string;
  errorStamp?: string | null;
  settingsPolicy?: SessionRecoverySettingsPolicy;
  interruptedResume?: InterruptedSessionResumeRequest;
};

const MANAGED_CLONE_RELOCATION_TIMEOUT_MS = 30 * 60 * 1000;

export type ManagedCloneRelocationRecoveryDetails = WebSocketRequestErrorDetails & {
  kind: "managed_clone_relocation_required" | "managed_clone_relocation_stale";
  error_stamp?: string;
  recovery_action?: "relocate_and_resume";
};

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

export type ContextContinuationDetails = WebSocketRequestErrorDetails & {
  kind: "session_restore_required";
  recovery_action: "continue_from_history";
  reason?: string;
  session_id?: string;
  generation?: number;
};

export type RecoveryInspectionBusyDetails = WebSocketRequestErrorDetails & {
  kind: "recovery_inspection_busy";
};

type WorkspaceRecoveryStatusResponse = {
  workspace_recovery?: WorkspaceRecoveryProjection | null;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function responseFailure(response: unknown, fallback: string): Error | null {
  if (!isRecord(response) || response.success !== false) return null;
  const message = typeof response.error === "string" && response.error ? response.error : fallback;
  return new Error(message);
}

function validRecoveryIdentity(value: unknown): value is SessionDeliveryRecoveryIdentity {
  if (!isRecord(value)) return false;
  return (
    [value.submission_id, value.stream_id, value.incarnation_id].every(
      (id) => typeof id === "string" && id.length > 0,
    ) &&
    [value.harness_generation, value.prompt_generation].every(
      (generation) =>
        typeof generation === "number" && Number.isSafeInteger(generation) && generation > 0,
    )
  );
}

const recoveryOutcomes: readonly unknown[] = [
  "attached",
  "settled",
  "uncertain",
  "blocked",
  "unavailable",
  "continued",
  "restored_blocked",
];

function validRecoveryActions(value: Record<string, unknown>): boolean {
  if (value.allowed_actions === undefined) return true;
  if (
    !Array.isArray(value.allowed_actions) ||
    value.allowed_actions.some((action) => typeof action !== "string")
  )
    return false;
  if (!value.allowed_actions.includes("resume_interrupted")) return true;
  return (
    validRecoveryIdentity(value.recovery_identity) &&
    typeof value.recovery_revision === "number" &&
    value.recovery_revision >= 1
  );
}

export function sessionDeliveryRecoveryResponse(
  value: unknown,
): SessionDeliveryRecoveryResponse | null {
  if (!isRecord(value) || !recoveryOutcomes.includes(value.outcome)) return null;
  if (
    typeof value.task_id !== "string" ||
    typeof value.session_id !== "string" ||
    typeof value.recovery_revision !== "number" ||
    !Number.isSafeInteger(value.recovery_revision) ||
    value.recovery_revision < 0
  )
    return null;
  if (value.reason !== undefined && typeof value.reason !== "string") return null;
  if (value.recovery_identity !== undefined && !validRecoveryIdentity(value.recovery_identity))
    return null;
  if (!validRecoveryActions(value)) return null;
  return value as SessionDeliveryRecoveryResponse;
}

export function sessionDeliveryRecoveryMessage(
  result: SessionDeliveryRecoveryResponse,
  t: Translator,
): string {
  switch (result.outcome) {
    case "continued":
      return t("task:interruptedRecoveryContinued");
    case "restored_blocked":
      return t("task:interruptedRecoveryRestoredBlocked");
    case "attached":
      return t("task:deliveryRecoveryAttached");
    case "settled":
      return t("task:deliveryRecoverySettled");
    case "uncertain":
      return t("task:deliveryRecoveryUncertain");
    case "unavailable":
      return t("task:deliveryRecoveryUnavailable");
    case "blocked":
      return blockedDeliveryRecoveryMessage(result.reason, t);
  }
}

function blockedDeliveryRecoveryMessage(reason: string | undefined, t: Translator): string {
  if (reason === "delivery_output_paused") return t("task:deliveryRecoveryOutputPaused");
  if (reason === "delivery_cancellation_pending")
    return t("task:deliveryRecoveryCancellationPending");
  if (reason === "delivery_storage_pressure") return t("task:deliveryRecoveryStoragePressure");
  if (reason === "missing_canonical_submission") {
    return t("task:deliveryRecoveryMissingSubmission");
  }
  if (
    reason === "recovery_identity_mismatch" ||
    reason === "execution_identity_changed" ||
    reason === "instance_identity_mismatch" ||
    reason === "delivery_identity_mismatch"
  ) {
    return t("task:deliveryRecoveryOwnershipBlocked");
  }
  return t("task:deliveryRecoveryBlocked");
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

/** Returns the typed conflict used when workspace inspection exhausts its wait budget. */
export function recoveryInspectionBusyDetails(
  error: unknown,
): RecoveryInspectionBusyDetails | null {
  if (!(error instanceof WebSocketRequestError) || !isRecord(error.details)) return null;
  if (error.details.kind !== "recovery_inspection_busy") return null;
  return error.details as RecoveryInspectionBusyDetails;
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

/** Translates the retryable inspection conflict without exposing transport text. */
export function recoveryInspectionBusyMessage(t: Translator): string {
  return t("task:workspaceRecoveryInspectionBusy");
}

export function managedCloneRelocationRecoveryDetails(
  error: unknown,
): ManagedCloneRelocationRecoveryDetails | null {
  if (!(error instanceof WebSocketRequestError) || !isRecord(error.details)) return null;
  if (
    error.details.kind !== "managed_clone_relocation_required" &&
    error.details.kind !== "managed_clone_relocation_stale"
  ) {
    return null;
  }
  return error.details as ManagedCloneRelocationRecoveryDetails;
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
  if (recoveryInspectionBusyDetails(error)) return recoveryInspectionBusyMessage(t);
  if (error instanceof Error) return error.message;
  return fallback;
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

function validateRecoveryRequest(options: SessionRecoveryRequest) {
  const { action, errorStamp, settingsPolicy, interruptedResume, failureMessage } = options;
  if (action === "relocate_and_resume" && !errorStamp) throw new Error(failureMessage);
  if (settingsPolicy && action !== "resume") throw new Error(failureMessage);
  if (interruptedResume && (action !== "resume" || settingsPolicy)) throw new Error(failureMessage);
}

function recoveryRequestPayload(options: SessionRecoveryRequest) {
  const { taskId, sessionId, action, errorStamp, settingsPolicy, interruptedResume } = options;
  return {
    task_id: taskId,
    session_id: sessionId,
    action,
    ...(action === "relocate_and_resume" ? { error_stamp: errorStamp } : {}),
    ...(interruptedResume ? { interrupted_resume: interruptedResume } : {}),
    ...(action === "resume" && settingsPolicy ? { settings_policy: settingsPolicy } : {}),
  };
}

function recoveryRequestTimeout(options: SessionRecoveryRequest) {
  if (options.action === "relocate_and_resume") return MANAGED_CLONE_RELOCATION_TIMEOUT_MS;
  if (options.interruptedResume) return 150_000;
  return 30_000;
}

/** Send one of the explicit session.recover actions. */
export async function requestSessionRecover(
  options: SessionRecoveryRequest,
): Promise<SessionDeliveryRecoveryResponse | void> {
  const { taskId, sessionId, action, failureMessage, interruptedResume } = options;
  validateRecoveryRequest(options);
  const client = getWebSocketClient();
  if (!client) throw new Error(failureMessage);
  const response = await client.request<unknown>(
    "session.recover",
    recoveryRequestPayload(options),
    recoveryRequestTimeout(options),
  );
  const failure = responseFailure(response, failureMessage);
  if (failure) throw failure;
  if (action === "retry_connection" || interruptedResume) {
    const recovery = sessionDeliveryRecoveryResponse(response);
    if (!recovery || recovery.task_id !== taskId || recovery.session_id !== sessionId) {
      throw new Error(failureMessage);
    }
    return recovery;
  }
}

/** Resume through recovery and retain the workspace projection for the caller. */
export async function resumeSession(
  taskId: string,
  sessionId: string,
  failureMessage: string,
): Promise<LaunchSessionResponse> {
  const client = getWebSocketClient();
  if (!client) throw new Error(failureMessage);
  const response = await client.request<LaunchSessionResponse>(
    "session.recover",
    { task_id: taskId, session_id: sessionId, action: "resume" },
    60_000,
  );
  const failure = responseFailure(response, failureMessage);
  if (failure) throw failure;
  return response;
}

/** Read current recovery state without reconstructing or inspecting the workspace. */
export async function getWorkspaceRecoveryStatus(
  taskId: string,
  sessionId: string,
  failureMessage: string,
): Promise<WorkspaceRecoveryProjection | null> {
  const client = getWebSocketClient();
  if (!client) throw new Error(failureMessage);
  const response = await client.request<WorkspaceRecoveryStatusResponse>(
    "session.workspace_recovery.get",
    { task_id: taskId, session_id: sessionId },
    10_000,
  );
  return response.workspace_recovery ?? null;
}

/** Restore the existing task workspace without starting the provider. */
export async function restoreSessionWorkspace(
  taskId: string,
  sessionId: string,
  failureMessage: string,
): Promise<LaunchSessionResponse> {
  const { request } = buildRestoreWorkspaceRequest(taskId, sessionId);
  const response = await launchSession(request);
  const failure = responseFailure(response, failureMessage);
  if (failure) throw failure;
  return response;
}
