"use client";

import {
  createContext,
  createElement,
  memo,
  useEffect,
  useContext,
  useMemo,
  useRef,
  useState,
  type HTMLAttributes,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import ReactMarkdown, { type ExtraProps, type Components } from "react-markdown";
import rehypeRaw from "rehype-raw";
import rehypeSanitize from "rehype-sanitize";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconCode, IconMessagePlus } from "@tabler/icons-react";
import { remarkPlugins, markdownComponents } from "@/components/shared/markdown-components";
import { cn, toRelativePath } from "@/lib/utils";
import { PanelHeaderBarSplit } from "@/components/task/panel-primitives";
import { EditorCommentPopover } from "@/components/task/editor-comment-popover";
import { CommentViewPopover } from "@/components/task/comment-view-popover";
import { useMarkdownPreviewComments } from "@/hooks/domains/comments/use-markdown-preview-comments";
import {
  SOURCE_END_ATTR,
  SOURCE_START_ATTR,
  type SourceLineRange,
} from "@/lib/markdown/source-line-ranges";
import { commentsBeginInRange, commentsOverlapRange } from "@/lib/markdown/preview-comments";
import type { DiffComment } from "@/lib/state/slices/comments";

interface MarkdownPreviewToolbarProps {
  path: string;
  worktreePath?: string;
  commentCount: number;
  commentsEnabled: boolean;
  onTogglePreview: () => void;
}

function MarkdownPreviewToolbar({
  path,
  worktreePath,
  commentCount,
  commentsEnabled,
  onTogglePreview,
}: MarkdownPreviewToolbarProps) {
  return (
    <PanelHeaderBarSplit
      left={
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span className="font-mono">{toRelativePath(path, worktreePath)}</span>
          <span className="text-xs text-muted-foreground/60">Preview</span>
        </div>
      }
      right={
        <div className="flex items-center gap-1">
          {commentsEnabled && commentCount > 0 && (
            <div className="flex items-center gap-1 px-2 py-1 text-xs text-primary">
              <IconMessagePlus className="h-3.5 w-3.5" />
              <span>
                {commentCount} comment{commentCount > 1 ? "s" : ""}
              </span>
            </div>
          )}
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                size="sm"
                variant="ghost"
                onClick={onTogglePreview}
                className="h-8 w-8 p-0 cursor-pointer text-foreground"
                data-testid="markdown-preview-toggle"
              >
                <IconCode className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Show code</TooltipContent>
          </Tooltip>
        </div>
      }
    />
  );
}

interface MarkdownPreviewContentProps {
  path: string;
  content: string;
  worktreePath?: string;
  sessionId?: string;
  taskId?: string | null;
  repositoryId?: string | null;
  enableComments?: boolean;
  onTogglePreview: () => void;
}

type PositionedNode = {
  position?: {
    start?: { line?: number };
    end?: { line?: number };
  };
};

type SourceBlockProps = HTMLAttributes<HTMLElement> &
  ExtraProps & {
    children?: ReactNode;
    node?: PositionedNode;
    tag: keyof HTMLElementTagNameMap;
  };
type MarkdownSourceBlockProps = Omit<SourceBlockProps, "tag">;

type PreviewCommentContextValue = {
  comments: DiffComment[];
  showCommentsForRange: (range: SourceLineRange, position: { x: number; y: number }) => void;
};

const PreviewCommentContext = createContext<PreviewCommentContextValue | null>(null);

function sourceRangeFromNode(node: PositionedNode | undefined): SourceLineRange | null {
  const startLine = node?.position?.start?.line;
  const endLine = node?.position?.end?.line ?? startLine;
  if (!startLine || !endLine) return null;
  return { startLine, endLine };
}

function sourceDataAttrs(range: SourceLineRange | null) {
  if (!range) return {};
  return {
    [SOURCE_START_ATTR]: range.startLine,
    [SOURCE_END_ATTR]: range.endLine,
  };
}

