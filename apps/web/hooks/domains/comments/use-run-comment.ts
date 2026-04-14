import { useCallback } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { appendToQueue } from "@/lib/api/domains/queue-api";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
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
  const planModeEnabled = useAppStore((s) =>
    sessionId ? (s.chatInput.planModeBySessionId[sessionId] ?? false) : false,
  );

  const runComment = useCallback(
    async (comment: Comment): Promise<{ queued: boolean }> => {
      if (!sessionId || !taskId) return { queued: false };

      // Read session state fresh at call time to avoid stale closures.
      // Previously, isAgentBusy was captured in the useCallback closure and
      // could be stale if the session state changed between the last render
      // and the user clicking "Run".
      const state = storeApi.getState();
      const sid = state.tasks.activeSessionId;
      const activeSession = sid ? (state.taskSessions.items[sid] ?? null) : null;
      const isAgentBusy = activeSession?.state === "STARTING" || activeSession?.state === "RUNNING";

      const content = formatSingleComment(comment);

      try {
        if (isAgentBusy) {
          await appendToQueue({
            session_id: sessionId,
            task_id: taskId,
            content,
            ...(planModeEnabled && { plan_mode: true }),
          });
        } else {
          const client = getWebSocketClient();
          if (!client) return { queued: false };

          await client.request(
            "message.add",
            {
              task_id: taskId,
              session_id: sessionId,
              content,
              ...(planModeEnabled && { plan_mode: true }),
              ...(comment.source !== "plan" && { has_review_comments: true }),
            },
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
    [sessionId, taskId, storeApi, planModeEnabled, markCommentsSent],
  );

  return { runComment };
}
