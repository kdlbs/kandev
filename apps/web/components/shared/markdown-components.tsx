"use client";

import { createContext, isValidElement, useContext, type MouseEvent, type ReactNode } from "react";
import remarkGfm from "remark-gfm";
import remarkBreaks from "remark-breaks";
import remarkGemoji from "remark-gemoji";
import { InlineCode } from "@/components/task/chat/messages/inline-code";
import { CodeBlock } from "@/components/task/chat/messages/code-block";
import { MermaidBlock } from "@/components/shared/mermaid-block";
import { ResizableMarkdownTable } from "@/components/shared/resizable-markdown-table";
import { isMermaidContent } from "@/components/editors/tiptap/tiptap-mermaid-extension";
import { usePanelActions } from "@/hooks/use-panel-actions";
import { useAppStore } from "@/components/state-provider";
import { getSessionWorkspacePath } from "@/lib/session-workspace-path";
import {
  resolveMarkdownFileTarget,
  type MarkdownFileRootAlias,
} from "@/lib/markdown/file-link-target";

/** Shared remark plugins used by all markdown renderers */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export const remarkPlugins: any[] = [remarkGfm, remarkBreaks, remarkGemoji];

// `normalizeMarkdown` (pure string transform) and its cached variant live in
// the React-free markdown cache module. Re-exported here so existing importers
// keep working.
export { normalizeMarkdown } from "@/lib/markdown/normalize-cache";

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

export type MarkdownFileLinkContextValue = {
  worktreePath?: string | null;
  onOpenFile?: (path: string) => void;
  fileRootAliases?: readonly MarkdownFileRootAlias[];
};

export const MarkdownFileLinkContext = createContext<MarkdownFileLinkContextValue>({});
export const MarkdownTaskContext = createContext<string | null>(null);

function isBlockCode(rawContent: string, hasLanguage: boolean): boolean {
  return hasLanguage || rawContent.includes("\n");
}

type MarkdownLinkProps = {
  href?: string;
  children?: ReactNode;
};

function MarkdownFileAnchor({
  href,
  children,
  worktreePath,
  openFile,
  fileRootAliases,
}: MarkdownLinkProps & {
  worktreePath: string | null | undefined;
  openFile: (path: string) => void;
  fileRootAliases?: readonly MarkdownFileRootAlias[];
}) {
  const target = resolveMarkdownFileTarget(href, {
    workspaceRoot: worktreePath,
    fileRootAliases,
  });
  const filePath = target?.kind === "file" ? target.path : null;
  const isBlocked = target?.kind === "blocked";
  const isInternal =
    !!target || (href?.startsWith("/") && !href.startsWith("//")) || href?.startsWith("#");

  const handleClick = target
    ? (event: MouseEvent<HTMLAnchorElement>) => {
        event.preventDefault();
        if (filePath) openFile(filePath);
      }
    : undefined;

  return (
    <a
      href={href}
      target={isInternal ? "_self" : "_blank"}
      rel={isInternal ? undefined : "noopener noreferrer"}
      onClick={handleClick}
      aria-disabled={isBlocked ? true : undefined}
    >
      {children}
    </a>
  );
}

function MarkdownFallbackLink(
  props: MarkdownLinkProps & {
    worktreePath?: string | null;
    fileRootAliases?: readonly MarkdownFileRootAlias[];
  },
) {
  const { openFile } = usePanelActions();
  const activeWorktreePath = useAppStore((state) => {
    const sessionId = state.tasks.activeSessionId;
    if (!sessionId) return null;
    return getSessionWorkspacePath(state.taskSessions.items[sessionId]);
  });

  return (
    <MarkdownFileAnchor
      {...props}
      worktreePath={props.worktreePath ?? activeWorktreePath}
      fileRootAliases={props.fileRootAliases}
      openFile={openFile}
    />
  );
}

function MarkdownLink(props: MarkdownLinkProps) {
  const linkContext = useContext(MarkdownFileLinkContext);
  if (linkContext.onOpenFile) {
    return (
      <MarkdownFileAnchor
        {...props}
        worktreePath={linkContext.worktreePath}
        fileRootAliases={linkContext.fileRootAliases}
        openFile={linkContext.onOpenFile}
      />
    );
  }
  return (
    <MarkdownFallbackLink
      {...props}
      worktreePath={linkContext.worktreePath}
      fileRootAliases={linkContext.fileRootAliases}
    />
  );
}

function MarkdownCode({ className, children }: MarkdownCodeProps) {
  const taskId = useContext(MarkdownTaskContext);
  const rawContent = getTextContent(children);
  const content = rawContent.replace(/\n$/, "");
  const lang = className?.replace("language-", "") ?? null;
  const hasLanguage = className?.startsWith("language-") ?? false;
  const isBlock = isBlockCode(rawContent, hasLanguage);
  if (isBlock && isMermaidContent(lang, content)) {
    return <MermaidBlock code={content} taskId={taskId} />;
  }
  if (isBlock) {
    return <CodeBlock className={className}>{content}</CodeBlock>;
  }
  return <InlineCode>{content}</InlineCode>;
}

/**
 * Shared markdown component overrides for ReactMarkdown.
 * Element styles (headings, lists, inline code, etc.) are handled by
 * the `.markdown-body` CSS class in globals.css. Only behavioral overrides
 * (code routing, link target, table overflow wrapper) remain here.
 */
export const markdownComponents = {
  code: MarkdownCode,
  a: MarkdownLink,
  table: ResizableMarkdownTable,
};
