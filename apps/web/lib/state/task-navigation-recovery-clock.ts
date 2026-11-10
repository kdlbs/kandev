export const TASK_NAVIGATION_VISIBLE_BUDGET_MS = 30_000;

// i18n-exempt: internal cancellation sentinel; never rendered to users.
const TASK_NAVIGATION_CLOCK_ABORT_MESSAGE = "Task navigation recovery clock disposed";

export class TaskNavigationCycleDeadlineError extends Error {
  constructor() {
    super("Task navigation recovery cycle deadline exceeded");
    this.name = "TaskNavigationCycleDeadlineError";
  }
}

type RecoveryClockVisibility = {
  isVisible: () => boolean;
  subscribe: (listener: () => void) => () => void;
};

type RecoveryClockOptions = {
  budgetMs?: number;
  now?: () => number;
  onDeadline?: () => void;
  visibility?: RecoveryClockVisibility;
};

type PendingWait = {
  remainingMs: number;
  timer: ReturnType<typeof setTimeout> | null;
  visibleSince: number | null;
  resolve: () => void;
  reject: (error: unknown) => void;
  signal: AbortSignal;
  onAbort: () => void;
};

function monotonicNow(): number {
  return typeof performance !== "undefined" ? performance.now() : Date.now();
}

function documentVisibility(): RecoveryClockVisibility {
  if (typeof document === "undefined" || typeof document.addEventListener !== "function") {
    return { isVisible: () => true, subscribe: () => () => {} };
  }
  return {
    isVisible: () => document.visibilityState !== "hidden",
    subscribe: (listener) => {
      document.addEventListener("visibilitychange", listener);
      return () => document.removeEventListener("visibilitychange", listener);
    },
  };
}

function clockAbortError(): Error {
  const error = new Error(TASK_NAVIGATION_CLOCK_ABORT_MESSAGE);
  error.name = "AbortError";
  return error;
}

/**
 * Visible-time budget for one task-navigation recovery cycle. Hiding the tab
 * pauses delayed retries and the remaining budget; an in-flight read keeps its
 * own request deadline.
 */
export class TaskNavigationRecoveryClock {
  private readonly now: () => number;
  private readonly visibility: RecoveryClockVisibility;
  private readonly onDeadline: () => void;
  private remainingMs: number;
  private expiredFlag = false;
  private started = false;
  private disposed = false;
  private visibleSince: number | null = null;
  private deadlineTimer: ReturnType<typeof setTimeout> | null = null;
  private waitState: PendingWait | null = null;
  private unsubscribe: (() => void) | null = null;

  constructor(options: RecoveryClockOptions = {}) {
    this.now = options.now ?? monotonicNow;
    this.visibility = options.visibility ?? documentVisibility();
    this.onDeadline = options.onDeadline ?? (() => {});
    this.remainingMs = options.budgetMs ?? TASK_NAVIGATION_VISIBLE_BUDGET_MS;
  }

  start(): void {
    if (this.started || this.disposed) return;
    this.started = true;
    this.unsubscribe = this.visibility.subscribe(this.handleVisibilityChange);
    if (this.visibility.isVisible()) this.resume();
  }

  get expired(): boolean {
    return this.expiredFlag;
  }

  remainingVisibleMs(): number {
    if (this.expiredFlag) return 0;
    if (this.visibleSince === null) return this.remainingMs;
    return Math.max(0, this.remainingMs - (this.now() - this.visibleSince));
  }

