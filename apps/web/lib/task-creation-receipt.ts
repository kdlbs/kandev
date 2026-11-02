import type { createTask, fetchTaskByExternalId } from "@/lib/api";
import type { CreateTaskResponse, Task } from "@/lib/types/http";
import { generateUUID } from "@/lib/uuid";

type CreateTaskParams = Parameters<typeof createTask>[0];
type TaskLookupResponse = Awaited<ReturnType<typeof fetchTaskByExternalId>>;

const RECEIPT_VERSION = 1;
const STORAGE_PREFIX = "kandev.taskCreationReceipt.v1";
export const TASK_CREATION_EXTERNAL_ID_PREFIX = "ui-task-create/";

/**
 * Observed state of one browser submission. `submitting` and `unknown` both
 * mean the browser cannot prove the request's task allocation; `incomplete`
 * means the task exists but its own required creation work has not settled.
 */
export type TaskCreationReceiptState =
  | "submitting"
  | "unknown"
  | "incomplete"
  | "accepted"
  | "unavailable";

/** The retained identity, exact submitted values, and observed result. */
export type TaskCreationReceipt = {
  version: typeof RECEIPT_VERSION;
  submissionId: string;
  externalId: string;
  userId: string | null;
  workspaceId: string;
  draftId: string;
  /** The form title at submission, including an auto-generated title. */
  draftTitle?: string;
  payload: CreateTaskParams;
  state: TaskCreationReceiptState;
  /** Sticky proof that an earlier attempt can still finish after any miss. */
  everUncertain: boolean;
  taskId: string | null;
  creationComplete: boolean | null;
  /** Whether the payload can be sent again without losing required values. */
  replayable: boolean;
  runtimeId: string | null;
  handoffAttempted: boolean;
  taskSessionId: string | null;
  queuedForStepId: string | null;
  updatedAt: string;
};

export type TaskCreationScope = {
  userId: string | null;
  workspaceId: string;
  draftId: string;
};

export type NativeCreateObservation =
  | {
      kind: "accepted";
      taskId: string;
      taskSessionId: string | null;
      queuedForStepId: string | null;
    }
  | {
      kind: "incomplete";
      taskId: string;
      taskSessionId: string | null;
      queuedForStepId: string | null;
    }
  | { kind: "unknown"; taskId: string | null };

export type TaskLookupObservation =
  | { kind: "accepted"; taskId: string }
  | { kind: "incomplete"; taskId: string }
  | { kind: "absent" }
  | { kind: "mismatch" };

const RECEIPT_STATES: ReadonlySet<TaskCreationReceiptState> = new Set([
  "submitting",
  "unknown",
  "incomplete",
  "accepted",
  "unavailable",
]);

export function resolveTaskCreationDraftId(parentTaskId?: string | null): string {
  return parentTaskId ? `subtask:${parentTaskId}` : "task";
}

export function receiptStorageKey(scope: TaskCreationScope): string {
  const user = scope.userId ?? "anonymous";
  return `${STORAGE_PREFIX}.${user}.${scope.workspaceId}.${scope.draftId}`;
}

/** True when every payload value needed for an exact resend is still present. */
export function isCapturedPayloadReplayable(payload: CreateTaskParams): boolean {
  return (payload.attachments ?? []).every(
    (attachment) => !attachment.data || Boolean(attachment.attachment_id),
  );
}

export function createTaskCreationReceipt(input: {
  scope: TaskCreationScope;
  payload: CreateTaskParams;
  runtimeId: string | null;
  draftTitle?: string;
}): TaskCreationReceipt {
  const submissionId = generateUUID();
  const payload: CreateTaskParams = {
    ...input.payload,
    external_id: `${TASK_CREATION_EXTERNAL_ID_PREFIX}${submissionId}`,
  };
  return {
    version: RECEIPT_VERSION,
    submissionId,
    externalId: payload.external_id as string,
    userId: input.scope.userId,
    workspaceId: input.scope.workspaceId,
    draftId: input.scope.draftId,
    draftTitle: input.draftTitle,
    payload,
    state: "submitting",
    everUncertain: false,
    taskId: null,
    creationComplete: null,
    replayable: isCapturedPayloadReplayable(payload),
    runtimeId: input.runtimeId,
    handoffAttempted: false,
    taskSessionId: null,
    queuedForStepId: null,
    updatedAt: new Date().toISOString(),
  };
}

/**
 * Strips inline attachment bytes and credentials before persistence. A payload
 * that required inline bytes keeps its descriptor but is marked non-replayable
 * so a restored dialog can only look the task up.
 */
