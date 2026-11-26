"use client";

import type { MessageAttachment } from "@/lib/services/session-launch-service";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type MutableRefObject,
  type RefObject,
  type SetStateAction,
} from "react";
import { useTranslation } from "react-i18next";
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
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useTouchSelectionTracking } from "@/hooks/use-touch-selection-tracking";

export type MarkdownCommentView = {
  comments: DiffComment[];
  position: { x: number; y: number };
} | null;

type UseMarkdownPreviewCommentsArgs = {
  path: string;
  repositoryId?: string | null;
  repositoryName?: string;
  content: string;
  sessionId?: string;
  taskId?: string | null;
  enabled: boolean;
  rootRef: RefObject<HTMLDivElement | null>;
};

export function isIgnoredTarget(target: EventTarget | null): boolean {
  if (!(target instanceof Element)) return false;
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

function useMarkdownSelectionCaptureEvents({
  enabled,
  touchTracking,
  rootRef,
  onSelectionStart,
  cancelPendingSelection,
  scheduleSelection,
  stopTouchCapture,
  setCurrentSelection,
  touchCaptureActiveRef,
}: {
  enabled: boolean;
  touchTracking: boolean;
  rootRef: RefObject<HTMLDivElement | null>;
  onSelectionStart: () => void;
  cancelPendingSelection: () => void;
  scheduleSelection: (requiresTouchOwnership: boolean) => void;
  stopTouchCapture: () => void;
  setCurrentSelection: Dispatch<SetStateAction<MarkdownPreviewSelection | null>>;
  touchCaptureActiveRef: MutableRefObject<boolean>;
}) {
  useEffect(() => {
    const root = rootRef.current;
    if (!root || !enabled) {
      stopTouchCapture();
      return;
    }

    const handleSelectionEnd = (event: MouseEvent | TouchEvent) => {
      if (isIgnoredTarget(event.target)) return;
      scheduleSelection(event.type === "touchend");
    };
    const handleSelectionStart = (event: MouseEvent | TouchEvent) => {
      if (isIgnoredTarget(event.target)) {
        stopTouchCapture();
        if (
          !(event.target instanceof Element) ||
          !event.target.closest(
            ".floating-comment-btn, [data-selection-action-dock], [data-markdown-comment-popover]",
          )
        ) {
          setCurrentSelection(null);
        }
        return;
      }
      cancelPendingSelection();
      touchCaptureActiveRef.current = event.type === "touchstart";
      // Touch selections follow selectionchange; a swipe over selected text must keep the dock.
      if (touchTracking && event.type === "touchstart") return;
      setCurrentSelection(null);
      onSelectionStart();
    };
    const handleOutsidePointerDown = (event: PointerEvent) => {
      const target = event.target;
      if (target instanceof Node && root.contains(target)) return;
      stopTouchCapture();
      if (
        target instanceof Element &&
        target.closest(
          ".floating-comment-btn, [data-selection-action-dock], [data-markdown-comment-popover]",
        )
      ) {
        return;
      }
      setCurrentSelection(null);
    };

    root.addEventListener("mouseup", handleSelectionEnd);
    root.addEventListener("touchend", handleSelectionEnd);
    root.addEventListener("mousedown", handleSelectionStart);
    root.addEventListener("touchstart", handleSelectionStart);
    root.ownerDocument.addEventListener("pointerdown", handleOutsidePointerDown, true);
    return () => {
      root.removeEventListener("mouseup", handleSelectionEnd);
      root.removeEventListener("touchend", handleSelectionEnd);
      root.removeEventListener("mousedown", handleSelectionStart);
      root.removeEventListener("touchstart", handleSelectionStart);
      root.ownerDocument.removeEventListener("pointerdown", handleOutsidePointerDown, true);
      stopTouchCapture();
    };
  }, [
    cancelPendingSelection,
    enabled,
    onSelectionStart,
    rootRef,
    scheduleSelection,
    setCurrentSelection,
    stopTouchCapture,
    touchCaptureActiveRef,
    touchTracking,
  ]);
}

function useMarkdownSelectionCapture({
  content,
  enabled,
  touchTracking,
  rootRef,
  onSelectionStart,
}: {
  content: string;
  enabled: boolean;
  touchTracking: boolean;
  rootRef: RefObject<HTMLDivElement | null>;
  onSelectionStart: () => void;
}) {
  const [currentSelection, setCurrentSelection] = useState<MarkdownPreviewSelection | null>(null);
  const touchCaptureActiveRef = useRef(true);
  const followTouchSelection = useCallback(
    (selection: Selection | null) => {
      if (!touchCaptureActiveRef.current) return;
      const root = rootRef.current;
      setCurrentSelection(
        root && selection ? resolveMarkdownDomSelection(root, content, selection) : null,
      );
    },
    [content, rootRef],
  );
  useTouchSelectionTracking(rootRef, enabled && touchTracking, followTouchSelection);
  const resolveSelectionRef = useRef<() => void>(() => {});
  const timeoutRef = useRef<number | null>(null);
  const captureGenerationRef = useRef(0);

  const cancelPendingSelection = useCallback(() => {
    captureGenerationRef.current += 1;
    if (timeoutRef.current !== null) {
      window.clearTimeout(timeoutRef.current);
      timeoutRef.current = null;
    }
  }, []);

  const stopTouchCapture = useCallback(() => {
    touchCaptureActiveRef.current = false;
    cancelPendingSelection();
  }, [cancelPendingSelection]);

  const scheduleSelection = useCallback(
    (requiresTouchOwnership: boolean) => {
      cancelPendingSelection();
      const generation = ++captureGenerationRef.current;
      timeoutRef.current = window.setTimeout(() => {
        timeoutRef.current = null;
        if (generation !== captureGenerationRef.current) return;
        if (requiresTouchOwnership && !touchCaptureActiveRef.current) return;
        resolveSelectionRef.current();
      }, 10);
    },
    [cancelPendingSelection],
  );

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

  useMarkdownSelectionCaptureEvents({
    enabled,
    touchTracking,
    rootRef,
    onSelectionStart,
    cancelPendingSelection,
    scheduleSelection,
    stopTouchCapture,
    setCurrentSelection,
    touchCaptureActiveRef,
  });

  useEffect(() => stopTouchCapture, [stopTouchCapture]);

  return { currentSelection, setCurrentSelection, stopTouchCapture };
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

function useMarkdownCommentDismissals({
  touch,
  closeCommentView,
  setCurrentSelection,
  setTextSelection,
}: {
  touch: boolean;
  closeCommentView: () => void;
  setCurrentSelection: Dispatch<SetStateAction<MarkdownPreviewSelection | null>>;
  setTextSelection: Dispatch<SetStateAction<MarkdownPreviewSelection | null>>;
}) {
  const dismissForResize = useCallback(() => {
    if (!touch) setCurrentSelection(null);
    closeCommentView();
  }, [closeCommentView, setCurrentSelection, touch]);
  const dismissOverlays = useCallback(() => {
    if (!touch) {
      setCurrentSelection(null);
      setTextSelection(null);
    }
    closeCommentView();
  }, [closeCommentView, setCurrentSelection, setTextSelection, touch]);
  return { dismissForResize, dismissOverlays };
}

function useMarkdownCommentSubmitters({
  path,
  repositoryId,
  repositoryName,
  sessionId,
  taskId,
  textSelection,
  clearTextSelection,
}: {
  path: string;
  repositoryId?: string | null;
  repositoryName?: string;
  sessionId?: string;
  taskId?: string | null;
  textSelection: MarkdownPreviewSelection | null;
  clearTextSelection: () => void;
}) {
  const { t } = useTranslation();
  const addComment = useCommentsStore((s) => s.addComment);
  const { runComment } = useRunComment({ sessionId: sessionId ?? null, taskId: taskId ?? null });
  const { toast } = useToast();
  const canRunComment = Boolean(sessionId && taskId);

  const createComment = useCallback(
    (text: string, attachments?: MessageAttachment[]): DiffComment | null => {
      if (!sessionId || !textSelection) return null;
      const comment = buildMarkdownPreviewComment({
        filePath: path,
        repositoryId: repositoryId ?? undefined,
        repositoryName,
        sessionId,
        selectedText: textSelection.selectedText,
        text,
        startLine: textSelection.startLine,
        endLine: textSelection.endLine,
      });
      if (attachments?.length) comment.attachments = attachments;
      addComment(comment);
      clearTextSelection();
      return comment;
    },
    [addComment, clearTextSelection, path, repositoryId, repositoryName, sessionId, textSelection],
  );

  const submitComment = useCallback(
    (text: string, _selected?: string, attachments?: MessageAttachment[]) => {
      const comment = createComment(text, attachments);
      if (!comment) return;
      toast({
        title: t("editors:commentAdded"),
        description: t("editors:commentWillBeSentWithNextMessage"),
      });
    },
    [createComment, t, toast],
  );

  const submitAndRunComment = useCallback(
    async (text: string, _selected?: string, attachments?: MessageAttachment[]) => {
      if (!canRunComment) {
        toast({
          title: t("editors:failedToSendComment"),
          description: t("editors:openTaskSessionBeforeSending"),
          variant: "error",
        });
        return;
      }
      const comment = createComment(text, attachments);
      if (!comment) return;
      try {
        const { queued } = await runComment(comment);
        toast({
          title: t("editors:commentSent"),
          description: queued ? t("editors:queuedForTheAgent") : t("editors:sentToTheAgent"),
        });
      } catch {
        toast({
          title: t("editors:failedToSendComment"),
          description: t("editors:pleaseTryAgain"),
          variant: "error",
        });
      }
    },
    [canRunComment, createComment, runComment, t, toast],
  );

  return { submitComment, submitAndRunComment: canRunComment ? submitAndRunComment : undefined };
}

function useMarkdownCommentRangeView(
  comments: DiffComment[],
  onShow: (view: MarkdownCommentView) => void,
) {
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
  stopTouchCapture,
}: {
  enabled: boolean;
  clearCurrentSelection: (selection: MarkdownPreviewSelection | null) => void;
  clearTextSelection: (selection: MarkdownPreviewSelection | null) => void;
  closeCommentView: () => void;
  stopTouchCapture: () => void;
}) {
  useEffect(() => {
    if (enabled) return;
    stopTouchCapture();
    clearCurrentSelection(null);
    clearTextSelection(null);
    closeCommentView();
  }, [clearCurrentSelection, clearTextSelection, closeCommentView, enabled, stopTouchCapture]);
}

function useVisibleMarkdownCommentActions({
  setCommentView,
}: {
  setCommentView: Dispatch<SetStateAction<MarkdownCommentView>>;
}) {
  const { t } = useTranslation();
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
      toast({ title: t("editors:commentDeleted") });
    },
    [removeComment, setCommentView, t, toast],
  );
  const updateVisibleComment = useCallback(
    (commentId: string, text: string, attachments?: MessageAttachment[]) => {
      const updates = { text, ...(attachments && { attachments }) };
      updateComment(commentId, updates);
      setCommentView((view) => {
        if (!view) return view;
        return {
          ...view,
          comments: view.comments.map((comment) =>
            comment.id === commentId ? { ...comment, ...updates } : comment,
          ),
        };
      });
      toast({ title: t("editors:commentUpdated") });
    },
    [setCommentView, t, toast, updateComment],
  );

  return { removeVisibleComment, updateVisibleComment };
}

