import { useEffect, useRef } from "react";
import type { SessionRecoveryNoticeKind } from "./use-session-resumption-operations";

type RecoveryResumptionState = "resumed" | "running";

type RecoveryFeedbackSetters = {
  setError: (value: string | null) => void;
  setNotice?: (value: string | null) => void;
  setNoticeKind?: (kind: SessionRecoveryNoticeKind | null) => void;
  setRecoveryFailure?: (value: null) => void;
  setResumptionState: (value: RecoveryResumptionState) => void;
};

function isActiveSessionState(state: string | undefined): boolean {
  return state === "STARTING" || state === "RUNNING" || state === "WAITING_FOR_INPUT";
}

function isRecoveryReadyState(state: string | undefined): boolean {
  return state === "RUNNING" || state === "WAITING_FOR_INPUT";
}

/** Clear automatic recovery feedback after another recovery makes the session active. */
export function useSessionRecoveryFeedback({
  sessionId,
  sessionState,
  error,
  notice,
  noticeKind,
  setters,
}: {
  sessionId: string | null;
  sessionState: string | undefined;
  error: string | null;
  notice: string | null;
  noticeKind: SessionRecoveryNoticeKind | null;
  setters: RecoveryFeedbackSetters;
}): void {
  const { setError, setNotice, setNoticeKind, setRecoveryFailure, setResumptionState } = setters;
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
    const inspectionReady = noticeKind === "inspection_busy" && isRecoveryReadyState(sessionState);
    if (notice !== null && (noticeKind !== "inspection_busy" || inspectionReady)) {
      setNotice?.(null);
      setNoticeKind?.(null);
    }
    setRecoveryFailure?.(null);
    setResumptionState(sessionState === "RUNNING" ? "running" : "resumed");
  }, [
    error,
    notice,
    noticeKind,
    sessionId,
    sessionState,
    setError,
    setNotice,
    setNoticeKind,
    setRecoveryFailure,
    setResumptionState,
  ]);
}
