'use client';

import { useState } from 'react';
import { IconAlertTriangle, IconInfoCircle, IconPointFilled, IconHandStop, IconChevronDown, IconChevronRight } from '@tabler/icons-react';
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

export function StatusMessage({ comment }: { comment: Message }) {
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

  // Simple system message: no metadata, no progress, just a short message
  const isSimpleStatus = !isError && !isWarning && progress === null && !statusLine && !metadata?.message;

  if (isSimpleStatus) {
    return (
      <div className="flex items-center justify-center py-1">
        <div className="flex items-center gap-1.5 px-3 py-1 rounded-full bg-primary/10 border border-primary/20 text-xs text-primary">
          <IconPointFilled className="h-2 w-2" />
          <span>{message}</span>
        </div>
      </div>
    );
  }

  // Cancelled turn gets a special compact warning style
  if (metadata?.cancelled) {
    return (
      <div className="flex items-center justify-center py-1">
        <div className="flex items-center gap-1.5 px-3 py-1.5 rounded-full bg-amber-500/10 border border-amber-500/30 text-xs text-amber-600 dark:text-amber-400">
          <IconHandStop className="h-3 w-3" />
          <span>{message}</span>
        </div>
      </div>
    );
  }

  const Icon = isError ? IconAlertTriangle : isWarning ? IconAlertTriangle : IconInfoCircle;
  const iconClass = isError ? 'text-red-500' : isWarning ? 'text-amber-500' : 'text-muted-foreground';
  const label = isError ? 'Error' : isWarning ? 'Warning' : 'Status';

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

  return (
    <div
      className={cn(
        'w-full rounded-lg border px-4 py-3 text-sm',
        isError
          ? 'border-red-500/40 bg-red-500/10 text-foreground'
          : isWarning
          ? 'border-amber-500/40 bg-amber-500/10 text-foreground'
          : 'border-border/60 bg-muted/30 text-foreground'
      )}
    >
      <div className="flex items-center gap-2 mb-2 text-[11px] uppercase tracking-wide opacity-70">
        <Icon className={cn('h-3.5 w-3.5', iconClass)} />
        <span>{label}</span>
      </div>
      <div className="whitespace-pre-wrap">{message || 'An error occurred'}</div>

      {/* Expandable error details */}
      {hasErrorDetails && errorDetails.length > 0 && (
        <div className="mt-3 border-t border-red-500/20 pt-2">
          <button
            onClick={() => setIsExpanded(!isExpanded)}
            className="flex items-center gap-1 text-xs text-red-600 dark:text-red-400 hover:text-red-700 dark:hover:text-red-300 transition-colors"
          >
            {isExpanded ? (
              <IconChevronDown className="h-3.5 w-3.5" />
            ) : (
              <IconChevronRight className="h-3.5 w-3.5" />
            )}
            <span>{isExpanded ? 'Hide details' : 'Show details'}</span>
          </button>

          {isExpanded && (
            <div className="mt-2 space-y-2">
              {errorDetails.map((detail, index) => (
                <div key={index}>
                  <div className="text-[10px] uppercase tracking-wide text-red-600/70 dark:text-red-400/70 mb-0.5">
                    {detail.label}
                  </div>
                  <pre className="text-xs bg-red-500/5 rounded p-2 overflow-x-auto whitespace-pre-wrap break-all font-mono">
                    {detail.value}
                  </pre>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {progress !== null && (
        <div className="mt-2">
          <div className="flex items-center justify-between text-[11px] text-muted-foreground mb-1">
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
  );
}
