'use client';

import { memo } from 'react';
import type { PlanContextItem } from '@/lib/types/context';
import { ContextChip } from './context-chip';
import { LazyPlanPreview } from './lazy-plan-preview';

export const PlanItem = memo(function PlanItem({ item }: { item: PlanContextItem }) {
  return (
    <ContextChip
      kind="plan"
      label={item.label}
      pinned={item.pinned}
      preview={<LazyPlanPreview taskId={item.taskId ?? null} />}
      onClick={item.onOpen}
      onUnpin={item.onUnpin}
      onRemove={item.onRemove}
    />
  );
});
