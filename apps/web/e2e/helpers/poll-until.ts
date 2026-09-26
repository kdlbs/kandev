import { dwell } from "./causal-waits";

const POLL_INTERVAL_MS = 250;

export async function pollUntil<T>(
  read: () => T | Promise<T>,
  isReady: (value: T) => boolean,
  timeout: number,
  message: string,
): Promise<T> {
  const deadline = Date.now() + timeout;
  let lastError: unknown;

  while (true) {
    try {
      const value = await read();
      if (isReady(value)) return value;
      lastError = undefined;
    } catch (error) {
      lastError = error;
    }

    const remaining = deadline - Date.now();
    if (remaining <= 0) {
      throw new Error(`${message} within ${timeout}ms`, {
        cause: lastError,
      });
    }
    await dwell(
      Math.min(POLL_INTERVAL_MS, remaining),
      "poll-interval",
      `${message}; the service has not reported the required state`,
    );
  }
}
