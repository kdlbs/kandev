import { ApiError } from "@/lib/api/client";

const MIN_START_INTERVAL_MS = 1_000;
const RETRY_DELAYS_MS = [2_000, 5_000, 15_000, 30_000] as const;
const MAX_TIMEOUT_MS = 2_147_483_647;

function retryDelayMs(error: unknown, failureCount: number): number | null {
  const isTemporaryApiFailure =
    error instanceof ApiError && [429, 502, 503, 504].includes(error.status);
  const isNetworkFailure = error instanceof TypeError;
  if (!isTemporaryApiFailure && !isNetworkFailure) return null;

  const baseDelay = RETRY_DELAYS_MS[Math.min(failureCount, RETRY_DELAYS_MS.length - 1)];
  const retryAfterSeconds = error instanceof ApiError ? error.retryAfterSeconds : undefined;
  const retryAfterMs =
    retryAfterSeconds && Number.isFinite(retryAfterSeconds) && retryAfterSeconds > 0
      ? Math.min(retryAfterSeconds * 1_000, MAX_TIMEOUT_MS)
      : 0;
  return Math.max(baseDelay, retryAfterMs);
}

function isCancellation(error: unknown): boolean {
  return error instanceof DOMException && error.name === "AbortError";
}

/** Coordinates all inbox triggers for one workspace and authentication scope. */
export class NeedsYouInboxRefreshCoordinator {
  private controller: AbortController | null = null;
  private inFlight: Promise<void> | null = null;
  private pendingTimer: number | undefined;
  private pendingPromise: Promise<void> | null = null;
  private resolvePending: (() => void) | null = null;
  private dirty = false;
  private disposed = false;
  private suspended = false;
  private failureCount = 0;
  private cooldownUntil = 0;
  private lastStartedAt = -Infinity;

  constructor(
    readonly scopeKey: string,
    private readonly read: (signal: AbortSignal) => Promise<void>,
    private readonly onError: (error: unknown) => void,
  ) {}

  refresh(manual = false): Promise<void> {
    if (this.disposed) return Promise.resolve();
    if (manual && this.suspended) {
      this.suspended = false;
      this.failureCount = 0;
      this.cooldownUntil = 0;
    }
    if (this.suspended) return Promise.resolve();

    this.dirty = true;
    if (this.inFlight) return this.inFlight;
    return this.schedule();
  }

  dispose() {
    if (this.disposed) return;
    this.disposed = true;
    this.controller?.abort();
    this.controller = null;
    this.dirty = false;
    if (this.pendingTimer !== undefined) {
      window.clearTimeout(this.pendingTimer);
      this.pendingTimer = undefined;
    }
    this.resolvePending?.();
    this.resolvePending = null;
    this.pendingPromise = null;
  }

  private schedule(): Promise<void> {
    if (this.disposed || this.suspended || !this.dirty) return Promise.resolve();
    const startAt = Math.max(this.lastStartedAt + MIN_START_INTERVAL_MS, this.cooldownUntil);
    const delay = startAt - Date.now();
    if (delay <= 0) return this.start();

    if (this.pendingPromise) return this.pendingPromise;
    this.pendingPromise = new Promise<void>((resolve) => {
      this.resolvePending = resolve;
    });
    this.pendingTimer = window.setTimeout(
      () => {
        this.pendingTimer = undefined;
        const resolve = this.resolvePending;
        this.resolvePending = null;
        this.pendingPromise = null;
        if (resolve) void this.schedule().then(resolve);
      },
      Math.min(delay, MAX_TIMEOUT_MS),
    );
    return this.pendingPromise;
  }

  private start(): Promise<void> {
    this.dirty = false;
    this.lastStartedAt = Date.now();
    const controller = new AbortController();
    this.controller = controller;
    const operation = Promise.resolve()
      .then(() => this.read(controller.signal))
      .then(() => {
        if (this.disposed || controller.signal.aborted) return;
        this.failureCount = 0;
        this.cooldownUntil = 0;
      })
      .catch((error: unknown) => {
        if (this.disposed || controller.signal.aborted || isCancellation(error)) return;
        this.onError(error);
        const delay = retryDelayMs(error, this.failureCount);
        if (delay === null) {
          this.suspended = true;
          this.dirty = false;
          return;
        }
        this.failureCount += 1;
        this.cooldownUntil = Date.now() + delay;
      })
      .finally(() => {
        if (this.controller === controller) this.controller = null;
        if (this.inFlight === operation) this.inFlight = null;
        if (!this.disposed && !this.suspended && this.dirty) void this.schedule();
      });
    this.inFlight = operation;
    return operation;
  }
}
