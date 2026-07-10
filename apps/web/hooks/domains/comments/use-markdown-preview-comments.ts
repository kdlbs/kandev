"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type RefObject,
  type SetStateAction,
} from "react";
import { useToast } from "@/components/toast-provider";
import { useCommentsStore, type DiffComment } from "@/lib/state/slices/comments";
import {
  resolveMarkdownDomSelection,
  type MarkdownPreviewSelection,
  type SourceLineRange,
} from "@/lib/markdown/source-line-ranges";
import { buildMarkdownPreviewComment, commentsOverlapRange } from "@/lib/markdown/preview-comments";
import { useDiffFileComments } from "./use-diff-comments";
import { useRunComment } from "./use-run-comment";

export type MarkdownCommentView = {
  comments: DiffComment[];
  position: { x: number; y: number };
} | null;

type UseMarkdownPreviewCommentsArgs = {
  path: string;
  repositoryId?: string | null;
  content: string;
  sessionId?: string;
  taskId?: string | null;
  enabled: boolean;
  rootRef: RefObject<HTMLDivElement | null>;
};

function isIgnoredTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return Boolean(
    target.closest(
      [
        ".floating-comment-btn",
        "[data-markdown-comment-popover]",
        "[data-markdown-preview-toolbar]",
        "button",
        "a",
        "textarea",
        "input",
        "select",
      ].join(","),
    ),
  );
}

function clearBrowserSelection() {
  window.getSelection()?.removeAllRanges();
}

function isCommentShortcut(event: KeyboardEvent): boolean {
  return (
    (event.metaKey || event.ctrlKey) &&
    ((event.shiftKey && event.key.toLowerCase() === "c") || event.key.toLowerCase() === "i")
  );
}

function useMarkdownSelectionCapture({
  content,
  enabled,
  rootRef,
  onSelectionStart,
}: {
  content: string;
  enabled: boolean;
  rootRef: RefObject<HTMLDivElement | null>;
  onSelectionStart: () => void;
}) {
  const [currentSelection, setCurrentSelection] = useState<MarkdownPreviewSelection | null>(null);
  const resolveSelectionRef = useRef<() => void>(() => {});
  const timeoutRef = useRef<number | null>(null);

  const resolveSelection = useCallback(() => {
    if (!enabled) return;
    const root = rootRef.current;
    const selection = window.getSelection();
    if (!root || !selection) return;
    const resolved = resolveMarkdownDomSelection(root, content, selection);
    setCurrentSelection(resolved);
  }, [content, enabled, rootRef]);

  useEffect(() => {
    resolveSelectionRef.current = resolveSelection;
  }, [resolveSelection]);

  useEffect(() => {
    const root = rootRef.current;
    if (!root || !enabled) return;

    const handleSelectionEnd = (event: MouseEvent | TouchEvent) => {
      if (isIgnoredTarget(event.target)) return;
      if (timeoutRef.current !== null) window.clearTimeout(timeoutRef.current);
      timeoutRef.current = window.setTimeout(() => {
        timeoutRef.current = null;
        resolveSelectionRef.current();
      }, 10);
    };
    const handleSelectionStart = (event: MouseEvent | TouchEvent) => {
      if (isIgnoredTarget(event.target)) return;
      setCurrentSelection(null);
      onSelectionStart();
    };

    root.addEventListener("mouseup", handleSelectionEnd);
    root.addEventListener("touchend", handleSelectionEnd);
    root.addEventListener("mousedown", handleSelectionStart);
    root.addEventListener("touchstart", handleSelectionStart);
    return () => {
      root.removeEventListener("mouseup", handleSelectionEnd);
      root.removeEventListener("touchend", handleSelectionEnd);
      root.removeEventListener("mousedown", handleSelectionStart);
      root.removeEventListener("touchstart", handleSelectionStart);
      if (timeoutRef.current !== null) {
        window.clearTimeout(timeoutRef.current);
        timeoutRef.current = null;
      }
    };
  }, [enabled, onSelectionStart, rootRef]);

  return { currentSelection, setCurrentSelection };
}

