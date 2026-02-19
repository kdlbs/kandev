"use client";

import React, { useState, useCallback, useRef, useEffect } from "react";
import { createPortal } from "react-dom";
import { IconPlus, IconTrash, IconGripHorizontal } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import { cn } from "@/lib/utils";

type SelectionPosition = {
  x: number;
  y: number;
};

type PlanSelectionPopoverProps = {
  selectedText: string;
  position: SelectionPosition;
  onAdd: (comment: string, selectedText: string) => void;
  onClose: () => void;
  editingComment?: string;
  onDelete?: () => void;
};

const POPOVER_WIDTH = 340;
const POPOVER_HEIGHT = 180;
const MARGIN = 8;

function computePopoverPosition(position: SelectionPosition): { left: number; top: number } {
  // Place directly below cursor, aligned to left of click
  let left = position.x;
  let top = position.y + 4;

  // Clamp horizontal — keep popover on screen
  if (left + POPOVER_WIDTH > window.innerWidth - MARGIN) {
    left = window.innerWidth - POPOVER_WIDTH - MARGIN;
  }
  if (left < MARGIN) left = MARGIN;

  // If overflows bottom, flip above cursor
  if (top + POPOVER_HEIGHT > window.innerHeight - MARGIN) {
    top = position.y - POPOVER_HEIGHT - 4;
  }
  if (top < MARGIN) top = MARGIN;

  return { left, top };
}

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

export function PlanSelectionPopover({
  selectedText,
  position,
  onAdd,
  onClose,
  editingComment,
  onDelete,
}: PlanSelectionPopoverProps) {
  const [comment, setComment] = useState(editingComment || "");
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const popoverRef = useRef<HTMLDivElement>(null);
  const { offset, onMouseDown } = useDrag();

  useEffect(() => {
    textareaRef.current?.focus();
  }, []);
  usePopoverDismiss(onClose, popoverRef);

  const handleSubmit = useCallback(() => {
    if (!comment.trim()) return;
    onAdd(comment.trim(), selectedText);
    onClose();
  }, [comment, onAdd, selectedText, onClose]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        handleSubmit();
      }
    },
    [handleSubmit],
  );

  const isDisabled = !comment.trim();
  const previewText =
    selectedText.length > 80 ? selectedText.slice(0, 80).trim() + "\u2026" : selectedText;
  const { left, top } = computePopoverPosition(position);

  // Portal to document.body to escape dockview's CSS transform containing block
  return createPortal(
    <div
      ref={popoverRef}
      className={cn(
        "fixed z-50 rounded-xl border border-border/50 bg-popover/95 backdrop-blur-sm shadow-xl",
        "animate-in fade-in-0 zoom-in-95 duration-150",
      )}
      style={{
        width: POPOVER_WIDTH,
        left: left + offset.dx,
        top: top + offset.dy,
      }}
    >
      {/* Drag handle */}
      <div
        onMouseDown={onMouseDown}
        className="flex items-center justify-center py-1.5 cursor-grab active:cursor-grabbing border-b border-border/30"
      >
        <IconGripHorizontal className="h-3.5 w-3.5 text-muted-foreground/50" />
      </div>

      <div className="p-3">
        {/* Text preview */}
        <p className="mb-2 text-xs text-muted-foreground line-clamp-2 leading-relaxed italic">
          &ldquo;{previewText}&rdquo;
        </p>

        {/* Comment input */}
        <Textarea
          ref={textareaRef}
          value={comment}
          onChange={(e) => setComment(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Add your comment or instruction..."
          className="min-h-[60px] resize-none text-sm border-border/50 focus:border-primary/50"
        />

        {/* Actions */}
        <div className="mt-2 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span className="text-[10px] text-muted-foreground/70">
              ⌘+Enter to {editingComment ? "update" : "add"}
            </span>
            {editingComment && onDelete && (
              <Button
                size="sm"
                variant="ghost"
                onClick={() => {
                  onDelete();
                  onClose();
                }}
                className="h-6 px-1.5 text-muted-foreground hover:text-destructive cursor-pointer"
              >
                <IconTrash className="h-3 w-3" />
              </Button>
            )}
          </div>
          <Button
            size="sm"
            onClick={handleSubmit}
            disabled={isDisabled}
            className="h-7 gap-1 text-xs cursor-pointer"
          >
            <IconPlus className="h-3 w-3" />
            {editingComment ? "Update" : "Add"}
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  );
}
