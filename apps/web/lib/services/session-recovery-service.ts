import type { AgentErrorCause } from "@/lib/types/task-status-summary";
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
  | "continued";

export type SessionDeliveryRecoveryResponse = {
  task_id: string;
  session_id: string;
  outcome: SessionDeliveryRecoveryOutcome;
  reason?: string;
  recovery_revision: number;
};

export type SessionRecoverySettingsPolicy = "provider_restored";

export type SessionRecoveryRequest = {
  taskId: string;
  sessionId: string;
  action: SessionRecoveryAction;
  failureMessage: string;
  errorStamp?: string | null;
  settingsPolicy?: SessionRecoverySettingsPolicy;
  sessionMetadata?: Record<string, unknown> | null;
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

const recoveryOutcomes: readonly unknown[] = [
  "attached",
  "settled",
  "uncertain",
  "blocked",
  "unavailable",
  "continued",
];

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
  return value as SessionDeliveryRecoveryResponse;
}

export function sessionDeliveryRecoveryMessage(
  result: SessionDeliveryRecoveryResponse,
  t: Translator,
): string {
  switch (result.outcome) {
    case "continued":
    case "attached":
      return t("task:deliveryRecoveryAttached");
    case "settled":
      return t("task:deliveryRecoverySettled");
    case "uncertain":
      return t("task:deliveryRecoveryUncertain");
    case "unavailable":
      return t("task:deliveryRecoveryUnavailable");
    case "blocked":
      return sessionDeliveryRecoveryReasonMessage(result.reason, t);
  }
}

