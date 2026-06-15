"use client";

import {
  createContext,
  isValidElement,
  useCallback,
  useContext,
  type MouseEvent,
  type ReactNode,
} from "react";
import remarkGfm from "remark-gfm";
import remarkBreaks from "remark-breaks";
import remarkGemoji from "remark-gemoji";
import { InlineCode } from "@/components/task/chat/messages/inline-code";
import { CodeBlock } from "@/components/task/chat/messages/code-block";
import { MermaidBlock } from "@/components/shared/mermaid-block";
import { isMermaidContent } from "@/components/editors/tiptap/tiptap-mermaid-extension";
import { usePanelActions } from "@/hooks/use-panel-actions";
import { useAppStore } from "@/components/state-provider";

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
};

export const MarkdownFileLinkContext = createContext<MarkdownFileLinkContextValue>({});

function isBlockCode(rawContent: string, hasLanguage: boolean): boolean {
  return hasLanguage || rawContent.includes("\n");
}

function looksLikeFilePath(path: string): boolean {
  const lastSegment = path.split("/").pop() ?? "";
  return lastSegment.includes(".") && !path.endsWith("/");
}

function isExternalHref(href: string): boolean {
  return /^[a-z][a-z\d+.-]*:/i.test(href) || href.startsWith("//");
}

function stripHashAndQuery(href: string): string {
  return href.split(/[?#]/, 1)[0] ?? "";
}

function decodeHrefPath(href: string): string | null {
  try {
    return decodeURIComponent(stripHashAndQuery(href));
  } catch {
    return null;
  }
}

function hasParentTraversal(path: string): boolean {
  return path.split("/").includes("..");
}

function resolveMarkdownFileHref(
  href: string | undefined,
  worktreePath: string | null | undefined,
) {
  if (!href || href.startsWith("#") || isExternalHref(href)) return null;

  const path = decodeHrefPath(href);
  if (!path || path.startsWith("~/") || hasParentTraversal(path)) return null;

  if (path.startsWith("/")) {
    const normalizedRoot = worktreePath?.replace(/\\/g, "/").replace(/\/$/, "");
    const normalizedPath = path.replace(/\\/g, "/");
    if (!normalizedRoot || !normalizedPath.startsWith(`${normalizedRoot}/`)) return null;
    const relativePath = normalizedPath.slice(normalizedRoot.length + 1);
    return looksLikeFilePath(relativePath) ? relativePath : null;
  }

  const normalizedPath = path.replace(/\\/g, "/").replace(/^\.\//, "");
  if (normalizedPath.startsWith("../")) return null;
  return looksLikeFilePath(normalizedPath) ? normalizedPath : null;
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
}: MarkdownLinkProps & {
  worktreePath: string | null | undefined;
  openFile: (path: string) => void;
}) {
  const filePath = resolveMarkdownFileHref(href, worktreePath);
  const isInternal = href?.startsWith("/") || href?.startsWith("#");

  const handleClick = useCallback(
    (event: MouseEvent<HTMLAnchorElement>) => {
      if (!filePath) return;
      event.preventDefault();
      openFile(filePath);
    },
    [filePath, openFile],
  );

  return (
    <a
      href={href}
      target={isInternal ? "_self" : "_blank"}
      rel={isInternal ? undefined : "noopener noreferrer"}
      onClick={filePath ? handleClick : undefined}
    >
      {children}
    </a>
  );
}

function MarkdownFallbackLink(props: MarkdownLinkProps) {
  const { openFile } = usePanelActions();
  const worktreePath = useAppStore((state) => {
    const sessionId = state.tasks.activeSessionId;
    if (!sessionId) return null;
    return state.taskSessions.items[sessionId]?.worktree_path ?? null;
  });

  return <MarkdownFileAnchor {...props} worktreePath={worktreePath} openFile={openFile} />;
}

function MarkdownLink(props: MarkdownLinkProps) {
  const linkContext = useContext(MarkdownFileLinkContext);
  if (linkContext.onOpenFile) {
    return (
      <MarkdownFileAnchor
        {...props}
        worktreePath={linkContext.worktreePath}
        openFile={linkContext.onOpenFile}
      />
    );
  }
  return <MarkdownFallbackLink {...props} />;
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
  a: MarkdownLink,
  table: ({ children }: { children?: ReactNode }) => (
    <div className="overflow-x-auto">
      <table>{children}</table>
    </div>
  ),
};
