"use client";

import { memo } from "react";
import type { PreviewFeedbackContextItem } from "@/lib/types/context";
import { ContextChip } from "./context-chip";

export const PreviewFeedbackItem = memo(function PreviewFeedbackItem({
  item,
}: {
  item: PreviewFeedbackContextItem;
}) {
  return <ContextChip kind="preview-feedback" label={item.label} onClick={item.onOpen} />;
});
