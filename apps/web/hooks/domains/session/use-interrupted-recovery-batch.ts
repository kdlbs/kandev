import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  sessionDeliveryRecoveryResponse,
  type SessionDeliveryRecoveryResponse,
} from "@/lib/services/session-recovery-service";
import {
  continueInterruptedSession,
  interruptedRecoveryKey,
  readInterruptedCheckpoint,
  readInterruptedRecoveryResult,
} from "@/lib/services/interrupted-session-recovery";

export type InterruptedRecoveryCandidate = {
  taskId: string;
  sessionId: string;
  label: string;
  observed: SessionDeliveryRecoveryResponse;
};

export function useInterruptedRecoveryBatch(candidates: InterruptedRecoveryCandidate[]) {
  const { t } = useTranslation();
  const [selected, setSelected] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [completed, setCompleted] = useState(0);
  const [results, setResults] = useState<Record<string, SessionDeliveryRecoveryResponse>>({});
  const [failures, setFailures] = useState<string[]>([]);
  const inFlight = useRef(false);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const toggle = (sessionId: string) => {
    if (inFlight.current) return;
    setSelected((current) => {
      if (current.includes(sessionId)) return current.filter((id) => id !== sessionId);
      if (current.length >= 20) return current;
      return [...current, sessionId];
    });
  };
  const run = async (instruction: string, acknowledged: boolean) => {
    const items = candidates.filter((item) => selected.includes(item.sessionId));
    if (inFlight.current || !acknowledged || !instruction.trim() || !items.length) return;
    inFlight.current = true;
    setBusy(true);
    setCompleted(0);
    setFailures([]);
    try {
      for (const [index, item] of items.entries()) {
        if (!mounted.current) break;
        try {
          const checkpoint = await continueBatchItem(
            item,
            instruction,
            acknowledged,
            t("task:interruptedRecoveryFailed"),
          );
          const result = checkpoint.result;
          if (mounted.current && result)
            setResults((current) => ({
              ...current,
              [interruptedRecoveryKey(item.observed)]: result,
            }));
        } catch (cause) {
          if (!mounted.current) continue;
          const blocked = blockedBatchResult(cause);
          if (blocked)
            setResults((current) => ({
              ...current,
              [interruptedRecoveryKey(item.observed)]: blocked,
            }));
          else setFailures((current) => [...current, interruptedRecoveryKey(item.observed)]);
        } finally {
          if (mounted.current) setCompleted(index + 1);
        }
      }
    } finally {
      inFlight.current = false;
      if (mounted.current) setBusy(false);
    }
  };
  const persistedResults = Object.fromEntries(
    candidates.flatMap((item) => {
      const result =
        results[interruptedRecoveryKey(item.observed)] ??
        readInterruptedRecoveryResult(item.taskId, item.sessionId, item.observed);
      return result ? [[item.sessionId, result]] : [];
    }),
  );
  return {
    selected,
    toggle,
    run,
    busy,
    completed,
    results: persistedResults,
    failures: candidates
      .filter((item) => failures.includes(interruptedRecoveryKey(item.observed)))
      .map((item) => item.sessionId),
  };
}

function continueBatchItem(
  item: InterruptedRecoveryCandidate,
  instruction: string,
  acknowledged: boolean,
  failureMessage: string,
) {
  const previous = readInterruptedCheckpoint(item.taskId, item.sessionId, item.observed);
  return continueInterruptedSession({
    taskId: item.taskId,
    sessionId: item.sessionId,
    observed: item.observed,
    instruction: previous?.request.instruction ?? instruction,
    acknowledged,
    failureMessage,
  });
}

function blockedBatchResult(cause: unknown): SessionDeliveryRecoveryResponse | null {
  if (!cause || typeof cause !== "object" || !("deliveryRecovery" in cause)) return null;
  return sessionDeliveryRecoveryResponse(cause.deliveryRecovery);
}
