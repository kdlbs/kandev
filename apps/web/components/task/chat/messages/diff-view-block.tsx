'use client';

import { cn } from '@/lib/utils';
import { DiffViewInline } from '@/components/diff';
import type { FileDiffData } from '@/lib/diff/types';

type DiffViewBlockProps = {
  /** Diff data in the new format */
  data: FileDiffData;
  /** Additional class name */
  className?: string;
};

export function DiffViewBlock({ data, className }: DiffViewBlockProps) {
  return (
    <div className={cn("mt-3 rounded-md border border-border/50 bg-background/60 px-3 py-2 text-xs", className)}>
      <DiffViewInline data={data} />
    </div>
  );
}
