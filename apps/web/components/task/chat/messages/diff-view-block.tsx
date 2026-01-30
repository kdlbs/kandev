'use client';

import { DiffModeEnum, DiffView } from '@git-diff-view/react';
import { useTheme } from 'next-themes';
import { IconCode } from '@tabler/icons-react';
import { cn } from '@/lib/utils';
import type { DiffPayload } from '@/components/task/chat/types';

type DiffViewBlockProps = {
  diff: DiffPayload;
  title?: string;
  showTitle?: boolean;
  className?: string;
};

export function DiffViewBlock({
  diff,
  title = 'Diff',
  showTitle = true,
  className
}: DiffViewBlockProps) {
  const { resolvedTheme } = useTheme();
  const diffTheme = resolvedTheme === 'dark' ? 'dark' : 'light';

  return (
    <div className={cn("mt-3 rounded-md border border-border/50 bg-background/60 px-3 py-2 text-xs", className)}>
      {showTitle && (
        <div className="flex items-center gap-2 text-muted-foreground mb-1 uppercase tracking-wide">
          <IconCode className="h-3.5 w-3.5" />
          <span>{title}</span>
        </div>
      )}
      <DiffView
        data={{
          hunks: diff.hunks,
          oldFile: diff.oldFile ?? { fileName: 'before', fileLang: 'plaintext' },
          newFile: diff.newFile ?? { fileName: 'after', fileLang: 'plaintext' },
        }}
        diffViewMode={DiffModeEnum.Unified}
        diffViewTheme={diffTheme}
      />
    </div>
  );
}
