import { useCallback, useRef } from "react";

export type RecoveryOperation = { requestKey: string; sessionKey: string; operationId: number };

/** Fences in-flight recovery calls so a stale response cannot write newer state. */
export function useRecoveryOperationFence(
  requestKey: string,
  sessionKey: string,
  errorStamp?: string | null,
) {
  const activeRequestKeyRef = useRef(requestKey);
  const activeSessionKeyRef = useRef(sessionKey);
  const latestErrorStampRef = useRef(errorStamp);
  const operationGenerationRef = useRef(0);
  activeSessionKeyRef.current = sessionKey;
  latestErrorStampRef.current = errorStamp;
  if (activeRequestKeyRef.current !== requestKey) {
    activeRequestKeyRef.current = requestKey;
    operationGenerationRef.current += 1;
  }

  const beginOperation = useCallback(
    (): RecoveryOperation => ({
      requestKey,
      sessionKey,
      operationId: ++operationGenerationRef.current,
    }),
    [requestKey, sessionKey],
  );

  const isCurrentOperation = useCallback(
    (operation: RecoveryOperation) =>
      activeRequestKeyRef.current === operation.requestKey &&
      activeSessionKeyRef.current === operation.sessionKey &&
      operationGenerationRef.current === operation.operationId,
    [],
  );

  const isCurrentSession = useCallback(
    (operation: RecoveryOperation) => activeSessionKeyRef.current === operation.sessionKey,
    [],
  );

  const matchesLatestErrorStamp = useCallback(
    (stamp: string | undefined) => Boolean(stamp && latestErrorStampRef.current === stamp),
    [],
  );

  return { beginOperation, isCurrentOperation, isCurrentSession, matchesLatestErrorStamp };
}
