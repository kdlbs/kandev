'use client';

import { IconCheck, IconX, IconLoader2, IconTerminal } from '@tabler/icons-react';
import { cn } from '@/lib/utils';
import type { Message } from '@/lib/types/http';
import { Badge } from '@kandev/ui/badge';

interface ScriptExecutionMetadata {
  script_type: 'setup' | 'cleanup';
  command: string;
  status: 'starting' | 'running' | 'exited' | 'failed';
  exit_code?: number;
  started_at?: string;
  completed_at?: string;
  process_id?: string;
  error?: string;
}

export function ScriptExecutionMessage({ comment }: { comment: Message }) {
  const metadata = comment.metadata as ScriptExecutionMetadata | undefined;

  // Show fallback if metadata is missing or incomplete
  if (!metadata || !metadata.script_type) {
    return (
      <div className="w-full rounded-lg border border-yellow-500/40 bg-yellow-500/5 px-4 py-3 text-sm">
        <div className="flex items-center gap-2 text-yellow-600 dark:text-yellow-400">
          <IconTerminal className="h-4 w-4" />
          <span>Script Execution (metadata unavailable)</span>
        </div>
        {comment.content && (
          <pre className="mt-2 font-mono text-xs bg-black/5 dark:bg-white/5 rounded px-3 py-2 border border-border/40 overflow-x-auto max-h-[300px] overflow-y-auto whitespace-pre-wrap break-words">
            {comment.content}
          </pre>
        )}
      </div>
    );
  }

  const { script_type, command, status, exit_code, error } = metadata;
  const isSetup = script_type === 'setup';
  const isRunning = status === 'starting' || status === 'running';
  const isSuccess = status === 'exited' && exit_code === 0;

  // Status icon and styling
  const StatusIcon = isRunning
    ? IconLoader2
    : isSuccess
    ? IconCheck
    : IconX;

  const statusText = isRunning
    ? 'Running...'
    : isSuccess
    ? 'Completed'
    : 'Failed';

  const statusColor = isRunning
    ? 'text-blue-500'
    : isSuccess
    ? 'text-green-500'
    : 'text-red-500';

  const borderColor = isRunning
    ? 'border-blue-500/40'
    : isSuccess
    ? 'border-green-500/40'
    : 'border-red-500/40';

  const bgColor = isRunning
    ? 'bg-blue-500/5'
    : isSuccess
    ? 'bg-green-500/5'
    : 'bg-red-500/5';

  return (
    <div
      className={cn(
        'w-full rounded-lg border px-4 py-3 text-sm',
        borderColor,
        bgColor
      )}
    >
      {/* Header */}
      <div className="flex items-center justify-between gap-2 mb-2">
        <div className="flex items-center gap-2">
          <IconTerminal className="h-4 w-4 text-muted-foreground" />
          <Badge variant={isSetup ? 'default' : 'secondary'} className="text-xs">
            {isSetup ? 'Setup Script' : 'Cleanup Script'}
          </Badge>
          <div className={cn('flex items-center gap-1.5 text-xs', statusColor)}>
            <StatusIcon className={cn('h-3.5 w-3.5', isRunning && 'animate-spin')} />
            <span>{statusText}</span>
          </div>
        </div>
      </div>

      {/* Command */}
      <div className="mb-2 font-mono text-xs bg-muted/50 rounded px-2 py-1.5 border border-border/40">
        <span className="text-muted-foreground">$</span> {command}
      </div>

      {/* Output */}
      {comment.content && comment.content.trim() !== '' && (
        <div className="mb-2">
          <div className="text-[11px] uppercase tracking-wide text-muted-foreground mb-1">
            Output
          </div>
          <pre className="font-mono text-xs bg-black/5 dark:bg-white/5 rounded px-3 py-2 border border-border/40 overflow-x-auto max-h-[300px] overflow-y-auto whitespace-pre-wrap break-words">
            {comment.content}
          </pre>
        </div>
      )}

      {/* Error message */}
      {error && (
        <div className="mb-2 text-xs text-red-600 dark:text-red-400 bg-red-500/10 rounded px-2 py-1.5 border border-red-500/20">
          {error}
        </div>
      )}

      {/* Footer with exit code */}
      {!isRunning && exit_code !== undefined && (
        <div className="flex items-center justify-between text-[11px] text-muted-foreground pt-2 border-t border-border/40">
          <span>Exit code: {exit_code}</span>
          {metadata.started_at && metadata.completed_at && (
            <span>
              Duration: {calculateDuration(metadata.started_at, metadata.completed_at)}
            </span>
          )}
        </div>
      )}
    </div>
  );
}

// Helper function to calculate duration
function calculateDuration(startedAt: string, completedAt: string): string {
  try {
    const start = new Date(startedAt).getTime();
    const end = new Date(completedAt).getTime();
    const durationMs = end - start;

    if (durationMs < 1000) {
      return `${durationMs}ms`;
    } else if (durationMs < 60000) {
      return `${(durationMs / 1000).toFixed(1)}s`;
    } else {
      const minutes = Math.floor(durationMs / 60000);
      const seconds = Math.floor((durationMs % 60000) / 1000);
      return `${minutes}m ${seconds}s`;
    }
  } catch {
    return 'N/A';
  }
}
