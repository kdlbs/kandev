'use client';

import { IconChevronRight, IconClock, IconCpu } from '@tabler/icons-react';
import { useSessionTurn } from '@/hooks/use-session-turn';

type TurnSummaryProps = {
  sessionId: string | null;
};

/**
 * A compact display of the last completed turn's duration and model.
 * Only shows when there's a completed turn and no active turn running.
 */
export function TurnSummary({ sessionId }: TurnSummaryProps) {
  const { lastCompletedTurn, lastTurnDuration, isActive, sessionModel } = useSessionTurn(sessionId);

  // Don't show if there's an active turn running or no completed turn
  if (isActive || !lastCompletedTurn || !lastTurnDuration) {
    return null;
  }

  return (
    <div className="flex items-center gap-2 py-2 mt-2 pl-3">
      <IconChevronRight className="h-3.5 w-3.5 text-muted-foreground/50" />
      <div className="flex items-center gap-1.5 text-xs text-emerald-600 dark:text-emerald-400">
        <IconClock className="h-3.5 w-3.5" />
        <span className="font-medium">{lastTurnDuration.formatted}</span>
      </div>
      {sessionModel && (
        <>
          <span className="text-muted-foreground/30">·</span>
          <div className="flex items-center gap-1.5 text-xs text-sky-600 dark:text-sky-400">
            <IconCpu className="h-3.5 w-3.5" />
            <span className="font-medium">{sessionModel}</span>
          </div>
        </>
      )}
    </div>
  );
}

