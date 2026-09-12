import { useCallback, useRef } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { appendToQueue, queueMessage } from "@/lib/api/domains/queue-api";
import { useAppStoreApi } from "@/components/state-provider";
import { useCommentsStore } from "@/lib/state/slices/comments";
import {
  formatReviewCommentsAsMarkdown,
  formatPRFeedbackAsMarkdown,
  formatWalkthroughCommentsAsMarkdown,
  formatAgentMessageCommentsAsMarkdown,
} from "@/lib/state/slices/comments/format";
import type {
  Comment,
  DiffComment,
  PlanComment,
  FileEditorComment,
  PRFeedbackComment,
  WalkthroughComment,
  AgentMessageComment,
} from "@/lib/state/slices/comments";
import type { Message } from "@/lib/types/http";
import { deriveSessionInputMode } from "@/hooks/domains/session/session-input-mode";
import { generateUUID } from "@/lib/utils";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";
import { sendMessageRequest } from "@/hooks/use-message-handler";
import { t } from "@/lib/i18n";
import { planCommentAdmissionConflict } from "@/lib/plan-comment-refs";
import { getTaskPlanComments } from "@/lib/api/domains/plan-comment-api";
import { listTaskSessions } from "@/lib/api/domains/session-api";
import { findTaskInSnapshots } from "@/lib/kanban/find-task";
import { captureTaskSessionActivityEpochs } from "@/lib/state/slices/session/activity-epochs";

/**
 * Format a single comment into markdown suitable for sending to the agent.
 */
function formatSingleComment(comment: Comment): string {
  switch (comment.source) {
    case "diff":
      return formatReviewCommentsAsMarkdown([comment as DiffComment]);
    case "plan":
      return "";
    case "pr-feedback":
      return formatPRFeedbackAsMarkdown([comment as PRFeedbackComment]);
    case "walkthrough":
      return formatWalkthroughCommentsAsMarkdown([comment as WalkthroughComment]);
    case "agent-message":
      return formatAgentMessageCommentsAsMarkdown([comment as AgentMessageComment]);
    case "file-editor": {
      const fc = comment as FileEditorComment;
      const lines: string[] = ["### File Comment", ""];
      let loc = fc.filePath;
      if (fc.startLine && fc.endLine && fc.endLine !== fc.startLine) {
        loc = `${fc.filePath}:${fc.startLine}-${fc.endLine}`;
      } else if (fc.startLine) {
        loc = `${fc.filePath}:${fc.startLine}`;
      }
      lines.push(`**${loc}**`);
      if (fc.selectedText) {
        lines.push("```");
        lines.push(fc.selectedText);
        lines.push("```");
      }
      lines.push(`> ${fc.text}`);
      lines.push("", "---", "");
      return lines.join("\n");
    }
  }
}

type UseRunCommentParams = {
  sessionId: string | null;
  taskId: string | null;
};

type QueuePayload = {
  session_id: string;
  task_id: string;
  session_incarnation_id: string;
  content: string;
  plan_mode?: boolean;
};

type MessagePayload = {
  task_id: string;
  session_id: string;
  client_message_id: string;
  content: string;
  plan_mode?: boolean;
  has_review_comments?: boolean;
};

const NO_PRIMARY_SESSION = "no-primary-session" as const;

export type PlanCommentRunUnavailableReason =
  | typeof NO_PRIMARY_SESSION
  | "primary-session-unavailable";

export type PlanCommentRunAvailability =
  | { session: TaskSession; inputMode: "direct" | "queue"; reason: null }
  | { session: null; inputMode: "unavailable"; reason: PlanCommentRunUnavailableReason };

function primarySessionFromTask(
  state: AppState,
  taskId: string,
): { known: boolean; session?: TaskSession } {
  const task =
    state.kanban.tasks.find((candidate) => candidate.id === taskId) ??
    findTaskInSnapshots(taskId, state.kanbanMulti.snapshots);
  if (!task || task.primarySessionId === undefined) return { known: false };
  if (task.primarySessionId === null) return { known: true };
  const session =
    state.taskSessions.items[task.primarySessionId] ??
    state.taskSessionsByTask.itemsByTaskId[taskId]?.find(
      (candidate) => candidate.id === task.primarySessionId,
    );
  return { known: true, session: session?.task_id === taskId ? session : undefined };
}