export function toPersistedTaskCreationReceipt(receipt: TaskCreationReceipt): TaskCreationReceipt {
  const attachments = receipt.payload.attachments;
  if (!attachments?.length) return receipt;
  return {
    ...receipt,
    replayable: receipt.replayable && isCapturedPayloadReplayable(receipt.payload),
    payload: {
      ...receipt.payload,
      attachments: attachments.map((attachment) => {
        const { data: _inlineData, ...descriptor } = attachment;
        return descriptor;
      }),
    },
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isNonEmptyString(value: unknown): value is string {
  return typeof value === "string" && value.length > 0;
}

function hasReceiptIdentity(value: Record<string, unknown>): boolean {
  return (
    isNonEmptyString(value.submissionId) &&
    isNonEmptyString(value.externalId) &&
    isNonEmptyString(value.workspaceId) &&
    isNonEmptyString(value.draftId)
  );
}

function hasReceiptOutcome(value: Record<string, unknown>): boolean {
  if (value.taskId !== null && !isNonEmptyString(value.taskId)) return false;
  if (value.creationComplete !== null && typeof value.creationComplete !== "boolean") return false;
  return true;
}

export function parseTaskCreationReceipt(value: unknown): TaskCreationReceipt | null {
  if (!isRecord(value)) return null;
  if (value.version !== RECEIPT_VERSION) return null;
  if (!hasReceiptIdentity(value)) return null;
  if (
    typeof value.state !== "string" ||
    !RECEIPT_STATES.has(value.state as TaskCreationReceiptState)
  ) {
    return null;
  }
  if (value.draftTitle !== undefined && typeof value.draftTitle !== "string") return null;
  if (!isRecord(value.payload)) return null;
  if (typeof value.everUncertain !== "boolean" || typeof value.replayable !== "boolean") {
    return null;
  }
  if (!hasReceiptOutcome(value)) return null;
  return value as unknown as TaskCreationReceipt;
}

/** Writes the receipt and reports whether the browser retained it. */
export function saveTaskCreationReceipt(receipt: TaskCreationReceipt): boolean {
  if (typeof window === "undefined") return false;
  try {
    window.sessionStorage.setItem(
      receiptStorageKey(receipt),
      JSON.stringify(toPersistedTaskCreationReceipt(receipt)),
    );
    return true;
  } catch {
    return false;
  }
}

export function loadTaskCreationReceipt(scope: TaskCreationScope): TaskCreationReceipt | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = window.sessionStorage.getItem(receiptStorageKey(scope));
    if (!raw) return null;
    const parsed = parseTaskCreationReceipt(JSON.parse(raw));
    if (!parsed) return null;
    if (
      parsed.userId !== scope.userId ||
      parsed.workspaceId !== scope.workspaceId ||
      parsed.draftId !== scope.draftId
    ) {
      return null;
    }
    return parsed;
  } catch {
    return null;
  }
}

export function removeTaskCreationReceipt(scope: TaskCreationScope): void {
  if (typeof window === "undefined") return;
  try {
    window.sessionStorage.removeItem(receiptStorageKey(scope));
  } catch {
    // A failed removal leaves a stale receipt, which is safer than losing it.
  }
}

function stringField(value: unknown): string | null {
  return typeof value === "string" && value.length > 0 ? value : null;
}

function readTaskSessionId(response: CreateTaskResponse): string | null {
  return response.session_id ?? response.primary_session_id ?? null;
}

/**
 * Decodes the native create response. Missing metadata, a workspace or
 * external-identity mismatch, or a conflicting task ID is uncertain and never
 * adopts the response's task. A complete response without an external ID is
 * the valid identity-lost case and keeps its verified task.
 */
export function decodeNativeCreateResponse(
  response: CreateTaskResponse,
  receipt: TaskCreationReceipt,
): NativeCreateObservation {
  const taskId = stringField(response?.id);
  if (!taskId) return { kind: "unknown", taskId: null };
  if (response.workspace_id !== receipt.workspaceId) {
    return { kind: "unknown", taskId: null };
  }
  const responseExternalId = stringField(response.external_id);
  if (responseExternalId && responseExternalId !== receipt.externalId) {
    return { kind: "unknown", taskId: null };
  }
  if (receipt.taskId && receipt.taskId !== taskId) {
    return { kind: "unknown", taskId: null };
  }
  if (typeof response.deduplicated !== "boolean") return { kind: "unknown", taskId };
  if (typeof response.creation_complete !== "boolean") return { kind: "unknown", taskId };
  const common = {
    taskId,
    taskSessionId: readTaskSessionId(response),
    queuedForStepId: stringField(response.queued_for_step_id),
  };
  if (response.creation_complete === false) return { kind: "incomplete", ...common };
  return { kind: "accepted", ...common };
}

/**
 * Classifies a read-only lookup. Workspace and external identity are
 * mandatory on this route, so a response that omits either cannot become the
 * observed task.
 */
export function classifyTaskLookup(
  response: TaskLookupResponse,
  receipt: TaskCreationReceipt,
): TaskLookupObservation {
  const taskId = stringField(response?.id);
  if (!taskId) return { kind: "mismatch" };
  if (response.workspace_id !== receipt.workspaceId) {
    return { kind: "mismatch" };
  }
  const externalId = stringField(response.external_id);
  if (!externalId || externalId !== receipt.externalId) return { kind: "mismatch" };
  if (receipt.taskId && receipt.taskId !== taskId) return { kind: "mismatch" };
  if (typeof response.creation_complete !== "boolean") return { kind: "mismatch" };
  if (response.creation_complete === false) return { kind: "incomplete", taskId };
  return { kind: "accepted", taskId };
}

/** Reads the hydrated session identity for an already-created task. */
export function taskDetailSessionId(task: Task): string | null {
  return task.primary_session_id ?? null;
}

/** Updates only the branch-discard consent fields on the captured payload. */
export function withRenewedDiscardConsent(
  payload: CreateTaskParams,
  consentedDirtyFiles: string[],
): CreateTaskParams {
  return {
    ...payload,
    repositories: payload.repositories?.map((repository) => ({
      ...repository,
      ...(repository.fresh_branch
        ? { confirm_discard: true, consented_dirty_files: consentedDirtyFiles }
        : {}),
    })),
  };
}
