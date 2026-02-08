'use client';

import { memo } from 'react';
import { IconFileText, IconListCheck } from '@tabler/icons-react';
import type { ActiveDocument } from '@/lib/state/slices/ui/types';

type DocumentReferenceIndicatorProps = {
  activeDocument: ActiveDocument;
};

export const DocumentReferenceIndicator = memo(function DocumentReferenceIndicator({
  activeDocument,
}: DocumentReferenceIndicatorProps) {
  const isPlan = activeDocument.type === 'plan';
  const label = isPlan
    ? 'Plan will be included in the prompt'
    : `${activeDocument.name} will be included in the prompt`;

  return (
    <div className="flex items-center gap-2 px-3 py-1.5 text-xs text-primary/80 bg-primary/5 rounded-lg border border-primary/10">
      {isPlan ? (
        <IconListCheck className="h-3.5 w-3.5 shrink-0" />
      ) : (
        <IconFileText className="h-3.5 w-3.5 shrink-0" />
      )}
      <span className="truncate">{label}</span>
    </div>
  );
});
