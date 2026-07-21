import type {
  Comment,
  DiffComment,
  PlanComment,
  PRFeedbackComment,
  WalkthroughComment,
  AgentMessageComment,
} from "./types";
import {
  isAgentMessageComment,
  isDiffComment,
  isPlanComment,
  isPRFeedbackComment,
  isWalkthroughComment,
} from "./types";

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

/** Convert text to blockquote, handling multiline text properly. */
export function toBlockquote(text: string): string {
  return text
    .split("\n")
    .map((line) => `> ${line}`)
    .join("\n");
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
    lines.push(toBlockquote(c.text));
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
 * Format walkthrough feedback as markdown for sending to agent.
 */
export function formatWalkthroughCommentsAsMarkdown(comments: WalkthroughComment[]): string {
  if (!comments || comments.length === 0) return "";

  const lines: string[] = ["### Walkthrough Feedback", ""];
  for (const c of comments) {
    const lineRange = c.startLine === c.endLine ? `${c.startLine}` : `${c.startLine}-${c.endLine}`;
    const repoPrefix = c.repo ? `${c.repo}/` : "";
    const title = c.walkthroughTitle || "Walkthrough";

    lines.push(`**${title} · Step ${c.stepIndex + 1} / ${c.stepCount}**`);
    lines.push(`**${repoPrefix}${c.filePath}:${lineRange}**`);
    lines.push("");
    lines.push("Agent walkthrough text:");
    lines.push(toBlockquote(c.stepText));
    lines.push("");
    lines.push("User feedback:");
    lines.push(toBlockquote(c.text));
    lines.push("");
  }

  lines.push("---");
  lines.push("");
  return lines.join("\n");
}

/** Format feedback anchored to ordinary agent replies. */
export function formatAgentMessageCommentsAsMarkdown(comments: AgentMessageComment[]): string {
  if (!comments || comments.length === 0) return "";

  const lines: string[] = ["### Agent Message Comments", ""];
  for (const comment of comments) {
    lines.push("Selected agent response:");
    lines.push(toBlockquote(comment.selectedText));
    lines.push("");
    lines.push("User feedback:");
    lines.push(toBlockquote(comment.text));
    lines.push("");
  }
  lines.push("---", "");
  return lines.join("\n");
}

/**
 * Format all pending comments for inclusion in a chat message.
 */
export function formatCommentsForMessage(comments: Comment[]): {
  diffComments: DiffComment[];
  planComments: PlanComment[];
  prFeedbackComments: PRFeedbackComment[];
  walkthroughComments: WalkthroughComment[];
  agentMessageComments: AgentMessageComment[];
} {
  const diffComments: DiffComment[] = [];
  const planComments: PlanComment[] = [];
  const prFeedbackComments: PRFeedbackComment[] = [];
  const walkthroughComments: WalkthroughComment[] = [];
  const agentMessageComments: AgentMessageComment[] = [];

  for (const c of comments) {
    if (isDiffComment(c)) diffComments.push(c);
    else if (isPlanComment(c)) planComments.push(c);
    else if (isPRFeedbackComment(c)) prFeedbackComments.push(c);
    else if (isWalkthroughComment(c)) walkthroughComments.push(c);
    else if (isAgentMessageComment(c)) agentMessageComments.push(c);
  }

  return {
    diffComments,
    planComments,
    prFeedbackComments,
    walkthroughComments,
    agentMessageComments,
  };
}
