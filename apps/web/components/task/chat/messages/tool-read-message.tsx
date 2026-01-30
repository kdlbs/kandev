'use client';

import { useState, useRef, memo, useCallback } from 'react';
import { IconCheck, IconX, IconFileCode2 } from '@tabler/icons-react';
import { GridSpinner } from '@/components/grid-spinner';
import { FilePathButton } from './file-path-button';
import type { Message } from '@/lib/types/http';
import { ExpandableRow } from './expandable-row';

type ReadFileOutput = {
  content?: string;
  line_count?: number;
  truncated?: boolean;
  language?: string;
};

type ReadFilePayload = {
  file_path?: string;
  offset?: number;
  limit?: number;
  output?: ReadFileOutput;
};

type NormalizedPayload = {
  kind?: string;
  read_file?: ReadFilePayload;
};

type ToolReadMetadata = {
  tool_call_id?: string;
  status?: 'pending' | 'running' | 'complete' | 'error';
  normalized?: NormalizedPayload;
};

type ToolReadMessageProps = {
  comment: Message;
  worktreePath?: string;
  sessionId?: string;
  onOpenFile?: (path: string) => void;
};

export const ToolReadMessage = memo(function ToolReadMessage({ comment, worktreePath, onOpenFile }: ToolReadMessageProps) {
  const metadata = comment.metadata as ToolReadMetadata | undefined;
  const [manualExpandState, setManualExpandState] = useState<boolean | null>(null);
  const prevStatusRef = useRef(metadata?.status);

  const status = metadata?.status;
  const normalized = metadata?.normalized;
  const readFile = normalized?.read_file;
  const readOutput = readFile?.output;

  const filePath = readFile?.file_path;
  const hasOutput = !!readOutput?.content;
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
    if (readOutput?.line_count) {
      return `Read ${readOutput.line_count} line${readOutput.line_count !== 1 ? 's' : ''}`;
    }
    return 'Read';
  };

  return (
    <ExpandableRow
      icon={<IconFileCode2 className="h-4 w-4 text-muted-foreground" />}
      header={
        <div className="flex items-center gap-2 text-xs min-w-0">
          <span className="inline-flex items-center gap-1.5 shrink-0 whitespace-nowrap">
            <span className="font-mono text-xs text-muted-foreground">{getSummary()}</span>
            {!isSuccess && getStatusIcon()}
          </span>
          {filePath && (
            <span className="min-w-0">
              <FilePathButton
                filePath={filePath}
                worktreePath={worktreePath}
                onOpenFile={onOpenFile}
              />
            </span>
          )}
          {readOutput?.truncated && (
            <span className="text-xs text-amber-500/80 shrink-0">(truncated)</span>
          )}
        </div>
      }
      hasExpandableContent={hasOutput}
      isExpanded={isExpanded}
      onToggle={handleToggle}
    >
      {readOutput?.content && (
        <div className="relative rounded-md border border-border/50 overflow-hidden bg-muted/20">
          <pre className="text-xs p-3 overflow-x-auto max-h-[200px] overflow-y-auto">
            <code>{readOutput.content}</code>
          </pre>
        </div>
      )}
    </ExpandableRow>
  );
});
