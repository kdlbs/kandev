"use client";

import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type Dispatch,
  type KeyboardEvent as ReactKeyboardEvent,
  type MouseEvent as ReactMouseEvent,
  type ReactNode,
  type RefObject,
  type SetStateAction,
} from "react";
import { createPortal } from "react-dom";
import { useShallow } from "zustand/react/shallow";
import { IconMessage, IconPlus, IconPlayerPlay, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { PlanSelectionPopover } from "@/components/task/plan-selection-popover";
import { useCommentsStore } from "@/lib/state/slices/comments";
import type { AgentMessageComment } from "@/lib/state/slices/comments";
import { useRunComment } from "@/hooks/domains/comments/use-run-comment";
import {
  createMessageTextAnchor,
  getMessageSelection,
  isSelectableAgentMessage,
  resolveMessageTextAnchor,
  restoreMessageCommentHighlights,
  type MessageSelection,
} from "@/lib/chat/agent-message-comments";
import type { Message } from "@/lib/types/http";
import { cn, generateUUID } from "@/lib/utils";

type MessageCommentSurfaceProps = {
  message: Message;
  sessionId?: string | null;
  isTurnActive: boolean;
  children: ReactNode;
};

type CommentTarget = {
  selection: MessageSelection;
  position: { x: number; y: number };
  editingCommentId?: string;
  editingText?: string;
};

const EMPTY_COMMENT_IDS: string[] = [];
const FLOATING_MARGIN = 8;

function commentFromTarget(
  message: Message,
  sessionId: string,
  target: CommentTarget,
  renderedText: string,
  feedback: string,
): AgentMessageComment {
  const { start, end } = target.selection;
  return {
    id: generateUUID(),
    sessionId,
    source: "agent-message",
    messageId: message.id,
    selectedText: renderedText.slice(start, end),
    text: feedback.trim(),
    createdAt: new Date().toISOString(),
    status: "pending",
    anchor: createMessageTextAnchor(message.id, renderedText, start, end),
  };
}

function getTriggerPosition(rect: DOMRect, size: number) {
  const left = Math.min(
    Math.max(rect.right - size, FLOATING_MARGIN),
    window.innerWidth - size - FLOATING_MARGIN,
  );
  const above = rect.top - size - FLOATING_MARGIN;
  const top = above >= FLOATING_MARGIN ? above : rect.bottom + FLOATING_MARGIN;
  return { left, top };
}

function SelectionCommentTrigger({
  selection,
  isTouch,
  onOpen,
  portalContainer,
}: {
  selection: MessageSelection;
  isTouch: boolean;
  onOpen: () => void;
  portalContainer?: HTMLElement | null;
}) {
  const size = isTouch ? 44 : 28;
  const position = getTriggerPosition(selection.rect, size + 8);
  const portalRect = portalContainer?.getBoundingClientRect();
  return createPortal(
    <div
      className={cn(
        "pointer-events-auto z-[60] rounded-lg border border-border/50 bg-popover p-1 shadow-lg",
        portalContainer ? "absolute" : "fixed",
      )}
      style={{
        left: position.left - (portalRect?.left ?? 0),
        top: position.top - (portalRect?.top ?? 0),
      }}
    >
      <button
        type="button"
        title="Comment (Cmd+Shift+C)"
        aria-label="Comment on selection"
        data-testid="agent-message-comment-trigger"
        className="flex cursor-pointer items-center justify-center rounded bg-accent text-white transition-transform duration-150 ease-out hover:bg-accent/80 active:scale-[0.96]"
        style={{ width: size, height: size }}
        onMouseDown={(event) => event.preventDefault()}
        onClick={onOpen}
      >
        <IconMessage className="h-3.5 w-3.5" />
      </button>
    </div>,
    portalContainer ?? document.body,
  );
}

function DrawerCommentActions({
  isEditing,
  disabled,
  onAdd,
  onRun,
  onDelete,
}: {
  isEditing: boolean;
  disabled: boolean;
  onAdd: () => void;
  onRun?: () => void;
  onDelete?: () => void;
}) {
  return (
    <div className="flex items-center justify-between gap-3">
      {isEditing && onDelete ? (
        <Button
          type="button"
          size="sm"
          variant="ghost"
          aria-label="Delete comment"
          onClick={onDelete}
          className="min-h-11 cursor-pointer px-3 text-muted-foreground hover:text-destructive"
        >
          <IconTrash className="h-4 w-4" />
        </Button>
      ) : (
        <span />
      )}
      <div className="inline-flex">
        <Button
          type="button"
          size="sm"
          variant={onRun && !isEditing ? "outline" : "default"}
          disabled={disabled}
          onClick={onAdd}
          data-testid="agent-message-comment-add"
          className={`min-h-11 cursor-pointer gap-1.5 px-4 transition-transform duration-150 ease-out active:scale-[0.96] ${onRun && !isEditing ? "rounded-r-none border-r-0" : ""}`}
        >
          <IconPlus className="h-4 w-4" />
          {isEditing ? "Update" : "Add"}
        </Button>
        {onRun && !isEditing ? (
          <Button
            type="button"
            size="sm"
            disabled={disabled}
            onClick={onRun}
            data-testid="agent-message-comment-run"
            className="min-h-11 cursor-pointer gap-1.5 rounded-l-none px-4 transition-transform duration-150 ease-out active:scale-[0.96]"
          >
            <IconPlayerPlay className="h-4 w-4" />
            Run
          </Button>
        ) : null}
      </div>
    </div>
  );
}

function MessageCommentDrawer({
  target,
  open,
  onClose,
  onAdd,
  onRun,
  onDelete,
}: {
  target: CommentTarget | null;
  open: boolean;
  onClose: () => void;
  onAdd: (feedback: string) => void;
  onRun: (feedback: string) => void;
  onDelete: (() => void) | undefined;
}) {
  const [feedback, setFeedback] = useState(target?.editingText ?? "");
  const targetKey = `${target?.editingCommentId ?? "new"}:${target?.selection.start ?? 0}:${target?.selection.end ?? 0}`;

  useEffect(() => {
    setFeedback(target?.editingText ?? "");
  }, [targetKey, target?.editingText]);

  if (!target) return null;
  const isEditing = target.editingCommentId !== undefined;
  const submit = () => {
    if (feedback.trim()) onAdd(feedback.trim());
  };
  const run = () => {
    if (feedback.trim()) onRun(feedback.trim());
  };

  return (
    <Drawer open={open} onOpenChange={(next) => !next && onClose()}>
      <DrawerContent
        className="z-[60] max-h-[82dvh] pb-[calc(1rem+env(safe-area-inset-bottom))]"
        data-testid="agent-message-comment-drawer"
      >
        <DrawerHeader className="shrink-0 px-4 pb-3 text-left">
          <DrawerTitle>{isEditing ? "Edit comment" : "Comment"}</DrawerTitle>
          <DrawerDescription className="line-clamp-2 text-pretty italic">
            &ldquo;{target.selection.selectedText}&rdquo;
          </DrawerDescription>
        </DrawerHeader>
        <div className="min-h-0 overflow-y-auto px-4 pb-2">
          <Textarea
            value={feedback}
            onChange={(event) => setFeedback(event.target.value)}
            onKeyDown={(event) => {
              if (event.key !== "Enter" || (!event.metaKey && !event.ctrlKey)) return;
              event.preventDefault();
              if (event.shiftKey && !isEditing) run();
              else submit();
            }}
            placeholder="Add your comment or instruction..."
            aria-label="Comment on agent response"
            className="mb-3 min-h-20 resize-none text-sm border-border/50 focus:border-primary/50"
            autoFocus
            data-testid="agent-message-comment-input"
          />
          <DrawerCommentActions
            isEditing={isEditing}
            disabled={!feedback.trim()}
            onAdd={submit}
            onRun={isEditing ? undefined : run}
            onDelete={onDelete}
          />
        </div>
      </DrawerContent>
    </Drawer>
  );
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

function useCommentTargetState(rootRef: RefObject<HTMLDivElement | null>, isSelectable: boolean) {
  const [target, setTarget] = useState<CommentTarget | null>(null);
  const [composerOpen, setComposerOpen] = useState(false);

  const close = useCallback(() => {
    setComposerOpen(false);
    setTarget(null);
    window.getSelection()?.removeAllRanges();
  }, []);

  const setNewSelection = useCallback((selection: MessageSelection, openComposer: boolean) => {
    setTarget({
      selection,
      position: { x: selection.rect.right, y: selection.rect.bottom },
    });
    setComposerOpen(openComposer);
  }, []);

  const captureSelection = useCallback(() => {
    if (!rootRef.current || !isSelectable) return;
    const selection = getMessageSelection(rootRef.current, window.getSelection());
    if (selection) setNewSelection(selection, false);
  }, [isSelectable, rootRef, setNewSelection]);

  useEffect(() => {
    if (!isSelectable) return;
    const handleShortcut = (event: KeyboardEvent) => {
      if (!((event.metaKey || event.ctrlKey) && event.shiftKey && event.key.toLowerCase() === "c"))
        return;
      if (!rootRef.current) return;
      const selection = getMessageSelection(rootRef.current, window.getSelection());
      if (!selection) return;
      event.preventDefault();
      event.stopImmediatePropagation();
      setNewSelection(selection, true);
    };
    document.addEventListener("keydown", handleShortcut, true);
    return () => document.removeEventListener("keydown", handleShortcut, true);
  }, [isSelectable, rootRef, setNewSelection]);

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
        position,
        editingCommentId: comment.id,
        editingText: comment.text,
      });
      setComposerOpen(true);
      window.getSelection()?.removeAllRanges();
    },
    [pendingComments, rootRef, setComposerOpen, setTarget],
  );

  const handleCommentClick = useCallback(
    (event: ReactMouseEvent<HTMLDivElement>) => {
      const element = (event.target as HTMLElement).closest<HTMLElement>(
        "[data-agent-message-comment-id]",
      );
      const commentId = element?.dataset.agentMessageCommentId;
      if (!commentId) return;
      event.preventDefault();
      event.stopPropagation();
      openExistingComment(commentId, { x: event.clientX, y: event.clientY });
    },
    [openExistingComment],
  );

  const handleCommentKeyDown = useCallback(
    (event: ReactKeyboardEvent<HTMLDivElement>) => {
      if (event.key !== "Enter" && event.key !== " ") return;
      const element = (event.target as HTMLElement).closest<HTMLElement>(
        ".agent-message-comment-badge[data-agent-message-comment-id]",
      );
      const commentId = element?.dataset.agentMessageCommentId;
      if (!commentId) return;
      event.preventDefault();
      const rect = element.getBoundingClientRect();
      openExistingComment(commentId, { x: rect.right, y: rect.bottom });
    },
    [openExistingComment],
  );

  return { handleCommentClick, handleCommentKeyDown };
}

