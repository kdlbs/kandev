"use client";

import { memo } from "react";
import ReactMarkdown from "react-markdown";
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
};

const localRemarkPlugins = [...remarkPlugins, remarkTaskLinks];

// Extend the default sanitize schema to allow relative hrefs (for task links like /office/tasks/KAN-42).
// The default schema only allows http/https/irc/mailto protocols, stripping relative paths.
const { href: _hrefProto, ...otherProtocols } = defaultSchema.protocols ?? {};
const sanitizeSchema = withMarkdownMathSanitizeSchema({
  ...defaultSchema,
  protocols: otherProtocols,
});

export const MarkdownComment = memo(function MarkdownComment({ content }: MarkdownCommentProps) {
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
