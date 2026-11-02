import type { TaskNavigationIdentity } from "./task-navigation-reads";

export type LoadTaskNavigationIdentity = (
  taskId: string,
  signal: AbortSignal,
) => Promise<TaskNavigationIdentity>;

export const TASK_NAVIGATION_ATTEMPT_TIMEOUT_MS = 10_000;

export function abortError(): Error {
  const error = new Error("Task navigation read aborted");
  error.name = "AbortError";
  return error;
}

export function raceAbort<T>(promise: Promise<T>, signal: AbortSignal): Promise<T> {
  if (signal.aborted) return Promise.reject(abortError());
  return new Promise<T>((resolve, reject) => {
    const onAbort = () => reject(abortError());
    signal.addEventListener("abort", onAbort, { once: true });
    promise.then(
      (value) => {
        signal.removeEventListener("abort", onAbort);
        resolve(value);
      },
      (error) => {
        signal.removeEventListener("abort", onAbort);
        reject(error);
      },
    );
  });
}

export async function loadNavigationAttempt(
  taskId: string,
  cycleSignal: AbortSignal,
  loadIdentity: LoadTaskNavigationIdentity,
): Promise<TaskNavigationIdentity> {
  const attemptController = new AbortController();
  const abortAttempt = () => attemptController.abort();
  let timedOut = false;
  if (cycleSignal.aborted) attemptController.abort();
  else cycleSignal.addEventListener("abort", abortAttempt, { once: true });
  const timeout = setTimeout(() => {
    timedOut = true;
    attemptController.abort();
  }, TASK_NAVIGATION_ATTEMPT_TIMEOUT_MS);
  try {
    return await raceAbort(
      loadIdentity(taskId, attemptController.signal),
      attemptController.signal,
    );
  } catch (error) {
    if (timedOut && !cycleSignal.aborted) {
      throw new TypeError("Task navigation read timed out");
    }
    throw error;
  } finally {
    clearTimeout(timeout);
    cycleSignal.removeEventListener("abort", abortAttempt);
    attemptController.abort();
  }
}