function useResetMarkdownSelectionOnIdentity({
  identity,
  stopTouchCapture,
  setCurrentSelection,
  setTextSelection,
  setCommentView,
}: {
  identity: string;
  stopTouchCapture: () => void;
  setCurrentSelection: Dispatch<SetStateAction<MarkdownPreviewSelection | null>>;
  setTextSelection: Dispatch<SetStateAction<MarkdownPreviewSelection | null>>;
  setCommentView: Dispatch<SetStateAction<MarkdownCommentView>>;
}) {
  const previousIdentityRef = useRef(identity);
  useEffect(() => {
    if (previousIdentityRef.current === identity) return;
    previousIdentityRef.current = identity;
    stopTouchCapture();
    setCurrentSelection(null);
    setTextSelection(null);
    setCommentView(null);
    clearBrowserSelection();
  }, [identity, setCurrentSelection, setCommentView, setTextSelection, stopTouchCapture]);
}

function useMarkdownPreviewActions({
  path,
  repositoryId,
  repositoryName,
  sessionId,
  taskId,
  comments,
  textSelection,
  closeComposer,
  setCommentView,
}: {
  path: string;
  repositoryId?: string | null;
  repositoryName?: string;
  sessionId?: string;
  taskId?: string | null;
  comments: DiffComment[];
  textSelection: MarkdownPreviewSelection | null;
  closeComposer: () => void;
  setCommentView: Dispatch<SetStateAction<MarkdownCommentView>>;
}) {
  const submitters = useMarkdownCommentSubmitters({
    path,
    repositoryId,
    repositoryName,
    sessionId,
    taskId,
    textSelection,
    clearTextSelection: closeComposer,
  });
  const showCommentsForRange = useMarkdownCommentRangeView(comments, setCommentView);
  const { removeVisibleComment, updateVisibleComment } = useVisibleMarkdownCommentActions({
    setCommentView,
  });
  return {
    ...submitters,
    showCommentsForRange,
    removeComment: removeVisibleComment,
    updateComment: updateVisibleComment,
  };
}

