"use client";

import { isValidElement, type ReactNode } from "react";
import remarkGfm from "remark-gfm";
import remarkBreaks from "remark-breaks";
import remarkGemoji from "remark-gemoji";
import { InlineCode } from "@/components/task/chat/messages/inline-code";
import { CodeBlock } from "@/components/task/chat/messages/code-block";
import { MermaidBlock } from "@/components/shared/mermaid-block";
import { isMermaidContent } from "@/components/editors/tiptap/tiptap-mermaid-extension";

/** Shared remark plugins used by all markdown renderers */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export const remarkPlugins: any[] = [remarkGfm, remarkBreaks, remarkGemoji];

/**
 * Recursively extracts text content from React children.
 * Optimized with fast paths for common cases (string/number).
 */
export function getTextContent(children: ReactNode): string {
  if (typeof children === "string") return children;
  if (typeof children === "number") return String(children);
  if (children == null) return "";

  if (Array.isArray(children)) {
    let result = "";
    for (let i = 0; i < children.length; i++) {
      result += getTextContent(children[i]);
    }
    return result;
  }

  if (isValidElement(children)) {
    const props = children.props as { children?: ReactNode };
    if (props.children) {
      return getTextContent(props.children);
    }
  }
  return "";
}

/**
 * Shared markdown component overrides for ReactMarkdown.
 * Element styles (headings, lists, inline code, etc.) are handled by
 * the `.markdown-body` CSS class in globals.css. Only behavioral overrides
 * (code routing, link target, table overflow wrapper) remain here.
 */
export const markdownComponents = {
  code: ({ className, children }: { className?: string; children?: ReactNode }) => {
    const content = getTextContent(children).replace(/\n$/, "");
    const lang = className?.replace("language-", "") ?? null;
    if (isMermaidContent(lang, content)) {
      return <MermaidBlock code={content} />;
    }
    const hasLanguage = className?.startsWith("language-");
    const hasNewlines = content.includes("\n");
    if (hasLanguage || hasNewlines) {
      return <CodeBlock className={className}>{content}</CodeBlock>;
    }
    return <InlineCode>{content}</InlineCode>;
  },
  a: ({ href, children }: { href?: string; children?: ReactNode }) => (
    <a href={href} target="_blank" rel="noopener noreferrer">
      {children}
    </a>
  ),
  table: ({ children }: { children?: ReactNode }) => (
    <div className="overflow-x-auto">
      <table>{children}</table>
    </div>
  ),
};
