"use client";

import { memo } from "react";
import { ContextChip } from "./context-chip";

type SelectionComment = {
  id: string;
  selectedText: string;
  text: string;
};

export const SelectionCommentItem = memo(function SelectionCommentItem({
  kind,
  label,
  comments,
  onClick,
  onRemove,
}: {
  kind: "plan-comment" | "agent-message-comment";
  label: string;
  comments: SelectionComment[];
  onClick?: () => void;
  onRemove?: () => void;
}) {
  const preview = (
    <div className="space-y-1.5">
      {comments.map((comment) => (
        <div key={comment.id} className="space-y-0.5 text-xs">
          {comment.selectedText ? (
            <div className="line-clamp-2 text-muted-foreground italic">
              &ldquo;{comment.selectedText}&rdquo;
            </div>
          ) : null}
          <div className="break-words">{comment.text}</div>
        </div>
      ))}
    </div>
  );

  return (
    <ContextChip
      kind={kind}
      label={label}
      preview={preview}
      onClick={onClick}
      onRemove={onRemove}
    />
  );
});
