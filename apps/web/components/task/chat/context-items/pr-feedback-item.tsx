"use client";

import { memo } from "react";
import type { PRFeedbackContextItem } from "@/lib/types/context";
import { ContextChip } from "./context-chip";

export const PRFeedbackItem = memo(function PRFeedbackItem({
  item,
}: {
  item: PRFeedbackContextItem;
}) {
  const preview = (
    <div className="space-y-1.5">
      {item.comments.map((c) => (
        <div key={c.id} className="text-xs space-y-0.5">
          <div className="break-words whitespace-pre-wrap">{c.content}</div>
        </div>
      ))}
    </div>
  );

  return (
    <ContextChip kind="pr-feedback" label={item.label} preview={preview} onRemove={item.onRemove} />
  );
});
