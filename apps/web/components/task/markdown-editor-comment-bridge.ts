"use client";

import { useCallback, useMemo } from "react";
import { useDiffFileComments } from "@/hooks/domains/comments/use-diff-comments";
import { buildDiffComment } from "@/lib/diff/comment-utils";
import { useCommentsStore } from "@/lib/state/slices/comments";
import type {
  MarkdownComment,
  MarkdownCommentSubmission,
} from "@/components/editors/markdown/hybrid-markdown-editor";

function clampSourceOffset(content: string, offset: number): number {
  return Math.max(0, Math.min(content.length, offset));
}

export function sourceOffsetAtLine(content: string, line: number): number {
  const targetLine = Math.max(1, line);
  if (targetLine === 1) return 0;
  let currentLine = 1;
  for (let offset = 0; offset < content.length; offset += 1) {
    if (content[offset] !== "\n") continue;
    currentLine += 1;
    if (currentLine === targetLine) return offset + 1;
  }
  return content.length;
}

export function sourceLineEndOffset(content: string, line: number): number {
  const start = sourceOffsetAtLine(content, line);
  const newline = content.indexOf("\n", start);
  if (newline === -1) return content.length;
  return newline > start && content[newline - 1] === "\r" ? newline - 1 : newline;
}

export function sourceLinesAtOffsets(
  content: string,
  start: number,
  endExclusive: number,
): { startLine: number; endLine: number; selectedText: string } {
  const safeStart = clampSourceOffset(content, start);
  const safeEnd = Math.max(safeStart, clampSourceOffset(content, endExclusive));
  const lineAt = (offset: number) => {
    let line = 1;
    for (let index = 0; index < offset; index += 1) {
      if (content[index] === "\n") line += 1;
    }
    return line;
  };
  return {
    startLine: lineAt(safeStart),
    endLine: lineAt(Math.max(safeStart, safeEnd - 1)),
    selectedText: content.slice(safeStart, safeEnd),
  };
}

function sourceCommentRange(content: string, startLine: number, endLine: number) {
  const start = sourceOffsetAtLine(content, startLine);
  const end = Math.max(start, sourceLineEndOffset(content, endLine));
  return { start, endExclusive: end };
}

export function useMarkdownEditorCommentState({
  path,
  content,
  sessionId,
  repositoryId,
  enableComments,
  providedComments,
  onComment,
}: {
  path: string;
  content: string;
  sessionId?: string | null;
  repositoryId?: string | null;
  enableComments: boolean;
  providedComments?: readonly MarkdownComment[];
  onComment?: (comment: MarkdownCommentSubmission) => void;
}) {
  const fileComments = useDiffFileComments(sessionId ?? "", path, repositoryId ?? undefined);
  const addComment = useCommentsStore((state) => state.addComment);
  const hybridComments = useMemo<MarkdownComment[]>(
    () =>
      providedComments
        ? [...providedComments]
        : fileComments.map((comment) => {
            const range = sourceCommentRange(content, comment.startLine, comment.endLine);
            return {
              id: comment.id,
              start: range.start,
              endExclusive: range.endExclusive,
              body: comment.text,
            };
          }),
    [content, fileComments, providedComments],
  );

  const handleHybridComment = useCallback(
    (submission: MarkdownCommentSubmission) => {
      if (enableComments && sessionId) {
        const sourceLines = sourceLinesAtOffsets(
          content,
          submission.start,
          submission.endExclusive,
        );
        const comment = buildDiffComment({
          filePath: path,
          sessionId,
          startLine: sourceLines.startLine,
          endLine: sourceLines.endLine,
          side: "additions",
          text: submission.text,
          codeContent: sourceLines.selectedText,
        });
        if (repositoryId) comment.repositoryId = repositoryId;
        addComment(comment);
      }
      onComment?.(submission);
    },
    [addComment, content, enableComments, onComment, path, repositoryId, sessionId],
  );

  return { hybridComments, handleHybridComment };
}
