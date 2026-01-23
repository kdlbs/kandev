import { useMemo } from 'react';
import { useAppStore } from '@/components/state-provider';
import type { Turn } from '@/lib/types/http';

/**
 * Format duration in seconds to a human-readable string.
 * @param seconds - Duration in seconds
 * @returns Formatted string like "1m 23s" or "1h 2m 3s"
 */
function formatDuration(seconds: number): string {
  if (seconds < 0) return '0s';

  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = Math.floor(seconds % 60);

  if (hours > 0) {
    return `${hours}h ${minutes}m ${secs}s`;
  }
  if (minutes > 0) {
    return `${minutes}m ${secs}s`;
  }
  return `${secs}s`;
}

/**
 * Hook to get the last completed turn for a session.
 *
 * @param sessionId - The session ID to get the last turn for
 * @returns The last completed turn with its duration and model
 */
export function useSessionTurn(sessionId: string | null) {
  const turns = useAppStore((state) => sessionId ? state.turns.bySession[sessionId] : undefined);
  const activeTurnId = useAppStore((state) => sessionId ? state.turns.activeBySession[sessionId] : null);
  const session = useAppStore((state) => sessionId ? state.taskSessions.items[sessionId] : null);

  // Get model from session's agent_profile_snapshot
  const sessionModel = useMemo(() => {
    if (!session?.agent_profile_snapshot) return null;
    const snapshot = session.agent_profile_snapshot as Record<string, unknown>;
    const model = snapshot.model;
    return typeof model === 'string' ? model : null;
  }, [session?.agent_profile_snapshot]);

  // Find the last completed turn (most recent by completed_at)
  const lastCompletedTurn = useMemo<Turn | null>(() => {
    if (!turns || turns.length === 0) return null;

    const completedTurns = turns.filter((t: Turn) => t.completed_at);
    if (completedTurns.length === 0) return null;

    // Sort by completed_at descending and return the most recent
    return completedTurns.sort((a: Turn, b: Turn) => {
      const aTime = new Date(a.completed_at!).getTime();
      const bTime = new Date(b.completed_at!).getTime();
      return bTime - aTime;
    })[0];
  }, [turns]);

  // Calculate duration for the last completed turn
  const lastTurnDuration = useMemo(() => {
    if (!lastCompletedTurn?.started_at || !lastCompletedTurn?.completed_at) {
      return null;
    }

    const startTime = new Date(lastCompletedTurn.started_at).getTime();
    const endTime = new Date(lastCompletedTurn.completed_at).getTime();
    const durationSeconds = Math.floor((endTime - startTime) / 1000);

    return {
      seconds: durationSeconds,
      formatted: formatDuration(durationSeconds),
    };
  }, [lastCompletedTurn]);

  // Check if there's an active (running) turn
  const isActive = !!activeTurnId;

  return {
    lastCompletedTurn,
    lastTurnDuration,
    isActive,
    sessionModel,
  };
}