export function isInteractiveSourceClickTarget(target: EventTarget | null): boolean {
  if (!(target instanceof Element)) return false;
  return Boolean(
    target.closest("a, button, input, textarea, select, [role='button'], [contenteditable]"),
  );
}

function CommentBadge({
  onClick,
}: {
  onClick: (event: React.MouseEvent<HTMLButtonElement>) => void;
}) {
  return (
    <button
      type="button"
      className="markdown-preview-comment-badge"
      data-testid="markdown-preview-comment-badge"
      aria-label="Edit markdown comment"
      onClick={onClick}
    >
      <IconMessagePlus className="h-3 w-3" />
    </button>
  );
}

function SourceBlock({ tag, node, children, className, onClick, ...rest }: SourceBlockProps) {
  const commentContext = useContext(PreviewCommentContext);
  const range = sourceRangeFromNode(node);
  const comments = commentContext?.comments ?? [];
  const isCommented = range ? commentsOverlapRange(comments, range) : false;
  const canRenderBadge = tag !== "blockquote" && tag !== "li";
  const hasCommentBadge = canRenderBadge && range ? commentsBeginInRange(comments, range) : false;
  const handleClick = (event: React.MouseEvent<HTMLElement>) => {
    onClick?.(event);
    if (!range || !isCommented || event.defaultPrevented || !commentContext) return;
    if (isInteractiveSourceClickTarget(event.target)) return;
    if (window.getSelection()?.toString().trim()) return;
    commentContext.showCommentsForRange(range, { x: event.clientX, y: event.clientY });
  };
  const handleBadgeClick = (event: React.MouseEvent<HTMLButtonElement>) => {
    if (!range || !commentContext) return;
    event.preventDefault();
    event.stopPropagation();
    commentContext.showCommentsForRange(range, { x: event.clientX, y: event.clientY });
  };
  const element = createElement(
    tag,
    {
      ...rest,
      ...sourceDataAttrs(range),
      "data-testid": isCommented
        ? "markdown-preview-commented-range"
        : "markdown-preview-source-block",
      className: cn(
        "markdown-preview-source-block",
        isCommented && "markdown-preview-commented-range",
        className,
      ),
      onClick: handleClick,
    },
    children,
    hasCommentBadge && tag !== "pre" ? <CommentBadge onClick={handleBadgeClick} /> : null,
  );

  if (!hasCommentBadge || tag !== "pre") return element;
  return (
    <div className="markdown-preview-pre-comment-wrapper">
      {element}
      <CommentBadge onClick={handleBadgeClick} />
    </div>
  );
}

function MarkdownPreviewTable({ node, children }: MarkdownSourceBlockProps) {
  return (
    <SourceBlock tag="div" node={node} className="overflow-x-auto">
      <table>{children}</table>
    </SourceBlock>
  );
}

function sourceComponent(tag: keyof HTMLElementTagNameMap) {
  return function MarkdownPreviewSourceComponent(props: MarkdownSourceBlockProps) {
    return <SourceBlock {...props} tag={tag} />;
  };
}

const markdownPreviewComponents: Components = {
  ...markdownComponents,
  p: sourceComponent("p"),
  h1: sourceComponent("h1"),
  h2: sourceComponent("h2"),
  h3: sourceComponent("h3"),
  h4: sourceComponent("h4"),
  h5: sourceComponent("h5"),
  h6: sourceComponent("h6"),
  li: sourceComponent("li"),
  blockquote: sourceComponent("blockquote"),
  pre: sourceComponent("pre"),
  table: (props) => <MarkdownPreviewTable {...(props as MarkdownSourceBlockProps)} />,
};

type MarkdownPreviewCommentState = ReturnType<typeof useMarkdownPreviewComments>;

