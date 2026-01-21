import { useEffect, useMemo, useState } from 'react';
import { useAppStore } from '@/components/state-provider';

/**
 * Hook to get a live timer for the active turn of a session.
 * Updates every second while there's an active turn.
 * 
 * @param sessionId - The session ID to track
 * @returns The formatted duration string (MM:SS or HH:MM:SS)
 */
export function useActiveTurnTimer(sessionId: string | null) {
  const turns = useAppStore((state) => sessionId ? state.turns.bySession[sessionId] : undefined);
  const activeTurnId = useAppStore((state) => sessionId ? state.turns.activeBySession[sessionId] : null);
  
  // Find the active turn
  const activeTurn = useMemo(() => {
    if (!turns || !activeTurnId) return null;
    return turns.find((t) => t.id === activeTurnId) ?? null;
  }, [turns, activeTurnId]);

  const [elapsedSeconds, setElapsedSeconds] = useState(() => {
    if (!activeTurn?.started_at) return 0;
    return Math.floor((Date.now() - new Date(activeTurn.started_at).getTime()) / 1000);
  });

  useEffect(() => {
    if (!activeTurn?.started_at) {
      return;
    }

    const startTime = new Date(activeTurn.started_at).getTime();

    const calculateElapsed = () => {
      return Math.floor((Date.now() - startTime) / 1000);
    };

    const interval = setInterval(() => {
      setElapsedSeconds(calculateElapsed());
    }, 1000);

    return () => clearInterval(interval);
  }, [activeTurn?.started_at]);

  // Format as MM:SS or HH:MM:SS
  const formattedDuration = useMemo(() => {
    if (elapsedSeconds <= 0) return '0:00';
    
    const hours = Math.floor(elapsedSeconds / 3600);
    const minutes = Math.floor((elapsedSeconds % 3600) / 60);
    const seconds = elapsedSeconds % 60;
    
    if (hours > 0) {
      return `${hours}:${minutes.toString().padStart(2, '0')}:${seconds.toString().padStart(2, '0')}`;
    }
    return `${minutes}:${seconds.toString().padStart(2, '0')}`;
  }, [elapsedSeconds]);

  return {
    isActive: !!activeTurn,
    elapsedSeconds,
    formattedDuration,
  };
}

