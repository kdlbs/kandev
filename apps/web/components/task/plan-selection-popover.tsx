"use client";

import React, { useState, useCallback, useRef, useEffect } from "react";
import { createPortal } from "react-dom";
import { IconPlus, IconTrash, IconGripHorizontal, IconPlayerPlay } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { floatingBounds, placeFloatingRect } from "@/components/task/floating-selection-position";

type SelectionPosition = {
  x: number;
  y: number;
};

type PlanSelectionPopoverProps = {
  selectedText: string;
  position: SelectionPosition;
  onAdd: (comment: string, selectedText: string) => boolean | void;
  onAddAndRun?: (comment: string, selectedText: string) => boolean | void;
  onClose: () => void;
  editingComment?: string;
  onDelete?: () => void;
  testId?: string;
  inputTestId?: string;
  addButtonTestId?: string;
  runButtonTestId?: string;
  portalContainer?: HTMLElement | null;
  errorMessage?: string | null;
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
  onAdd: (comment: string, selectedText: string) => boolean | void,
  onClose: () => void,
  onAddAndRun?: (comment: string, selectedText: string) => boolean | void,
) {
  const handleSubmit = useCallback(() => {
    if (!comment.trim()) return;
    if (onAdd(comment.trim(), selectedText) !== false) onClose();
  }, [comment, onAdd, selectedText, onClose]);

  const handleSubmitAndRun = useCallback(() => {
    if (!comment.trim() || !onAddAndRun) return;
    if (onAddAndRun(comment.trim(), selectedText) !== false) onClose();
  }, [comment, onAddAndRun, selectedText, onClose]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        if (e.shiftKey && onAddAndRun) {
          handleSubmitAndRun();
        } else {
          handleSubmit();
        }
      }
    },
    [handleSubmit, handleSubmitAndRun, onAddAndRun],
  );

  return {
    handleSubmit,
    handleSubmitAndRun,
    handleKeyDown,
    isDisabled: !comment.trim(),
    previewText:
      selectedText.length > 80 ? selectedText.slice(0, 80).trim() + "\u2026" : selectedText,
  };
}

function PopoverActions({
  isEditing,
  isDisabled,
  onSubmit,
  onSubmitAndRun,
  onDelete,
  addButtonTestId,
  runButtonTestId,
}: {
  isEditing: boolean;
  isDisabled: boolean;
  onSubmit: () => void;
  onSubmitAndRun?: () => void;
  onDelete?: () => void;
  addButtonTestId?: string;
  runButtonTestId?: string;
}) {
  return (
    <div className="mt-2 flex items-center justify-between">
      <div className="flex items-center gap-2">
        <span className="text-[10px] text-muted-foreground/70">
          ⌘+Enter to {isEditing ? "update" : "add"}
          {onSubmitAndRun && !isEditing ? ", ⌘+Shift+Enter to run" : ""}
        </span>
        {isEditing && onDelete && (
          <Button
            size="sm"
            variant="ghost"
            onClick={onDelete}
            aria-label="Delete comment"
            className="h-6 px-1.5 text-muted-foreground hover:text-destructive cursor-pointer"
          >
            <IconTrash className="h-3 w-3" />
          </Button>
        )}
      </div>
      <TooltipProvider delayDuration={400}>
        <div className="inline-flex">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                size="sm"
                variant={onSubmitAndRun && !isEditing ? "outline" : "default"}
                onClick={onSubmit}
                disabled={isDisabled}
                data-testid={addButtonTestId}
                className={`h-7 gap-1 text-xs cursor-pointer ${onSubmitAndRun && !isEditing ? "rounded-r-none border-r-0" : ""}`}
              >
                <IconPlus className="h-3 w-3" />
                {isEditing ? "Update" : "Add"}
              </Button>
            </TooltipTrigger>
            <TooltipContent side="bottom">
              <p>{isEditing ? "Update comment" : "Save comment for review"}</p>
            </TooltipContent>
          </Tooltip>
          {onSubmitAndRun && !isEditing && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  size="sm"
                  onClick={onSubmitAndRun}
                  disabled={isDisabled}
                  data-testid={runButtonTestId}
                  className="h-7 gap-1 rounded-l-none text-xs cursor-pointer"
                >
                  <IconPlayerPlay className="h-3 w-3" />
                  Run
                </Button>
              </TooltipTrigger>
              <TooltipContent side="bottom">
                <p>Save and send to agent</p>
              </TooltipContent>
            </Tooltip>
          )}
        </div>
      </TooltipProvider>
    </div>
  );
}

export function PlanSelectionPopover({
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
}: PlanSelectionPopoverProps) {
  const [comment, setComment] = useState(editingComment || "");
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const popoverRef = useRef<HTMLDivElement>(null);
  const { offset, onMouseDown } = useDrag();

  useEffect(() => {
    textareaRef.current?.focus();
  }, []);
  usePopoverDismiss(onClose, popoverRef);
  const effectiveOnAddAndRun = editingComment ? undefined : onAddAndRun;
  const { handleSubmit, handleSubmitAndRun, handleKeyDown, isDisabled, previewText } =
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

  const handleDelete = onDelete
    ? () => {
        onDelete();
        onClose();
      }
    : undefined;

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
          placeholder="Add your comment or instruction..."
          className="min-h-[60px] resize-none text-sm border-border/50 focus:border-primary/50"
          data-testid={inputTestId}
        />
        {errorMessage ? (
          <p role="alert" className="mt-2 text-xs text-destructive">
            {errorMessage}
          </p>
        ) : null}
        <PopoverActions
          isEditing={!!editingComment}
          isDisabled={isDisabled}
          onSubmit={handleSubmit}
          onSubmitAndRun={effectiveOnAddAndRun ? handleSubmitAndRun : undefined}
          onDelete={handleDelete}
          addButtonTestId={addButtonTestId}
          runButtonTestId={runButtonTestId}
        />
      </div>
    </div>,
    portalContainer ?? document.body,
  );
}
