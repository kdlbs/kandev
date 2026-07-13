"use client";

import { memo } from "react";
import type { WalkthroughCommentContextItem } from "@/lib/types/context";
import { getFileName } from "@/lib/utils/file-path";
import { ContextChip } from "./context-chip";

function lineLabel(start: number, end: number) {
  return start === end ? `${start}` : `${start}-${end}`;
}

export const WalkthroughCommentItem = memo(function WalkthroughCommentItem({
  item,
}: {
  item: WalkthroughCommentContextItem;
}) {
  const preview = (
    <div className="space-y-2">
      {item.comments.map((comment) => (
        <div key={comment.id} className="text-xs space-y-1">
          <div className="font-medium text-muted-foreground truncate" title={comment.filePath}>
            {getFileName(comment.filePath)}:{lineLabel(comment.startLine, comment.endLine)}
          </div>
          <div className="line-clamp-2 text-muted-foreground">{comment.stepText}</div>
          <div className="break-words">{comment.text}</div>
          <button
            type="button"
            className="text-muted-foreground hover:text-foreground cursor-pointer"
            onClick={() => item.onRemoveComment(comment.id)}
          >
            Remove
          </button>
        </div>
      ))}
    </div>
  );

  return (
    <ContextChip
      kind="walkthrough-comment"
      label={item.label}
      preview={preview}
      onRemove={item.onRemove}
    />
  );
});
