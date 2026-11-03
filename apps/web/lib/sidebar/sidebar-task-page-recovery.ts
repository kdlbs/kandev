import {
  isReadAbort,
  isTemporaryReadError,
  readRetryAfterMs,
  ReadAttemptDeadlineError,
} from "@/lib/api/temporary-read-error";
import { abortError, raceAbort } from "@/lib/state/task-navigation-attempt";
import {
  TaskNavigationCycleDeadlineError,
  TaskNavigationRecoveryClock,
} from "@/lib/state/task-navigation-recovery-clock";

/** Shared bounded schedule: five total attempts inside thirty seconds of visible time. */
export const SIDEBAR_RECOVERY_RETRY_DELAYS_MS = [2_000, 5_000, 10_000, 10_000] as const;
export const SIDEBAR_RECOVERY_MAX_ATTEMPTS = SIDEBAR_RECOVERY_RETRY_DELAYS_MS.length + 1;
export const SIDEBAR_RECOVERY_ATTEMPT_DEADLINE_MS = 10_000;

export type SidebarPageRecoveryCycle<T> = {
  promise: Promise<T>;
  abort: () => void;
};

export type SidebarPageRecoveryArgs<T> = {
  attempt: (signal: AbortSignal) => Promise<T>;
  onAttemptStarted?: (attempt: number) => void;
  onAttemptFailed?: (error: unknown, nextAttempt: number) => void;
};

async function runAttempt<T>(load: (signal: AbortSignal) => Promise<T>, cycleSignal: AbortSignal) {
  const attempt = new AbortController();
  const abortAttempt = () => attempt.abort();
  let timedOut = false;
  if (cycleSignal.aborted) attempt.abort();
  else cycleSignal.addEventListener("abort", abortAttempt, { once: true });
  const deadline = setTimeout(() => {
    timedOut = true;
    attempt.abort();
  }, SIDEBAR_RECOVERY_ATTEMPT_DEADLINE_MS);
  try {
    return await raceAbort(load(attempt.signal), attempt.signal);
  } catch (error) {
    if (timedOut && !cycleSignal.aborted) throw new ReadAttemptDeadlineError();
    throw error;
  } finally {
    clearTimeout(deadline);
    cycleSignal.removeEventListener("abort", abortAttempt);
    attempt.abort();
  }
}

/**
 * One bounded recovery cycle for a shared sidebar page read. The visible-time
 * clock pauses retries and the budget while the tab is hidden; cycle expiry and
 * final release abort the current attempt even when a transport ignores it.
 */
export function startSidebarPageRecoveryCycle<T>({
  attempt,
  onAttemptStarted,
  onAttemptFailed,
}: SidebarPageRecoveryArgs<T>): SidebarPageRecoveryCycle<T> {
  const controller = new AbortController();
  let cycleExpired = false;
  const clock = new TaskNavigationRecoveryClock({
    onDeadline: () => {
      cycleExpired = true;
      controller.abort();
    },
  });
  const run = async () => {
    clock.start();
    for (let attemptIndex = 0; attemptIndex < SIDEBAR_RECOVERY_MAX_ATTEMPTS; attemptIndex++) {
      if (controller.signal.aborted) throw abortError();
      onAttemptStarted?.(attemptIndex);
      try {
        return await runAttempt(attempt, controller.signal);
      } catch (error) {
        if (cycleExpired) throw new TaskNavigationCycleDeadlineError();
        if (controller.signal.aborted || isReadAbort(error)) throw abortError();
        if (!isTemporaryReadError(error) || attemptIndex === SIDEBAR_RECOVERY_MAX_ATTEMPTS - 1) {
          throw error;
        }
        const delayMs = Math.max(
          SIDEBAR_RECOVERY_RETRY_DELAYS_MS[attemptIndex],
          readRetryAfterMs(error),
        );
        onAttemptFailed?.(error, attemptIndex + 1);
        await clock.wait(delayMs, controller.signal);
      }
    }
    throw new TaskNavigationCycleDeadlineError();
  };
  const promise = run().finally(() => clock.dispose());
  return { promise, abort: () => controller.abort() };
}
