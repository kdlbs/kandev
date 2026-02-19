"use client";

import { memo } from "react";
import type { CommentContextItem } from "@/lib/types/context";
import { getFileName } from "@/lib/utils/file-path";
import { ContextChip } from "./context-chip";
import { CommentDisplay } from "@/components/diff/comment-display";

export const CommentItem = memo(function CommentItem({ item }: { item: CommentContextItem }) {
  const fileName = getFileName(item.filePath);

  const preview = (
    <div className="space-y-1.5">
      <div className="text-xs font-medium text-muted-foreground truncate" title={item.filePath}>
        {fileName}
      </div>
      <div className="space-y-1">
        {item.comments.map((comment) => (
          <CommentDisplay
            key={comment.id}
            comment={comment}
            compact
            onDelete={() => item.onRemoveComment(comment.id)}
          />
        ))}
      </div>
    </div>
  );

  return (
    <ContextChip
      kind="comment"
      label={item.label}
      preview={preview}
      onClick={item.onOpen}
      onRemove={item.onRemove}
    />
  );
});
