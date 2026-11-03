import {
  MAX_EVENT_REQUEST_BYTES,
  MAX_EVENTS_PER_REQUEST,
  MAX_OUTBOX_BYTES,
  MAX_OUTBOX_OBSERVATIONS,
  RETRY_INITIAL_MS,
  RETRY_MAX_MS,
  encodeObservationBytes,
  type EventResponse,
  type Observation,
} from "../protocol.js";

export interface OutboxOptions {
  readonly send: (events: readonly Observation[]) => Promise<EventResponse>;
  readonly signal: AbortSignal;
  readonly log: (event: string, fields?: Record<string, unknown>) => void;
  /** Test seam: overrides Date.now for deterministic backoff assertions. */
  readonly now?: () => number;
  /** Test seam: overrides the abortable timer used by retry backoff. */
  readonly sleep?: (ms: number, signal: AbortSignal) => Promise<void>;
  /** Test seam: overrides jitter randomness. */
  readonly random?: () => number;
}

interface QueueItem {
  readonly observation: Observation;
  readonly bytes: number;
  attempts: number;
}

/**
 * Bounded sender outbox. A terminal fact leaves the queue only after the host
 * accepts it. A failed batch applies one awaited, abortable backoff to the whole
 * queue, so a fast 503 or network failure cannot burn every retry before the
 * first delay elapses and a new enqueue cannot bypass the failed batch's delay.
 */
export class Outbox {
  private readonly send: OutboxOptions["send"];
  private readonly log: OutboxOptions["log"];
  private readonly now: () => number;
  private readonly sleep: (ms: number, signal: AbortSignal) => Promise<void>;
  private readonly random: () => number;
  private readonly abort = new AbortController();
  private readonly queue: QueueItem[] = [];
  private bytes = 0;
  private wake: (() => void) | undefined;
  private closed = false;
  private readonly done: Promise<void>;
  private retryAt = 0;
  private gapReported = false;

  constructor(options: OutboxOptions) {
    this.send = options.send;
    this.log = options.log;
    this.now = options.now ?? Date.now;
    this.sleep = options.sleep ?? abortableSleep;
    this.random = options.random ?? Math.random;
    if (options.signal.aborted) {
      this.closed = true;
      this.abort.abort();
    } else {
      options.signal.addEventListener(
        "abort",
        () => {
          this.closed = true;
          this.abort.abort();
          this.notify();
        },
        { once: true },
      );
    }
    this.done = this.run();
  }

  enqueue(observation: Observation): void {
    if (this.closed) return;
    const bytes = encodeObservationBytes(observation);
    this.queue.push({ observation, bytes, attempts: 0 });
    this.bytes += bytes;
    this.trim();
    this.notify();
  }

  async close(): Promise<void> {
    if (!this.closed) {
      this.closed = true;
      this.abort.abort();
      this.notify();
    }
    await this.done;
  }

  private trim(): void {
    while (this.queue.length > MAX_OUTBOX_OBSERVATIONS || this.bytes > MAX_OUTBOX_BYTES) {
      const dropped = this.queue.shift();
      if (!dropped) break;
      this.bytes -= dropped.bytes;
      if (!this.gapReported) {
        this.gapReported = true;
        this.log("outbox_gap", { dropped: dropped.observation.eventId });
      }
    }
    if (this.queue.length === 0) this.retryAt = 0;
  }

  private notify(): void {
    const wake = this.wake;
    this.wake = undefined;
    wake?.();
  }

  private async run(): Promise<void> {
    while (!this.closed) {
      if (this.queue.length === 0) {
        await this.waitForWork();
        continue;
      }
      const delay = this.retryAt - this.now();
      if (delay > 0) {
        await this.sleep(delay, this.abort.signal);
        continue;
      }
      const batch = this.takeBatch();
      if (batch.length === 0) continue;
      try {
        const response = await this.send(batch.map((item) => item.observation));
        const retried = this.accept(batch, response);
        if (retried.length === 0) {
          this.retryAt = 0;
        } else {
          this.scheduleBackoff(retried);
        }
      } catch {
        if (this.closed) return;
        this.retryLater(batch);
      }
    }
  }

  private waitForWork(): Promise<void> {
    if (this.abort.signal.aborted) return Promise.resolve();
    return new Promise((resolve) => {
      const onAbort = () => {
        this.wake = undefined;
        resolve();
      };
      this.abort.signal.addEventListener("abort", onAbort, { once: true });
      this.wake = () => {
        this.abort.signal.removeEventListener("abort", onAbort);
        resolve();
      };
    });
  }

  private takeBatch(): QueueItem[] {
    const batch: QueueItem[] = [];
    let bytes = 0;
    for (let index = 0; index < this.queue.length; index += 1) {
      const item = this.queue[index];
      if (!item) continue;
      if (
        batch.length > 0 &&
        (batch.length >= MAX_EVENTS_PER_REQUEST ||
          bytes + item.bytes > MAX_EVENT_REQUEST_BYTES - 4096)
      ) {
        break;
      }
      if (item.bytes > MAX_EVENT_REQUEST_BYTES - 4096) {
        this.remove(item);
        index -= 1;
        this.log("outbox_gap", { dropped: item.observation.eventId, reason: "too_large" });
        continue;
      }
      batch.push(item);
      bytes += item.bytes;
    }
    return batch;
  }

  private accept(batch: QueueItem[], response: EventResponse): QueueItem[] {
    const accepted = new Set(response.accepted);
    const rejected = new Set(response.rejected.map((item) => item.eventId));
    const retried: QueueItem[] = [];
    for (const item of batch) {
      if (!accepted.has(item.observation.eventId) && !rejected.has(item.observation.eventId)) {
        if (this.retryItem(item)) retried.push(item);
        continue;
      }
      this.remove(item);
      if (rejected.has(item.observation.eventId)) {
        this.log("observation_rejected", { eventId: item.observation.eventId });
      }
    }
    if (response.rejected.length === 0) {
      this.gapReported = false;
    }
    return retried;
  }

  private retryLater(batch: QueueItem[]): void {
    const retried = batch.filter((item) => this.retryItem(item));
    if (retried.length === 0) return;
    this.scheduleBackoff(retried);
  }

  private scheduleBackoff(items: readonly QueueItem[]): void {
    const attempts = Math.max(...items.map((item) => item.attempts), 1);
    const delay = Math.min(RETRY_MAX_MS, RETRY_INITIAL_MS * 2 ** (attempts - 1));
    const jitter = Math.floor(delay * 0.25 * this.random());
    this.retryAt = this.now() + delay + jitter;
  }

  /** Returns false when the item exhausted its bounded retry budget. */
  private retryItem(item: QueueItem): boolean {
    item.attempts += 1;
    if (item.attempts > 20) {
      this.remove(item);
      this.log("observation_retry_exhausted", { eventId: item.observation.eventId });
      return false;
    }
    return true;
  }

  private remove(item: QueueItem): void {
    const index = this.queue.indexOf(item);
    if (index < 0) return;
    this.queue.splice(index, 1);
    this.bytes -= item.bytes;
  }
}

function abortableSleep(ms: number, signal: AbortSignal): Promise<void> {
  if (signal.aborted) return Promise.resolve();
  return new Promise((resolve) => {
    const timer = setTimeout(finish, ms);
    const onAbort = () => finish();
    function finish() {
      clearTimeout(timer);
      signal.removeEventListener("abort", onAbort);
      resolve();
    }
    signal.addEventListener("abort", onAbort, { once: true });
  });
}