function useMessageCommentActions({
  message,
  sessionId,
  target,
  rootRef,
  close,
}: {
  message: Message;
  sessionId: string | null | undefined;
  target: CommentTarget | null;
  rootRef: RefObject<HTMLDivElement | null>;
  close: () => void;
}) {
  const addComment = useCommentsStore((state) => state.addComment);
  const updateComment = useCommentsStore((state) => state.updateComment);
  const removeComment = useCommentsStore((state) => state.removeComment);
  const { runComment } = useRunComment({ sessionId: sessionId ?? null, taskId: message.task_id });

  const saveComment = useCallback(
    (feedback: string): AgentMessageComment | null => {
      if (!target || !sessionId || !feedback.trim()) return null;
      if (target.editingCommentId) {
        updateComment(target.editingCommentId, { text: feedback.trim() });
        return null;
      }
      const renderedText = rootRef.current?.textContent ?? "";
      const comment = commentFromTarget(message, sessionId, target, renderedText, feedback);
      addComment(comment);
      return comment;
    },
    [addComment, message, rootRef, sessionId, target, updateComment],
  );

  const handleAdd = useCallback(
    (feedback: string) => {
      saveComment(feedback);
      close();
    },
    [close, saveComment],
  );

  const handleRun = useCallback(
    (feedback: string) => {
      const comment = saveComment(feedback);
      close();
      if (comment) {
        void runComment(comment).catch((error) =>
          console.error("Failed to run agent message comment:", error),
        );
      }
    },
    [close, runComment, saveComment],
  );

  const handleDelete = target?.editingCommentId
    ? () => {
        removeComment(target.editingCommentId!);
        close();
      }
    : undefined;

  return { handleAdd, handleRun, handleDelete };
}

