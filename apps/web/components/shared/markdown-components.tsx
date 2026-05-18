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

const FENCE_OPEN_RE = /^ {0,3}(`{3,})/;
const TRAILING_FENCE_RE = /(`{3,})\s*$/;

function pureCloseLength(line: string, openCount: number): number | null {
  const match = /^ {0,3}(`{3,})\s*$/.exec(line);
  if (!match || match[1].length < openCount) return null;
  return match[1].length;
}

function gluedCloseLength(line: string, openCount: number): number | null {
  const match = TRAILING_FENCE_RE.exec(line);
  if (!match || match[1].length < openCount) return null;
  // Reject pure-fence lines (already handled by pureCloseLength) and lines
  // where everything before the trailing run is whitespace only.
  const head = line.slice(0, line.length - match[0].length).trimEnd();
  if (head.length === 0) return null;
  return match[1].length;
}

/**
 * Pre-process a markdown string to repair fenced code blocks that have their
 * closing fence glued to the last code line (`...}\`\`\`\n`prose`). Without
 * this, CommonMark/GFM treats the glued backticks as code content, so the
 * fence never closes and following prose gets swallowed into one huge code
 * node. We split such lines into `<content>\n<backticks>` only when we're
 * inside an open fence whose opener run length is ≤ the trailing run length.
 *
 * Pure string preprocessing, intentionally not a remark plugin.
 */
export function normalizeMarkdown(input: string): string {
  if (!input || input.length === 0) return input;
  const hadTrailingNewline = input.endsWith("\n");
  const lines = input.split("\n");
  const out: string[] = [];
  let openCount: number | null = null;

  for (const line of lines) {
    if (openCount === null) {
      const opener = FENCE_OPEN_RE.exec(line);
      if (opener) openCount = opener[1].length;
      out.push(line);
      continue;
    }
    if (pureCloseLength(line, openCount) !== null) {
      openCount = null;
      out.push(line);
      continue;
    }
    const glued = gluedCloseLength(line, openCount);
    if (glued !== null) {
      const trailingMatch = TRAILING_FENCE_RE.exec(line)!;
      const head = line.slice(0, line.length - trailingMatch[0].length);
      out.push(head);
      out.push("`".repeat(glued));
      openCount = null;
      continue;
    }
    out.push(line);
  }

  const result = out.join("\n");
  return hadTrailingNewline && !result.endsWith("\n") ? result + "\n" : result;
}

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

type MarkdownCodeProps = {
  className?: string;
  children?: ReactNode;
};

function isBlockCode(rawContent: string, hasLanguage: boolean): boolean {
  return hasLanguage || rawContent.includes("\n");
}

/**
 * Shared markdown component overrides for ReactMarkdown.
 * Element styles (headings, lists, inline code, etc.) are handled by
 * the `.markdown-body` CSS class in globals.css. Only behavioral overrides
 * (code routing, link target, table overflow wrapper) remain here.
 */
export const markdownComponents = {
  code: ({ className, children }: MarkdownCodeProps) => {
    const rawContent = getTextContent(children);
    const content = rawContent.replace(/\n$/, "");
    const lang = className?.replace("language-", "") ?? null;
    const hasLanguage = className?.startsWith("language-") ?? false;
    const isBlock = isBlockCode(rawContent, hasLanguage);
    if (isBlock && isMermaidContent(lang, content)) {
      return <MermaidBlock code={content} />;
    }
    if (isBlock) {
      return <CodeBlock className={className}>{content}</CodeBlock>;
    }
    return <InlineCode>{content}</InlineCode>;
  },
  a: ({ href, children }: { href?: string; children?: ReactNode }) => {
    const isInternal = href?.startsWith("/") || href?.startsWith("#");
    return (
      <a
        href={href}
        target={isInternal ? "_self" : "_blank"}
        rel={isInternal ? undefined : "noopener noreferrer"}
      >
        {children}
      </a>
    );
  },
  table: ({ children }: { children?: ReactNode }) => (
    <div className="overflow-x-auto">
      <table>{children}</table>
    </div>
  ),
};
