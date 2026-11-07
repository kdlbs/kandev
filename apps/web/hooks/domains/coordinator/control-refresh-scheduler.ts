/** How often the control snapshot is re-read while nothing else asks for it. */
export const CONTROL_REFRESH_INTERVAL_MS = 30_000;
/** Quiet time after the last live event before one re-read covers the burst. */
export const CONTROL_EVENT_SETTLE_MS = 2_000;
/** Live events start at most one re-read per this window, even when they never stop. */
export const CONTROL_EVENT_MIN_GAP_MS = 10_000;
/** Retry delays after consecutive failed reads; the last value repeats. */
export const CONTROL_RETRY_BACKOFF_MS = [5_000, 10_000, 20_000, 40_000, 60_000];

/** `retry` asks for the backoff schedule; `ok` returns to the normal interval. */
export type ControlRefreshOutcome = "ok" | "retry";

export type ControlRefreshSchedulerOptions = {
  run: (signal: AbortSignal) => Promise<ControlRefreshOutcome>;
  intervalMs?: number;
  settleMs?: number;
  minGapMs?: number;
  backoffMs?: number[];
  /**
   * A periodic or live-event read due while this returns false is skipped; the
   * caller's foreground-return read (one `request()`) covers it instead.
   */
  isVisible?: () => boolean;
  /** The time of the next automatic retry after a failure, or null once a read succeeds. */
  onRetryScheduled?: (at: number | null) => void;
};

export type ControlRefreshScheduler = {
  /** Read now, or, while a read runs, once more right after it. Resolves when that read ends. */
  request: () => Promise<void>;
  /** A live event: settle, then read once, at most once per minimum gap. */
  event: () => void;
  dispose: () => void;
};

type Deferred = { promise: Promise<void>; resolve: () => void };

function deferred(): Deferred {
  let resolve = () => {};
  const promise = new Promise<void>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

/**
 * One read lifetime for the control snapshot: a periodic read, coalesced
 * live-event reads and a manual read never overlap. A request that arrives
 * while a read is in flight never aborts it; all such requests share one
 * follow-up read that starts when the current one ends.
 */
export function createControlRefreshScheduler(
  options: ControlRefreshSchedulerOptions,
): ControlRefreshScheduler {
  const intervalMs = options.intervalMs ?? CONTROL_REFRESH_INTERVAL_MS;
  const settleMs = options.settleMs ?? CONTROL_EVENT_SETTLE_MS;
  const minGapMs = options.minGapMs ?? CONTROL_EVENT_MIN_GAP_MS;
  const backoff = options.backoffMs ?? CONTROL_RETRY_BACKOFF_MS;
  const controller = new AbortController();
  let inFlight: Promise<void> | null = null;
  let followUp: Deferred | null = null;
  let periodicTimer: ReturnType<typeof setTimeout> | undefined;
  let eventTimer: ReturnType<typeof setTimeout> | undefined;
  let firstEventAt: number | null = null;
  let lastEventReadAt = -Infinity;
  let failures = 0;
  let disposed = false;

  const clearTimers = () => {
    clearTimeout(periodicTimer);
    clearTimeout(eventTimer);
    periodicTimer = undefined;
    eventTimer = undefined;
    firstEventAt = null;
  };

  const schedulePeriodic = () => {
    const delay = failures > 0 ? backoff[Math.min(failures, backoff.length) - 1] : intervalMs;
    options.onRetryScheduled?.(failures > 0 ? Date.now() + delay : null);
    periodicTimer = setTimeout(() => {
      periodicTimer = undefined;
      if (options.isVisible && !options.isVisible()) return;
      void request();
    }, delay);
  };

  const launch = (): Promise<void> => {
    // Any read that starts now also covers the events and ticks that were waiting.
    clearTimers();
    const current = (async () => {
      let outcome: ControlRefreshOutcome = "retry";
      try {
        outcome = await options.run(controller.signal);
      } catch {
        outcome = "retry";
      }
      inFlight = null;
      if (disposed) return;
      failures = outcome === "ok" ? 0 : failures + 1;
      const next = followUp;
      followUp = null;
      if (next) {
        // The follow-up starts now, so no retry time is pending any more.
        options.onRetryScheduled?.(null);
        void launch().then(next.resolve);
        return;
      }
      schedulePeriodic();
    })();
    inFlight = current;
    return current;
  };

  function request(): Promise<void> {
    if (disposed) return Promise.resolve();
    if (inFlight) {
      followUp ??= deferred();
      return followUp.promise;
    }
    return launch();
  }

  const event = () => {
    if (disposed) return;
    const now = Date.now();
    firstEventAt ??= now;
    const settled = Math.min(now + settleMs, firstEventAt + minGapMs);
    const at = Math.max(settled, lastEventReadAt + minGapMs);
    clearTimeout(eventTimer);
    eventTimer = setTimeout(
      () => {
        eventTimer = undefined;
        firstEventAt = null;
        if (options.isVisible && !options.isVisible()) return;
        lastEventReadAt = Date.now();
        void request();
      },
      Math.max(0, at - now),
    );
  };

  const dispose = () => {
    disposed = true;
    clearTimers();
    controller.abort();
    followUp?.resolve();
    followUp = null;
  };

  return { request, event, dispose };
}
