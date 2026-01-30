'use client';

import { useState, memo, useCallback } from 'react';
import { IconAlertTriangle, IconInfoCircle, IconHandStop, IconChevronDown, IconChevronRight } from '@tabler/icons-react';
import { cn } from '@/lib/utils';
import type { Message } from '@/lib/types/http';
import type { StatusMetadata } from '@/components/task/chat/types';

interface ErrorMetadata extends StatusMetadata {
  error?: string;
  text?: string;
  error_data?: Record<string, unknown>;
  stderr?: string[];
  provider?: string;
  provider_agent?: string;
}

export const StatusMessage = memo(function StatusMessage({ comment }: { comment: Message }) {
  const [isExpanded, setIsExpanded] = useState(false);
  const metadata = comment.metadata as ErrorMetadata | undefined;
  const progress =
    typeof metadata?.progress === 'number' ? Math.min(Math.max(metadata.progress, 0), 100) : null;
  const statusLine = metadata?.stage || metadata?.status;
  const message = metadata?.message || comment.content || statusLine || 'Status update';
  const isError = comment.type === 'error' || metadata?.variant === 'error';
  const isWarning = metadata?.variant === 'warning' || metadata?.cancelled === true;

  // Check if there are error details to show
  const hasErrorDetails = isError && (metadata?.error_data || metadata?.error || metadata?.text || metadata?.stderr);

  // Format error details for display
  const formatErrorDetails = () => {
    const details: { label: string; value: string }[] = [];

    if (metadata?.stderr && metadata.stderr.length > 0) {
      details.push({ label: 'Agent Output', value: metadata.stderr.join('\n') });
    }
    if (metadata?.error) {
      details.push({ label: 'Error', value: metadata.error });
    }
    if (metadata?.text) {
      details.push({ label: 'Details', value: metadata.text });
    }
    if (metadata?.error_data) {
      // Don't show stderr again in error_data since we already show it above
      const filteredData = { ...metadata.error_data };
      delete filteredData.stderr;
      if (Object.keys(filteredData).length > 0) {
        details.push({ label: 'Error Data', value: JSON.stringify(filteredData, null, 2) });
      }
    }

    return details;
  };

  const errorDetails = hasErrorDetails ? formatErrorDetails() : [];
  const hasExpandableContent = hasErrorDetails && errorDetails.length > 0;

  // useCallback must be called before any early returns (React hooks rules)
  const handleToggle = useCallback(() => {
    if (hasExpandableContent) setIsExpanded((prev) => !prev);
  }, [hasExpandableContent]);

  // Simple system message: no metadata, no progress, just a short message
  const isSimpleStatus = !isError && !isWarning && progress === null && !statusLine && !metadata?.message;

  if (isSimpleStatus) {
    return (
      <div className="flex items-center gap-3 w-full py-2">
        <div className="flex-1 h-px bg-border" />
        <span className="text-xs text-muted-foreground/60 whitespace-nowrap">{message}</span>
        <div className="flex-1 h-px bg-border" />
      </div>
    );
  }

  // Cancelled turn gets a special separator style
  if (metadata?.cancelled) {
    return (
      <div className="flex items-center gap-3 w-full py-2">
        <div className="flex-1 h-px bg-amber-500/30" />
        <div className="flex items-center gap-1.5 text-xs text-amber-600 dark:text-amber-400">
          <IconHandStop className="h-3 w-3" />
          <span>{message}</span>
        </div>
        <div className="flex-1 h-px bg-amber-500/30" />
      </div>
    );
  }

  const Icon = isError ? IconAlertTriangle : isWarning ? IconAlertTriangle : IconInfoCircle;
  const iconClass = isError ? 'text-red-500' : isWarning ? 'text-amber-500' : 'text-muted-foreground';
  const textClass = isError ? 'text-red-600 dark:text-red-400' : isWarning ? 'text-amber-600 dark:text-amber-400' : 'text-muted-foreground';

  return (
    <div className="w-full group">
      <div
        className={cn(
          'flex items-start gap-3 w-full rounded px-2 py-1 -mx-2 transition-colors',
          hasExpandableContent && 'hover:bg-muted/50 cursor-pointer'
        )}
        onClick={handleToggle}
      >
        {/* Icon with hover-to-show chevron */}
        <div className={cn(
          'flex-shrink-0 mt-0.5 relative w-4 h-4',
          hasExpandableContent && 'cursor-pointer'
        )}>
          <Icon className={cn(
            'h-4 w-4 absolute inset-0 transition-opacity',
            iconClass,
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
          <div className={cn('text-xs font-mono', textClass)}>
            {message || 'An error occurred'}
          </div>

          {/* Expanded error details */}
          {isExpanded && hasExpandableContent && (
            <div className="mt-2 pl-4 border-l-2 border-border/30 space-y-2">
              {errorDetails.map((detail) => (
                <div key={detail.label}>
                  <div className="text-[10px] uppercase tracking-wide text-muted-foreground/60 mb-0.5">
                    {detail.label}
                  </div>
                  <pre className="text-xs bg-muted/30 rounded p-2 overflow-x-auto whitespace-pre-wrap break-all font-mono">
                    {detail.value}
                  </pre>
                </div>
              ))}
            </div>
          )}

          {/* Progress bar */}
          {progress !== null && (
            <div className="mt-2">
              <div className="flex items-center justify-between text-[10px] text-muted-foreground mb-1">
                <span>{statusLine ?? 'Progress'}</span>
                <span>{Math.round(progress)}%</span>
              </div>
              <div className="h-1.5 rounded-full bg-muted/70">
                <div
                  className="h-full rounded-full bg-primary/80 transition-[width]"
                  style={{ width: `${progress}%` }}
                />
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
});
