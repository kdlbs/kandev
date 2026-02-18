'use client';

import { useState, memo } from 'react';
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

function getStatusStyle(isError: boolean, isWarning: boolean) {
  if (isError) {
    return { Icon: IconAlertTriangle, iconClass: 'text-red-500', textClass: 'text-red-600 dark:text-red-400' };
  }
  if (isWarning) {
    return { Icon: IconAlertTriangle, iconClass: 'text-amber-500', textClass: 'text-amber-600 dark:text-amber-400' };
  }
  return { Icon: IconInfoCircle, iconClass: 'text-muted-foreground', textClass: 'text-muted-foreground' };
}

function formatErrorDetails(metadata: ErrorMetadata | undefined): { label: string; value: string }[] {
  const details: { label: string; value: string }[] = [];
  if (!metadata) return details;

  if (metadata.stderr && metadata.stderr.length > 0) {
    details.push({ label: 'Agent Output', value: metadata.stderr.join('\n') });
  }
  if (metadata.error) {
    details.push({ label: 'Error', value: metadata.error });
  }
  if (metadata.text) {
    details.push({ label: 'Details', value: metadata.text });
  }
  if (metadata.error_data) {
    const filteredData = { ...metadata.error_data };
    delete filteredData.stderr;
    if (Object.keys(filteredData).length > 0) {
      details.push({ label: 'Error Data', value: JSON.stringify(filteredData, null, 2) });
    }
  }
  return details;
}

function StatusExpandIcon({ hasExpandableContent, isExpanded, iconClass, Icon }: {
  hasExpandableContent: boolean;
  isExpanded: boolean;
  iconClass: string;
  Icon: React.ElementType;
}) {
  return (
    <div className={cn('flex-shrink-0 mt-0.5 relative w-4 h-4', hasExpandableContent && 'cursor-pointer')}>
      <Icon className={cn('h-4 w-4 absolute inset-0 transition-opacity', iconClass, hasExpandableContent && 'group-hover:opacity-0')} />
      {hasExpandableContent && (
        isExpanded
          ? <IconChevronDown className="h-4 w-4 text-muted-foreground absolute inset-0 opacity-0 group-hover:opacity-100 transition-opacity" />
          : <IconChevronRight className="h-4 w-4 text-muted-foreground absolute inset-0 opacity-0 group-hover:opacity-100 transition-opacity" />
      )}
    </div>
  );
}

function StatusProgressBar({ progress, statusLine }: { progress: number; statusLine: string | undefined }) {
  return (
    <div className="mt-2">
      <div className="flex items-center justify-between text-[10px] text-muted-foreground mb-1">
        <span>{statusLine ?? 'Progress'}</span>
        <span>{Math.round(progress)}%</span>
      </div>
      <div className="h-1.5 rounded-full bg-muted/70">
        <div className="h-full rounded-full bg-primary/80 transition-[width]" style={{ width: `${progress}%` }} />
      </div>
    </div>
  );
}

function parseStatusMetadata(comment: Message) {
  const metadata = comment.metadata as ErrorMetadata | undefined;
  const progress = typeof metadata?.progress === 'number' ? Math.min(Math.max(metadata.progress, 0), 100) : null;
  const statusLine = metadata?.stage || metadata?.status;
  const message = metadata?.message || comment.content || statusLine || 'Status update';
  const isError = comment.type === 'error' || metadata?.variant === 'error';
  const isWarning = metadata?.variant === 'warning' || metadata?.cancelled === true;
  return { metadata, progress, statusLine, message, isError, isWarning };
}

function computeExpandableContent(isError: boolean, metadata: ErrorMetadata | undefined) {
  if (!isError) return { hasExpandableContent: false, errorDetails: [] as { label: string; value: string }[] };
  const hasErrorDetails = metadata?.error_data || metadata?.error || metadata?.text || metadata?.stderr;
  if (!hasErrorDetails) return { hasExpandableContent: false, errorDetails: [] as { label: string; value: string }[] };
  const errorDetails = formatErrorDetails(metadata);
  return { hasExpandableContent: errorDetails.length > 0, errorDetails };
}

function SimpleStatusMessage({ message }: { message: string }) {
  return (
    <div className="flex items-center gap-3 w-full py-2">
      <div className="flex-1 h-px bg-border" />
      <span className="text-xs text-muted-foreground/60 whitespace-nowrap">{message}</span>
      <div className="flex-1 h-px bg-border" />
    </div>
  );
}

function CancelledStatusMessage({ message }: { message: string }) {
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

function ExpandableErrorDetails({ errorDetails }: { errorDetails: { label: string; value: string }[] }) {
  return (
    <div className="mt-2 pl-4 border-l-2 border-border/30 space-y-2">
      {errorDetails.map((detail) => (
        <div key={detail.label}>
          <div className="text-[10px] uppercase tracking-wide text-muted-foreground/60 mb-0.5">{detail.label}</div>
          <pre className="text-xs bg-muted/30 rounded p-2 overflow-x-auto whitespace-pre-wrap break-all font-mono">{detail.value}</pre>
        </div>
      ))}
    </div>
  );
}

type StatusMessageBodyProps = {
  message: string;
  textClass: string;
  isExpanded: boolean;
  errorDetails: { label: string; value: string }[];
  hasExpandableContent: boolean;
  progress: number | null;
  statusLine: string | undefined;
};

function StatusMessageBody({ message, textClass, isExpanded, errorDetails, hasExpandableContent, progress, statusLine }: StatusMessageBodyProps) {
  return (
    <div className="flex-1 min-w-0 pt-0.5">
      <div className={cn('text-xs font-mono', textClass)}>{message || 'An error occurred'}</div>
      {isExpanded && hasExpandableContent && <ExpandableErrorDetails errorDetails={errorDetails} />}
      {progress !== null && <StatusProgressBar progress={progress} statusLine={statusLine} />}
    </div>
  );
}

export const StatusMessage = memo(function StatusMessage({ comment }: { comment: Message }) {
  const [isExpanded, setIsExpanded] = useState(false);
  const { metadata, progress, statusLine, message, isError, isWarning } = parseStatusMetadata(comment);
  const { hasExpandableContent, errorDetails } = computeExpandableContent(isError, metadata);
  const isSimpleStatus = !isError && !isWarning && progress === null && !statusLine && !metadata?.message;

  if (isSimpleStatus) return <SimpleStatusMessage message={message} />;
  if (metadata?.cancelled) return <CancelledStatusMessage message={message} />;

  const { Icon, iconClass, textClass } = getStatusStyle(isError, isWarning);

  return (
    <div className="w-full group">
      <div
        className={cn('flex items-start gap-3 w-full rounded px-2 py-1 -mx-2 transition-colors', hasExpandableContent && 'hover:bg-muted/50 cursor-pointer')}
        onClick={hasExpandableContent ? () => setIsExpanded((prev) => !prev) : undefined}
      >
        <StatusExpandIcon hasExpandableContent={hasExpandableContent} isExpanded={isExpanded} iconClass={iconClass} Icon={Icon} />
        <StatusMessageBody
          message={message}
          textClass={textClass}
          isExpanded={isExpanded}
          errorDetails={errorDetails}
          hasExpandableContent={hasExpandableContent}
          progress={progress}
          statusLine={statusLine}
        />
      </div>
    </div>
  );
});
