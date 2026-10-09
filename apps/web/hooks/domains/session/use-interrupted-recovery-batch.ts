import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  sessionDeliveryRecoveryResponse,
  type SessionDeliveryRecoveryResponse,
} from "@/lib/services/session-recovery-service";
import {
  continueInterruptedSession,
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
            setResults((current) => ({ ...current, [item.sessionId]: result }));
        } catch (cause) {
          if (!mounted.current) continue;
          const blocked = blockedBatchResult(cause);
          if (blocked) setResults((current) => ({ ...current, [item.sessionId]: blocked }));
          else setFailures((current) => [...current, item.sessionId]);
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
      const result = readInterruptedRecoveryResult(item.taskId, item.sessionId);
      return result ? [[item.sessionId, result]] : [];
    }),
  );
  return {
    selected,
    toggle,
    run,
    busy,
    completed,
    results: { ...persistedResults, ...results },
    failures,
  };
}

function continueBatchItem(
  item: InterruptedRecoveryCandidate,
  instruction: string,
  acknowledged: boolean,
  failureMessage: string,
) {
  const previous = readInterruptedCheckpoint(item.taskId, item.sessionId);
  const observed = previous
    ? {
        ...item.observed,
        recovery_revision: previous.request.recovery_revision,
        recovery_identity: previous.request.recovery_identity,
      }
    : item.observed;
  return continueInterruptedSession({
    taskId: item.taskId,
    sessionId: item.sessionId,
    observed,
    instruction: previous?.request.instruction ?? instruction,
    acknowledged,
    failureMessage,
  });
}

function blockedBatchResult(cause: unknown): SessionDeliveryRecoveryResponse | null {
  if (!cause || typeof cause !== "object" || !("deliveryRecovery" in cause)) return null;
  return sessionDeliveryRecoveryResponse(cause.deliveryRecovery);
}
