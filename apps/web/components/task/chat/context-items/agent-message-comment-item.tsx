"use client";

import { memo } from "react";
import type { AgentMessageCommentContextItem } from "@/lib/types/context";
import { SelectionCommentItem } from "./selection-comment-item";

export const AgentMessageCommentItem = memo(function AgentMessageCommentItem({
  item,
}: {
  item: AgentMessageCommentContextItem;
}) {
  return (
    <SelectionCommentItem
      kind="agent-message-comment"
      label={item.label}
      comments={item.comments}
      onRemove={item.onRemove}
    />
  );
});
