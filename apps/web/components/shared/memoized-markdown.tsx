"use client";

import { memo } from "react";
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
 * `content` is a primitive, so React compares it by value. Object-ref churn
 * from a message refetch can therefore never trigger a re-parse: an identical
 * string bails out of the memo and re-uses the previously parsed element tree.
 * The normalized string itself is also cached (LRU) so two messages with the
 * same content share a single normalize pass.
 */
type MemoizedMarkdownProps = MarkdownFileLinkContextValue & {
  content: string;
};

export const MemoizedMarkdown = memo(function MemoizedMarkdown({
  content,
  worktreePath,
  onOpenFile,
}: MemoizedMarkdownProps) {
  return (
    <MarkdownFileLinkContext.Provider value={{ worktreePath, onOpenFile }}>
      <ReactMarkdown remarkPlugins={remarkPlugins} components={markdownComponents}>
        {normalizeCached(content)}
      </ReactMarkdown>
    </MarkdownFileLinkContext.Provider>
  );
});