function useMarkdownCommentShortcut({
  currentSelection,
  enabled,
  rootRef,
  onOpenSelection,
}: {
  currentSelection: MarkdownPreviewSelection | null;
  enabled: boolean;
  rootRef: RefObject<HTMLDivElement | null>;
  onOpenSelection: () => void;
}) {
  useEffect(() => {
    const root = rootRef.current;
    if (!root || !enabled) return;
    const handleKeyDown = (event: KeyboardEvent) => {
      if (!isCommentShortcut(event) || !currentSelection) return;
      event.preventDefault();
      event.stopPropagation();
      onOpenSelection();
    };
    root.addEventListener("keydown", handleKeyDown, true);
    return () => root.removeEventListener("keydown", handleKeyDown, true);
  }, [currentSelection, enabled, onOpenSelection, rootRef]);
}

function useMarkdownCommentSubmitters({
  path,
  repositoryId,
  sessionId,
  taskId,
  textSelection,
  clearTextSelection,
}: {
  path: string;
  repositoryId?: string | null;
  sessionId?: string;
  taskId?: string | null;
  textSelection: MarkdownPreviewSelection | null;
  clearTextSelection: () => void;
}) {
  const addComment = useCommentsStore((s) => s.addComment);
  const { runComment } = useRunComment({ sessionId: sessionId ?? null, taskId: taskId ?? null });
  const { toast } = useToast();
  const canRunComment = Boolean(sessionId && taskId);

  const createComment = useCallback(
    (text: string): DiffComment | null => {
      if (!sessionId || !textSelection) return null;
      const comment = buildMarkdownPreviewComment({
        filePath: path,
        repositoryId: repositoryId ?? undefined,
        sessionId,
        selectedText: textSelection.selectedText,
        text,
        startLine: textSelection.startLine,
        endLine: textSelection.endLine,
      });
      addComment(comment);
      clearTextSelection();
      return comment;
    },
    [addComment, clearTextSelection, path, repositoryId, sessionId, textSelection],
  );

  const submitComment = useCallback(
    (text: string) => {
      const comment = createComment(text);
      if (!comment) return;
      toast({
        title: "Comment added",
        description: "Your comment will be sent with your next message.",
      });
    },
    [createComment, toast],
  );

  const submitAndRunComment = useCallback(
    async (text: string) => {
      if (!canRunComment) {
        toast({
          title: "Failed to send comment",
          description: "Open a task session before sending to the agent.",
          variant: "error",
        });
        return;
      }
      const comment = createComment(text);
      if (!comment) return;
      try {
        const { queued } = await runComment(comment);
        toast({
          title: "Comment sent",
          description: queued ? "Queued for the agent." : "Sent to the agent.",
        });
      } catch {
        toast({
          title: "Failed to send comment",
          description: "Please try again.",
          variant: "error",
        });
      }
    },
    [canRunComment, createComment, runComment, toast],
  );

  return { submitComment, submitAndRunComment: canRunComment ? submitAndRunComment : undefined };
}

function useMarkdownCommentRangeView({
  comments,
  onShow,
}: {
  comments: DiffComment[];
  onShow: (view: MarkdownCommentView) => void;
}) {
  return useCallback(
    (range: SourceLineRange, position: { x: number; y: number }) => {
      const rangeComments = comments.filter((comment) => commentsOverlapRange([comment], range));
      if (rangeComments.length === 0) return;
      onShow({ comments: rangeComments, position });
    },
    [comments, onShow],
  );
}