export function useMarkdownPreviewComments({
  path,
  repositoryId,
  repositoryName,
  content,
  sessionId,
  taskId,
  enabled,
  rootRef,
}: UseMarkdownPreviewCommentsArgs) {
  const touch = useTouchDrawer();
  const [textSelection, setTextSelection] = useState<MarkdownPreviewSelection | null>(null);
  const [commentView, setCommentView] = useState<MarkdownCommentView>(null);
  const comments = useDiffFileComments(
    sessionId ?? "",
    path,
    repositoryId ?? undefined,
    repositoryName ?? "",
  );

  const closeCommentView = useCallback(() => setCommentView(null), []);
  const { currentSelection, setCurrentSelection, stopTouchCapture } = useMarkdownSelectionCapture({
    content,
    enabled,
    touchTracking: touch && !textSelection,
    rootRef,
    onSelectionStart: closeCommentView,
  });

  const identity = JSON.stringify([
    path,
    repositoryId ?? "",
    repositoryName ?? "",
    sessionId ?? "",
  ]);
  useResetMarkdownSelectionOnIdentity({
    identity,
    stopTouchCapture,
    setCurrentSelection,
    setTextSelection,
    setCommentView,
  });

  const openComposer = useCallback(() => {
    if (!currentSelection) return;
    stopTouchCapture();
    setTextSelection(currentSelection);
    setCurrentSelection(null);
  }, [currentSelection, setCurrentSelection, stopTouchCapture]);

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
    stopTouchCapture,
  });

  const closeComposer = useCallback(() => {
    stopTouchCapture();
    setTextSelection(null);
    clearBrowserSelection();
  }, [stopTouchCapture]);
  const { dismissForResize, dismissOverlays } = useMarkdownCommentDismissals({
    touch,
    closeCommentView,
    setCurrentSelection,
    setTextSelection,
  });

  const actions = useMarkdownPreviewActions({
    path,
    repositoryId,
    repositoryName,
    sessionId,
    taskId,
    comments,
    textSelection,
    closeComposer,
    setCommentView,
  });

  return {
    touch,
    comments,
    attachmentTaskId: sessionId ? taskId : null,
    currentSelection,
    textSelection,
    commentView,
    openComposer,
    closeComposer,
    ...actions,
    closeCommentView,
    dismissOverlays,
    dismissForResize,
  };
}
