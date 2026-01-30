'use client';

import { useState, useRef, memo, useCallback } from 'react';
import { IconCheck, IconX, IconSearch } from '@tabler/icons-react';
import { GridSpinner } from '@/components/grid-spinner';
import { toRelativePath } from '@/lib/utils';
import { FilePathButton } from './file-path-button';
import type { Message } from '@/lib/types/http';
import { ExpandableRow } from './expandable-row';

type CodeSearchOutput = {
  files?: string[];
  file_count?: number;
  truncated?: boolean;
};

type CodeSearchPayload = {
  query?: string;
  pattern?: string;
  path?: string;
  glob?: string;
  output?: CodeSearchOutput;
};

type NormalizedPayload = {
  kind?: string;
  code_search?: CodeSearchPayload;
};

type ToolSearchMetadata = {
  tool_call_id?: string;
  status?: 'pending' | 'running' | 'complete' | 'error';
  normalized?: NormalizedPayload;
};

type ToolSearchMessageProps = {
  comment: Message;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
};

export const ToolSearchMessage = memo(function ToolSearchMessage({ comment, worktreePath, onOpenFile }: ToolSearchMessageProps) {
  const metadata = comment.metadata as ToolSearchMetadata | undefined;
  const [manualExpandState, setManualExpandState] = useState<boolean | null>(null);
  const prevStatusRef = useRef(metadata?.status);

  const status = metadata?.status;
  const normalized = metadata?.normalized;
  const codeSearch = normalized?.code_search;
  const searchOutput = codeSearch?.output;

  const searchPath = codeSearch?.path;
  const searchPattern = codeSearch?.glob || codeSearch?.pattern || codeSearch?.query;
  const hasOutput = !!(searchOutput?.files && searchOutput.files.length > 0);
  const isSuccess = status === 'complete';

  // Reset manual state when status changes (allows auto-expand behavior to resume)
  if (prevStatusRef.current !== status) {
    prevStatusRef.current = status;
    if (manualExpandState !== null) {
      setManualExpandState(null);
    }
  }

  // Derive expanded state: manual override takes precedence, otherwise auto-expand when running
  const autoExpanded = status === 'running';
  const isExpanded = manualExpandState ?? autoExpanded;

  const handleToggle = useCallback(() => {
    setManualExpandState((prev) => !(prev ?? autoExpanded));
  }, [autoExpanded]);

  const getStatusIcon = () => {
    switch (status) {
      case 'complete':
        return <IconCheck className="h-3.5 w-3.5 text-green-500" />;
      case 'error':
        return <IconX className="h-3.5 w-3.5 text-red-500" />;
      case 'running':
        return <GridSpinner className="text-muted-foreground" />;
      default:
        return null;
    }
  };

  // Generate summary text
  const getSummary = () => {
    if (searchOutput?.files && searchOutput.files.length > 0) {
      const count = searchOutput.file_count || searchOutput.files.length;
      return `Found ${count} file${count !== 1 ? 's' : ''}`;
    }
    return 'Searching';
  };

  return (
    <ExpandableRow
      icon={<IconSearch className="h-4 w-4 text-muted-foreground" />}
      header={
        <div className="flex items-center gap-2 text-xs">
          <span className="inline-flex items-center gap-1.5">
            <span className="font-mono text-xs text-muted-foreground">{getSummary()}</span>
            {!isSuccess && getStatusIcon()}
          </span>
          {searchPattern && (
            <span className="text-xs text-muted-foreground/60 font-mono">{searchPattern}</span>
          )}
          {searchPath && (
            <span className="text-xs text-muted-foreground/60 truncate font-mono bg-muted/30 px-1.5 py-0.5 rounded">
              {toRelativePath(searchPath, worktreePath)}
            </span>
          )}
        </div>
      }
      hasExpandableContent={hasOutput}
      isExpanded={isExpanded}
      onToggle={handleToggle}
    >
      {searchOutput?.files && (
        <div className="rounded-md border border-border/50 overflow-hidden bg-muted/20">
          <div className="text-xs space-y-0.5 max-h-[200px] overflow-y-auto p-1">
            {searchOutput.files.map((file) => (
              <FilePathButton
                key={file}
                filePath={file}
                worktreePath={worktreePath}
                onOpenFile={onOpenFile}
                variant="list-item"
              />
            ))}
            {searchOutput.truncated && (
              <div className="text-amber-500/80 mt-1 px-2">...and more files (truncated)</div>
            )}
          </div>
        </div>
      )}
    </ExpandableRow>
  );
});
