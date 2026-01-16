'use client';

import { IconAlertCircle, IconAlertTriangle } from '@tabler/icons-react';
import type { TaskSessionState } from '@/lib/types/http';

type RunningIndicatorProps = {
  state: TaskSessionState;
};

const STATE_CONFIG: Record<TaskSessionState, { label: string; icon: 'spinner' | 'error' | 'warning' | null }> = {
  CREATED: { label: 'Agent is being created', icon: 'spinner' },
  STARTING: { label: 'Agent is starting', icon: 'spinner' },
  RUNNING: { label: 'Agent is running', icon: 'spinner' },
  WAITING_FOR_INPUT: { label: 'Agent is waiting for input', icon: null },
  COMPLETED: { label: '', icon: null },
  FAILED: { label: 'Agent has encountered an error', icon: 'error' },
  CANCELLED: { label: 'Agent has been cancelled', icon: 'warning' },
};

export function RunningIndicator({ state }: RunningIndicatorProps) {
  const config = STATE_CONFIG[state];

  // Don't show indicator for completed state or waiting for input
  if (!config.icon) {
    return null;
  }

  const isError = config.icon === 'error';
  const isWarning = config.icon === 'warning';

  return (
    <div
      className={`flex items-center gap-2 px-3 py-2 rounded-md text-xs ${
        isError
          ? 'bg-destructive/10 text-destructive border border-destructive/20'
          : isWarning
          ? 'bg-yellow-500/10 text-yellow-600 dark:text-yellow-500 border border-yellow-500/20'
          : 'bg-muted text-muted-foreground'
      }`}
      role="status"
      aria-label={config.label}
    >
      {config.icon === 'spinner' && (
        <>
          <span className="uppercase tracking-wide font-medium">{config.label}</span>
          <span className="flex items-center gap-1 ml-1">
            <span
              className="w-1.5 h-1.5 rounded-full bg-current animate-bounce"
              style={{ animationDelay: '0ms', animationDuration: '1s' }}
            />
            <span
              className="w-1.5 h-1.5 rounded-full bg-current animate-bounce"
              style={{ animationDelay: '150ms', animationDuration: '1s' }}
            />
            <span
              className="w-1.5 h-1.5 rounded-full bg-current animate-bounce"
              style={{ animationDelay: '300ms', animationDuration: '1s' }}
            />
          </span>
        </>
      )}
      {config.icon === 'error' && (
        <>
          <IconAlertCircle className="h-3.5 w-3.5 flex-shrink-0" aria-hidden="true" />
          <span className="uppercase tracking-wide font-medium">{config.label}</span>
        </>
      )}
      {config.icon === 'warning' && (
        <>
          <IconAlertTriangle className="h-3.5 w-3.5 flex-shrink-0" aria-hidden="true" />
          <span className="uppercase tracking-wide font-medium">{config.label}</span>
        </>
      )}
    </div>
  );
}
