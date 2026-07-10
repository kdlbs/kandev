"use client";

import { useCallback, useEffect, useRef, useState } from "react";

export type ActionFeedbackState = "idle" | "pending" | "success" | "error";

type RunResult<T> = { ok: true; value: T } | { ok: false; error: unknown };

const DEFAULT_RESET_MS = 1800;
const DEFAULT_MIN_PENDING_MS = 350;

/**
 * Tracks the visible state of an async action so a button can render an
 * idle/loading/success/error animation even when the underlying call resolves
 * in single-digit milliseconds. Without this, fast operations like SQLite
 * VACUUM finish before any UI state flips, leaving the user wondering if
 * anything happened.
 *
 * Lifecycle:
 *   idle → pending → success | error → idle (auto, after resetMs)
 *
 * minPendingMs holds the pending state for at least this long so the spinner
 * is perceptible. resetMs is how long success/error sticks before reverting.
 */
export function useActionFeedback(opts?: { resetMs?: number; minPendingMs?: number }) {
  const resetMs = opts?.resetMs ?? DEFAULT_RESET_MS;
  const minPendingMs = opts?.minPendingMs ?? DEFAULT_MIN_PENDING_MS;
  const [state, setState] = useState<ActionFeedbackState>("idle");
  const resetTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (resetTimer.current) clearTimeout(resetTimer.current);
    };
  }, []);

  const run = useCallback(
    async <T>(fn: () => Promise<T>): Promise<RunResult<T>> => {
      if (resetTimer.current) {
        clearTimeout(resetTimer.current);
        resetTimer.current = null;
      }
      setState("pending");
      const startedAt = Date.now();
      let result: RunResult<T>;
      try {
        const value = await fn();
        result = { ok: true, value };
      } catch (error) {
        result = { ok: false, error };
      }
      const elapsed = Date.now() - startedAt;
      if (elapsed < minPendingMs) {
        await new Promise((r) => setTimeout(r, minPendingMs - elapsed));
      }
      setState(result.ok ? "success" : "error");
      resetTimer.current = setTimeout(() => setState("idle"), resetMs);
      return result;
    },
    [minPendingMs, resetMs],
  );

  return { state, run };
}
