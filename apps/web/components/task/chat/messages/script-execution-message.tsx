'use client';

import { useState } from 'react';
import { IconCheck, IconX, IconLoader2, IconTerminal, IconChevronDown, IconChevronRight } from '@tabler/icons-react';
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
  const [isExpanded, setIsExpanded] = useState(false);
  const metadata = comment.metadata as ScriptExecutionMetadata | undefined;

  // Show fallback if metadata is missing or incomplete
  if (!metadata || !metadata.script_type) {
    const hasContent = !!comment.content;

    return (
      <div className="w-full">
        <div className="flex items-start gap-3 w-full">
          <div className="flex-shrink-0 mt-0.5">
            <IconTerminal className="h-4 w-4 text-yellow-600 dark:text-yellow-400" />
          </div>
          <div className="flex-1 min-w-0">
            <button
              type="button"
              onClick={() => hasContent && setIsExpanded(!isExpanded)}
              className={cn(
                'inline-flex items-center gap-1.5 text-left',
                hasContent && 'cursor-pointer hover:opacity-70 transition-opacity'
              )}
              disabled={!hasContent}
            >
              <span className="text-xs font-mono text-yellow-600 dark:text-yellow-400">
                Script Execution (metadata unavailable)
              </span>
              {hasContent && (
                isExpanded ? (
                  <IconChevronDown className="h-3.5 w-3.5 text-muted-foreground/50 flex-shrink-0" />
                ) : (
                  <IconChevronRight className="h-3.5 w-3.5 text-muted-foreground/50 flex-shrink-0" />
                )
              )}
            </button>
            {isExpanded && comment.content && (
              <div className="mt-2 pl-4 border-l-2 border-border/30">
                <pre className="font-mono text-xs bg-muted/30 rounded px-3 py-2 overflow-x-auto max-h-[300px] overflow-y-auto whitespace-pre-wrap break-words">
                  {comment.content}
                </pre>
              </div>
            )}
          </div>
        </div>
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

  const statusColor = isRunning
    ? 'text-blue-500'
    : isSuccess
      ? 'text-green-500'
      : 'text-red-500';

  const hasDetails = comment.content || error || exit_code !== undefined;

  return (
    <div className="w-full">
      {/* Icon + Summary Row */}
      <div className="flex items-start gap-3 w-full">
        {/* Icon */}
        <div className="flex-shrink-0 mt-0.5">
          <IconTerminal className="h-4 w-4 text-muted-foreground" />
        </div>

        {/* Content */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 text-xs">
            <button
              type="button"
              onClick={() => hasDetails && setIsExpanded(!isExpanded)}
              className={cn(
                'inline-flex items-center gap-1.5 text-left',
                hasDetails && 'cursor-pointer hover:opacity-70 transition-opacity'
              )}
              disabled={!hasDetails}
            >
              <Badge variant={isSetup ? 'default' : 'secondary'} className="text-xs">
                {isSetup ? 'Setup' : 'Cleanup'}
              </Badge>
              <span className="font-mono text-xs text-muted-foreground">
                {command}
              </span>
              {/* Status indicator - only show if not success */}
              {!isSuccess && (
                <StatusIcon className={cn('h-3.5 w-3.5', statusColor, isRunning && 'animate-spin')} />
              )}
              {hasDetails && (
                isExpanded ? (
                  <IconChevronDown className="h-3.5 w-3.5 text-muted-foreground/50 flex-shrink-0" />
                ) : (
                  <IconChevronRight className="h-3.5 w-3.5 text-muted-foreground/50 flex-shrink-0" />
                )
              )}
            </button>
          </div>

          {/* Expanded Details */}
          {isExpanded && hasDetails && (
            <div className="mt-2 pl-4 border-l-2 border-border/30 space-y-2">
              {/* Output */}
              {comment.content && comment.content.trim() !== '' && (
                <div>
                  <div className="text-[10px] uppercase tracking-wide text-muted-foreground/60 mb-1">
                    Output
                  </div>
                  <pre className="font-mono text-xs bg-muted/30 rounded px-3 py-2 overflow-x-auto max-h-[300px] overflow-y-auto whitespace-pre-wrap break-words">
                    {comment.content}
                  </pre>
                </div>
              )}

              {/* Error message */}
              {error && (
                <div>
                  <div className="text-[10px] uppercase tracking-wide text-red-600/70 dark:text-red-400/70 mb-1">
                    Error
                  </div>
                  <div className="text-xs text-red-600 dark:text-red-400 bg-red-500/10 rounded px-2 py-1.5">
                    {error}
                  </div>
                </div>
              )}

              {/* Footer with exit code */}
              {!isRunning && exit_code !== undefined && (
                <div className="flex items-center justify-between text-[10px] text-muted-foreground pt-2 border-t border-border/30">
                  <span>Exit code: {exit_code}</span>
                  {metadata.started_at && metadata.completed_at && (
                    <span>
                      Duration: {calculateDuration(metadata.started_at, metadata.completed_at)}
                    </span>
                  )}
                </div>
              )}
            </div>
          )}
        </div>
      </div>
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