  wait(delayMs: number, signal: AbortSignal): Promise<void> {
    if (this.expiredFlag || this.disposed) {
      return Promise.reject(new TaskNavigationCycleDeadlineError());
    }
    if (signal.aborted) return Promise.reject(clockAbortError());
    if (this.waitState) this.cancelWait(this.waitState, clockAbortError());
    return new Promise<void>((resolve, reject) => {
      const state: PendingWait = {
        remainingMs: Math.max(0, delayMs),
        timer: null,
        visibleSince: null,
        resolve,
        reject,
        signal,
        onAbort: () => {
          if (this.waitState === state) this.waitState = null;
          this.clearWaitResources(state);
          reject(clockAbortError());
        },
      };
      this.waitState = state;
      signal.addEventListener("abort", state.onAbort, { once: true });
      this.scheduleWait();
    });
  }

  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    this.clearDeadlineTimer();
    this.unsubscribe?.();
    this.unsubscribe = null;
    if (this.waitState) this.cancelWait(this.waitState, clockAbortError());
  }

  private readonly handleVisibilityChange = () => {
    if (!this.started || this.disposed) return;
    if (this.visibility.isVisible()) this.resume();
    else this.pause();
  };

  private resume() {
    if (this.visibleSince !== null || this.expiredFlag || this.disposed) return;
    this.visibleSince = this.now();
    this.scheduleDeadline();
    this.scheduleWait();
  }

  private pause() {
    if (this.visibleSince === null) return;
    this.consumeVisibleElapsed();
    this.visibleSince = null;
    this.clearDeadlineTimer();
    if (this.waitState?.timer) {
      clearTimeout(this.waitState.timer);
      this.waitState.timer = null;
    }
    if (this.remainingMs <= 0) this.expire();
  }

  private consumeVisibleElapsed() {
    const now = this.now();
    if (this.visibleSince !== null) {
      this.remainingMs = Math.max(0, this.remainingMs - Math.max(0, now - this.visibleSince));
    }
    const wait = this.waitState;
    if (wait && wait.visibleSince !== null) {
      wait.remainingMs = Math.max(0, wait.remainingMs - Math.max(0, now - wait.visibleSince));
      wait.visibleSince = null;
    }
  }

  private scheduleDeadline() {
    this.clearDeadlineTimer();
    if (this.expiredFlag || this.visibleSince === null) return;
    if (this.remainingMs <= 0) {
      this.expire();
      return;
    }
    this.deadlineTimer = setTimeout(() => {
      this.deadlineTimer = null;
      this.expire();
    }, this.remainingMs);
  }

  private clearDeadlineTimer() {
    if (this.deadlineTimer !== null) {
      clearTimeout(this.deadlineTimer);
      this.deadlineTimer = null;
    }
  }

  private scheduleWait() {
    const state = this.waitState;
    if (!state || this.visibleSince === null || this.expiredFlag || this.disposed) return;
    if (state.timer !== null) {
      clearTimeout(state.timer);
      state.timer = null;
    }
    if (state.remainingMs <= 0) {
      this.resolveWait(state);
      return;
    }
    const budget = this.remainingVisibleMs();
    if (budget <= 0) {
      this.expire();
      return;
    }
    const scheduledAt = this.now();
    state.visibleSince = scheduledAt;
    state.timer = setTimeout(
      () => {
        state.timer = null;
        state.visibleSince = null;
        if (this.waitState !== state || this.expiredFlag || this.disposed) return;
        state.remainingMs = Math.max(0, state.remainingMs - (this.now() - scheduledAt));
        if (state.remainingMs <= 0) {
          this.resolveWait(state);
          return;
        }
        if (this.remainingVisibleMs() <= 0) {
          this.expire();
          return;
        }
        this.scheduleWait();
      },
      Math.min(state.remainingMs, budget),
    );
  }

  private resolveWait(state: PendingWait) {
    if (this.waitState === state) this.waitState = null;
    this.clearWaitResources(state);
    state.resolve();
  }

  private cancelWait(state: PendingWait, error: unknown) {
    if (this.waitState === state) this.waitState = null;
    this.clearWaitResources(state);
    state.reject(error);
  }

  private clearWaitResources(state: PendingWait) {
    if (state.timer !== null) {
      clearTimeout(state.timer);
      state.timer = null;
    }
    state.visibleSince = null;
    state.signal.removeEventListener("abort", state.onAbort);
  }

  private expire() {
    if (this.expiredFlag || this.disposed) return;
    this.expiredFlag = true;
    this.remainingMs = 0;
    this.clearDeadlineTimer();
    if (this.waitState) this.cancelWait(this.waitState, new TaskNavigationCycleDeadlineError());
    this.onDeadline();
  }
}
