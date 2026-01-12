'use client';

import { memo, useMemo, useState } from 'react';
import { DiffModeEnum, DiffView } from '@git-diff-view/react';
import { IconArrowBackUp, IconCopy, IconLayoutColumns, IconLayoutRows } from '@tabler/icons-react';
import { useTheme } from 'next-themes';
import { Badge } from '@kandev/ui/badge';
import { Button } from '@kandev/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { cn } from '@/lib/utils';
import { getLocalStorage, setLocalStorage } from '@/lib/local-storage';
import { buildDiffData, CHANGED_FILES, DIFF_SAMPLES } from '@/components/task/task-data';

type TaskChangesPanelProps = {
  selectedDiffPath: string | null;
  onClearSelected: () => void;
};

const DEFAULT_DIFF_MODE: 'unified' | 'split' = 'unified';

const TaskChangesPanel = memo(function TaskChangesPanel({
  selectedDiffPath,
  onClearSelected,
}: TaskChangesPanelProps) {
  const [diffViewMode, setDiffViewMode] = useState<'unified' | 'split'>(() =>
    getLocalStorage('task-diff-view-mode', DEFAULT_DIFF_MODE)
  );
  const { resolvedTheme } = useTheme();

  const diffTargets = useMemo(
    () => (selectedDiffPath ? [selectedDiffPath] : CHANGED_FILES.map((file) => file.path)),
    [selectedDiffPath]
  );
  const diffModeEnum = diffViewMode === 'split' ? DiffModeEnum.Split : DiffModeEnum.Unified;
  const diffTheme = resolvedTheme === 'dark' ? 'dark' : 'light';
  const selectedDiffContent = selectedDiffPath
    ? DIFF_SAMPLES[selectedDiffPath]?.newContent ?? ''
    : '';
  const isSingleDiffSelected = Boolean(selectedDiffPath && DIFF_SAMPLES[selectedDiffPath]);

  return (
    <div className="flex flex-col gap-2 h-full">
      <div className="flex items-center justify-between gap-3">
        <Badge variant="secondary" className="rounded-full text-xs">
          {selectedDiffPath ?? 'All files'}
        </Badge>
        <div className="flex items-center gap-1.5">
          {selectedDiffPath && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-7 px-2 text-xs cursor-pointer"
                  onClick={onClearSelected}
                >
                  All changes
                </Button>
              </TooltipTrigger>
              <TooltipContent>Show all changes</TooltipContent>
            </Tooltip>
          )}
          <Tooltip>
            <TooltipTrigger asChild>
              <span>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-7 px-2 text-xs cursor-pointer"
                  disabled={!isSingleDiffSelected}
                  onClick={async () => {
                    if (!isSingleDiffSelected) return;
                    await navigator.clipboard.writeText(selectedDiffContent);
                  }}
                >
                  <IconCopy className="h-3.5 w-3.5" />
                </Button>
              </span>
            </TooltipTrigger>
            <TooltipContent>Copy file contents</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <span>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-7 px-2 text-xs cursor-pointer"
                  disabled={!isSingleDiffSelected}
                >
                  <IconArrowBackUp className="h-3.5 w-3.5" />
                </Button>
              </span>
            </TooltipTrigger>
            <TooltipContent>Discard changes</TooltipContent>
          </Tooltip>
          <div className="inline-flex rounded-md border border-border overflow-hidden">
            <Tooltip>
              <TooltipTrigger asChild>
                <span>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className={cn(
                      'h-7 px-2 text-xs rounded-none cursor-pointer',
                      diffViewMode === 'unified' && 'bg-muted'
                    )}
                    onClick={() => {
                      setDiffViewMode('unified');
                      setLocalStorage('task-diff-view-mode', 'unified');
                    }}
                  >
                    <IconLayoutRows className="h-3.5 w-3.5" />
                  </Button>
                </span>
              </TooltipTrigger>
              <TooltipContent>Unified view</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger asChild>
                <span>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className={cn(
                      'h-7 px-2 text-xs rounded-none cursor-pointer',
                      diffViewMode === 'split' && 'bg-muted'
                    )}
                    onClick={() => {
                      setDiffViewMode('split');
                      setLocalStorage('task-diff-view-mode', 'split');
                    }}
                  >
                    <IconLayoutColumns className="h-3.5 w-3.5" />
                  </Button>
                </span>
              </TooltipTrigger>
              <TooltipContent>Split view</TooltipContent>
            </Tooltip>
          </div>
        </div>
      </div>
      <div className="flex-1 min-h-0 overflow-y-auto rounded-lg bg-background p-3 h-full">
        <div className="space-y-4">
          {diffTargets.map((path) => (
            <div key={path} className="space-y-2">
              {!selectedDiffPath && (
                <div className="flex items-center justify-between">
                  <Badge variant="secondary" className="rounded-full text-xs">
                    {path}
                  </Badge>
                </div>
              )}
              <DiffView data={buildDiffData(path)} diffViewMode={diffModeEnum} diffViewTheme={diffTheme} />
            </div>
          ))}
        </div>
      </div>
    </div>
  );
});

export { TaskChangesPanel };
