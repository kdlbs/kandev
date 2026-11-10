"use client";

import {
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
  type Dispatch,
  type MouseEvent,
  type ReactNode,
  type RefObject,
  type SetStateAction,
} from "react";
import { useShallow } from "zustand/react/shallow";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useCommentsStore } from "@/lib/state/slices/comments";
import type { AgentMessageComment } from "@/lib/state/slices/comments";
import { useRunComment } from "@/hooks/domains/comments/use-run-comment";
import {
  agentMessageCommentHighlightName,
  createMessageTextAnchor,
  getMessageSelection,
  isSelectableAgentMessage,
  resolveMessageTextAnchor,
  type MessageSelection,
} from "@/lib/chat/agent-message-comments";
import type { Message } from "@/lib/types/http";
import { generateUUID } from "@/lib/utils";
import {
  messageCommentDecorationAtPoint,
  supportsCustomHighlights,
  useMessageCommentDecorations,
  useMessageCommentShortcut,
  useMessageCommentMouseSelection,
} from "./use-message-comment-dom";
import {
  MessageCommentDecorationOverlay,
  MessageCustomHighlightStyle,
} from "./message-comment-decorations";
import { MessageCommentControls, type CommentTarget } from "./message-comment-controls";
import { useTranslation } from "react-i18next";

type MessageCommentSurfaceProps = {
  message: Message;
  sessionId?: string | null;
  isTurnActive: boolean;
  /** Office thread comment author; set when the surface wraps a thread comment instead of a chat reply. */
  authorName?: string;
  /** Disable new extracts for comments that cannot be submitted. */
  readOnly?: boolean;
  /** Control Run independently from display attribution. */
  allowRun?: boolean;
  children: ReactNode;
};

const EMPTY_COMMENT_IDS: string[] = [];
const INTERACTIVE_MESSAGE_TARGETS =
  'a, button, input, textarea, select, summary, [role="button"], [role="link"], [contenteditable="true"]';

function isInteractiveMessageTarget(target: EventTarget | null) {
  return target instanceof Element && Boolean(target.closest(INTERACTIVE_MESSAGE_TARGETS));
}

function useDecorationClickHandler(
  decorations: ReturnType<typeof useMessageCommentDecorations>,
  openExistingComment: (commentId: string, position: { x: number; y: number }) => void,
) {
  return useCallback(
    (event: MouseEvent<HTMLDivElement>) => {
      if (isInteractiveMessageTarget(event.target)) return;
      const selection = window.getSelection();
      if (selection && !selection.isCollapsed) return;
      const decoration = messageCommentDecorationAtPoint(decorations, event.clientX, event.clientY);
      if (!decoration) return;
      event.preventDefault();
      openExistingComment(decoration.comment.id, { x: event.clientX, y: event.clientY });
    },
    [decorations, openExistingComment],
  );
}

function commentFromTarget(
  message: Message,
  sessionId: string,
  target: CommentTarget,
  renderedText: string,
  { feedback, authorName }: { feedback: string; authorName?: string },
): AgentMessageComment | null {
  const resolved = resolveMessageTextAnchor(target.anchor, renderedText);
  if (!resolved) return null;
  const anchor = createMessageTextAnchor(message.id, renderedText, resolved.start, resolved.end);
  return {
    id: generateUUID(),
    sessionId,
    source: "agent-message",
    messageId: message.id,
    selectedText: anchor.selectedText,
    text: feedback.trim(),
    createdAt: new Date().toISOString(),
    status: "pending",
    anchor,
    ...(authorName ? { authorName } : {}),
  };
}

function usePendingCommentsForMessage(sessionId: string | null | undefined, messageId: string) {
  return useCommentsStore(
    useShallow((state) => {
      const commentIds = sessionId
        ? (state.bySession[sessionId] ?? EMPTY_COMMENT_IDS)
        : EMPTY_COMMENT_IDS;
      return commentIds.flatMap((id) => {
        const comment = state.byId[id];
        return comment &&
          comment.source === "agent-message" &&
          comment.status === "pending" &&
          comment.messageId === messageId
          ? [comment]
          : [];
      });
    }),
  );
}

