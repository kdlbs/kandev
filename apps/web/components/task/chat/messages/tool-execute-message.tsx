'use client';

import { memo } from 'react';
import { IconCheck, IconX, IconTerminal } from '@tabler/icons-react';
import { GridSpinner } from '@/components/grid-spinner';
import { transformPathsInText } from '@/lib/utils';
import type { Message } from '@/lib/types/http';
import { ExpandableRow } from './expandable-row';
import { useExpandState } from './use-expand-state';

type ShellExecOutput = {
  exit_code?: number;
  stdout?: string;
  stderr?: string;
};

type ShellExecPayload = {
  command?: string;
  work_dir?: string;
  output?: ShellExecOutput;
};

type ToolExecuteMetadata = {
  tool_call_id?: string;
  status?: 'pending' | 'running' | 'complete' | 'error';
  normalized?: { shell_exec?: ShellExecPayload };
};

type ToolExecuteMessageProps = {
  comment: Message;
  worktreePath?: string;
};

function ExecuteStatusIcon({ status, exitCode }: { status: string | undefined; exitCode: number | undefined }) {
  if (status === 'complete') {
    return exitCode === 0
      ? <IconCheck className="h-3.5 w-3.5 text-green-500" />
      : <IconX className="h-3.5 w-3.5 text-red-500" />;
  }
  if (status === 'error') return <IconX className="h-3.5 w-3.5 text-red-500" />;
  if (status === 'running') return <GridSpinner className="text-muted-foreground" />;
  return null;
}

type ExecuteOutputProps = {
  displayWorkDir: string | null;
  workDir: string | undefined;
  hasOutput: string | undefined | false;
  output: ShellExecOutput | undefined;
};

function ExecuteOutputContent({ displayWorkDir, workDir, hasOutput, output }: ExecuteOutputProps) {
  return (
    <div className="pl-4 border-l-2 border-border/30 space-y-2">
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
  );
}

function parseExecuteMetadata(comment: Message) {
  const metadata = comment.metadata as ToolExecuteMetadata | undefined;
  const status = metadata?.status;
  const shellExec = metadata?.normalized?.shell_exec;
  const output = shellExec?.output;
  const workDir = shellExec?.work_dir;
  const hasOutput = output?.stdout || output?.stderr;
  const hasExpandableContent = hasOutput || workDir;
  const isSuccess = status === 'complete' && (output?.exit_code === 0 || output?.exit_code === undefined);
  return { status, output, workDir, hasOutput, hasExpandableContent, isSuccess };
}

export const ToolExecuteMessage = memo(function ToolExecuteMessage({ comment, worktreePath }: ToolExecuteMessageProps) {
  const { status, output, workDir, hasOutput, hasExpandableContent, isSuccess } = parseExecuteMetadata(comment);
  const autoExpanded = status === 'running';
  const { isExpanded, handleToggle } = useExpandState(status, autoExpanded);
  const displayWorkDir = workDir ? transformPathsInText(workDir, worktreePath) : null;

  return (
    <ExpandableRow
      icon={<IconTerminal className="h-4 w-4 text-muted-foreground" />}
      header={
        <div className="flex items-center gap-2 text-xs">
          <span className="inline-flex items-center gap-1.5">
            <span className="font-mono text-xs text-muted-foreground">{transformPathsInText(comment.content, worktreePath)}</span>
            {!isSuccess && <ExecuteStatusIcon status={status} exitCode={output?.exit_code} />}
          </span>
        </div>
      }
      hasExpandableContent={!!hasExpandableContent}
      isExpanded={isExpanded}
      onToggle={handleToggle}
    >
      <ExecuteOutputContent displayWorkDir={displayWorkDir} workDir={workDir} hasOutput={hasOutput} output={output} />
    </ExpandableRow>
  );
});
