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
  currentFilePath?: string;
  onOpenFile?: (path: string) => void;
  onOpenLink?: (url: string) => boolean | void;
};

export const MarkdownFileLinkContext = createContext<MarkdownFileLinkContextValue>({});
export const MarkdownTaskContext = createContext<string | null>(null);

function isBlockCode(rawContent: string, hasLanguage: boolean): boolean {
  return hasLanguage || rawContent.includes("\n");
}

const WEB_TLD_EXTENSIONS = new Set(["ai", "app", "cloud", "co", "com", "dev", "io", "net", "org"]);

function looksLikeFilePath(path: string): boolean {
  const lastSegment = path.split("/").pop() ?? "";
  if (!lastSegment.includes(".") || path.endsWith("/")) return false;
  const extension = lastSegment.split(".").pop() ?? "";
  if (!/^[a-z0-9]{1,8}$/i.test(extension)) return false;
  return !WEB_TLD_EXTENSIONS.has(extension.toLowerCase());
}

export function isExternalMarkdownHref(href: string): boolean {
  return /^[a-z][a-z\d+.-]*:/i.test(href) || href.startsWith("//");
}

function stripHashAndQuery(href: string): string {
  return href.split(/[?#]/, 1)[0] ?? "";
}

// Strip a trailing source-location / omp read selector so a file link resolves
// to the bare path: ":42", ":42:5", and omp ranges like ":16-20", ":50+150",
// ":5-16,960-973", ":2-4:raw", ":raw", ":conflicts".
function stripSourceLocationSuffix(path: string): string {
  const part = String.raw`\d+(?:[-+]\d+)?(?:,\d+(?:[-+]\d+)?)*|raw|conflicts`;
  return path.replace(new RegExp(`:(?:${part})(?::(?:${part}))*$`), "");
}

function decodeHrefPath(href: string): string | null {
  try {
    return stripSourceLocationSuffix(decodeURIComponent(stripHashAndQuery(href)));
  } catch {
    return null;
  }
}

function looksLikeHostAbsolutePath(path: string): boolean {
  return /^\/(?:[A-Za-z]:|Users|home|root|tmp|var|etc|usr|opt|mnt|Volumes)\//i.test(path);
}

function firstAbsoluteSegment(path: string): string | null {
  const first = path.replace(/^\/+/, "").split("/")[0];
  return first || null;
}

function normalizeRepositoryPath(
  path: string,
  baseSegments: readonly string[] = [],
): string | null {
  const normalizedSegments: string[] = [];
  for (const segment of [...baseSegments, ...path.replace(/\\/g, "/").split("/")]) {
    if (!segment || segment === ".") continue;
    if (segment === "..") {
      if (normalizedSegments.length === 0) return null;
      normalizedSegments.pop();
      continue;
    }
    normalizedSegments.push(segment);
  }
  return normalizedSegments.length > 0 ? normalizedSegments.join("/") : null;
}

function normalizeAbsolutePath(path: string): string | null {
  const normalized = path.replace(/\\/g, "/");
  if (!normalized.startsWith("/")) return normalizeRepositoryPath(normalized);
  const repositoryPath = normalizeRepositoryPath(normalized);
  return repositoryPath ? `/${repositoryPath}` : null;
}

function resolveAbsoluteMarkdownFileHref(path: string, worktreePath: string | null | undefined) {
  const normalizedRoot = worktreePath ? normalizeAbsolutePath(worktreePath) : null;
  const normalizedPath = normalizeAbsolutePath(path);
  if (!normalizedPath) return null;

  if (normalizedRoot === "/") {
    const relativePath = normalizedPath.replace(/^\/+/, "");
    return looksLikeFilePath(relativePath) ? relativePath : null;
  }
  if (normalizedRoot && normalizedPath.startsWith(`${normalizedRoot}/`)) {
    const relativePath = normalizedPath.slice(normalizedRoot.length + 1);
    return looksLikeFilePath(relativePath) ? relativePath : null;
  }
  if (
    normalizedRoot &&
    firstAbsoluteSegment(normalizedPath) === firstAbsoluteSegment(normalizedRoot)
  ) {
    return null;
  }
  if (looksLikeHostAbsolutePath(normalizedPath)) return null;
  const rootRelativePath = normalizedPath.replace(/^\/+/, "");
  return looksLikeFilePath(rootRelativePath) ? rootRelativePath : null;
}

function resolveRelativeMarkdownPath(path: string, currentFilePath?: string): string | null {
  const currentSegments = currentFilePath?.replace(/\\/g, "/").split("/") ?? [];
  if (currentFilePath) currentSegments.pop();
  return normalizeRepositoryPath(path, currentSegments);
}

export function resolveMarkdownFileHref(
  href: string | undefined,
  worktreePath: string | null | undefined,
  currentFilePath?: string,
) {
  if (!href || href.startsWith("#") || isExternalMarkdownHref(href)) return null;

  const path = decodeHrefPath(href);
  if (!path || path.startsWith("~/")) return null;

  if (path.startsWith("/")) {
    return resolveAbsoluteMarkdownFileHref(path, worktreePath);
  }

  const resolvedPath = resolveRelativeMarkdownPath(path, currentFilePath);
  return resolvedPath && looksLikeFilePath(resolvedPath) ? resolvedPath : null;
}

type MarkdownLinkProps = {
  href?: string;
  children?: ReactNode;
};

function MarkdownFileAnchor({
  href,
  children,
  worktreePath,
  currentFilePath,
  openFile,
  onOpenLink,
}: MarkdownLinkProps & {
  worktreePath: string | null | undefined;
  currentFilePath?: string;
  openFile: (path: string) => void;
  onOpenLink?: (url: string) => boolean | void;
}) {
  const filePath = resolveMarkdownFileHref(href, worktreePath, currentFilePath);
  const isInternal = !!filePath || href?.startsWith("/") || href?.startsWith("#");

  let handleClick: ((event: MouseEvent<HTMLAnchorElement>) => void) | undefined;
  if (onOpenLink && href && !href.startsWith("#")) {
    handleClick = (event) => {
      if (onOpenLink(href) === false) return;
      event.preventDefault();
    };
  } else if (filePath) {
    handleClick = (event) => {
      event.preventDefault();
      openFile(filePath);
    };
  }

  return (
    <a
      href={href}
      target={isInternal ? "_self" : "_blank"}
      rel={isInternal ? undefined : "noopener noreferrer"}
      onClick={handleClick}
    >
      {children}
    </a>
  );
}

function MarkdownFallbackLink({
  currentFilePath,
  worktreePath: providedWorktreePath,
  ...props
}: MarkdownLinkProps & {
  currentFilePath?: string;
  worktreePath?: string | null;
}) {
  const { openFile } = usePanelActions();
  const worktreePath = useAppStore((state) => {
    if (providedWorktreePath !== undefined) return providedWorktreePath;
    const sessionId = state.tasks.activeSessionId;
    if (!sessionId) return null;
    return getSessionWorkspacePath(state.taskSessions.items[sessionId]);
  });

  return (
    <MarkdownFileAnchor
      {...props}
      worktreePath={worktreePath}
      currentFilePath={currentFilePath}
      openFile={openFile}
    />
  );
}

function MarkdownLink(props: MarkdownLinkProps) {
  const linkContext = useContext(MarkdownFileLinkContext);
  if (linkContext.onOpenLink || linkContext.onOpenFile) {
    return (
      <MarkdownFileAnchor
        {...props}
        worktreePath={linkContext.worktreePath}
        currentFilePath={linkContext.currentFilePath}
        openFile={linkContext.onOpenFile ?? (() => undefined)}
        onOpenLink={linkContext.onOpenLink}
      />
    );
  }
  if (linkContext.currentFilePath || linkContext.worktreePath !== undefined) {
    return (
      <MarkdownFallbackLink
        {...props}
        currentFilePath={linkContext.currentFilePath}
        worktreePath={linkContext.worktreePath}
      />
    );
  }
  return <MarkdownFallbackLink {...props} />;
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
