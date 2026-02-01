'use client';

import { IconCode } from '@tabler/icons-react';
import { cn } from '@/lib/utils';
import { DiffViewInline } from '@/components/diff';
import type { FileDiffData } from '@/lib/diff/types';

type DiffViewBlockProps = {
  /** Diff data in the new format */
  data: FileDiffData;
  /** Title to display */
  title?: string;
  /** Whether to show the title */
  showTitle?: boolean;
  /** Additional class name */
  className?: string;
};

export function DiffViewBlock({
  data,
  title = 'Diff',
  showTitle = true,
  className
}: DiffViewBlockProps) {
  return (
    <div className={cn("mt-3 rounded-md border border-border/50 bg-background/60 px-3 py-2 text-xs", className)}>
      {showTitle && (
        <div className="flex items-center gap-2 text-muted-foreground mb-1 uppercase tracking-wide">
          <IconCode className="h-3.5 w-3.5" />
          <span>{title}</span>
        </div>
      )}
      <DiffViewInline data={data} />
    </div>
  );
}
