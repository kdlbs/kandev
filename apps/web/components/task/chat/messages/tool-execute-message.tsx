'use client';

import { useState, useRef } from 'react';
import { IconCheck, IconX, IconLoader2, IconTerminal, IconChevronDown, IconChevronRight } from '@tabler/icons-react';
import { cn } from '@/lib/utils';
import type { Message } from '@/lib/types/http';

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

export function ToolExecuteMessage({ comment }: { comment: Message }) {
  const metadata = comment.metadata as ToolExecuteMetadata | undefined;
  const [manualExpandState, setManualExpandState] = useState<boolean | null>(null);
  const prevStatusRef = useRef(metadata?.status);

  const status = metadata?.status;
  const output = metadata?.normalized?.shell_exec?.output;
  const hasOutput = output?.stdout || output?.stderr;
  const isSuccess = status === 'complete' && (output?.exit_code === 0 || output?.exit_code === undefined);

  // Reset manual state when status changes (allows auto-expand behavior to resume)
  if (prevStatusRef.current !== status) {
    prevStatusRef.current = status;
    if (manualExpandState !== null) {
      setManualExpandState(null);
    }
  }

  // Derive expanded state: manual override takes precedence, otherwise auto-expand when running
  const isExpanded = manualExpandState ?? (status === 'running');

  const getStatusIcon = () => {
    switch (status) {
      case 'complete':
        return output?.exit_code === 0 ? <IconCheck className="h-3.5 w-3.5 text-green-500" /> : <IconX className="h-3.5 w-3.5 text-red-500" />;
      case 'error':
        return <IconX className="h-3.5 w-3.5 text-red-500" />;
      case 'running':
        return <IconLoader2 className="h-3.5 w-3.5 text-blue-500 animate-spin" />;
      default:
        return null;
    }
  };

  return (
    <div className="w-full">
      <div className="flex items-start gap-3 w-full">
        <div className="flex-shrink-0 mt-0.5">
          <IconTerminal className="h-4 w-4 text-muted-foreground" />
        </div>

        <div className="flex-1 min-w-0 pt-0.5">
          <div className="flex items-center gap-2 text-xs">
            <button
              type="button"
              onClick={() => {
                if (hasOutput) {
                  setManualExpandState(!isExpanded);
                }
              }}
              className={cn(
                'inline-flex items-center gap-1.5 text-left',
                hasOutput && 'cursor-pointer hover:opacity-70 transition-opacity'
              )}
              disabled={!hasOutput}
            >
              <span className="font-mono text-xs">{comment.content}</span>
              {!isSuccess && getStatusIcon()}
              {hasOutput && (isExpanded ? <IconChevronDown className="h-3.5 w-3.5 text-muted-foreground/50" /> : <IconChevronRight className="h-3.5 w-3.5 text-muted-foreground/50" />)}
            </button>
          </div>

          {isExpanded && hasOutput && (
            <div className="mt-2 pl-4 border-l-2 border-border/30">
              {output?.stdout && (
                <pre className="text-xs bg-muted/30 rounded p-2 overflow-x-auto whitespace-pre-wrap">
                  {output.stdout}
                </pre>
              )}
              {output?.stderr && (
                <pre className="mt-2 text-xs bg-red-500/10 text-red-600 dark:text-red-400 rounded p-2 overflow-x-auto whitespace-pre-wrap">
                  {output.stderr}
                </pre>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
