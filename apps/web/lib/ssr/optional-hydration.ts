export const OPTIONAL_HYDRATION_TIMEOUT_MS = 5_000;

export type OptionalHydrationResult<T> =
  | { status: "fulfilled"; value: T }
  | { status: "unavailable" };

/** Starts a shared deadline for optional route-enrichment requests. */
export function beginOptionalHydration() {
  let deadlineTimer: ReturnType<typeof setTimeout>;
  const deadline = new Promise<void>((resolve) => {
    deadlineTimer = setTimeout(resolve, OPTIONAL_HYDRATION_TIMEOUT_MS);
  });

  return {
    load<T>(
      label: string,
      operation: (signal: AbortSignal) => Promise<T>,
    ): Promise<OptionalHydrationResult<T>> {
      return new Promise((resolve) => {
        const controller = new AbortController();
        let settled = false;
        const settle = (value: OptionalHydrationResult<T>) => {
          if (settled) return;
          settled = true;
          resolve(value);
        };

        void Promise.resolve()
          .then(() => operation(controller.signal))
          .then(
            (value) => settle({ status: "fulfilled", value }),
            (error) => {
              if (settled) return;
              console.warn(
                `[session-page-state] optional ${label} failed; continuing without it`,
                error,
              );
              settle({ status: "unavailable" });
            },
          );
        void deadline.then(() => {
          if (!settled) {
            controller.abort();
            console.warn(
              `[session-page-state] optional ${label} timed out after ${OPTIONAL_HYDRATION_TIMEOUT_MS}ms; continuing without it`,
            );
            settle({ status: "unavailable" });
          }
        });
      });
    },
    complete() {
      clearTimeout(deadlineTimer);
    },
  };
}
