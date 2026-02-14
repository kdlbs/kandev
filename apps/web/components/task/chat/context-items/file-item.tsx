'use client';

import { memo, useCallback } from 'react';
import type { FileContextItem } from '@/lib/types/context';
import { ContextChip } from './context-chip';
import { LazyFilePreview } from './lazy-file-preview';

export const FileItem = memo(function FileItem({
  item,
  sessionId,
}: {
  item: FileContextItem;
  sessionId?: string | null;
}) {
  const handleClick = useCallback(() => {
    item.onOpen(item.path);
  }, [item]);

  return (
    <ContextChip
      kind="file"
      label={item.label}
      pinned={item.pinned}
      preview={<LazyFilePreview path={item.path} sessionId={sessionId ?? null} />}
      onClick={handleClick}
      onUnpin={item.onUnpin}
      onRemove={item.onRemove}
    />
  );
});