export function MessageCommentSurface({
  message,
  sessionId,
  isTurnActive,
  children,
}: MessageCommentSurfaceProps) {
  const rootRef = useRef<HTMLDivElement>(null);
  const useDrawer = useTouchDrawer();
  const pendingComments = usePendingCommentsForMessage(sessionId, message.id);
  const isSelectable = Boolean(sessionId) && isSelectableAgentMessage(message, isTurnActive, false);
  const targetState = useCommentTargetState(rootRef, isSelectable);
  const { target, composerOpen, setTarget, setComposerOpen, close, captureSelection } = targetState;
  const commentHandlers = useExistingCommentHandlers(
    rootRef,
    pendingComments,
    setTarget,
    setComposerOpen,
  );
  const actions = useMessageCommentActions({ message, sessionId, target, rootRef, close });
  const portalContainer = rootRef.current?.closest<HTMLElement>('[role="dialog"]');

  useLayoutEffect(() => {
    if (!rootRef.current) return;
    restoreMessageCommentHighlights(rootRef.current, pendingComments);
  }, [pendingComments, children, message.content]);

  return (
    <>
      <div
        ref={rootRef}
        className="agent-message-comment-body"
        data-agent-message-body="true"
        data-message-id={message.id}
        onMouseUp={captureSelection}
        onTouchEnd={captureSelection}
        onClick={commentHandlers.handleCommentClick}
        onKeyDown={commentHandlers.handleCommentKeyDown}
      >
        {children}
      </div>
      {target && !composerOpen ? (
        <SelectionCommentTrigger
          selection={target.selection}
          isTouch={useDrawer}
          onOpen={() => setComposerOpen(true)}
          portalContainer={portalContainer}
        />
      ) : null}
      {target && composerOpen && !useDrawer ? (
        <PlanSelectionPopover
          key={target.editingCommentId ?? `${target.selection.start}:${target.selection.end}`}
          selectedText={target.selection.selectedText}
          position={target.position}
          onAdd={(feedback) => actions.handleAdd(feedback)}
          onAddAndRun={
            target.editingCommentId ? undefined : (feedback) => actions.handleRun(feedback)
          }
          onClose={close}
          editingComment={target.editingText}
          onDelete={actions.handleDelete}
          testId="agent-message-comment-popover"
          inputTestId="agent-message-comment-input"
          addButtonTestId="agent-message-comment-add"
          runButtonTestId="agent-message-comment-run"
          portalContainer={portalContainer}
        />
      ) : null}
      {useDrawer ? (
        <MessageCommentDrawer
          target={target}
          open={Boolean(target && composerOpen)}
          onClose={close}
          onAdd={actions.handleAdd}
          onRun={actions.handleRun}
          onDelete={actions.handleDelete}
        />
      ) : null}
    </>
  );
}
