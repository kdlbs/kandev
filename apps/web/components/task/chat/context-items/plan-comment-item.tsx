"use client";

import { memo } from "react";
import type { PlanCommentContextItem } from "@/lib/types/context";
import { SelectionCommentItem } from "./selection-comment-item";

export const PlanCommentItem = memo(function PlanCommentItem({
  item,
}: {
  item: PlanCommentContextItem;
}) {
  return (
    <SelectionCommentItem
      kind="plan-comment"
      label={item.label}
      comments={item.comments}
      onClick={item.onOpen}
      onRemove={item.onRemove}
    />
  );
});