function listedPrimarySession(state: AppState, taskId: string): TaskSession | undefined {
  const projected = primarySessionFromTask(state, taskId);
  if (projected.known) return projected.session;
  const taskSessions = state.taskSessionsByTask?.itemsByTaskId[taskId] ?? [];
  const listed = taskSessions.find((candidate) => candidate.is_primary);
  if (listed) {
    const live = state.taskSessions.items[listed.id];
    if (!live || live.is_primary) return live ?? listed;
  }
  return Object.values(state.taskSessions.items).find(
    (candidate) => candidate.task_id === taskId && candidate.is_primary,
  );
}

function listedTaskSessions(state: AppState, taskId: string): TaskSession[] {
  const sessions = new Map(
    (state.taskSessionsByTask.itemsByTaskId[taskId] ?? []).map((session) => [session.id, session]),
  );
  for (const session of Object.values(state.taskSessions.items)) {
    if (session.task_id === taskId) sessions.set(session.id, session);
  }
  return [...sessions.values()];
}

function taskPrimaryRoutingFingerprint(state: AppState, taskId: string): string {
  const task =
    state.kanban.tasks.find((candidate) => candidate.id === taskId) ??
    findTaskInSnapshots(taskId, state.kanbanMulti.snapshots);
  const sessions = listedTaskSessions(state, taskId)
    .sort((left, right) => left.id.localeCompare(right.id))
    .map((session) => [
      session.id,
      session.is_primary,
      session.state,
      session.queue_incarnation_id,
    ]);
  return JSON.stringify([task?.primarySessionId, task?.primarySessionState, sessions]);
}

function applyTaskPrimaryProjection(
  taskId: string,
  primarySessionId: string | null,
  primarySessionState: TaskSession["state"] | undefined,
  storeApi: ReturnType<typeof useAppStoreApi>,
) {
  storeApi.setState((state) => {
    const updateTasks = (tasks: typeof state.kanban.tasks) =>
      tasks.map((task) => {
        if (task.id !== taskId) return task;
        const next = { ...task, primarySessionId };
        if (primarySessionState) next.primarySessionState = primarySessionState;
        else delete next.primarySessionState;
        return next;
      });
    return {
      kanban: { ...state.kanban, tasks: updateTasks(state.kanban.tasks) },
      kanbanMulti: {
        ...state.kanbanMulti,
        snapshots: Object.fromEntries(
          Object.entries(state.kanbanMulti.snapshots).map(([workflowId, snapshot]) => [
            workflowId,
            { ...snapshot, tasks: updateTasks(snapshot.tasks) },
          ]),
        ),
      },
    };
  });
}

/** Resolve the live primary and its admission mode from one store snapshot. */
export function resolvePlanCommentRunAvailability(
  state: AppState,
  taskId: string | null,
): PlanCommentRunAvailability {
  if (!taskId) {
    return { session: null, inputMode: "unavailable", reason: NO_PRIMARY_SESSION };
  }
  const primary = listedPrimarySession(state, taskId);
  if (!primary) {
    return { session: null, inputMode: "unavailable", reason: NO_PRIMARY_SESSION };
  }
  const queuedCount = state.queue.metaBySessionId[primary.id]?.count ?? 0;
  const inputMode = deriveSessionInputMode(primary, queuedCount);
  if (inputMode === "unavailable") {
    return { session: null, inputMode, reason: "primary-session-unavailable" };
  }
  return { session: primary, inputMode, reason: null };
}

export class PlanCommentRunError extends Error {
  constructor(
    readonly code:
      | PlanCommentRunUnavailableReason
      | "plan-comment-not-persisted"
      | "plan-comments-changed"
      | "primary-session-changed"
      | "delivery-failed",
  ) {
    super(planCommentRunErrorMessage(code));
    this.name = "PlanCommentRunError";
  }
}

