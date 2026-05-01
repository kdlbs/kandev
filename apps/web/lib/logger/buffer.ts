/**
 * In-memory ring buffer of recent frontend log entries.
 *
 * Used to capture browser-side `console.*` calls and unhandled errors so they
 * can be attached to user-submitted Improve Kandev reports without unbounded
 * memory growth.
 */

export type LogLevel = "debug" | "info" | "warn" | "error";

export interface LogEntry {
  timestamp: string; // ISO8601
  level: LogLevel;
  source: string; // "console" | "window.onerror" | "unhandledrejection"
  message: string;
  args?: unknown[]; // serialized best-effort
  stack?: string; // populated when the entry originated from an Error
}

export const DEFAULT_CAPACITY = 500;

class RingBuffer {
  private entries: LogEntry[] = [];
  private capacity: number;

  constructor(capacity: number = DEFAULT_CAPACITY) {
    this.capacity = Math.max(1, capacity);
  }

  push(entry: LogEntry): void {
    if (this.entries.length >= this.capacity) {
      this.entries.shift();
    }
    this.entries.push(entry);
  }

  snapshot(): LogEntry[] {
    // Shallow-copy each entry and the args slice so callers cannot mutate
    // buffered state (e.g. when the snapshot is later JSON-serialized in place).
    return this.entries.map((e) => ({
      ...e,
      args: e.args ? [...e.args] : undefined,
    }));
  }

  clear(): void {
    this.entries = [];
  }

  size(): number {
    return this.entries.length;
  }
}

let defaultBuffer: RingBuffer | null = null;

export function getLogBuffer(): RingBuffer {
  if (!defaultBuffer) {
    defaultBuffer = new RingBuffer();
  }
  return defaultBuffer;
}

export function snapshotLogs(): LogEntry[] {
  return getLogBuffer().snapshot();
}

export function clearLogs(): void {
  getLogBuffer().clear();
}

/** Exposed for tests. */
export function _resetForTesting(): void {
  defaultBuffer = new RingBuffer();
}
