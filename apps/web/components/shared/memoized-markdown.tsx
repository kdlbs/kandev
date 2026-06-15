"use client";

import { memo, useMemo } from "react";
import ReactMarkdown from "react-markdown";
import {
  MarkdownFileLinkContext,
  markdownComponents,
  remarkPlugins,
  type MarkdownFileLinkContextValue,
} from "@/components/shared/markdown-components";
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
};

export const MemoizedMarkdown = memo(function MemoizedMarkdown({
  content,
  worktreePath,
  onOpenFile,
}: MemoizedMarkdownProps) {
  const fileLinkContext = useMemo(() => ({ worktreePath, onOpenFile }), [worktreePath, onOpenFile]);

  return (
    <MarkdownFileLinkContext.Provider value={fileLinkContext}>
      <ReactMarkdown remarkPlugins={remarkPlugins} components={markdownComponents}>
        {normalizeCached(content)}
      </ReactMarkdown>
    </MarkdownFileLinkContext.Provider>
  );
});
