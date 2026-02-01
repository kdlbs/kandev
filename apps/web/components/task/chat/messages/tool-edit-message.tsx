'use client';

import { useState, useRef, memo, useCallback } from 'react';
import { IconCheck, IconX, IconEdit, IconFilePlus } from '@tabler/icons-react';
import { GridSpinner } from '@/components/grid-spinner';
import { transformPathsInText } from '@/lib/utils';
import { FilePathButton } from './file-path-button';
import type { Message } from '@/lib/types/http';
import { DiffViewBlock } from './diff-view-block';
import { ExpandableRow } from './expandable-row';
import { transformFileMutation, type FileMutation } from '@/lib/diff';

type ModifyFilePayload = {
  file_path?: string;
  mutations?: FileMutation[];
};

type NormalizedPayload = {
  kind?: string;
  modify_file?: ModifyFilePayload;
};

type ToolEditMetadata = {
  tool_call_id?: string;
  status?: 'pending' | 'running' | 'complete' | 'error';
  normalized?: NormalizedPayload;
};

type ToolEditMessageProps = {
  comment: Message;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
};

export const ToolEditMessage = memo(function ToolEditMessage({ comment, worktreePath, onOpenFile }: ToolEditMessageProps) {
  const metadata = comment.metadata as ToolEditMetadata | undefined;
  const [manualExpandState, setManualExpandState] = useState<boolean | null>(null);
  const prevStatusRef = useRef(metadata?.status);

  const status = metadata?.status;
  const normalized = metadata?.normalized;
  const filePath = normalized?.modify_file?.file_path;
  const mutation = normalized?.modify_file?.mutations?.[0];
  const writeContent = mutation?.content; // For Write tool (create operations)
  const isWriteOperation = mutation?.type === 'create';

  // Transform mutation to FileDiffData using the new adapter
  const diffData = filePath && mutation
    ? transformFileMutation(filePath, mutation)
    : null;

  const hasExpandableContent = !!(diffData?.diff || writeContent);
  const isSuccess = status === 'complete';
  const Icon = isWriteOperation ? IconFilePlus : IconEdit;

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

  // Count lines for Write content
  const lineCount = writeContent ? writeContent.split('\n').length : 0;
  const getSummary = () => {
    if (isWriteOperation && lineCount > 0) {
      return `Write ${lineCount} line${lineCount !== 1 ? 's' : ''}`;
    }
    return transformPathsInText(comment.content, worktreePath);
  };

  return (
    <ExpandableRow
      icon={<Icon className="h-4 w-4 text-muted-foreground" />}
      header={
        <div className="flex items-center gap-2 text-xs min-w-0">
          <span className="inline-flex items-center gap-1.5 shrink-0 whitespace-nowrap">
            <span className="font-mono text-xs text-muted-foreground">{getSummary()}</span>
            {!isSuccess && getStatusIcon()}
          </span>
          {filePath && (
            <span className="min-w-0 flex-1">
              <FilePathButton
                filePath={filePath}
                worktreePath={worktreePath}
                onOpenFile={onOpenFile}
              />
            </span>
          )}
        </div>
      }
      hasExpandableContent={hasExpandableContent}
      isExpanded={isExpanded}
      onToggle={handleToggle}
    >
      {diffData?.diff ? (
        <DiffViewBlock data={diffData} showTitle={false} className="mt-0 border-0 px-0" />
      ) : writeContent ? (
        <div className="pl-4 border-l-2 border-border/30">
          <pre className="text-xs bg-muted/30 rounded p-2 overflow-x-auto max-h-[300px] overflow-y-auto whitespace-pre-wrap">
            {writeContent}
          </pre>
        </div>
      ) : null}
    </ExpandableRow>
  );
});
