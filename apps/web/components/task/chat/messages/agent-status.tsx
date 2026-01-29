'use client';

import { useCallback, useMemo, useState, useEffect, useRef } from 'react';
import { IconAlertCircle, IconAlertTriangle, IconCopy, IconCheck } from '@tabler/icons-react';
import type { Message, TaskSessionState } from '@/lib/types/http';
import { useSessionTurn } from '@/hooks/domains/session/use-session-turn';

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
 * Starts timer when isRunning becomes true, resets when it becomes false.
 */
function useRunningTimer(isRunning: boolean) {
  const [elapsedSeconds, setElapsedSeconds] = useState(0);
  const startTimeRef = useRef<number | null>(null);

  useEffect(() => {
    if (!isRunning) {
      // Reset refs when not running (state reset handled below)
      startTimeRef.current = null;
      return;
    }

    // Start timer
    if (!startTimeRef.current) {
      startTimeRef.current = Date.now();
    }
    const interval = setInterval(() => {
      if (startTimeRef.current) {
        setElapsedSeconds(Math.floor((Date.now() - startTimeRef.current) / 1000));
      }
    }, 1000);
    return () => clearInterval(interval);
  }, [isRunning]);

  // Reset elapsed seconds when not running (outside effect to avoid lint warning)
  const displaySeconds = isRunning ? elapsedSeconds : 0;

  // Format as Xs or XmXs
  const formatted = useMemo(() => {
    return formatDuration(displaySeconds);
  }, [displaySeconds]);

  return { elapsedSeconds: displaySeconds, formatted };
}

function GridSpinner({ className }: { className?: string }) {
  return (
    <span className={`spinner-grid ${className ?? ''}`} role="status" aria-label="Loading">
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
      <span className="spinner-grid-cube" />
    </span>
  );
}

function CopyButton({ content }: { content: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(content);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  }, [content]);

  return (
    <button
      onClick={handleCopy}
      className="inline-flex items-center justify-center h-5 w-5 rounded hover:bg-muted/50 text-muted-foreground/60 hover:text-muted-foreground transition-colors"
      title="Copy last response"
    >
      {copied ? (
        <IconCheck className="h-3.5 w-3.5 text-emerald-500" />
      ) : (
        <IconCopy className="h-3.5 w-3.5" />
      )}
    </button>
  );
}

export function AgentStatus({ sessionState, sessionId, messages = [] }: AgentStatusProps) {
  const { lastTurnDuration, isActive: isTurnActive } = useSessionTurn(sessionId);

  const config = sessionState ? STATE_CONFIG[sessionState] : null;
  const isRunning = config?.icon === 'spinner';

  // Use local timer that starts when agent is running
  const { formatted: runningDuration, elapsedSeconds } = useRunningTimer(isRunning);
  const isError = config?.icon === 'error';
  const isWarning = config?.icon === 'warning';

  // Get last agent message content for copy button
  const lastAgentContent = useMemo(() => {
    for (let i = messages.length - 1; i >= 0; i--) {
      if (messages[i].author_type === 'agent' && messages[i].content) {
        return messages[i].content;
      }
    }
    return null;
  }, [messages]);

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
        <span className="inline-flex items-center gap-2 text-xs">
          <GridSpinner className="text-primary" />
          <span className="text-muted-foreground">{config.label}</span>
          {elapsedSeconds > 0 && (
            <span className="text-muted-foreground/60 tabular-nums">
              {runningDuration}
            </span>
          )}
        </span>
      </div>
    );
  }

  // Completed/idle state - show duration and copy button if available
  const showCompletedState = !isTurnActive && (displayDuration || lastAgentContent);
  if (showCompletedState) {
    return (
      <div className="flex items-center gap-2 py-2">
        <span className="inline-flex items-center gap-2 text-xs text-muted-foreground">
          {displayDuration && <span className="tabular-nums">{displayDuration}</span>}
          {lastAgentContent && <CopyButton content={lastAgentContent} />}
        </span>
      </div>
    );
  }

  return null;
}