function useCommentTargetState(
  rootRef: RefObject<HTMLDivElement | null>,
  isSelectable: boolean,
  messageId: string,
) {
  const [target, setTarget] = useState<CommentTarget | null>(null);
  const [composerOpen, setComposerOpen] = useState(false);

  const close = useCallback(() => {
    setComposerOpen(false);
    setTarget(null);
    window.getSelection()?.removeAllRanges();
  }, []);

  const setNewSelection = useCallback(
    (selection: MessageSelection, openComposer: boolean) => {
      const renderedText = rootRef.current?.textContent ?? "";
      setTarget({
        selection,
        anchor: createMessageTextAnchor(messageId, renderedText, selection.start, selection.end),
        position: { x: selection.rect.right, y: selection.rect.bottom },
      });
      setComposerOpen(openComposer);
    },
    [messageId, rootRef],
  );

  const captureSelection = useCallback(() => {
    if (!rootRef.current || !isSelectable) return;
    const selection = getMessageSelection(rootRef.current, window.getSelection());
    if (selection) setNewSelection(selection, false);
  }, [isSelectable, rootRef, setNewSelection]);

  useMessageCommentMouseSelection(rootRef, isSelectable && !composerOpen, captureSelection);

  const openFromShortcut = useCallback(
    (selection: MessageSelection) => setNewSelection(selection, true),
    [setNewSelection],
  );
  useMessageCommentShortcut(rootRef, isSelectable, openFromShortcut);

  useEffect(() => {
    if (!target || composerOpen) return;
    const dismiss = (event: Event) => {
      const element = event.target as HTMLElement | null;
      if (element?.closest('[data-testid="agent-message-comment-trigger"]')) return;
      setTarget(null);
    };
    const dismissOnScroll = () => setTarget(null);
    document.addEventListener("pointerdown", dismiss, true);
    document.addEventListener("scroll", dismissOnScroll, true);
    return () => {
      document.removeEventListener("pointerdown", dismiss, true);
      document.removeEventListener("scroll", dismissOnScroll, true);
    };
  }, [composerOpen, target]);

  return { target, setTarget, composerOpen, setComposerOpen, close, captureSelection };
}

function useExistingCommentHandlers(
  rootRef: RefObject<HTMLDivElement | null>,
  pendingComments: AgentMessageComment[],
  setTarget: Dispatch<SetStateAction<CommentTarget | null>>,
  setComposerOpen: Dispatch<SetStateAction<boolean>>,
) {
  const openExistingComment = useCallback(
    (commentId: string, position: { x: number; y: number }) => {
      const comment = pendingComments.find((item) => item.id === commentId);
      const root = rootRef.current;
      if (!comment || !root) return;
      const range = resolveMessageTextAnchor(comment.anchor, root.textContent ?? "");
      if (!range) return;
      setTarget({
        selection: {
          ...range,
          selectedText: comment.selectedText,
          rect: new DOMRect(position.x, position.y, 0, 0),
        },
        anchor: comment.anchor,
        position,
        editingCommentId: comment.id,
        editingText: comment.text,
      });
      setComposerOpen(true);
      window.getSelection()?.removeAllRanges();
    },
    [pendingComments, rootRef, setComposerOpen, setTarget],
  );

  return openExistingComment;
}