export function sessionDeliveryRecoveryReasonMessage(
  reason: string | undefined,
  t: Translator,
): string {
  if (reason === "delivery_history_gap") return t("task:deliveryHistoryGapBody");
  if (reason === "delivery_gap_recovery_unsupported")
    return t("task:deliveryHistoryGapUnsupported");
  if (reason === "delivery_output_paused") return t("task:deliveryRecoveryOutputPaused");
  if (reason === "delivery_cancellation_pending")
    return t("task:deliveryRecoveryCancellationPending");
  if (reason === "delivery_storage_pressure") return t("task:deliveryRecoveryStoragePressure");
  if (reason === "missing_canonical_submission" || reason === "unresolved_durable_work") {
    return t("task:deliveryRecoveryMissingSubmission");
  }
  if (reason === "recovery_identity_incomplete") {
    return t("task:deliveryRecoveryIdentityIncomplete");
  }
  if (reason === "retained_evidence_ambiguous") {
    return t("task:deliveryRecoveryAmbiguousEvidence");
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

export function runtimeCleanupFailureDetails(error: unknown, sessionId?: string) {
  if (!(error instanceof WebSocketRequestError) || !isRecord(error.details)) return null;
  const details = error.details;
  if (details.kind !== "runtime_cleanup_failed" || typeof details.session_id !== "string")
    return null;
  if (sessionId && details.session_id !== sessionId) return null;
  return {
    sessionId: details.session_id,
    errorStamp: typeof details.error_stamp === "string" ? details.error_stamp : undefined,
  };
}

/** Copy only host-owned cleanup evidence; transport detail remains absent. */
export function runtimeCleanupFailureCauses(error: unknown, sessionId?: string): AgentErrorCause[] {
  if (!runtimeCleanupFailureDetails(error, sessionId)) return [];
  const causes = (error as WebSocketRequestError).details?.causes;
  const cause = Array.isArray(causes)
    ? causes.find(
        (candidate: unknown) =>
          isRecord(candidate) &&
          candidate.operation === "resume" &&
          candidate.code === "runtime_cleanup_failed",
      )
    : undefined;
  return [
    {
      operation: "resume",
      code: "runtime_cleanup_failed",
      ...(isRecord(cause) && typeof cause.prompt_not_sent === "boolean"
        ? { prompt_not_sent: cause.prompt_not_sent }
        : {}),
    },
  ];
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
  if (runtimeCleanupFailureDetails(error)) return t("task:sessionRuntimeCleanupFailedBody");
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
  const { action, errorStamp, settingsPolicy, failureMessage } = options;
  if (action === "relocate_and_resume" && !errorStamp) throw new Error(failureMessage);
  if (settingsPolicy && action !== "resume") throw new Error(failureMessage);
}

function recoveryRequestPayload(options: SessionRecoveryRequest) {
  const { taskId, sessionId, action, errorStamp, settingsPolicy } = options;
  return {
    task_id: taskId,
    session_id: sessionId,
    action,
    ...(action === "relocate_and_resume" ? { error_stamp: errorStamp } : {}),
    ...(action === "resume" && settingsPolicy ? { settings_policy: settingsPolicy } : {}),
  };
}

type InferenceClosureResume = {
  operationId: string;
  sourceExecutionId: string;
  candidateExecutionId: string;
  incarnationId: string;
  stage: string;
};

type InferenceClosureIdentity = Record<string, unknown> & {
  operation_id: string;
  source_execution_id: string;
  candidate_execution_id: string;
  incarnation_id: string;
};

function hasInferenceClosureIdentity(
  closure: Record<string, unknown>,
  sessionId: string,
): closure is InferenceClosureIdentity {
  return (
    closure.version === 1 &&
    closure.session_id === sessionId &&
    typeof closure.operation_id === "string" &&
    !!closure.operation_id &&
    typeof closure.source_execution_id === "string" &&
    !!closure.source_execution_id &&
    typeof closure.candidate_execution_id === "string" &&
    !!closure.candidate_execution_id &&
    closure.source_execution_id !== closure.candidate_execution_id &&
    typeof closure.incarnation_id === "string" &&
    !!closure.incarnation_id
  );
}

function inferenceClosureResume(options: SessionRecoveryRequest): InferenceClosureResume | null {
  if (options.action !== "resume") return null;
  const value = options.sessionMetadata?.inference_scope_closure;
  if (value == null) return null;
  if (typeof value !== "object" || Array.isArray(value)) throw new Error(options.failureMessage);
  const closure = value as Record<string, unknown>;
  if (closure.stage === "consumed") return null;
  if (
    !hasInferenceClosureIdentity(closure, options.sessionId) ||
    typeof closure.stage !== "string" ||
    !["fenced", "observed", "settled", "committed"].includes(closure.stage)
  ) {
    throw new Error(options.failureMessage);
  }
  return {
    operationId: closure.operation_id,
    sourceExecutionId: closure.source_execution_id,
    candidateExecutionId: closure.candidate_execution_id,
    incarnationId: closure.incarnation_id,
    stage: closure.stage,
  };
}

function assertReadyInferenceClosure(
  value: unknown,
  closure: InferenceClosureResume,
  sessionId: string,
  failureMessage: string,
): void {
  if (
    !isRecord(value) ||
    value.version !== 1 ||
    value.operation_id !== closure.operationId ||
    value.session_id !== sessionId ||
    value.incarnation_id !== closure.incarnationId ||
    value.source_execution_id !== closure.sourceExecutionId ||
    value.candidate_execution_id !== closure.candidateExecutionId ||
    (value.stage !== "committed" && value.stage !== "consumed") ||
    value.claims_settled !== true
  )
    throw new Error(failureMessage);
}

function recoveryRequestTimeout(options: SessionRecoveryRequest) {
  if (options.action === "relocate_and_resume") return MANAGED_CLONE_RELOCATION_TIMEOUT_MS;
  return 30_000;
}

async function prepareInferenceClosure(
  client: NonNullable<ReturnType<typeof getWebSocketClient>>,
  options: SessionRecoveryRequest,
): Promise<InferenceClosureResume | null> {
  const closure = inferenceClosureResume(options);
  if (closure && closure.stage !== "committed") {
    const stopped = await client.request<unknown>(
      "session.stop",
      {
        session_id: options.sessionId,
        expected_execution_id: closure.sourceExecutionId,
        await_terminal: true,
        inference_closure_operation_id: closure.operationId,
      },
      60_000,
    );
    assertReadyInferenceClosure(stopped, closure, options.sessionId, options.failureMessage);
  }
  return closure;
}

/** Send one of the explicit session.recover actions. */
export async function requestSessionRecover(
  options: SessionRecoveryRequest,
): Promise<SessionDeliveryRecoveryResponse | void> {
  const { taskId, sessionId, action, failureMessage } = options;
  validateRecoveryRequest(options);
  const client = getWebSocketClient();
  if (!client) throw new Error(failureMessage);

  const closure = await prepareInferenceClosure(client, options);
  const response = await client.request<unknown>(
    "session.recover",
    {
      ...recoveryRequestPayload(options),
      ...(closure ? { inference_closure_operation_id: closure.operationId } : {}),
    },
    recoveryRequestTimeout(options),
  );
  const failure = responseFailure(response, failureMessage);
  if (failure) throw failure;
  if (action === "retry_connection") {
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
  sessionMetadata?: Record<string, unknown> | null,
): Promise<LaunchSessionResponse> {
  const client = getWebSocketClient();
  if (!client) throw new Error(failureMessage);
  const closure = await prepareInferenceClosure(client, {
    taskId,
    sessionId,
    action: "resume",
    failureMessage,
    sessionMetadata,
  });
  const response = await client.request<LaunchSessionResponse>(
    "session.recover",
    {
      task_id: taskId,
      session_id: sessionId,
      action: "resume",
      ...(closure ? { inference_closure_operation_id: closure.operationId } : {}),
    },
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
