"use client";

import { useEffect, useState, type Dispatch, type SetStateAction } from "react";
import { createPortal } from "react-dom";
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
import { PlanSelectionPopover } from "@/components/task/plan-selection-popover";
import { floatingBounds, placeFloatingRect } from "@/components/task/floating-selection-position";
import type { AgentMessageComment } from "@/lib/state/slices/comments";
import type { MessageSelection } from "@/lib/chat/agent-message-comments";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";

export type CommentTarget = {
  selection: MessageSelection;
  anchor: AgentMessageComment["anchor"];
  position: { x: number; y: number };
  editingCommentId?: string;
  editingText?: string;
};

export type MessageCommentActions = {
  handleAdd: (feedback: string) => boolean | void;
  handleRun: (feedback: string) => boolean | void;
  handleDelete?: () => void;
  saveError: string | null;
};

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
  const { t } = useTranslation();
  const size = isTouch ? 44 : 28;
  const portalRect = portalContainer?.getBoundingClientRect();
  const outerSize = size + 8;
  const position = placeFloatingRect({
    left: selection.rect.right - outerSize,
    topCandidates: [selection.rect.top - outerSize - 8, selection.rect.bottom + 8],
    width: outerSize,
    height: outerSize,
    bounds: floatingBounds(portalRect),
  });
  return createPortal(
    <div
      className={cn(
        "pointer-events-auto z-40 rounded-lg border border-border/50 bg-popover p-1 shadow-lg",
        portalContainer ? "absolute" : "fixed",
      )}
      style={{
        left: position.left - (portalRect?.left ?? 0),
        top: position.top - (portalRect?.top ?? 0),
      }}
    >
      <button
        type="button"
        title={t("task:commentCmdShiftC")}
        aria-label={t("task:commentOnSelection")}
        data-testid="agent-message-comment-trigger"
        className="flex cursor-pointer items-center justify-center rounded bg-primary text-primary-foreground transition-transform duration-150 ease-out hover:bg-primary/90 active:scale-[0.96]"
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
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-between gap-3">
      {isEditing && onDelete ? (
        <Button
          type="button"
          size="sm"
          variant="ghost"
          aria-label={t("task:deleteComment")}
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
          {isEditing ? t("task:update") : t("task:add")}
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
            {t("task:run")}
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
  errorMessage,
}: {
  target: CommentTarget | null;
  open: boolean;
  onClose: () => void;
  onAdd: (feedback: string) => boolean | void;
  onRun: ((feedback: string) => boolean | void) | undefined;
  onDelete: (() => void) | undefined;
  errorMessage?: string | null;
}) {
  const { t } = useTranslation();
  const [feedback, setFeedback] = useState(target?.editingText ?? "");
  const targetKey = `${target?.editingCommentId ?? "new"}:${target?.selection.start ?? 0}:${target?.selection.end ?? 0}`;

  useEffect(() => {
    setFeedback(target?.editingText ?? "");
  }, [targetKey, target?.editingText]);

  if (!target) return null;
  const isEditing = target.editingCommentId !== undefined;
  const submit = () => {
    if (feedback.trim() && onAdd(feedback.trim()) !== false) onClose();
  };
  const run = () => {
    if (onRun && feedback.trim() && onRun(feedback.trim()) !== false) onClose();
  };

  return (
    <Drawer open={open} onOpenChange={(next) => !next && onClose()}>
      <DrawerContent
        className="z-[60] max-h-[82dvh] pb-[calc(1rem+env(safe-area-inset-bottom))]"
        data-testid="agent-message-comment-drawer"
      >
        <DrawerHeader className="shrink-0 px-4 pb-3 text-left">
          <DrawerTitle>{isEditing ? t("task:editComment") : t("task:comment")}</DrawerTitle>
          <DrawerDescription className="line-clamp-2 text-pretty italic">
            “{target.selection.selectedText}”
          </DrawerDescription>
        </DrawerHeader>
        <div className="min-h-0 overflow-y-auto px-4 pb-2">
          <Textarea
            value={feedback}
            onChange={(event) => setFeedback(event.target.value)}
            onKeyDown={(event) => {
              if (event.key !== "Enter" || (!event.metaKey && !event.ctrlKey)) return;
              event.preventDefault();
              if (event.shiftKey && !isEditing && onRun) run();
              else submit();
            }}
            placeholder={t("task:addYourCommentOrInstruction")}
            aria-label={t("task:commentOnAgentResponse")}
            className="mb-3 min-h-20 resize-none text-sm border-border/50 focus:border-primary/50"
            autoFocus
            data-testid="agent-message-comment-input"
          />
          {errorMessage ? (
            <p role="alert" className="mb-3 text-xs text-destructive">
              {errorMessage}
            </p>
          ) : null}
          <DrawerCommentActions
            isEditing={isEditing}
            disabled={!feedback.trim()}
            onAdd={submit}
            onRun={isEditing || !onRun ? undefined : run}
            onDelete={onDelete}
          />
        </div>
      </DrawerContent>
    </Drawer>
  );
}

export function MessageCommentControls({
  target,
  composerOpen,
  useDrawer,
  setComposerOpen,
  close,
  actions,
  handleRun,
  portalContainer,
}: {
  target: CommentTarget | null;
  composerOpen: boolean;
  useDrawer: boolean;
  setComposerOpen: Dispatch<SetStateAction<boolean>>;
  close: () => void;
  actions: MessageCommentActions;
  handleRun: ((feedback: string) => boolean | void) | undefined;
  portalContainer: HTMLElement | null;
}) {
  return (
    <>
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
          onAdd={actions.handleAdd}
          onAddAndRun={target.editingCommentId ? undefined : handleRun}
          onClose={close}
          editingComment={target.editingText}
          onDelete={actions.handleDelete}
          testId="agent-message-comment-popover"
          inputTestId="agent-message-comment-input"
          addButtonTestId="agent-message-comment-add"
          runButtonTestId="agent-message-comment-run"
          portalContainer={portalContainer}
          errorMessage={actions.saveError}
        />
      ) : null}
      {useDrawer ? (
        <MessageCommentDrawer
          target={target}
          open={Boolean(target && composerOpen)}
          onClose={close}
          onAdd={actions.handleAdd}
          onRun={handleRun}
          onDelete={actions.handleDelete}
          errorMessage={actions.saveError}
        />
      ) : null}
    </>
  );
}
