"use client";

import { memo, useCallback } from "react";
import { IconFile, IconFolder } from "@tabler/icons-react";
import type { FileContextItem } from "@/lib/types/context";
import { ContextChip } from "./context-chip";
import { LazyFilePreview } from "./lazy-file-preview";

export const FileItem = memo(function FileItem({
  item,
  sessionId,
}: {
  item: FileContextItem;
  sessionId?: string | null;
}) {
  const handleClick = useCallback(() => item.onOpen?.(item.path), [item]);
  const isDirectory = item.isDirectory;

  return (
    <ContextChip
      kind="file"
      label={item.label}
      pinned={item.pinned}
      dataTestId="chat-context-file"
      dataPath={item.path}
      dataIsDirectory={isDirectory}
      leadingIcon={
        isDirectory ? (
          <IconFolder className="h-3 w-3 shrink-0" />
        ) : (
          <IconFile className="h-3 w-3 shrink-0" />
        )
      }
      preview={
        isDirectory ? undefined : <LazyFilePreview path={item.path} sessionId={sessionId ?? null} />
      }
      onClick={item.onOpen ? handleClick : undefined}
      onUnpin={item.onUnpin}
      onRemove={item.onRemove}
    />
  );
});
