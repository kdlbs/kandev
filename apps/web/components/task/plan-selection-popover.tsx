"use client";

import React, { useState, useCallback, useRef, useEffect } from "react";
import { createPortal } from "react-dom";
import { IconPlus, IconTrash, IconGripHorizontal, IconPlayerPlay } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { floatingBounds, placeFloatingRect } from "@/components/task/floating-selection-position";
import { useTranslation } from "react-i18next";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";

type SelectionPosition = {
  x: number;
  y: number;
};

type PlanSelectionPopoverProps = {
  selectedText: string;
  position: SelectionPosition;
  onAdd: (comment: string, selectedText: string) => boolean | void | Promise<boolean | void>;
  onAddAndRun?: (comment: string, selectedText: string) => boolean | void | Promise<boolean | void>;
  onClose: () => void;
  editingComment?: string;
  onDelete?: () => boolean | void | Promise<boolean | void>;
  testId?: string;
  inputTestId?: string;
  addButtonTestId?: string;
  runButtonTestId?: string;
  portalContainer?: HTMLElement | null;
  errorMessage?: string | null;
  runDisabledReason?: string | null;
};

const POPOVER_WIDTH = 340;
const POPOVER_HEIGHT = 180;
const MARGIN = 8;

/** Drag support for the popover. */
function useDrag() {
  const [offset, setOffset] = useState({ dx: 0, dy: 0 });
  const dragging = useRef(false);
  const startPos = useRef({ x: 0, y: 0 });
  const startOffset = useRef({ dx: 0, dy: 0 });

  const onMouseDown = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      dragging.current = true;
      startPos.current = { x: e.clientX, y: e.clientY };
      startOffset.current = { ...offset };

      const onMouseMove = (ev: MouseEvent) => {
        if (!dragging.current) return;
        setOffset({
          dx: startOffset.current.dx + ev.clientX - startPos.current.x,
          dy: startOffset.current.dy + ev.clientY - startPos.current.y,
        });
      };
      const onMouseUp = () => {
        dragging.current = false;
        document.removeEventListener("mousemove", onMouseMove);
        document.removeEventListener("mouseup", onMouseUp);
      };
      document.addEventListener("mousemove", onMouseMove);
      document.addEventListener("mouseup", onMouseUp);
    },
    [offset],
  );

  return { offset, onMouseDown };
}

function usePopoverDismiss(
  onClose: () => void,
  popoverRef: React.RefObject<HTMLDivElement | null>,
) {
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (popoverRef.current && !popoverRef.current.contains(e.target as Node)) onClose();
    };
    const timer = setTimeout(() => {
      document.addEventListener("mousedown", handleClickOutside);
    }, 100);
    return () => {
      clearTimeout(timer);
      document.removeEventListener("mousedown", handleClickOutside);
    };
  }, [onClose, popoverRef]);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);
}

function usePopoverComposer(
  comment: string,
  selectedText: string,
  onAdd: (comment: string, selectedText: string) => boolean | void | Promise<boolean | void>,
  onClose: () => void,
  onAddAndRun?: (comment: string, selectedText: string) => boolean | void | Promise<boolean | void>,
) {
  const [isSubmitting, setIsSubmitting] = useState(false);
  const handleSubmit = useCallback(async () => {
    if (!comment.trim()) return;
    setIsSubmitting(true);
    try {
      if ((await onAdd(comment.trim(), selectedText)) !== false) onClose();
    } finally {
      setIsSubmitting(false);
    }
  }, [comment, onAdd, selectedText, onClose]);

  const handleSubmitAndRun = useCallback(async () => {
    if (!comment.trim() || !onAddAndRun) return;
    setIsSubmitting(true);
    try {
      if ((await onAddAndRun(comment.trim(), selectedText)) !== false) onClose();
    } finally {
      setIsSubmitting(false);
    }
  }, [comment, onAddAndRun, selectedText, onClose]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.repeat) return;
      if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        if (e.shiftKey && onAddAndRun) {
          void handleSubmitAndRun();
        } else {
          void handleSubmit();
        }
      }
    },
    [handleSubmit, handleSubmitAndRun, onAddAndRun],
  );

  return {
    handleSubmit,
    handleSubmitAndRun,
    handleKeyDown,
    isDisabled: !comment.trim() || isSubmitting,
    isSubmitting,
    previewText:
      selectedText.length > 80 ? selectedText.slice(0, 80).trim() + "\u2026" : selectedText,
  };
}

