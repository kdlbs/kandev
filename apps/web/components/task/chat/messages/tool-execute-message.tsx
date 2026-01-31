'use client';

import { useState, useRef, memo, useCallback } from 'react';
import { IconCheck, IconX, IconTerminal } from '@tabler/icons-react';
import { GridSpinner } from '@/components/grid-spinner';
import { transformPathsInText } from '@/lib/utils';
import type { Message } from '@/lib/types/http';
import { ExpandableRow } from './expandable-row';

type ShellExecOutput = {
  exit_code?: number;
  stdout?: string;
  stderr?: string;
};

type ShellExecPayload = {
  command?: string;
  work_dir?: string;
  description?: string;
  timeout?: number;
  background?: boolean;
  output?: ShellExecOutput;
};

type NormalizedPayload = {
  kind?: string;
  shell_exec?: ShellExecPayload;
};

type ToolExecuteMetadata = {
  tool_call_id?: string;
  status?: 'pending' | 'running' | 'complete' | 'error';
  normalized?: NormalizedPayload;
};

type ToolExecuteMessageProps = {
  comment: Message;
  worktreePath?: string;
};

export const ToolExecuteMessage = memo(function ToolExecuteMessage({ comment, worktreePath }: ToolExecuteMessageProps) {
  const metadata = comment.metadata as ToolExecuteMetadata | undefined;
  const [manualExpandState, setManualExpandState] = useState<boolean | null>(null);
  const prevStatusRef = useRef(metadata?.status);

  const status = metadata?.status;
  const shellExec = metadata?.normalized?.shell_exec;
  const output = shellExec?.output;
  const workDir = shellExec?.work_dir;
  const hasOutput = output?.stdout || output?.stderr;
  const hasExpandableContent = hasOutput || workDir;
  const isSuccess = status === 'complete' && (output?.exit_code === 0 || output?.exit_code === undefined);

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
        return output?.exit_code === 0 ? <IconCheck className="h-3.5 w-3.5 text-green-500" /> : <IconX className="h-3.5 w-3.5 text-red-500" />;
      case 'error':
        return <IconX className="h-3.5 w-3.5 text-red-500" />;
      case 'running':
        return <GridSpinner className="text-muted-foreground" />;
      default:
        return null;
    }
  };

  // Format work_dir for display (make relative if possible)
  const displayWorkDir = workDir ? transformPathsInText(workDir, worktreePath) : null;

  return (
    <ExpandableRow
      icon={<IconTerminal className="h-4 w-4 text-muted-foreground" />}
      header={
        <div className="flex items-center gap-2 text-xs">
          <span className="inline-flex items-center gap-1.5">
            <span className="font-mono text-xs text-muted-foreground">{transformPathsInText(comment.content, worktreePath)}</span>
            {!isSuccess && getStatusIcon()}
          </span>
        </div>
      }
      hasExpandableContent={!!hasExpandableContent}
      isExpanded={isExpanded}
      onToggle={handleToggle}
    >
      <div className="pl-4 border-l-2 border-border/30 space-y-2">
        {/* Only show cwd when there's no output */}
        {displayWorkDir && !hasOutput && (
          <div className="text-xs text-muted-foreground">
            <span className="opacity-60">cwd:</span>{' '}
            <span className="font-mono" title={workDir}>{displayWorkDir}</span>
          </div>
        )}
        {output?.stdout && (
          <pre className="text-xs bg-muted/30 rounded p-2 overflow-x-auto whitespace-pre-wrap max-h-[200px]">
            {output.stdout}
          </pre>
        )}
        {output?.stderr && (
          <pre className="text-xs bg-red-500/10 text-red-600 dark:text-red-400 rounded max-h-[200px] p-2 overflow-x-auto whitespace-pre-wrap">
            {output.stderr}
          </pre>
        )}
      </div>
    </ExpandableRow>
  );
});
