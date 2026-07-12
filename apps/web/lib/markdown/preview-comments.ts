import type { DiffComment } from "@/lib/state/slices/comments";
import { buildDiffComment } from "@/lib/diff/comment-utils";
import type { SourceLineRange } from "./source-line-ranges";

export type BuildMarkdownPreviewCommentArgs = SourceLineRange & {
  filePath: string;
  repositoryId?: string;
  sessionId: string;
  selectedText: string;
  text: string;
};

export function buildMarkdownPreviewComment(args: BuildMarkdownPreviewCommentArgs): DiffComment {
  const comment = buildDiffComment({
    filePath: args.filePath,
    sessionId: args.sessionId,
    startLine: args.startLine,
    endLine: args.endLine,
    side: "additions",
    text: args.text,
    codeContent: args.selectedText,
  });
  if (args.repositoryId) comment.repositoryId = args.repositoryId;
  return comment;
}

export function commentsOverlapRange(comments: DiffComment[], range: SourceLineRange): boolean {
  return comments.some(
    (comment) => comment.startLine <= range.endLine && comment.endLine >= range.startLine,
  );
}

export function commentsBeginInRange(comments: DiffComment[], range: SourceLineRange): boolean {
  return comments.some(
    (comment) => comment.startLine >= range.startLine && comment.startLine <= range.endLine,
  );
}
