import type { Comment, DiffComment, PlanComment } from "./types";
import { isDiffComment, isPlanComment } from "./types";

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
 */
export function formatPlanCommentsAsMarkdown(comments: PlanComment[]): string {
  if (!comments || comments.length === 0) return "";

  const lines: string[] = [];
  for (let i = 0; i < comments.length; i++) {
    const c = comments[i];
    lines.push(`Comment ${i + 1}:`);
    lines.push(`- Selected text: "${c.selectedText}"`);
    lines.push(`- Comment: "${c.text}"`);
  }
  return lines.join("\n");
}

/**
 * Format all pending comments for inclusion in a chat message.
 */
export function formatCommentsForMessage(comments: Comment[]): {
  diffComments: DiffComment[];
  planComments: PlanComment[];
} {
  const diffComments: DiffComment[] = [];
  const planComments: PlanComment[] = [];

  for (const c of comments) {
    if (isDiffComment(c)) diffComments.push(c);
    else if (isPlanComment(c)) planComments.push(c);
  }

  return { diffComments, planComments };
}