function useMessageCommentActions({
  message,
  sessionId,
  target,
  rootRef,
  close,
  authorName,
}: {
  authorName?: string;
  message: Message;
  sessionId: string | null | undefined;
  target: CommentTarget | null;
  rootRef: RefObject<HTMLDivElement | null>;
  close: () => void;
}) {
  const { t } = useTranslation();
  const addComment = useCommentsStore((state) => state.addComment);
  const updateComment = useCommentsStore((state) => state.updateComment);
  const removeComment = useCommentsStore((state) => state.removeComment);
  const [saveError, setSaveError] = useState<string | null>(null);
  const { runComment } = useRunComment({ sessionId: sessionId ?? null, taskId: message.task_id });

  useEffect(() => setSaveError(null), [target]);

  const saveComment = useCallback(
    (feedback: string): AgentMessageComment | true | null => {
      if (!target || !sessionId || !feedback.trim()) return null;
      if (target.editingCommentId) {
        setSaveError(null);
        updateComment(target.editingCommentId, { text: feedback.trim() });
        return true;
      }
      const renderedText = rootRef.current?.textContent ?? "";
      const comment = commentFromTarget(message, sessionId, target, renderedText, {
        feedback,
        authorName,
      });
      if (!comment) {
        setSaveError(t("task:agentResponseChangedSelectAgain"));
        return null;
      }
      setSaveError(null);
      addComment(comment);
      return comment;
    },
    [addComment, authorName, message, rootRef, sessionId, target, updateComment],
  );

  const handleAdd = useCallback(
    (feedback: string) => {
      if (!saveComment(feedback)) return false;
      return true;
    },
    [saveComment],
  );

  const handleRun = useCallback(
    (feedback: string) => {
      const comment = saveComment(feedback);
      if (!comment) return false;
      if (comment !== true) {
        void runComment(comment).catch((error) =>
          console.error("Failed to run agent message comment:", error),
        );
      }
      return true;
    },
    [runComment, saveComment],
  );

  const deleteComment = useCallback(() => {
    if (!target?.editingCommentId) return;
    removeComment(target.editingCommentId);
    close();
  }, [close, removeComment, target?.editingCommentId]);
  const handleDelete = target?.editingCommentId ? deleteComment : undefined;

  return { handleAdd, handleRun, handleDelete, saveError };
}

function canSelectMessageText(
  message: Message,
  sessionId: string | null | undefined,
  isTurnActive: boolean,
  readOnly: boolean,
): boolean {
  return !readOnly && Boolean(sessionId) && isSelectableAgentMessage(message, isTurnActive, false);
}

export function MessageCommentSurface({
  message,
  sessionId,
  isTurnActive,
  authorName,
  readOnly = false,
  allowRun = true,
  children,
}: MessageCommentSurfaceProps) {
  const rootRef = useRef<HTMLDivElement>(null);
  const highlightInstanceId = useId();
  const useDrawer = useTouchDrawer();
  const pendingComments = usePendingCommentsForMessage(sessionId, message.id);
  const isSelectable = canSelectMessageText(message, sessionId, isTurnActive, readOnly);
  const targetState = useCommentTargetState(rootRef, isSelectable, message.id);
  const { target, composerOpen, setTarget, setComposerOpen, close, captureSelection } = targetState;
  const openExistingComment = useExistingCommentHandlers(
    rootRef,
    pendingComments,
    setTarget,
    setComposerOpen,
  );
  const actions = useMessageCommentActions({
    message,
    sessionId,
    target,
    rootRef,
    close,
    authorName,
  });
  const handleRun = allowRun ? actions.handleRun : undefined;
  const portalContainer = rootRef.current?.closest<HTMLElement>('[role="dialog"]') ?? null;
  const highlightName = agentMessageCommentHighlightName(`${message.id}-${highlightInstanceId}`);
  const hasCustomHighlightSupport = supportsCustomHighlights();
  const decorations = useMessageCommentDecorations(
    rootRef,
    pendingComments,
    message.content,
    highlightName,
  );
  const handleDecorationClick = useDecorationClickHandler(decorations, openExistingComment);

  return (
    <>
      <div
        ref={rootRef}
        className="agent-message-comment-body relative"
        data-agent-message-body="true"
        data-message-id={message.id}
        data-agent-message-highlight-name={highlightName}
        onTouchEnd={captureSelection}
        onClick={handleDecorationClick}
      >
        {children}
        <MessageCommentDecorationOverlay
          decorations={decorations}
          useCustomHighlights={hasCustomHighlightSupport}
          onOpen={openExistingComment}
        />
      </div>
      {pendingComments.length > 0 && hasCustomHighlightSupport ? (
        <MessageCustomHighlightStyle highlightName={highlightName} />
      ) : null}
      <MessageCommentControls
        target={target}
        composerOpen={composerOpen}
        useDrawer={useDrawer}
        setComposerOpen={setComposerOpen}
        close={close}
        actions={actions}
        handleRun={handleRun}
        portalContainer={portalContainer}
      />
    </>
  );
}
