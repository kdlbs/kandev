import { useEffect, useRef } from "react";

type RecoveryResumptionState = "resumed" | "running";

type RecoveryFeedbackSetters = {
  setError: (value: string | null) => void;
  setNotice?: (value: string | null) => void;
  setResumptionState: (value: RecoveryResumptionState) => void;
};

function isActiveSessionState(state: string | undefined): boolean {
  return state === "STARTING" || state === "RUNNING" || state === "WAITING_FOR_INPUT";
}

/** Clear automatic recovery feedback after another recovery makes the session active. */
export function useSessionRecoveryFeedback(
  sessionId: string | null,
  sessionState: string | undefined,
  error: string | null,
  notice: string | null,
  setters: RecoveryFeedbackSetters,
): void {
  const { setError, setNotice, setResumptionState } = setters;
  const lastObservedSessionRef = useRef<{ id: string | null; state?: string }>({
    id: sessionId,
    state: sessionState,
  });

  useEffect(() => {
    const previous = lastObservedSessionRef.current;
    const stateChanged = previous.id === sessionId && previous.state !== sessionState;
    lastObservedSessionRef.current = { id: sessionId, state: sessionState };
    if (!stateChanged || !isActiveSessionState(sessionState)) return;
    if (error !== null) setError(null);
    if (notice !== null) setNotice?.(null);
    setResumptionState(sessionState === "RUNNING" ? "running" : "resumed");
  }, [error, notice, sessionId, sessionState, setError, setNotice, setResumptionState]);
}
