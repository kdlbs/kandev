import { useCallback } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { appendToQueue } from "@/lib/api/domains/queue-api";
import { useAppStoreApi } from "@/components/state-provider";
import { useCommentsStore } from "@/lib/state/slices/comments";
import {
  formatReviewCommentsAsMarkdown,
  formatPlanCommentsAsMarkdown,
  formatPRFeedbackAsMarkdown,
} from "@/lib/state/slices/comments/format";
import type {
  Comment,
  DiffComment,
  PlanComment,
  FileEditorComment,
  PRFeedbackComment,
} from "@/lib/state/slices/comments";

/**
 * Format a single comment into markdown suitable for sending to the agent.
 */
function formatSingleComment(comment: Comment): string {
  switch (comment.source) {
    case "diff":
      return formatReviewCommentsAsMarkdown([comment as DiffComment]);
    case "plan":
      return formatPlanCommentsAsMarkdown([comment as PlanComment]);
    case "pr-feedback":
      return formatPRFeedbackAsMarkdown([comment as PRFeedbackComment]);
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
  content: string;
  plan_mode?: boolean;
  has_review_comments?: boolean;
};

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
  const payload: MessagePayload = { task_id: taskId, session_id: sessionId, content };
  if (planModeEnabled) payload.plan_mode = true;
  if (comment.source !== "plan") payload.has_review_comments = true;
  return payload;
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
      if (!sessionId || !taskId) return { queued: false };

      // Read all derived values fresh from the store at call time to avoid
      // stale closures that could cause incorrect behavior (e.g. queuing
      // comments when the agent is idle, or sending with wrong plan mode).
      const state = storeApi.getState();
      const activeSession = state.taskSessions.items[sessionId] ?? null;
      const isAgentBusy = activeSession?.state === "STARTING" || activeSession?.state === "RUNNING";
      const planModeEnabled = state.chatInput.planModeBySessionId[sessionId] ?? false;
      const content = formatSingleComment(comment);

      try {
        if (isAgentBusy) {
          await appendToQueue(buildQueuePayload(sessionId, taskId, content, planModeEnabled));
        } else {
          const client = getWebSocketClient();
          if (!client) throw new Error("WebSocket client unavailable");
          await client.request(
            "message.add",
            buildMessagePayload(sessionId, taskId, content, planModeEnabled, comment),
            10000,
          );
        }

        markCommentsSent([comment.id]);
        return { queued: isAgentBusy };
      } catch (error) {
        console.error("Failed to send comment to agent:", error);
        throw error;
      }
    },
    [sessionId, taskId, storeApi, markCommentsSent],
  );

  return { runComment };
}
