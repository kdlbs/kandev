"use client";

import { useMemo, useState, useEffect } from "react";
import { IconAlertCircle, IconAlertTriangle } from "@tabler/icons-react";
import type { Message, TaskSessionState } from "@/lib/types/http";
import { useSessionTurn } from "@/hooks/domains/session/use-session-turn";
import { useAppStore } from "@/components/state-provider";
import { GridSpinner } from "@/components/grid-spinner";

type AgentStatusProps = {
  sessionState?: TaskSessionState;
  sessionId: string | null;
  messages?: Message[];
};

const STATE_CONFIG: Record<
  TaskSessionState,
  { label: string; icon: "spinner" | "error" | "warning" | null }
> = {
  CREATED: { label: "", icon: null },
  STARTING: { label: "Agent is starting", icon: "spinner" },
  RUNNING: { label: "Agent is running", icon: "spinner" },
  WAITING_FOR_INPUT: { label: "", icon: null },
  COMPLETED: { label: "", icon: null },
  FAILED: { label: "Agent has encountered an error", icon: "error" },
  CANCELLED: { label: "", icon: null },
};

/**
 * Format duration in seconds to a human-readable string.
 */
function formatDuration(seconds: number): string {
  if (seconds < 0) return "0s";
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
    if (!lastAgentMsg && msg.author_type === "agent") {
      lastAgentMsg = msg;
    } else if (lastAgentMsg && msg.author_type === "user") {
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
  const [elapsedSeconds, setElapsedSeconds] = useState(() => {
    if (!isRunning || !turnStartedAt) return 0;
    return Math.floor((Date.now() - new Date(turnStartedAt).getTime()) / 1000);
  });

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

  // Keep showing elapsed time while isRunning, even if turnStartedAt was cleared
  const displaySeconds = isRunning ? elapsedSeconds : 0;

  // Format as Xs or XmXs
  const formatted = useMemo(() => {
    return formatDuration(displaySeconds);
  }, [displaySeconds]);

  return { elapsedSeconds: displaySeconds, formatted };
}

function useActiveTurn(sessionId: string | null) {
  const turns = useAppStore((state) => (sessionId ? state.turns.bySession[sessionId] : undefined));
  const activeTurnId = useAppStore((state) =>
    sessionId ? state.turns.activeBySession[sessionId] : null,
  );
  return useMemo(() => {
    if (!turns || !activeTurnId) return null;
    return turns.find((t) => t.id === activeTurnId) ?? null;
  }, [turns, activeTurnId]);
}

function AgentErrorStatus({ config }: { config: { label: string } }) {
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

function AgentWarningStatus({ config }: { config: { label: string } }) {
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

function AgentRunningStatus({
  config,
  elapsedSeconds,
  runningDuration,
}: {
  config: { label: string };
  elapsedSeconds: number;
  runningDuration: string;
}) {
  return (
    <div className="py-2" role="status" aria-label={config.label}>
      <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
        {config.label}
        <GridSpinner className="text-muted-foreground" />
        {elapsedSeconds > 0 && (
          <span className="text-muted-foreground/60 tabular-nums">{runningDuration}</span>
        )}
      </span>
    </div>
  );
}

function useAgentStatusData(sessionId: string | null, messages: Message[], isRunning: boolean) {
  const { lastTurnDuration, isActive: isTurnActive } = useSessionTurn(sessionId);
  const activeTurn = useActiveTurn(sessionId);
  const { formatted: runningDuration, elapsedSeconds } = useRunningTimer(
    isRunning,
    activeTurn?.started_at ?? null,
  );
  const fallbackDuration = useMemo(() => {
    if (lastTurnDuration) return null;
    return calculateTurnDurationFromMessages(messages);
  }, [messages, lastTurnDuration]);
  const displayDuration = lastTurnDuration?.formatted ?? fallbackDuration;
  return { isTurnActive, runningDuration, elapsedSeconds, displayDuration };
}

export function AgentStatus({ sessionState, sessionId, messages = [] }: AgentStatusProps) {
  const config = sessionState ? STATE_CONFIG[sessionState] : null;
  const isRunning = config?.icon === "spinner";
  const isError = config?.icon === "error";
  const isWarning = config?.icon === "warning";

  const { isTurnActive, runningDuration, elapsedSeconds, displayDuration } = useAgentStatusData(
    sessionId,
    messages,
    isRunning,
  );

  if (isError && config) return <AgentErrorStatus config={config} />;
  if (isWarning && config) return <AgentWarningStatus config={config} />;
  if (isRunning && config)
    return (
      <AgentRunningStatus
        config={config}
        elapsedSeconds={elapsedSeconds}
        runningDuration={runningDuration}
      />
    );

  if (!isTurnActive && displayDuration) {
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
