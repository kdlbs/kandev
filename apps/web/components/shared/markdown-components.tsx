"use client";

import {
  createContext,
  isValidElement,
  useContext,
  useMemo,
  type MouseEvent,
  type ReactNode,
} from "react";
import { useShallow } from "zustand/react/shallow";
import type { Options as MarkdownOptions } from "react-markdown";
import { MarkdownImage } from "./markdown-image";
import remarkGfm from "remark-gfm";
import remarkBreaks from "remark-breaks";
import remarkGemoji from "remark-gemoji";
import remarkMath from "remark-math";
import rehypeKatex from "rehype-katex";
import { InlineCode } from "@/components/task/chat/messages/inline-code";
import { CodeBlock } from "@/components/task/chat/messages/code-block";
import { MermaidBlock } from "@/components/shared/mermaid-block";
import { ResizableMarkdownTable } from "@/components/shared/resizable-markdown-table";
import { isMermaidContent } from "@/components/editors/tiptap/tiptap-mermaid-extension";
import { usePanelActions } from "@/hooks/use-panel-actions";
import { useAppStore } from "@/components/state-provider";
import { getSessionWorkspacePath } from "@/lib/session-workspace-path";
import { remarkMathCompat } from "@/lib/markdown/remark-math-compat";
import {
  buildMarkdownWorkspaceSourcePrefixes,
  resolveMarkdownFileTarget,
  type MarkdownFileRootAlias,
} from "@/lib/markdown/file-link-target";

/** Shared remark plugins used by all markdown renderers */
type MarkdownPluginList = NonNullable<MarkdownOptions["remarkPlugins"]>;

export const remarkPlugins: MarkdownPluginList = [
  remarkGfm,
  remarkBreaks,
  remarkGemoji,
  remarkMath,
  remarkMathCompat,
];

/** Shared rehype plugins used by non-raw Markdown renderers. */
export const rehypePlugins: NonNullable<MarkdownOptions["rehypePlugins"]> = [rehypeKatex];

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
  sessionId?: string | null;
  worktreePath?: string | null;
  /** Confirmed native workspace used as the base for relative links. */
  relativeBasePath?: string | null;
  /** Files-root-relative checkout directories that preserve explicit prefixes. */
  sourcePrefixes?: readonly string[];
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
  relativeBasePath,
  sourcePrefixes,
  openFile,
  fileRootAliases,
}: MarkdownLinkProps & {
  worktreePath: string | null | undefined;
  relativeBasePath?: string | null;
  sourcePrefixes?: readonly string[];
  openFile: (path: string) => void;
  fileRootAliases?: readonly MarkdownFileRootAlias[];
}) {
  const target = resolveMarkdownFileTarget(href, {
    workspaceRoot: worktreePath,
    relativeBasePath,
    sourcePrefixes,
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
    relativeBasePath?: string | null;
    sourcePrefixes?: readonly string[];
    fileRootAliases?: readonly MarkdownFileRootAlias[];
  },
) {
  const { openFile } = usePanelActions();
  // Choose primitives into one shallow-compared snapshot. Returning a fresh
  // object from a bare Zustand selector makes React's external-store check see
  // a new snapshot on every read and re-render without end.
  const activeWorkspace = useAppStore(
    useShallow((state) => {
      const activeSession = state.tasks.activeSessionId
        ? state.taskSessions.items[state.tasks.activeSessionId]
        : undefined;
      // A surface that supplies its own worktree (for example a run
      // transcript) must use that session's native base, not another
      // globally active session's checkout.
      const renderingSession = props.worktreePath
        ? Object.values(state.taskSessions.items).find(
            (session) =>
              session.worktree_path === props.worktreePath ||
              getSessionWorkspacePath(session) === props.worktreePath,
          )
        : undefined;
      const session = renderingSession ?? activeSession;
      const sessionId = session?.id ?? state.tasks.activeSessionId;
      if (!session || !sessionId) return null;
      const liveWorktreeIds =
        state.sessionWorktreesBySessionId?.itemsBySessionId?.[sessionId] ?? [];
      const worktreePaths = [
        ...(session.worktrees ?? []).map((worktree) => worktree.worktree_path),
        ...liveWorktreeIds.map((id) => state.worktrees?.items?.[id]?.path),
      ].filter((path): path is string => Boolean(path));
      return {
        workspacePath: props.worktreePath ?? getSessionWorkspacePath(session) ?? null,
        agentWorkspacePath: session.agent_workspace_path ?? null,
        // Primitive key keeps the shallow snapshot stable; the prefix list is
        // derived outside the selector.
        worktreePaths: [...new Set(worktreePaths)].sort().join("\u0000"),
      };
    }),
  );
  const fallbackSourcePrefixes = useMemo(
    () =>
      buildMarkdownWorkspaceSourcePrefixes({
        workspaceRoot: activeWorkspace?.workspacePath,
        sessionWorktrees: activeWorkspace?.worktreePaths
          ? activeWorkspace.worktreePaths.split("\u0000").map((path) => ({ path }))
          : [],
      }),
    [activeWorkspace?.workspacePath, activeWorkspace?.worktreePaths],
  );

  return (
    <MarkdownFileAnchor
      {...props}
      worktreePath={props.worktreePath ?? activeWorkspace?.workspacePath}
      relativeBasePath={props.relativeBasePath ?? activeWorkspace?.agentWorkspacePath}
      sourcePrefixes={props.sourcePrefixes ?? fallbackSourcePrefixes}
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
        relativeBasePath={linkContext.relativeBasePath}
        sourcePrefixes={linkContext.sourcePrefixes}
        fileRootAliases={linkContext.fileRootAliases}
        openFile={linkContext.onOpenFile}
      />
    );
  }
  return (
    <MarkdownFallbackLink
      {...props}
      worktreePath={linkContext.worktreePath}
      relativeBasePath={linkContext.relativeBasePath}
      sourcePrefixes={linkContext.sourcePrefixes}
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
  img: MarkdownImage,
  table: ResizableMarkdownTable,
};
