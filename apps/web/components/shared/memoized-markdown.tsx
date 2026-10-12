"use client";

import { createElement, memo, useContext, useMemo, type ComponentProps } from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import {
  MarkdownFileLinkContext,
  MarkdownTaskContext,
  markdownComponents,
  rehypePlugins,
  remarkPlugins,
  type MarkdownFileLinkContextValue,
} from "@/components/shared/markdown-components";
import { ChatMotionSpan, useChatMarkdownMotion } from "./chat-markdown-motion";
import { normalizeCached } from "@/lib/markdown/normalize-cache";

const MAX_RENDER_ENTRIES = 128;
const MAX_RENDER_CHARACTERS = 512_000;
const renderedMarkdown = new Map<string, ReturnType<typeof ReactMarkdown>>();
let renderedCharacters = 0;

/** Cache only the synchronous library renderer's context-free element tree.
 * Task/file-link providers and animated or custom renderers remain per mount. */
function renderStaticMarkdown(content: string): ReturnType<typeof ReactMarkdown> {
  const cached = renderedMarkdown.get(content);
  if (cached) {
    renderedMarkdown.delete(content);
    renderedMarkdown.set(content, cached);
    return cached;
  }
  // This cache requires react-markdown's synchronous, hook-free export.
  // Re-check that contract when upgrading the dependency.
  const result = ReactMarkdown({
    children: content,
    remarkPlugins,
    rehypePlugins,
    components: markdownComponents,
  });
  if (content.length > MAX_RENDER_CHARACTERS) return result;
  while (
    renderedMarkdown.size >= MAX_RENDER_ENTRIES ||
    renderedCharacters + content.length > MAX_RENDER_CHARACTERS
  ) {
    const oldest = renderedMarkdown.keys().next().value;
    if (oldest === undefined) break;
    renderedMarkdown.delete(oldest);
    renderedCharacters -= oldest.length;
  }
  renderedMarkdown.set(content, result);
  renderedCharacters += content.length;
  return result;
}

/**
 * Markdown renderer behind a `memo` boundary keyed on the `content` string.
 *
 * `content` is a primitive, so React compares it by value. Keep optional
 * file-link props stable at the caller when possible; identical props bail out
 * of the memo and re-use the previously parsed element tree. The normalized
 * string and standard static element tree have bounded LRU caches,
 * so remounting a visited transcript does not repeat Markdown parsing.
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
  const resolvedComponents = useMemo(() => {
    const base: Components = components ?? markdownComponents;
    if (!animateText) return base;
    const Span = base.span ?? "span";
    return {
      ...base,
      span: (props: ComponentProps<typeof ChatMotionSpan>) => {
        const { node, ...attributes } = props;
        if (
          node?.properties?.["data-chat-text-motion"] !== undefined ||
          attributes["data-chat-text-motion"] !== undefined
        )
          return <ChatMotionSpan {...props} />;
        return typeof Span === "string" ? createElement(Span, attributes) : <Span {...props} />;
      },
    };
  }, [animateText, components]);

  return (
    <MarkdownTaskContext.Provider value={taskId}>
      <MarkdownFileLinkContext.Provider value={fileLinkContext}>
        {(!components || components === markdownComponents) && motionPlugins.length === 0 ? (
          renderStaticMarkdown(normalized)
        ) : (
          <ReactMarkdown
            remarkPlugins={remarkPlugins}
            rehypePlugins={[...rehypePlugins, ...motionPlugins]}
            components={resolvedComponents}
          >
            {normalized}
          </ReactMarkdown>
        )}
      </MarkdownFileLinkContext.Provider>
    </MarkdownTaskContext.Provider>
  );
});