function MarkdownPreviewCommentOverlays({
  commentsEnabled,
  overlayRoot,
  commentState,
}: {
  commentsEnabled: boolean;
  overlayRoot: HTMLElement | null;
  commentState: MarkdownPreviewCommentState;
}) {
  if (!commentsEnabled || !overlayRoot) return null;

  return createPortal(
    <>
      {commentState.currentSelection && !commentState.textSelection && (
        <Button
          size="sm"
          variant="secondary"
          className="floating-comment-btn fixed z-50 min-h-11 gap-1.5 shadow-lg animate-in fade-in-0 zoom-in-95 duration-100 cursor-pointer sm:min-h-8"
          style={{
            left: commentState.currentSelection.position.x + 8,
            top: commentState.currentSelection.position.y + 8,
          }}
          data-testid="markdown-preview-comment-button"
          onMouseDown={(event) => event.stopPropagation()}
          onClick={commentState.openComposer}
        >
          <IconMessagePlus className="h-3.5 w-3.5" />
          Comment
        </Button>
      )}
      {commentState.textSelection && (
        <div data-markdown-comment-popover>
          <EditorCommentPopover
            selectedText={commentState.textSelection.selectedText}
            lineRange={{
              start: commentState.textSelection.startLine,
              end: commentState.textSelection.endLine,
            }}
            position={commentState.textSelection.position}
            onSubmit={commentState.submitComment}
            onSubmitAndRun={commentState.submitAndRunComment}
            onClose={commentState.closeComposer}
          />
        </div>
      )}
      {commentState.commentView && (
        <CommentViewPopover
          comments={commentState.commentView.comments}
          position={commentState.commentView.position}
          onDelete={commentState.removeComment}
          onUpdate={commentState.updateComment}
          onClose={commentState.closeCommentView}
        />
      )}
    </>,
    overlayRoot,
  );
}

export const MarkdownPreviewContent = memo(function MarkdownPreviewContent({
  path,
  content,
  worktreePath,
  sessionId,
  taskId,
  repositoryId,
  enableComments = false,
  onTogglePreview,
}: MarkdownPreviewContentProps) {
  const rootRef = useRef<HTMLDivElement>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const [overlayRoot, setOverlayRoot] = useState<HTMLElement | null>(null);
  const commentsEnabled = enableComments && !!sessionId;
  const commentState = useMarkdownPreviewComments({
    path,
    content,
    sessionId,
    taskId,
    repositoryId,
    enabled: commentsEnabled,
    rootRef,
  });
  const previewCommentContextValue = useMemo(
    () => ({
      comments: commentState.comments,
      showCommentsForRange: commentState.showCommentsForRange,
    }),
    [commentState.comments, commentState.showCommentsForRange],
  );
  const dismissCommentOverlays = commentState.dismissOverlays;

  useEffect(() => {
    setOverlayRoot(document.body);
  }, []);

  useEffect(() => {
    if (!commentsEnabled) return;
    const scrollElement = scrollRef.current;
    const dismissOverlays = () => dismissCommentOverlays();
    scrollElement?.addEventListener("scroll", dismissOverlays, { passive: true });
    window.addEventListener("resize", dismissOverlays);
    return () => {
      scrollElement?.removeEventListener("scroll", dismissOverlays);
      window.removeEventListener("resize", dismissOverlays);
    };
  }, [commentsEnabled, dismissCommentOverlays]);

  return (
    <div className="relative flex h-full flex-col" data-testid="markdown-preview">
      <MarkdownPreviewToolbar
        path={path}
        worktreePath={worktreePath}
        commentCount={commentState.comments.length}
        commentsEnabled={commentsEnabled}
        onTogglePreview={onTogglePreview}
      />
      <div ref={scrollRef} className="flex-1 overflow-auto p-6">
        <div ref={rootRef} className="markdown-body max-w-3xl" tabIndex={commentsEnabled ? 0 : -1}>
          <PreviewCommentContext.Provider value={previewCommentContextValue}>
            <ReactMarkdown
              remarkPlugins={remarkPlugins}
              rehypePlugins={[rehypeRaw, rehypeSanitize]}
              components={markdownPreviewComponents}
            >
              {content}
            </ReactMarkdown>
          </PreviewCommentContext.Provider>
        </div>
      </div>
      <MarkdownPreviewCommentOverlays
        commentsEnabled={commentsEnabled}
        overlayRoot={overlayRoot}
        commentState={commentState}
      />
    </div>
  );
});