function planCommentRunErrorMessage(code: PlanCommentRunError["code"]): string {
  switch (code) {
    case NO_PRIMARY_SESSION:
      return t("task:noPrimarySessionForPlanComment");
    case "primary-session-unavailable":
      return t("task:primarySessionUnavailableForPlanComment");
    case "plan-comment-not-persisted":
      return t("task:planCommentNotReadyToRun");
    case "plan-comments-changed":
      return t("task:planCommentsChangedRetry");
    case "primary-session-changed":
      return t("task:primarySessionChangedRetry");
    default:
      return t("task:failedToRunPlanComment");
  }
}

async function runTaskPlanComment(
  comment: PlanComment,
  taskId: string,
  storeApi: ReturnType<typeof useAppStoreApi>,
  clientAdmissionId: string,
): Promise<{ queued: boolean }> {
  if (!comment.version || comment.version < 1) {
    throw new PlanCommentRunError("plan-comment-not-persisted");
  }
  const availability = resolvePlanCommentRunAvailability(storeApi.getState(), taskId);
  if (availability.reason || !availability.session) {
    throw new PlanCommentRunError(availability.reason ?? NO_PRIMARY_SESSION);
  }
  const planCommentRefs = [{ id: comment.id, version: comment.version }];
  if (availability.inputMode === "queue") {
    if (!availability.session.queue_incarnation_id) {
      throw new PlanCommentRunError("primary-session-unavailable");
    }
    await queueMessage({
      session_id: availability.session.id,
      session_incarnation_id: availability.session.queue_incarnation_id,
      task_id: taskId,
      client_queue_id: clientAdmissionId,
      content: "",
      plan_mode: true,
      plan_comment_refs: planCommentRefs,
      require_primary_session: true,
    });
    await refreshAcceptedPlanComments(taskId, storeApi);
    return { queued: true };
  }
  const created = await sendMessageRequest({
    taskId,
    resolvedSessionId: availability.session.id,
    clientMessageId: clientAdmissionId,
    finalMessage: "",
    modelToSend: undefined,
    planMode: true,
    planCommentRefs,
    requirePrimarySession: true,
  });
  if (created?.id && created.session_id) storeApi.getState().addMessage(created);
  await refreshAcceptedPlanComments(taskId, storeApi);
  return { queued: false };
}

async function refreshAcceptedPlanComments(
  taskId: string,
  storeApi: ReturnType<typeof useAppStoreApi>,
) {
  try {
    const snapshot = await getTaskPlanComments(taskId);
    storeApi.getState().setTaskPlanComments(taskId, snapshot);
  } catch (error) {
    // i18n-exempt: accepted delivery remains successful; foreground recovery retries this refresh.
    console.error("Failed to refresh task plan comments after delivery:", error);
  }
}

function buildQueuePayload(
  sessionId: string,
  taskId: string,
  sessionIncarnationId: string,
  content: string,
  planModeEnabled: boolean,
): QueuePayload {
  const payload: QueuePayload = {
    session_id: sessionId,
    session_incarnation_id: sessionIncarnationId,
    task_id: taskId,
    content,
  };
  if (planModeEnabled) payload.plan_mode = true;
  return payload;
}

function buildMessagePayload(
  sessionId: string,
  taskId: string,
  content: string,
  planModeEnabled: boolean,
  comment: Comment,
): MessagePayload {
  const payload: MessagePayload = {
    task_id: taskId,
    session_id: sessionId,
    client_message_id: generateUUID(),
    content,
  };
  if (planModeEnabled) payload.plan_mode = true;
  if (comment.source !== "plan") payload.has_review_comments = true;
  return payload;
}

function applyPrimarySessionChangeFallback(
  conflict: NonNullable<ReturnType<typeof planCommentAdmissionConflict>>,
  taskId: string,
  storeApi: ReturnType<typeof useAppStoreApi>,
) {
  if (conflict.primarySessionId === undefined) return;
  applyTaskPrimaryProjection(
    taskId,
    conflict.primarySessionId,
    conflict.primarySessionState,
    storeApi,
  );
  applySessionPrimaryProjection(
    taskId,
    conflict.primarySessionId,
    conflict.primarySessionState,
    storeApi,
  );
}

