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
 * Used by both chat messages and PR comment/review rendering.
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
  ol: ({ children }: { children?: ReactNode }) => (
    <ol className="list-decimal pl-5 my-4">{children}</ol>
  ),
  ul: ({ children }: { children?: ReactNode }) => (
    <ul className="list-disc pl-5 my-4">{children}</ul>
  ),
  li: ({ children }: { children?: ReactNode }) => <li className="my-0.5">{children}</li>,
  a: ({ href, children }: { href?: string; children?: ReactNode }) => (
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      className="text-blue-600 dark:text-blue-400 hover:underline underline-offset-2 cursor-pointer"
    >
      {children}
    </a>
  ),
  p: ({ children }: { children?: ReactNode }) => <p className="leading-[1.625]">{children}</p>,
  h1: ({ children }: { children?: ReactNode }) => (
    <h1 className="mt-5 mb-1.5 font-bold text-xl first:mt-0">{children}</h1>
  ),
  h2: ({ children }: { children?: ReactNode }) => (
    <h2 className="mt-5 mb-1.5 font-bold text-[1.0625rem] first:mt-0">{children}</h2>
  ),
  h3: ({ children }: { children?: ReactNode }) => (
    <h3 className="mt-5 mb-1.5 font-bold text-[0.9375rem] first:mt-0">{children}</h3>
  ),
  h4: ({ children }: { children?: ReactNode }) => (
    <h4 className="mt-5 mb-1.5 font-bold first:mt-0">{children}</h4>
  ),
  h5: ({ children }: { children?: ReactNode }) => (
    <h5 className="mt-5 mb-1.5 font-bold first:mt-0">{children}</h5>
  ),
  hr: ({ children }: { children?: ReactNode }) => <hr className="my-5">{children}</hr>,
  table: ({ children }: { children?: ReactNode }) => (
    <div className="my-3 overflow-x-auto">
      <table className="border-collapse border border-border rounded-lg overflow-hidden">
        {children}
      </table>
    </div>
  ),
  thead: ({ children }: { children?: ReactNode }) => (
    <thead className="bg-muted/50">{children}</thead>
  ),
  tbody: ({ children }: { children?: ReactNode }) => (
    <tbody className="divide-y divide-border">{children}</tbody>
  ),
  tr: ({ children }: { children?: ReactNode }) => (
    <tr className="border-b border-border last:border-b-0 hover:bg-muted/50">{children}</tr>
  ),
  th: ({ children }: { children?: ReactNode }) => (
    <th className="px-3 py-2 text-left text-xs font-semibold text-foreground border-r border-border last:border-r-0">
      {children}
    </th>
  ),
  td: ({ children }: { children?: ReactNode }) => (
    <td className="px-3 py-2 text-xs text-muted-foreground border-r border-border last:border-r-0">
      {children}
    </td>
  ),
};
