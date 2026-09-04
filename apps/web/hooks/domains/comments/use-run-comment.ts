import { useCallback } from "react";
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
import { findTaskInSnapshots } from "@/lib/kanban/find-task";

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

function applyTaskPrimaryProjection(
  taskId: string,
  primarySessionId: string,
  primarySessionState: TaskSession["state"] | undefined,
  storeApi: ReturnType<typeof useAppStoreApi>,
) {
  storeApi.setState((state) => {
    const updateTasks = (tasks: typeof state.kanban.tasks) =>
      tasks.map((task) =>
        task.id === taskId
          ? {
              ...task,
              primarySessionId,
              ...(primarySessionState ? { primarySessionState } : {}),
            }
          : task,
      );
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
      | "plan-comment-migration-pending"
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
    case "plan-comment-migration-pending":
      return t("task:planCommentMigrationPending");
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
): Promise<{ queued: boolean }> {
  if (storeApi.getState().taskPlans.commentsMigrationStatusByTaskId[taskId] !== "complete") {
    throw new PlanCommentRunError("plan-comment-migration-pending");
  }
  if (!comment.version || comment.version < 1) {
    throw new PlanCommentRunError("plan-comment-not-persisted");
  }
  const availability = resolvePlanCommentRunAvailability(storeApi.getState(), taskId);
  if (availability.reason || !availability.session) {
    throw new PlanCommentRunError(availability.reason ?? NO_PRIMARY_SESSION);
  }
  const planCommentRefs = [{ id: comment.id, version: comment.version }];
  if (availability.inputMode === "queue") {
    await queueMessage({
      session_id: availability.session.id,
      task_id: taskId,
      client_queue_id: generateUUID(),
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
    clientMessageId: generateUUID(),
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
  content: string,
  planModeEnabled: boolean,
): QueuePayload {
  const payload: QueuePayload = { session_id: sessionId, task_id: taskId, content };
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

function applyPrimarySessionChange(
  conflict: NonNullable<ReturnType<typeof planCommentAdmissionConflict>>,
  taskId: string,
  storeApi: ReturnType<typeof useAppStoreApi>,
) {
  if (!conflict.primarySessionId) return;
  applyTaskPrimaryProjection(
    taskId,
    conflict.primarySessionId,
    conflict.primarySessionState,
    storeApi,
  );
  const state = storeApi.getState();
  for (const candidate of Object.values(state.taskSessions.items)) {
    if (candidate.task_id !== taskId) continue;
    const shouldBePrimary = candidate.id === conflict.primarySessionId;
    if (candidate.is_primary === shouldBePrimary && !shouldBePrimary) continue;
    if (!shouldBePrimary || conflict.primarySessionState) {
      state.setTaskSession({
        ...candidate,
        is_primary: shouldBePrimary,
        ...(shouldBePrimary && conflict.primarySessionState
          ? { state: conflict.primarySessionState }
          : {}),
      });
    }
  }
}

function normalizePlanCommentRunError(
  error: unknown,
  taskId: string,
  storeApi: ReturnType<typeof useAppStoreApi>,
): PlanCommentRunError {
  if (error instanceof PlanCommentRunError) return error;
  const conflict = planCommentAdmissionConflict(error);
  if (conflict?.snapshot) storeApi.getState().setTaskPlanComments(taskId, conflict.snapshot);
  if (conflict?.code === "plan_comments_changed") {
    return new PlanCommentRunError("plan-comments-changed");
  }
  if (conflict?.code === "primary_session_changed") {
    applyPrimarySessionChange(conflict, taskId, storeApi);
    return new PlanCommentRunError("primary-session-changed");
  }
  return new PlanCommentRunError("delivery-failed");
}

async function runPersistedPlanComment(
  comment: PlanComment,
  taskId: string,
  storeApi: ReturnType<typeof useAppStoreApi>,
) {
  try {
    return await runTaskPlanComment(comment, taskId, storeApi);
  } catch (error) {
    // i18n-exempt: developer diagnostic; callers render localized errors.
    console.error("Failed to run task plan comment:", error);
    throw normalizePlanCommentRunError(error, taskId, storeApi);
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
    await appendToQueue(buildQueuePayload(sessionId, taskId, content, planModeEnabled));
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

  const runComment = useCallback(
    async (comment: Comment): Promise<{ queued: boolean }> => {
      if (!taskId) return { queued: false };
      if (comment.source === "plan") {
        return runPersistedPlanComment(comment, taskId, storeApi);
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
