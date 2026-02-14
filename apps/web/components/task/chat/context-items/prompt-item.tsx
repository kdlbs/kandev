'use client';

import { memo } from 'react';
import type { PromptContextItem } from '@/lib/types/context';
import { ContextChip } from './context-chip';
import { PromptPreview } from './prompt-preview';

export const PromptItem = memo(function PromptItem({ item }: { item: PromptContextItem }) {
  return (
    <ContextChip
      kind="prompt"
      label={item.label}
      pinned={item.pinned}
      preview={<PromptPreview content={item.promptContent ?? null} />}
      onClick={item.onClick}
      onUnpin={item.onUnpin}
      onRemove={item.onRemove}
    />
  );
});
