"use client";

import { memo } from "react";
import type { FileAttachmentContextItem } from "@/lib/types/context";
import { formatBytes } from "../file-attachment";
import { ContextChip } from "./context-chip";

export const FileAttachmentItem = memo(function FileAttachmentItem({
  item,
}: {
  item: FileAttachmentContextItem;
}) {
  const preview = (
    <div className="text-xs text-muted-foreground">
      {item.attachment.fileName} ({formatBytes(item.attachment.size)})
    </div>
  );

  return <ContextChip kind="file" label={item.label} preview={preview} onRemove={item.onRemove} />;
});
