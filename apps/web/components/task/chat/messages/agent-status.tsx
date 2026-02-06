'use client';

import { useMemo, useState, useEffect } from 'react';
import { IconAlertCircle, IconAlertTriangle } from '@tabler/icons-react';
import type { Message, TaskSessionState } from '@/lib/types/http';
import { useSessionTurn } from '@/hooks/domains/session/use-session-turn';
import { useAppStore } from '@/components/state-provider';
import { GridSpinner } from '@/components/grid-spinner';

type AgentStatusProps = {
  sessionState?: TaskSessionState;
  sessionId: string | null;
  messages?: Message[];
};

const STATE_CONFIG: Record<TaskSessionState, { label: string; icon: 'spinner' | 'error' | 'warning' | null }> = {
  CREATED: { label: 'Agent is being created', icon: 'spinner' },
  STARTING: { label: 'Agent is starting', icon: 'spinner' },
  RUNNING: { label: 'Agent is running', icon: 'spinner' },
  WAITING_FOR_INPUT: { label: '', icon: null },
  COMPLETED: { label: '', icon: null },
  FAILED: { label: 'Agent has encountered an error', icon: 'error' },
  CANCELLED: { label: 'Agent has been cancelled', icon: 'warning' },
};

/**
 * Format duration in seconds to a human-readable string.
 */
function formatDuration(seconds: number): string {
  if (seconds < 0) return '0s';
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = Math.floor(seconds % 60);
  if (hours > 0) return `${hours}h ${minutes}m ${secs}s`;
  if (minutes > 0) return `${minutes}m ${secs}s`;
  return `${secs}s`;
}

/**
 * Calculate turn duration from messages as a fallback when turn data is not available.
 * Finds the last user message and the last agent message to estimate duration.
 */
function calculateTurnDurationFromMessages(messages: Message[]): string | null {
  if (messages.length < 2) return null;

  // Find last agent message
  let lastAgentMsg: Message | null = null;
  let lastUserMsgBeforeAgent: Message | null = null;

  for (let i = messages.length - 1; i >= 0; i--) {
    const msg = messages[i];
    if (!lastAgentMsg && msg.author_type === 'agent') {
      lastAgentMsg = msg;
    } else if (lastAgentMsg && msg.author_type === 'user') {
      lastUserMsgBeforeAgent = msg;
      break;
    }
  }

  if (!lastAgentMsg || !lastUserMsgBeforeAgent) return null;

  const startTime = new Date(lastUserMsgBeforeAgent.created_at).getTime();
  const endTime = new Date(lastAgentMsg.created_at).getTime();
  const durationSeconds = Math.floor((endTime - startTime) / 1000);

  if (durationSeconds < 0) return null;
  return formatDuration(durationSeconds);
}

/**
 * Hook to track elapsed time while agent is running.
 * Uses the turn's started_at timestamp to calculate elapsed time, so it persists across page refreshes.
 */
function useRunningTimer(isRunning: boolean, turnStartedAt: string | null) {
  const [elapsedSeconds, setElapsedSeconds] = useState(0);

  useEffect(() => {
    if (!isRunning || !turnStartedAt) {
      return;
    }

    const startTime = new Date(turnStartedAt).getTime();

    const updateElapsed = () => {
      const elapsed = Math.floor((Date.now() - startTime) / 1000);
      setElapsedSeconds(elapsed);
    };

    // Update in the interval callback (not synchronously in effect)
    const interval = setInterval(updateElapsed, 1000);

    // Also update once immediately, but in the next tick to avoid sync update
    const timeoutId = setTimeout(updateElapsed, 0);

    return () => {
      clearInterval(interval);
      clearTimeout(timeoutId);
    };
  }, [isRunning, turnStartedAt]);

  // Reset to 0 when not running
  const displaySeconds = isRunning && turnStartedAt ? elapsedSeconds : 0;

  // Format as Xs or XmXs
  const formatted = useMemo(() => {
    return formatDuration(displaySeconds);
  }, [displaySeconds]);

  return { elapsedSeconds: displaySeconds, formatted };
}


export function AgentStatus({ sessionState, sessionId, messages = [] }: AgentStatusProps) {
  const { lastTurnDuration, isActive: isTurnActive } = useSessionTurn(sessionId);

  const config = sessionState ? STATE_CONFIG[sessionState] : null;
  const isRunning = config?.icon === 'spinner';

  // Get active turn to use its started_at timestamp for the timer
  const turns = useAppStore((state) => sessionId ? state.turns.bySession[sessionId] : undefined);
  const activeTurnId = useAppStore((state) => sessionId ? state.turns.activeBySession[sessionId] : null);
  const activeTurn = useMemo(() => {
    if (!turns || !activeTurnId) return null;
    return turns.find((t) => t.id === activeTurnId) ?? null;
  }, [turns, activeTurnId]);

  // Use timer that uses the turn's started_at timestamp, so it persists across page refreshes
  const { formatted: runningDuration, elapsedSeconds } = useRunningTimer(isRunning, activeTurn?.started_at ?? null);
  const isError = config?.icon === 'error';
  const isWarning = config?.icon === 'warning';

  // Calculate duration from messages as fallback
  const fallbackDuration = useMemo(() => {
    if (lastTurnDuration) return null; // Don't need fallback
    return calculateTurnDurationFromMessages(messages);
  }, [messages, lastTurnDuration]);

  const displayDuration = lastTurnDuration?.formatted ?? fallbackDuration;

  // Error state
  if (isError && config) {
    return (
      <div
        className="flex items-center gap-2 px-3 py-2 rounded-lg text-xs bg-destructive/10 text-destructive border border-destructive/20"
        role="status"
        aria-label={config.label}
      >
        <IconAlertCircle className="h-3.5 w-3.5 flex-shrink-0" aria-hidden="true" />
        <span className="font-medium">{config.label}</span>
      </div>
    );
  }

  // Warning state
  if (isWarning && config) {
    return (
      <div
        className="flex items-center gap-2 px-3 py-2 rounded-lg text-xs bg-yellow-500/10 text-yellow-600 dark:text-yellow-500 border border-yellow-500/20"
        role="status"
        aria-label={config.label}
      >
        <IconAlertTriangle className="h-3.5 w-3.5 flex-shrink-0" aria-hidden="true" />
        <span className="font-medium">{config.label}</span>
      </div>
    );
  }

  // Running state
  if (isRunning && config) {
    return (
      <div className="py-2" role="status" aria-label={config.label}>
        <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
          {config.label}
          <GridSpinner className="text-muted-foreground" />
          {elapsedSeconds > 0 && (
            <span className="text-muted-foreground/60 tabular-nums">
              {runningDuration}
            </span>
          )}
        </span>
      </div>
    );
  }

  // Completed/idle state - show duration if available
  const showCompletedState = !isTurnActive && displayDuration;
  if (showCompletedState) {
    return (
      <div className="flex items-center gap-2 py-2">
        <span className="inline-flex items-center gap-2 text-xs text-muted-foreground tabular-nums">
          {displayDuration}
        </span>
      </div>
    );
  }

  return null;
}