function useClearDisabledMarkdownComments({
  enabled,
  clearCurrentSelection,
  clearTextSelection,
  closeCommentView,
}: {
  enabled: boolean;
  clearCurrentSelection: (selection: MarkdownPreviewSelection | null) => void;
  clearTextSelection: (selection: MarkdownPreviewSelection | null) => void;
  closeCommentView: () => void;
}) {
  useEffect(() => {
    if (enabled) return;
    clearCurrentSelection(null);
    clearTextSelection(null);
    closeCommentView();
  }, [clearCurrentSelection, clearTextSelection, closeCommentView, enabled]);
}

function useVisibleMarkdownCommentActions({
  setCommentView,
}: {
  setCommentView: Dispatch<SetStateAction<MarkdownCommentView>>;
}) {
  const removeComment = useCommentsStore((s) => s.removeComment);
  const updateComment = useCommentsStore((s) => s.updateComment);
  const { toast } = useToast();
  const removeVisibleComment = useCallback(
    (commentId: string) => {
      removeComment(commentId);
      setCommentView((view) => {
        if (!view) return view;
        const nextComments = view.comments.filter((comment) => comment.id !== commentId);
        return nextComments.length > 0 ? { ...view, comments: nextComments } : null;
      });
      toast({ title: "Comment deleted" });
    },
    [removeComment, setCommentView, toast],
  );
  const updateVisibleComment = useCallback(
    (commentId: string, text: string) => {
      updateComment(commentId, { text });
      setCommentView((view) => {
        if (!view) return view;
        return {
          ...view,
          comments: view.comments.map((comment) =>
            comment.id === commentId ? { ...comment, text } : comment,
          ),
        };
      });
      toast({ title: "Comment updated" });
    },
    [setCommentView, toast, updateComment],
  );

  return { removeVisibleComment, updateVisibleComment };
}

export function useMarkdownPreviewComments({
  path,
  repositoryId,
  content,
  sessionId,
  taskId,
  enabled,
  rootRef,
}: UseMarkdownPreviewCommentsArgs) {
  const [textSelection, setTextSelection] = useState<MarkdownPreviewSelection | null>(null);
  const [commentView, setCommentView] = useState<MarkdownCommentView>(null);
  const comments = useDiffFileComments(sessionId ?? "", path, repositoryId ?? undefined);

  const closeCommentView = useCallback(() => setCommentView(null), []);
  const { currentSelection, setCurrentSelection } = useMarkdownSelectionCapture({
    content,
    enabled,
    rootRef,
    onSelectionStart: closeCommentView,
  });

  const openComposer = useCallback(() => {
    if (!currentSelection) return;
    setTextSelection(currentSelection);
    setCurrentSelection(null);
  }, [currentSelection, setCurrentSelection]);

  useMarkdownCommentShortcut({
    currentSelection,
    enabled,
    rootRef,
    onOpenSelection: openComposer,
  });

  useClearDisabledMarkdownComments({
    enabled,
    clearCurrentSelection: setCurrentSelection,
    clearTextSelection: setTextSelection,
    closeCommentView,
  });

  const closeComposer = useCallback(() => {
    setTextSelection(null);
    clearBrowserSelection();
  }, []);
  const dismissOverlays = useCallback(() => {
    setCurrentSelection(null);
    setTextSelection(null);
    closeCommentView();
  }, [closeCommentView, setCurrentSelection]);

  const { submitComment, submitAndRunComment } = useMarkdownCommentSubmitters({
    path,
    repositoryId,
    sessionId,
    taskId,
    textSelection,
    clearTextSelection: closeComposer,
  });
  const showCommentsForRange = useMarkdownCommentRangeView({
    comments,
    onShow: setCommentView,
  });
  const { removeVisibleComment, updateVisibleComment } = useVisibleMarkdownCommentActions({
    setCommentView,
  });

  return {
    comments,
    currentSelection,
    textSelection,
    commentView,
    openComposer,
    closeComposer,
    submitComment,
    submitAndRunComment,
    removeComment: removeVisibleComment,
    updateComment: updateVisibleComment,
    showCommentsForRange,
    closeCommentView,
    dismissOverlays,
  };
}
