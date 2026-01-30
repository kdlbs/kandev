'use client';

import { useState, memo } from 'react';
import { IconCheck, IconX, IconTerminal, IconChevronDown, IconChevronRight } from '@tabler/icons-react';
import { GridSpinner } from '@/components/grid-spinner';
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

export const ScriptExecutionMessage = memo(function ScriptExecutionMessage({ comment }: { comment: Message }) {
  const [isExpanded, setIsExpanded] = useState(false);
  const metadata = comment.metadata as ScriptExecutionMetadata | undefined;

  // Show fallback if metadata is missing or incomplete
  if (!metadata || !metadata.script_type) {
    const hasExpandableContent = !!comment.content;

    return (
      <div className="w-full group">
        <div
          className={cn(
            'flex items-start gap-3 w-full rounded px-2 py-1 -mx-2 transition-colors',
            hasExpandableContent && 'hover:bg-muted/50 cursor-pointer'
          )}
          onClick={() => { if (hasExpandableContent) setIsExpanded(!isExpanded); }}
        >
          <div className={cn(
            'flex-shrink-0 mt-0.5 relative w-4 h-4',
            hasExpandableContent && 'cursor-pointer'
          )}>
            <IconTerminal className={cn(
              'h-4 w-4 text-yellow-600 dark:text-yellow-400 absolute inset-0 transition-opacity',
              hasExpandableContent && 'group-hover:opacity-0'
            )} />
            {hasExpandableContent && (
              isExpanded
                ? <IconChevronDown className="h-4 w-4 text-muted-foreground absolute inset-0 opacity-0 group-hover:opacity-100 transition-opacity" />
                : <IconChevronRight className="h-4 w-4 text-muted-foreground absolute inset-0 opacity-0 group-hover:opacity-100 transition-opacity" />
            )}
          </div>

          <div className="flex-1 min-w-0 pt-0.5">
            <div className="flex items-center gap-2 text-xs">
              <span className="inline-flex items-center gap-1.5">
                <span className="text-xs font-mono text-yellow-600 dark:text-yellow-400">
                  Script Execution (metadata unavailable)
                </span>
              </span>
            </div>

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

  const StatusIcon = isSuccess ? IconCheck : IconX;
  const statusColor = isSuccess ? 'text-green-500' : 'text-red-500';
  const hasExpandableContent = !!(comment.content || error || exit_code !== undefined);

  return (
    <div className="w-full group">
      <div
        className={cn(
          'flex items-start gap-3 w-full rounded px-2 py-1 -mx-2 transition-colors',
          hasExpandableContent && 'hover:bg-muted/50 cursor-pointer'
        )}
        onClick={() => { if (hasExpandableContent) setIsExpanded(!isExpanded); }}
      >
        {/* Icon with hover-to-show chevron */}
        <div className={cn(
          'flex-shrink-0 mt-0.5 relative w-4 h-4',
          hasExpandableContent && 'cursor-pointer'
        )}>
          <IconTerminal className={cn(
            'h-4 w-4 text-muted-foreground absolute inset-0 transition-opacity',
            hasExpandableContent && 'group-hover:opacity-0'
          )} />
          {hasExpandableContent && (
            isExpanded
              ? <IconChevronDown className="h-4 w-4 text-muted-foreground absolute inset-0 opacity-0 group-hover:opacity-100 transition-opacity" />
              : <IconChevronRight className="h-4 w-4 text-muted-foreground absolute inset-0 opacity-0 group-hover:opacity-100 transition-opacity" />
          )}
        </div>

        {/* Content */}
        <div className="flex-1 min-w-0 pt-0.5">
          <div className="flex items-center gap-2 text-xs">
            <span className="inline-flex items-center gap-1.5 shrink-0 whitespace-nowrap">
              <Badge variant={isSetup ? 'default' : 'secondary'} className="text-xs">
                {isSetup ? 'Setup' : 'Cleanup'}
              </Badge>
              <span className="font-mono text-xs text-muted-foreground">
                {command}
              </span>
              {isRunning && <GridSpinner className="text-muted-foreground" />}
              {!isSuccess && !isRunning && (
                <StatusIcon className={cn('h-3.5 w-3.5', statusColor)} />
              )}
            </span>
          </div>

          {/* Expanded Details */}
          {isExpanded && hasExpandableContent && (
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
});

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
