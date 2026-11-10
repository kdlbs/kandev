"use client";

import { memo, useMemo } from "react";
import ReactMarkdown from "react-markdown";
import type { PluggableList } from "unified";
import rehypeSanitize, { defaultSchema } from "rehype-sanitize";
import {
  markdownComponents,
  rehypePlugins,
  remarkPlugins,
} from "@/components/shared/markdown-components";
import { withMarkdownMathSanitizeSchema } from "@/lib/markdown/math-sanitize-schema";
import { remarkTaskLinks } from "./task-identifier-link";

type MarkdownCommentProps = {
  content: string;
  /** Workspace task prefix (e.g. "KAN"); only matching identifiers are linked. */
  taskPrefix?: string;
  /** Workspace the prefix belongs to; carried in task links so they resolve there. */
  workspaceId?: string;
};

// Extend the default sanitize schema to allow relative hrefs (for task links like /office/tasks/KAN-42).
// The default schema only allows http/https/irc/mailto protocols, stripping relative paths.
const { href: _hrefProto, ...otherProtocols } = defaultSchema.protocols ?? {};
const sanitizeSchema = withMarkdownMathSanitizeSchema({
  ...defaultSchema,
  protocols: otherProtocols,
});

export const MarkdownComment = memo(function MarkdownComment({
  content,
  taskPrefix,
  workspaceId,
}: MarkdownCommentProps) {
  const localRemarkPlugins = useMemo(
    (): PluggableList => [...remarkPlugins, [remarkTaskLinks, { prefix: taskPrefix, workspaceId }]],
    [taskPrefix, workspaceId],
  );
  return (
    <div className="prose prose-sm max-w-none text-sm markdown-body">
      <ReactMarkdown
        remarkPlugins={localRemarkPlugins}
        rehypePlugins={[[rehypeSanitize, sanitizeSchema], ...rehypePlugins]}
        components={markdownComponents}
      >
        {content}
      </ReactMarkdown>
    </div>
  );
});
