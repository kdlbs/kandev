import { useCallback } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { appendToQueue } from "@/lib/api/domains/queue-api";
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
  isAgentBusy: boolean;
};

/**
 * Hook that provides a function to immediately send a comment to the agent.
 *
 * If the agent is idle, sends as a direct message.
 * If the agent is busy, appends to the queued message (or creates a new one).
 */
export function useRunComment({ sessionId, taskId, isAgentBusy }: UseRunCommentParams) {
  const markCommentsSent = useCommentsStore((s) => s.markCommentsSent);

  const runComment = useCallback(
    async (comment: Comment) => {
      if (!sessionId || !taskId) return;

      const content = formatSingleComment(comment);

      try {
        if (isAgentBusy) {
          await appendToQueue({
            session_id: sessionId,
            task_id: taskId,
            content,
          });
        } else {
          const client = getWebSocketClient();
          if (!client) return;

          await client.request(
            "message.add",
            {
              task_id: taskId,
              session_id: sessionId,
              content,
              has_review_comments: true,
            },
            10000,
          );
        }

        markCommentsSent([comment.id]);
      } catch (error) {
        console.error("Failed to send comment to agent:", error);
        throw error;
      }
    },
    [sessionId, taskId, isAgentBusy, markCommentsSent],
  );

  return { runComment };
}