function useCommentDelete(onDelete: PlanSelectionPopoverProps["onDelete"], onClose: () => void) {
  const [isDeleting, setIsDeleting] = useState(false);
  const deleteComment = useCallback(async () => {
    if (!onDelete) return;
    setIsDeleting(true);
    try {
      if ((await onDelete()) !== false) onClose();
    } finally {
      setIsDeleting(false);
    }
  }, [onClose, onDelete]);
  return { isDeleting, deleteComment: onDelete ? deleteComment : undefined };
}

function CommentMutationMessages({
  errorMessage,
  runDisabledReason,
  canRun,
  className,
}: {
  errorMessage?: string | null;
  runDisabledReason?: string | null;
  canRun: boolean;
  className: string;
}) {
  return (
    <>
      {errorMessage ? (
        <p role="alert" className={cn(className, "text-destructive")}>
          {errorMessage}
        </p>
      ) : null}
      {runDisabledReason && canRun ? (
        <p className={cn(className, "text-muted-foreground")}>{runDisabledReason}</p>
      ) : null}
    </>
  );
}

function PlanCommentDrawerHeader({
  editingComment,
  selectedText,
}: {
  editingComment?: string;
  selectedText: string;
}) {
  const { t } = useTranslation();
  return (
    <DrawerHeader className="shrink-0 px-4 pb-3 text-left">
      <DrawerTitle>{editingComment ? t("task:editComment") : t("task:comment")}</DrawerTitle>
      <DrawerDescription className="line-clamp-2 text-pretty italic">
        &ldquo;{selectedText}&rdquo;
      </DrawerDescription>
    </DrawerHeader>
  );
}

function DeleteCommentButton({ onDelete, disabled }: { onDelete?: () => void; disabled: boolean }) {
  const { t } = useTranslation();
  if (!onDelete) return null;
  return (
    <Button
      size="sm"
      variant="ghost"
      onClick={onDelete}
      disabled={disabled}
      aria-label={t("task:deleteComment")}
      className="h-6 px-1.5 text-muted-foreground hover:text-destructive cursor-pointer [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:min-w-11"
    >
      <IconTrash className="h-3 w-3" />
    </Button>
  );
}

function RunCommentButton({
  onSubmitAndRun,
  isDisabled,
  runButtonTestId,
  runDisabledReason,
}: {
  onSubmitAndRun?: () => void;
  isDisabled: boolean;
  runButtonTestId?: string;
  runDisabledReason?: string | null;
}) {
  const { t } = useTranslation();
  if (!onSubmitAndRun) return null;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex" tabIndex={runDisabledReason ? 0 : undefined}>
          <Button
            size="sm"
            onClick={onSubmitAndRun}
            disabled={isDisabled || !!runDisabledReason}
            data-testid={runButtonTestId}
            className="h-7 gap-1 rounded-l-none text-xs cursor-pointer [@media(pointer:coarse)]:h-11"
          >
            <IconPlayerPlay className="h-3 w-3" />
            {t("task:run")}
          </Button>
        </span>
      </TooltipTrigger>
      <TooltipContent side="bottom">
        <p>{runDisabledReason || t("task:saveAndSendToAgent")}</p>
      </TooltipContent>
    </Tooltip>
  );
}

