'use client';

import { useCallback, type ReactNode } from 'react';
import { cn } from '@kandev/ui/lib/utils';
import { Button } from '@kandev/ui/button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@kandev/ui/tooltip';
import { IconCopy, IconTextWrap, IconLayoutRows, IconLayoutColumns, IconPencil, IconArrowBackUp } from '@tabler/icons-react';
import type { RenderHeaderMetadataProps } from '@pierre/diffs';
import type { ViewMode } from '@/hooks/use-global-view-mode';

interface DiffHeaderToolbarOptions {
  filePath: string;
  diff?: string;
  wordWrap: boolean;
  onToggleWordWrap: () => void;
  viewMode: ViewMode;
  onToggleViewMode: () => void;
  onOpenFile?: (filePath: string) => void;
  onRevert?: (filePath: string) => void;
}

export function useDiffHeaderToolbar({
  filePath,
  diff,
  wordWrap,
  onToggleWordWrap,
  viewMode,
  onToggleViewMode,
  onOpenFile,
  onRevert,
}: DiffHeaderToolbarOptions) {
  const renderHeaderMetadata = useCallback(
    (props: RenderHeaderMetadataProps): ReactNode => {
      const resolvedPath = props.fileDiff?.name || filePath;

      return (
        <div className="flex items-center gap-1">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                className="h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100"
                onClick={() => navigator.clipboard.writeText(diff || '')}
              >
                <IconCopy className="h-3.5 w-3.5" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Copy diff</TooltipContent>
          </Tooltip>

          {onRevert && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100"
                  onClick={() => onRevert(resolvedPath)}
                >
                  <IconArrowBackUp className="h-3.5 w-3.5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Revert changes</TooltipContent>
            </Tooltip>
          )}

          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                className={cn(
                  'h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100',
                  wordWrap && 'opacity-100 bg-muted'
                )}
                onClick={onToggleWordWrap}
              >
                <IconTextWrap className="h-3.5 w-3.5" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Toggle word wrap</TooltipContent>
          </Tooltip>

          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                className="h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100"
                onClick={onToggleViewMode}
              >
                {viewMode === 'split' ? (
                  <IconLayoutRows className="h-3.5 w-3.5" />
                ) : (
                  <IconLayoutColumns className="h-3.5 w-3.5" />
                )}
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              {viewMode === 'split' ? 'Switch to unified view' : 'Switch to split view'}
            </TooltipContent>
          </Tooltip>

          {onOpenFile && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100"
                  onClick={() => onOpenFile(resolvedPath)}
                >
                  <IconPencil className="h-3.5 w-3.5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Edit</TooltipContent>
            </Tooltip>
          )}
        </div>
      );
    },
    [filePath, diff, wordWrap, onToggleWordWrap, viewMode, onToggleViewMode, onOpenFile, onRevert]
  );

  return renderHeaderMetadata;
}
