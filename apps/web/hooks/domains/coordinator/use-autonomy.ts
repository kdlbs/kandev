"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  getAutonomy,
  type AutonomyRead,
  type PauseState,
} from "@/lib/api/domains/coordinator-autonomy-api";
import { isUsableAutonomy, nextDeadlineMs, serverNowMs } from "@/lib/coordinator/autonomy";
import { useWebSocketClient } from "@/lib/ws/connection";

export const AUTONOMY_TIMER_FLOOR_MS = 5_000;
export const AUTONOMY_RETRY_MS = 30_000;

export type AutonomyInput = {
  value: AutonomyRead | null;
  loadedAt: number | null;
  error: boolean;
  loading: boolean;
  retry: () => void;
  /**
   * A write issued from this view counts as a read issued now: it outranks
   * every read issued before it, and `applyWrite` is ignored once a later read
   * has been issued. Absent where a view cannot write.
   */
  beginWrite?: () => number;
  applyWrite?: (token: number, patch: PauseState) => void;
};

type State = { value: AutonomyRead | null; loadedAt: number | null; error: boolean };
const EMPTY: State = { value: null, loadedAt: null, error: false };

/**
 * The autonomy read, re-read on mount, Try again, `coordinator.updated` with
 * `autonomy_changed`, and a timer for the next instant the strip would change.
 * One request sequence per hook: only the latest issued read may write, and a
 * failure keeps the last good value visible beside the error.
 */
export function useAutonomy(
  workspaceId: string,
  coordinatorId: string,
  enabled: boolean,
): AutonomyInput {
  const [state, setState] = useState<State>(EMPTY);
  const [loading, setLoading] = useState(enabled);
  const sequenceRef = useRef(0);
  const firedDeadlineRef = useRef<number | null>(null);
  const timerReadRef = useRef(false);
  const [retryTick, setRetryTick] = useState(0);

  const reload = useCallback(
    (fromTimer = false) => {
      const sequence = ++sequenceRef.current;
      timerReadRef.current = fromTimer;
      setLoading(true);
      getAutonomy(workspaceId, coordinatorId)
        .then((next) => {
          if (sequence !== sequenceRef.current) return;
          if (!isUsableAutonomy(next)) throw new Error("invalid autonomy read");
          setState({ value: next, loadedAt: Date.now(), error: false });
          setLoading(false);
        })
        .catch(() => {
          if (sequence !== sequenceRef.current) return;
          setState((prev) => ({ ...prev, error: true }));
          setLoading(false);
          if (fromTimer) setRetryTick((n) => n + 1);
        });
    },
    [workspaceId, coordinatorId],
  );

  useEffect(() => {
    setState(EMPTY);
    firedDeadlineRef.current = null;
    if (!enabled) {
      setLoading(false);
      return;
    }
    reload();
    return () => {
      sequenceRef.current += 1;
    };
  }, [enabled, reload]);

  const wsClient = useWebSocketClient();
  useEffect(() => {
    if (!enabled || !wsClient) return;
    return wsClient.on("coordinator.updated", (message) => {
      const payload = message.payload;
      if (payload.workspace_id !== workspaceId || payload.coordinator_id !== coordinatorId) return;
      if (payload.autonomy_changed !== true) return;
      reload();
    });
  }, [enabled, wsClient, workspaceId, coordinatorId, reload]);

  const { value, loadedAt } = state;
  useEffect(() => {
    if (!enabled || !value || loadedAt === null) return;
    const deadline = nextDeadlineMs(value);
    if (deadline === null || deadline === firedDeadlineRef.current) return;
    const delay = Math.max(
      AUTONOMY_TIMER_FLOOR_MS,
      deadline - serverNowMs(value, loadedAt, Date.now()),
    );
    const id = setTimeout(() => {
      firedDeadlineRef.current = deadline;
      reload(true);
    }, delay);
    return () => clearTimeout(id);
  }, [enabled, value, loadedAt, reload]);

  useEffect(() => {
    if (!enabled || retryTick === 0) return;
    const id = setTimeout(() => reload(true), AUTONOMY_RETRY_MS);
    return () => clearTimeout(id);
  }, [enabled, retryTick, reload]);

  const retry = useCallback(() => reload(), [reload]);
  const beginWrite = useCallback(() => ++sequenceRef.current, []);
  const applyWrite = useCallback((token: number, patch: PauseState) => {
    if (token !== sequenceRef.current) return;
    setState((prev) =>
      prev.value ? { ...prev, value: { ...prev.value, ...patch }, error: false } : prev,
    );
    setLoading(false);
  }, []);
  return {
    value: state.value,
    loadedAt: state.loadedAt,
    error: state.error,
    loading,
    retry,
    beginWrite,
    applyWrite,
  };
}
