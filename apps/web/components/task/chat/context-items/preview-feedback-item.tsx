"use client";

import { memo } from "react";
import type { PreviewFeedbackContextItem } from "@/lib/types/context";
import { ContextChip } from "./context-chip";

export const PreviewFeedbackItem = memo(function PreviewFeedbackItem({
  item,
}: {
  item: PreviewFeedbackContextItem;
}) {
  const preview = (
    <div className="space-y-2">
      {item.items.map((feedback) => (
        <div key={feedback.id} className="space-y-0.5 text-xs">
          <div className="truncate text-muted-foreground">
            {feedback.source_label} · {feedback.page_route}
          </div>
          <div className="break-words">{feedback.comment}</div>
        </div>
      ))}
    </div>
  );

  return (
    <ContextChip
      kind="preview-feedback"
      label={item.label}
      preview={preview}
      onClick={item.onOpen}
    />
  );
});
