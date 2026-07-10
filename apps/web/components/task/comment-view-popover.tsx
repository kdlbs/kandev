"use client";

import { useRef, useState, useCallback } from "react";
import { IconEdit, IconTrash, IconGripHorizontal } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { cn } from "@/lib/utils";
import type { DiffComment } from "@/lib/diff/types";
import { CommentForm } from "@/components/diff/comment-form";
import { useDraggablePopover, usePopoverDismiss } from "@/components/task/use-draggable-popover";

type CommentViewPopoverProps = {
  comments: DiffComment[];
  position: { x: number; y: number };
  onDelete: (commentId: string) => void;
  onUpdate?: (commentId: string, text: string) => void;
  onClose: () => void;
};

function formatLineRange(start: number, end: number) {
  return start === end ? `L${start}` : `L${start}-${end}`;
}

function CommentItem({
  comment,
  onDelete,
  onUpdate,
  isEditing,
  onStartEdit,
  onCancelEdit,
}: {
  comment: DiffComment;
  onDelete: (id: string) => void;
  onUpdate?: (id: string, text: string) => void;
  isEditing: boolean;
  onStartEdit: (id: string) => void;
  onCancelEdit: () => void;
}) {
  const handleUpdate = useCallback(
    (text: string) => {
      onUpdate?.(comment.id, text);
      onCancelEdit();
    },
    [comment.id, onCancelEdit, onUpdate],
  );

  return (
    <div className="p-3">
      <div className="flex items-center justify-between mb-2">
        <span className="px-2 py-0.5 rounded-md bg-muted text-[10px] font-mono text-muted-foreground">
          {formatLineRange(comment.startLine, comment.endLine)}
        </span>
        <div className="flex items-center gap-1">
          {onUpdate && !isEditing && (
            <Button
              size="sm"
              variant="ghost"
              className="h-6 w-6 p-0 cursor-pointer text-muted-foreground hover:text-foreground"
              aria-label="Edit comment"
              title="Edit comment"
              onClick={() => onStartEdit(comment.id)}
            >
              <IconEdit className="h-3.5 w-3.5" />
            </Button>
          )}
          <Button
            size="sm"
            variant="ghost"
            className="h-6 w-6 p-0 cursor-pointer text-muted-foreground hover:text-destructive"
            aria-label="Delete comment"
            title="Delete comment"
            onClick={() => onDelete(comment.id)}
          >
            <IconTrash className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>
      {isEditing ? (
        <CommentForm
          initialContent={comment.text}
          onSubmit={handleUpdate}
          onCancel={onCancelEdit}
          isEditing
        />
      ) : (
        <>
          {comment.codeContent && (
            <pre className="mb-2 p-2 rounded-md bg-muted/50 text-[10px] text-muted-foreground font-mono max-h-[80px] overflow-auto whitespace-pre-wrap">
              {comment.codeContent}
            </pre>
          )}
          <p className="text-xs text-foreground leading-relaxed whitespace-pre-wrap">
            {comment.text}
          </p>
        </>
      )}
    </div>
  );
}

export function CommentViewPopover({
  comments,
  position,
  onDelete,
  onUpdate,
  onClose,
}: CommentViewPopoverProps) {
  const popoverRef = useRef<HTMLDivElement>(null);
  const [editingCommentId, setEditingCommentId] = useState<string | null>(null);
  const { pos, onDragStart } = useDraggablePopover(position, 350, 200);
  usePopoverDismiss(onClose, popoverRef);

  return (
    <div
      ref={popoverRef}
      className={cn(
        "fixed z-50 w-[350px] max-h-[350px] overflow-auto rounded-xl border border-border/50 bg-popover/95 backdrop-blur-sm shadow-xl",
        "animate-in fade-in-0 zoom-in-95 duration-150",
      )}
      style={{ left: pos.left, top: pos.top }}
    >
      <div
        className="flex items-center justify-between px-3 pt-2 pb-1 cursor-grab active:cursor-grabbing select-none sticky top-0 bg-popover/95 backdrop-blur-sm z-10"
        onMouseDown={onDragStart}
      >
        <span className="text-xs text-muted-foreground">
          {comments.length} comment{comments.length !== 1 ? "s" : ""}
        </span>
        <IconGripHorizontal className="h-3.5 w-3.5 text-muted-foreground/40" />
      </div>
      <div className="divide-y divide-border/30">
        {comments.map((comment) => (
          <CommentItem
            key={comment.id}
            comment={comment}
            onDelete={onDelete}
            onUpdate={onUpdate}
            isEditing={editingCommentId === comment.id}
            onStartEdit={setEditingCommentId}
            onCancelEdit={() => setEditingCommentId(null)}
          />
        ))}
      </div>
    </div>
  );
}
