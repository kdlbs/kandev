"use client";

import { memo, useContext, useMemo } from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import {
  MarkdownFileLinkContext,
  MarkdownTaskContext,
  markdownComponents,
  remarkPlugins,
  type MarkdownFileLinkContextValue,
} from "@/components/shared/markdown-components";
import { ChatMotionSpan, useChatMarkdownMotion } from "./chat-markdown-motion";
import { normalizeCached } from "@/lib/markdown/normalize-cache";

/**
 * Markdown renderer behind a `memo` boundary keyed on the `content` string.
 *
 * `content` is a primitive, so React compares it by value. Keep optional
 * file-link props stable at the caller when possible; identical props bail out
 * of the memo and re-use the previously parsed element tree. The normalized
 * string itself is also cached (LRU) so two messages with the same content
 * share a single normalize pass.
 */
type MemoizedMarkdownProps = MarkdownFileLinkContextValue & {
  content: string;
  components?: Components;
  taskId?: string | null;
  animateText?: boolean;
};

export const MemoizedMarkdown = memo(function MemoizedMarkdown({
  content,
  worktreePath,
  onOpenFile,
  fileRootAliases,
  components,
  taskId = null,
  animateText = false,
}: MemoizedMarkdownProps) {
  const inheritedContext = useContext(MarkdownFileLinkContext);
  const fileLinkContext = useMemo(
    () => ({
      worktreePath: worktreePath ?? inheritedContext.worktreePath,
      onOpenFile: onOpenFile ?? inheritedContext.onOpenFile,
      fileRootAliases: fileRootAliases ?? inheritedContext.fileRootAliases,
    }),
    [
      fileRootAliases,
      inheritedContext.fileRootAliases,
      inheritedContext.onOpenFile,
      inheritedContext.worktreePath,
      onOpenFile,
      worktreePath,
    ],
  );
  const normalized = normalizeCached(content);
  const motionPlugins = useChatMarkdownMotion(normalized, animateText);
  const resolvedComponents = useMemo(
    () =>
      animateText
        ? { ...(components ?? markdownComponents), span: ChatMotionSpan }
        : (components ?? markdownComponents),
    [animateText, components],
  );

  return (
    <MarkdownTaskContext.Provider value={taskId}>
      <MarkdownFileLinkContext.Provider value={fileLinkContext}>
        <ReactMarkdown
          remarkPlugins={remarkPlugins}
          rehypePlugins={motionPlugins}
          components={resolvedComponents}
        >
          {normalized}
        </ReactMarkdown>
      </MarkdownFileLinkContext.Provider>
    </MarkdownTaskContext.Provider>
  );
});