function applySessionPrimaryProjection(
  taskId: string,
  primarySessionId: string | null,
  primarySessionState: TaskSession["state"] | undefined,
  storeApi: ReturnType<typeof useAppStoreApi>,
) {
  const state = storeApi.getState();
  for (const candidate of Object.values(state.taskSessions.items)) {
    if (candidate.task_id !== taskId) continue;
    const shouldBePrimary = primarySessionId !== null && candidate.id === primarySessionId;
    const stateChanged =
      shouldBePrimary &&
      primarySessionState !== undefined &&
      candidate.state !== primarySessionState;
    if (candidate.is_primary === shouldBePrimary && !stateChanged) continue;
    state.setTaskSession({
      ...candidate,
      is_primary: shouldBePrimary,
      ...(shouldBePrimary && primarySessionState ? { state: primarySessionState } : {}),
    });
  }
}

async function refreshPrimarySessionProjection(
  conflict: NonNullable<ReturnType<typeof planCommentAdmissionConflict>>,
  taskId: string,
  storeApi: ReturnType<typeof useAppStoreApi>,
) {
  const stateAtRequestStart = storeApi.getState();
  const sessionsAtRequestStart = listedTaskSessions(stateAtRequestStart, taskId);
  const sessionIdsAtRequestStart = new Set(sessionsAtRequestStart.map((session) => session.id));
  const activityEpochsAtRequestStart = captureTaskSessionActivityEpochs(
    stateAtRequestStart,
    taskId,
  );
  // Track every routing change, including a primary that changes away and back.
  let routingChanged = false;
  const unsubscribe = storeApi.subscribe((current, previous) => {
    if (
      taskPrimaryRoutingFingerprint(current, taskId) !==
      taskPrimaryRoutingFingerprint(previous, taskId)
    ) {
      routingChanged = true;
    }
  });
  try {
    const response = await listTaskSessions(taskId, { cache: "no-store" });
    if (routingChanged) return;
    const fetchedSessions = response.sessions ?? [];
    const fetchedSessionIds = new Set(fetchedSessions.map((session) => session.id));
    const sessionsAddedDuringLoad = listedTaskSessions(storeApi.getState(), taskId).filter(
      (session) => !sessionIdsAtRequestStart.has(session.id) && !fetchedSessionIds.has(session.id),
    );
    const sessions = [...fetchedSessions, ...sessionsAddedDuringLoad];
    storeApi.getState().setTaskSessionsForTask(taskId, sessions, activityEpochsAtRequestStart);
    const primary = sessions.find((candidate) => candidate.is_primary);
    applyTaskPrimaryProjection(taskId, primary?.id ?? null, primary?.state, storeApi);
    applySessionPrimaryProjection(taskId, primary?.id ?? null, primary?.state, storeApi);
  } catch (error) {
    if (routingChanged) return;
    // i18n-exempt: conflict recovery still applies the authoritative payload below.
    console.error("Failed to refresh task sessions after primary-session conflict:", error);
    applyPrimarySessionChangeFallback(conflict, taskId, storeApi);
  } finally {
    unsubscribe();
  }
}

async function normalizePlanCommentRunError(
  error: unknown,
  taskId: string,
  storeApi: ReturnType<typeof useAppStoreApi>,
): Promise<PlanCommentRunError> {
  if (error instanceof PlanCommentRunError) return error;
  const conflict = planCommentAdmissionConflict(error);
  if (conflict?.snapshot) storeApi.getState().setTaskPlanComments(taskId, conflict.snapshot);
  if (conflict?.code === "plan_comments_changed") {
    return new PlanCommentRunError("plan-comments-changed");
  }
  if (conflict?.code === "primary_session_changed") {
    await refreshPrimarySessionProjection(conflict, taskId, storeApi);
    return new PlanCommentRunError("primary-session-changed");
  }
  return new PlanCommentRunError("delivery-failed");
}