function PopoverActions({
  isEditing,
  isDisabled,
  onSubmit,
  onSubmitAndRun,
  onDelete,
  addButtonTestId,
  runButtonTestId,
  runDisabledReason,
  mutationPending,
}: {
  isEditing: boolean;
  isDisabled: boolean;
  onSubmit: () => void;
  onSubmitAndRun?: () => void;
  onDelete?: () => void;
  addButtonTestId?: string;
  runButtonTestId?: string;
  runDisabledReason?: string | null;
  mutationPending: boolean;
}) {
  const { t } = useTranslation();
  const splitAction = Boolean(onSubmitAndRun && !isEditing);
  return (
    <div className="mt-2 flex items-center justify-between">
      <div className="flex items-center gap-2">
        <span className="text-[10px] text-muted-foreground/70">
          {isEditing ? t("task:cmdEnterToUpdate") : t("task:cmdEnterToAdd")}
          {onSubmitAndRun && !isEditing ? t("task:shiftEnterToRun") : ""}
        </span>
        <DeleteCommentButton
          onDelete={isEditing ? onDelete : undefined}
          disabled={mutationPending}
        />
      </div>
      <TooltipProvider delayDuration={400}>
        <div className="inline-flex">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                size="sm"
                variant={splitAction ? "outline" : "default"}
                onClick={onSubmit}
                disabled={isDisabled}
                data-testid={addButtonTestId}
                className={cn(
                  "h-7 gap-1 text-xs cursor-pointer [@media(pointer:coarse)]:h-11",
                  splitAction && "rounded-r-none border-r-0",
                )}
              >
                <IconPlus className="h-3 w-3" />
                {isEditing ? t("task:update") : t("task:add")}
              </Button>
            </TooltipTrigger>
            <TooltipContent side="bottom">
              <p>{isEditing ? t("task:updateComment") : t("task:saveCommentForReview")}</p>
            </TooltipContent>
          </Tooltip>
          <RunCommentButton
            onSubmitAndRun={splitAction ? onSubmitAndRun : undefined}
            isDisabled={isDisabled}
            runButtonTestId={runButtonTestId}
            runDisabledReason={runDisabledReason}
          />
        </div>
      </TooltipProvider>
    </div>
  );
}

function DesktopPlanSelectionPopover({
  selectedText,
  position,
  onAdd,
  onAddAndRun,
  onClose,
  editingComment,
  onDelete,
  testId,
  inputTestId,
  addButtonTestId,
  runButtonTestId,
  portalContainer,
  errorMessage,
  runDisabledReason,
}: PlanSelectionPopoverProps) {
  const { t } = useTranslation();
  const [comment, setComment] = useState(editingComment || "");
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const popoverRef = useRef<HTMLDivElement>(null);
  const { offset, onMouseDown } = useDrag();
  const { isDeleting, deleteComment } = useCommentDelete(onDelete, onClose);

  useEffect(() => {
    textareaRef.current?.focus();
  }, []);
  usePopoverDismiss(onClose, popoverRef);
  const effectiveOnAddAndRun = editingComment ? undefined : onAddAndRun;
  const { handleSubmit, handleSubmitAndRun, handleKeyDown, isDisabled, isSubmitting, previewText } =
    usePopoverComposer(comment, selectedText, onAdd, onClose, effectiveOnAddAndRun);
  const portalRect = portalContainer?.getBoundingClientRect();
  const { left, top } = placeFloatingRect({
    left: position.x,
    topCandidates: [position.y + 4, position.y - POPOVER_HEIGHT - 4],
    width: POPOVER_WIDTH,
    height: POPOVER_HEIGHT,
    bounds: floatingBounds(portalRect),
    margin: MARGIN,
  });

  return createPortal(
    <div
      ref={popoverRef}
      className={cn(
        "pointer-events-auto z-[60] rounded-xl border border-border/50 bg-popover/95 backdrop-blur-sm shadow-xl",
        "animate-in fade-in-0 zoom-in-95 duration-150",
        portalContainer ? "absolute" : "fixed",
      )}
      data-testid={testId}
      style={{
        width: POPOVER_WIDTH,
        left: left + offset.dx - (portalRect?.left ?? 0),
        top: top + offset.dy - (portalRect?.top ?? 0),
      }}
    >
      <div
        onMouseDown={onMouseDown}
        className="flex items-center justify-center py-1.5 cursor-grab active:cursor-grabbing border-b border-border/30"
      >
        <IconGripHorizontal className="h-3.5 w-3.5 text-muted-foreground/50" />
      </div>
      <div className="p-3">
        <p className="mb-2 text-xs text-muted-foreground line-clamp-2 leading-relaxed italic">
          &ldquo;{previewText}&rdquo;
        </p>
        <Textarea
          ref={textareaRef}
          value={comment}
          onChange={(e) => setComment(e.target.value)}
          onKeyDown={handleKeyDown}
          disabled={isSubmitting || isDeleting}
          placeholder={t("task:addYourCommentOrInstruction")}
          className="min-h-[60px] resize-none text-sm border-border/50 focus:border-primary/50"
          data-testid={inputTestId}
        />
        <CommentMutationMessages
          errorMessage={errorMessage}
          runDisabledReason={runDisabledReason}
          canRun={Boolean(effectiveOnAddAndRun)}
          className="mt-2 text-xs"
        />
        <PopoverActions
          isEditing={!!editingComment}
          isDisabled={isDisabled || isDeleting}
          onSubmit={() => void handleSubmit()}
          onSubmitAndRun={effectiveOnAddAndRun ? () => void handleSubmitAndRun() : undefined}
          onDelete={deleteComment ? () => void deleteComment() : undefined}
          addButtonTestId={addButtonTestId}
          runButtonTestId={runButtonTestId}
          runDisabledReason={runDisabledReason}
          mutationPending={isSubmitting || isDeleting}
        />
      </div>
    </div>,
    portalContainer ?? document.body,
  );
}

