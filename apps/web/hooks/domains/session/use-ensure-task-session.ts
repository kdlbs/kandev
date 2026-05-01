import { useCallback, useEffect, useRef, useState } from "react";
import { ensureTaskSession } from "@/lib/services/session-launch-service";
import { useTaskSessions } from "@/hooks/use-task-sessions";

/** Minimal task shape consumed by useEnsureTaskSession. */
export type EnsureTaskInput = {
  id?: string | null;
} | null;

export type EnsureTaskSessionStatus = "idle" | "preparing" | "error";

export type UseEnsureTaskSessionResult = {
  status: EnsureTaskSessionStatus;
  error: Error | null;
  retry: () => void;
};

/**
 * Ensures the task has at least one session by delegating to the backend's
 * idempotent `session.ensure` endpoint. The backend resolves the agent profile
 * (task metadata → workflow step → workflow → workspace default) and chooses
 * prepare vs start based on the workflow step's auto_start_agent action — the
 * frontend stays thin and contract-free.
 *
 * Behavior:
 * - No-op while sessions are still loading.
 * - No-op when the task already has at least one session.
 * - No-op when `enabled === false` or `task.id` is missing.
 * - Idempotent per task id within a mount; switching tasks resets the latch.
 */
export function useEnsureTaskSession(
  task: EnsureTaskInput,
  opts?: { enabled?: boolean },
): UseEnsureTaskSessionResult {
  const enabled = opts?.enabled ?? true;
  const taskId = task?.id ?? null;
  const { sessions, isLoaded, loadSessions } = useTaskSessions(taskId);

  const [status, setStatus] = useState<EnsureTaskSessionStatus>("idle");
  const [error, setError] = useState<Error | null>(null);
  const [retryToken, setRetryToken] = useState(0);

  // Latch keyed by `${taskId}:${retryToken}` so a re-mount on the same task
  // doesn't refire, but switching tasks or calling retry() does.
  const launchedKeyRef = useRef<string | null>(null);

  /* eslint-disable react-hooks/set-state-in-effect -- ensuring a session is a side effect; status mirrors that external work */
  useEffect(() => {
    if (!enabled || !taskId || !isLoaded) return;
    if (sessions.length > 0) return;
    const key = `${taskId}:${retryToken}`;
    if (launchedKeyRef.current === key) return;
    launchedKeyRef.current = key;

    // Cancel guard so a stale resolution can't overwrite a switched-away task.
    let cancelled = false;
    setStatus("preparing");
    setError(null);
    ensureTaskSession(taskId)
      .then(async () => {
        if (cancelled || launchedKeyRef.current !== key) return;
        // Force-reload: backend may have returned an existing_* source our initial list missed.
        await loadSessions(true);
        if (cancelled || launchedKeyRef.current !== key) return;
        setStatus("idle");
      })
      .catch((err: unknown) => {
        if (cancelled || launchedKeyRef.current !== key) return;
        setStatus("error");
        setError(err instanceof Error ? err : new Error(String(err)));
        launchedKeyRef.current = null;
      });

    return () => {
      cancelled = true;
    };
  }, [enabled, taskId, isLoaded, loadSessions, sessions.length, retryToken]);
  /* eslint-enable react-hooks/set-state-in-effect */

  const retry = useCallback(() => {
    setStatus("idle");
    setError(null);
    setRetryToken((n) => n + 1);
  }, []);

  return { status, error, retry };
}
