"use client";

import { memo } from "react";
import type { PlanCommentContextItem } from "@/lib/types/context";
import { ContextChip } from "./context-chip";

export const PlanCommentItem = memo(function PlanCommentItem({
  item,
}: {
  item: PlanCommentContextItem;
}) {
  const preview = (
    <div className="space-y-1.5">
      {item.comments.map((comment) => (
        <div key={comment.id} className="text-xs space-y-0.5">
          {comment.selectedText && (
            <div className="text-muted-foreground italic line-clamp-2">
              &ldquo;{comment.selectedText}&rdquo;
            </div>
          )}
          <div className="break-words">{comment.text}</div>
        </div>
      ))}
    </div>
  );

  return (
    <ContextChip
      kind="plan-comment"
      label={item.label}
      preview={preview}
      onClick={item.onOpen}
      onRemove={item.onRemove}
    />
  );
});