function PlanSelectionDrawer({
  selectedText,
  onAdd,
  onAddAndRun,
  onClose,
  editingComment,
  onDelete,
  testId,
  inputTestId,
  addButtonTestId,
  runButtonTestId,
  errorMessage,
  runDisabledReason,
}: PlanSelectionPopoverProps) {
  const { t } = useTranslation();
  const [comment, setComment] = useState(editingComment || "");
  const effectiveOnAddAndRun = editingComment ? undefined : onAddAndRun;
  const { isDeleting, deleteComment } = useCommentDelete(onDelete, onClose);
  const { handleSubmit, handleSubmitAndRun, handleKeyDown, isDisabled, isSubmitting } =
    usePopoverComposer(comment, selectedText, onAdd, onClose, effectiveOnAddAndRun);
  const disabled = isDisabled || isDeleting;

  return (
    <Drawer open onOpenChange={(open) => !open && onClose()}>
      <DrawerContent
        className="z-[60] max-h-[82dvh] pb-[calc(1rem+env(safe-area-inset-bottom))]"
        data-testid={testId ?? "plan-comment-drawer"}
      >
        <PlanCommentDrawerHeader editingComment={editingComment} selectedText={selectedText} />
        <div className="min-h-0 overflow-y-auto px-4 pb-2">
          <Textarea
            value={comment}
            onChange={(event) => setComment(event.target.value)}
            onKeyDown={handleKeyDown}
            disabled={isSubmitting || isDeleting}
            placeholder={t("task:addYourCommentOrInstruction")}
            className="mb-3 min-h-20 resize-none text-sm border-border/50 focus:border-primary/50"
            data-testid={inputTestId}
            autoFocus
          />
          <CommentMutationMessages
            errorMessage={errorMessage}
            runDisabledReason={runDisabledReason}
            canRun={Boolean(effectiveOnAddAndRun)}
            className="mb-3 text-xs"
          />
          <div className="flex items-center justify-between gap-3">
            {editingComment && deleteComment ? (
              <Button
                type="button"
                size="sm"
                variant="ghost"
                onClick={() => void deleteComment()}
                disabled={isDeleting || isSubmitting}
                aria-label={t("task:deleteComment")}
                className="min-h-11 min-w-11 cursor-pointer px-3 text-muted-foreground hover:text-destructive"
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
                variant={effectiveOnAddAndRun ? "outline" : "default"}
                onClick={() => void handleSubmit()}
                disabled={disabled}
                data-testid={addButtonTestId}
                className={cn(
                  "min-h-11 cursor-pointer gap-1.5 px-4 active:scale-[0.96]",
                  effectiveOnAddAndRun && "rounded-r-none border-r-0",
                )}
              >
                <IconPlus className="h-4 w-4" />
                {editingComment ? t("task:update") : t("task:add")}
              </Button>
              {effectiveOnAddAndRun ? (
                <Button
                  type="button"
                  size="sm"
                  onClick={() => void handleSubmitAndRun()}
                  disabled={disabled || !!runDisabledReason}
                  data-testid={runButtonTestId}
                  className="min-h-11 cursor-pointer gap-1.5 rounded-l-none px-4 active:scale-[0.96]"
                >
                  <IconPlayerPlay className="h-4 w-4" />
                  {t("task:run")}
                </Button>
              ) : null}
            </div>
          </div>
        </div>
      </DrawerContent>
    </Drawer>
  );
}

export function PlanSelectionPopover(props: PlanSelectionPopoverProps) {
  const useDrawer = useTouchDrawer();
  return useDrawer ? (
    <PlanSelectionDrawer {...props} />
  ) : (
    <DesktopPlanSelectionPopover {...props} />
  );
}
