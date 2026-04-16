import type { Comment, DiffComment, PlanComment, PRFeedbackComment } from "./types";
import { isDiffComment, isPlanComment, isPRFeedbackComment } from "./types";

/**
 * Format diff review comments as human-readable markdown for sending to agent.
 */
export function formatReviewCommentsAsMarkdown(comments: DiffComment[]): string {
  if (!comments || comments.length === 0) return "";

  const lines: string[] = ["### Review Comments", ""];

  const byFile = new Map<string, DiffComment[]>();
  for (const comment of comments) {
    const existing = byFile.get(comment.filePath) || [];
    existing.push(comment);
    byFile.set(comment.filePath, existing);
  }

  for (const [filePath, fileComments] of byFile) {
    for (const comment of fileComments) {
      const lineRange =
        comment.startLine === comment.endLine
          ? `${comment.startLine}`
          : `${comment.startLine}-${comment.endLine}`;

      lines.push(`**${filePath}:${lineRange}**`);
      lines.push("```");
      lines.push(comment.codeContent);
      lines.push("```");
      lines.push(`> ${comment.text}`);
      lines.push("");
    }
  }

  lines.push("---");
  lines.push("");
  return lines.join("\n");
}

/**
 * Format plan comments as markdown for sending to agent.
 * Uses the same style as code review comments: code block for selected text + blockquote for comment.
 */
export function formatPlanCommentsAsMarkdown(comments: PlanComment[]): string {
  if (!comments || comments.length === 0) return "";

  const lines: string[] = ["### Plan Comments", ""];

  for (const c of comments) {
    if (c.selectedText) {
      lines.push("```");
      lines.push(c.selectedText);
      lines.push("```");
    }
    lines.push(`> ${c.text}`);
    lines.push("");
  }

  lines.push("---");
  lines.push("");
  return lines.join("\n");
}

/**
 * Format PR feedback comments as markdown for sending to agent.
 */
export function formatPRFeedbackAsMarkdown(comments: PRFeedbackComment[]): string {
  if (!comments || comments.length === 0) return "";

  const lines: string[] = ["### PR Feedback", ""];
  for (const c of comments) {
    lines.push(c.content);
    lines.push("");
  }
  lines.push("---");
  lines.push("");
  return lines.join("\n");
}

/**
 * Format all pending comments for inclusion in a chat message.
 */
export function formatCommentsForMessage(comments: Comment[]): {
  diffComments: DiffComment[];
  planComments: PlanComment[];
  prFeedbackComments: PRFeedbackComment[];
} {
  const diffComments: DiffComment[] = [];
  const planComments: PlanComment[] = [];
  const prFeedbackComments: PRFeedbackComment[] = [];

  for (const c of comments) {
    if (isDiffComment(c)) diffComments.push(c);
    else if (isPlanComment(c)) planComments.push(c);
    else if (isPRFeedbackComment(c)) prFeedbackComments.push(c);
  }

  return { diffComments, planComments, prFeedbackComments };
}
