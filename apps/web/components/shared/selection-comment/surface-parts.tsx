"use client";

import React, { useState, useCallback, useEffect, useReducer, type ReactNode } from "react";
import { IconPlus, IconTrash, IconPlayerPlay } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { DrawerDescription, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import type { SelectionCommentSurfaceProps } from "./surface-types";

export function usePopoverDismiss(
  onClose: () => void,
  popoverRef: React.RefObject<HTMLDivElement | null>,
  disabled: boolean,
) {
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (!disabled && popoverRef.current && !popoverRef.current.contains(e.target as Node)) {
        onClose();
      }
    };
    const timer = setTimeout(() => {
      document.addEventListener("mousedown", handleClickOutside);
    }, 100);
    return () => {
      clearTimeout(timer);
      document.removeEventListener("mousedown", handleClickOutside);
    };
  }, [disabled, onClose, popoverRef]);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !disabled) onClose();
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [disabled, onClose]);
}

export function usePopoverComposer(
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

export function useCommentDelete(
  onDelete: SelectionCommentSurfaceProps["onDelete"],
  onClose: () => void,
) {
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

export function CommentMutationMessages({
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

export function SurfaceDrawerHeader({
  editingComment,
  selectedText,
  contextLabel,
  evidence,
}: {
  editingComment?: string;
  selectedText: string;
  contextLabel?: string;
  evidence?: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <DrawerHeader className="shrink-0 px-4 pb-3 text-left">
      <DrawerTitle className="flex min-w-0 items-center gap-2">
        <span>{editingComment ? t("task:editComment") : t("task:comment")}</span>
        {contextLabel ? <SurfaceContextLabel label={contextLabel} /> : null}
      </DrawerTitle>
      <DrawerDescription className="line-clamp-2 text-pretty italic">
        “{selectedText}”
      </DrawerDescription>
      {evidence ? <div className="mt-2 min-w-0">{evidence}</div> : null}
    </DrawerHeader>
  );
}

export function SurfaceContextLabel({ label }: { label: string }) {
  return (
    <span
      className="min-w-0 truncate rounded-md bg-muted px-2 py-0.5 font-mono text-xs font-normal text-muted-foreground"
      data-testid="selection-comment-context-label"
    >
      {label}
    </span>
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

export function PopoverActions({
  isEditing,
  isDisabled,
  onSubmit,
  onSubmitAndRun,
  onDelete,
  addButtonTestId,
  runButtonTestId,
  runDisabledReason,
  mutationPending,
  onCancel,
}: {
  isEditing: boolean;
  isDisabled: boolean;
  onSubmit: () => void;
  onSubmitAndRun?: () => void;
  onDelete?: () => void;
  /** Shows a Close button that leaves without saving. */
  onCancel?: () => void;
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
        <div className="inline-flex items-center gap-1">
          {onCancel && (
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={onCancel}
              disabled={mutationPending}
              className="h-7 cursor-pointer text-xs [@media(pointer:coarse)]:h-11"
            >
              {t("task:close")}
            </Button>
          )}
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
        </div>
      </TooltipProvider>
    </div>
  );
}

export function isCommentMutationPending(isSubmitting: boolean, isDeleting: boolean): boolean {
  return isSubmitting || isDeleting;
}

export function useMutationProtectedClose(onClose: () => void, mutationPending: boolean) {
  return useCallback(() => {
    if (!mutationPending) onClose();
  }, [mutationPending, onClose]);
}

/** Local comment state, or the caller's controlled draft when both props are supplied. */
export function useSurfaceComment(
  editingComment: string | undefined,
  draftComment: string | undefined,
  onDraftChange: ((comment: string) => void) | undefined,
) {
  const [localComment, setLocalComment] = useState(editingComment || "");
  const controlled = draftComment !== undefined && onDraftChange !== undefined;
  return controlled
    ? ([draftComment, onDraftChange] as const)
    : ([localComment, setLocalComment] as const);
}

/** Re-renders on viewport resize so placement re-clamps to the new bounds. */
export function useViewportResizeRender() {
  const [, rerender] = useReducer((count: number) => count + 1, 0);
  useEffect(() => {
    window.addEventListener("resize", rerender);
    return () => window.removeEventListener("resize", rerender);
  }, []);
}
