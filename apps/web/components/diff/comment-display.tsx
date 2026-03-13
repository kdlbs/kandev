"use client";

import { Button } from "@kandev/ui/button";
import { IconTrash, IconEdit, IconMessage, IconPlayerPlay } from "@tabler/icons-react";
import type { DiffComment } from "@/lib/diff/types";
import { formatLineRange } from "@/lib/diff";

interface CommentDisplayProps {
  /** The comment to display */
  comment: DiffComment;
  /** Callback to delete the comment */
  onDelete?: () => void;
  /** Callback to edit the comment */
  onEdit?: () => void;
  /** Callback to run/send the comment to the agent immediately */
  onRun?: () => void;
  /** Whether to show the code content */
  showCode?: boolean;
  /** Compact mode for inline display */
  compact?: boolean;
}

type ActionButtonsProps = {
  onRun?: () => void;
  onEdit?: () => void;
  onDelete?: () => void;
  size: "compact" | "normal";
  stopPropagation?: boolean;
};

function CommentActionButtons({
  onRun,
  onEdit,
  onDelete,
  size,
  stopPropagation,
}: ActionButtonsProps) {
  const btnClass = size === "compact" ? "h-4 w-4 p-0" : "h-5 w-5 p-0";
  const wrap = (fn?: () => void) =>
    stopPropagation
      ? (e: React.MouseEvent) => {
          e.stopPropagation();
          fn?.();
        }
      : fn;

  return (
    <>
      {onRun && (
        <Button
          size="sm"
          variant="ghost"
          onClick={wrap(onRun)}
          aria-label="Run comment"
          title="Run comment"
          className={`cursor-pointer ${btnClass} text-emerald-600 hover:text-emerald-500 dark:text-emerald-400`}
        >
          <IconPlayerPlay className="h-3 w-3" />
        </Button>
      )}
      {onEdit && (
        <Button
          size="sm"
          variant="ghost"
          onClick={wrap(onEdit)}
          aria-label="Edit comment"
          title="Edit comment"
          className={`cursor-pointer ${btnClass}`}
        >
          <IconEdit className="h-3 w-3" />
        </Button>
      )}
      {onDelete && (
        <Button
          size="sm"
          variant="ghost"
          onClick={wrap(onDelete)}
          aria-label="Delete comment"
          title="Delete comment"
          className={`cursor-pointer ${btnClass} hover:text-destructive`}
        >
          <IconTrash className="h-3 w-3" />
        </Button>
      )}
    </>
  );
}

export function CommentDisplay({
  comment,
  onDelete,
  onEdit,
  onRun,
  showCode = false,
  compact = false,
}: CommentDisplayProps) {
  const lineRange = formatLineRange(comment.startLine, comment.endLine);

  if (compact) {
    return (
      <div
        className={`group flex items-start gap-2 rounded border border-border bg-muted/50 px-2 py-1.5 text-xs${onEdit ? " cursor-pointer hover:bg-muted/80" : ""}`}
        onClick={onEdit}
      >
        <IconMessage className="mt-0.5 h-3 w-3 shrink-0 text-muted-foreground" />
        <div className="min-w-0 flex-1">
          <span className="text-muted-foreground">{lineRange}</span>
          <span className="mx-1 text-muted-foreground">·</span>
          <span className="break-words">{comment.text}</span>
        </div>
        <div className="flex shrink-0 gap-0.5 opacity-0 group-hover:opacity-100">
          <CommentActionButtons
            onRun={onRun}
            onEdit={onEdit}
            onDelete={onDelete}
            size="compact"
            stopPropagation
          />
        </div>
      </div>
    );
  }

  return (
    <div className="group rounded-md border border-border bg-card p-2 shadow-sm">
      <div className="mb-1.5 flex items-center justify-between">
        <div className="flex items-center gap-1.5 text-xs">
          <IconMessage className="h-3.5 w-3.5 text-blue-500" />
          <span className="font-medium">{lineRange}</span>
          <span className="text-muted-foreground">
            ({comment.side === "additions" ? "new" : "old"})
          </span>
        </div>
        <div className="flex gap-1 opacity-0 transition-opacity group-hover:opacity-100">
          <CommentActionButtons onRun={onRun} onEdit={onEdit} onDelete={onDelete} size="normal" />
        </div>
      </div>
      {showCode && comment.codeContent && (
        <pre className="mb-2 overflow-x-auto rounded bg-muted p-1.5 text-[10px] leading-tight">
          <code>{comment.codeContent}</code>
        </pre>
      )}
      <p className="whitespace-pre-wrap text-xs leading-relaxed text-foreground">{comment.text}</p>
    </div>
  );
}