async function runPersistedPlanComment(
  comment: PlanComment,
  taskId: string,
  storeApi: ReturnType<typeof useAppStoreApi>,
  clientAdmissionId: string,
) {
  try {
    return await runTaskPlanComment(comment, taskId, storeApi, clientAdmissionId);
  } catch (error) {
    // i18n-exempt: developer diagnostic; callers render localized errors.
    console.error("Failed to run task plan comment:", error);
    throw await normalizePlanCommentRunError(error, taskId, storeApi);
  }
}

async function runSessionComment({
  comment,
  sessionId,
  taskId,
  storeApi,
  markCommentsSent,
}: {
  comment: Comment;
  sessionId: string;
  taskId: string;
  storeApi: ReturnType<typeof useAppStoreApi>;
  markCommentsSent: (commentIds: string[]) => void;
}): Promise<{ queued: boolean }> {
  const state = storeApi.getState();
  const activeSession = state.taskSessions.items[sessionId] ?? null;
  const queuedCount = state.queue.metaBySessionId[sessionId]?.count ?? 0;
  const inputMode = deriveSessionInputMode(activeSession, queuedCount);
  const planModeEnabled = state.chatInput.planModeBySessionId[sessionId] ?? false;
  const content = formatSingleComment(comment);

  if (inputMode === "unavailable") {
    throw new Error("Session is not available for input");
  }
  if (inputMode === "queue") {
    if (!activeSession?.queue_incarnation_id) {
      throw new Error("Session is not available for input");
    }
    await appendToQueue(
      buildQueuePayload(
        sessionId,
        taskId,
        activeSession.queue_incarnation_id,
        content,
        planModeEnabled,
      ),
    );
  } else {
    const client = getWebSocketClient();
    if (!client) throw new Error("WebSocket client unavailable");
    const created = await client.request<Message | undefined>(
      "message.add",
      buildMessagePayload(sessionId, taskId, content, planModeEnabled, comment),
      10000,
    );
    if (created?.id && created.session_id) storeApi.getState().addMessage(created);
  }

  markCommentsSent([comment.id]);
  return { queued: inputMode === "queue" };
}

/**
 * Hook that provides a function to immediately send a comment to the agent.
 *
 * If the agent is idle, sends as a direct message.
 * If the agent is busy, appends to the queued message (or creates a new one).
 *
 * The busy check reads fresh state from the store at call time to avoid
 * stale closures that could incorrectly queue comments when the agent is idle.
 */
export function useRunComment({ sessionId, taskId }: UseRunCommentParams) {
  const markCommentsSent = useCommentsStore((s) => s.markCommentsSent);
  const storeApi = useAppStoreApi();
  const pendingPlanCommentAdmissionRef = useRef<{ key: string; id: string } | null>(null);

  const runComment = useCallback(
    async (comment: Comment): Promise<{ queued: boolean }> => {
      if (!taskId) return { queued: false };
      if (comment.source === "plan") {
        const admissionKey = JSON.stringify({
          taskId,
          commentId: comment.id,
          version: comment.version,
          text: comment.text,
        });
        if (pendingPlanCommentAdmissionRef.current?.key !== admissionKey) {
          pendingPlanCommentAdmissionRef.current = { key: admissionKey, id: generateUUID() };
        }
        const admission = pendingPlanCommentAdmissionRef.current;
        const result = await runPersistedPlanComment(comment, taskId, storeApi, admission.id);
        if (pendingPlanCommentAdmissionRef.current === admission) {
          pendingPlanCommentAdmissionRef.current = null;
        }
        return result;
      }
      if (!sessionId) return { queued: false };
      try {
        return await runSessionComment({ comment, sessionId, taskId, storeApi, markCommentsSent });
      } catch (error) {
        console.error("Failed to send comment to agent:", error);
        throw error;
      }
    },
    [sessionId, taskId, storeApi, markCommentsSent],
  );

  return { runComment };
}
