/**
 * Reference-free, bounded staging/fallback buffer for browser diagnostics.
 * Console producers never await persistence or network work.
 */

export type LogLevel = "debug" | "info" | "warn" | "error";

export interface LogEntry {
  timestamp: string;
  level: LogLevel;
  source: string;
  message: string;
  args?: unknown[];
  stack?: string;
  url?: string;
  task_id?: string;
  identity_scope?: string;
}

export type BufferLoss = {
  capacity: number;
  entry_too_large: number;
};

export const DEFAULT_CAPACITY = 500;
export const DEFAULT_BYTE_CAPACITY = 2 * 1024 * 1024;
export const MAX_ENTRY_BYTES = 64 * 1024;

type StoredEntry = { entry: LogEntry; bytes: number };

export class RingBuffer {
  private entries: StoredEntry[] = [];
  private bytes = 0;
  private readonly loss: BufferLoss = { capacity: 0, entry_too_large: 0 };

  constructor(
    private readonly capacity: number = DEFAULT_CAPACITY,
    private readonly byteCapacity: number = DEFAULT_BYTE_CAPACITY,
  ) {}

  push(entry: LogEntry): boolean {
    const detached = cloneEntry(entry);
    const bytes = encodedBytes(detached);
    if (bytes > MAX_ENTRY_BYTES) {
      this.loss.entry_too_large += 1;
      return false;
    }
    while (
      this.entries.length >= Math.max(1, this.capacity) ||
      this.bytes + bytes > this.byteCapacity
    ) {
      const eviction = this.evictionIndex(entry.level);
      if (eviction < 0) {
        this.loss.capacity += 1;
        return false;
      }
      const [removed] = this.entries.splice(eviction, 1);
      this.bytes -= removed.bytes;
      this.loss.capacity += 1;
    }
    this.entries.push({ entry: detached, bytes });
    this.bytes += bytes;
    return true;
  }

  snapshot(identityScope?: string): LogEntry[] {
    return this.entries
      .filter(({ entry }) => identityScope === undefined || entry.identity_scope === identityScope)
      .map(({ entry }) => cloneEntry(entry));
  }

  clear(): void {
    this.entries = [];
    this.bytes = 0;
  }

  size(): number {
    return this.entries.length;
  }

  byteSize(): number {
    return this.bytes;
  }

  statistics(): BufferLoss {
    return { ...this.loss };
  }

  private evictionIndex(incoming: LogLevel): number {
    const lowPriority = this.entries.findIndex(
      ({ entry }) => entry.level === "debug" || entry.level === "info",
    );
    if (lowPriority >= 0) return lowPriority;
    return incoming === "warn" || incoming === "error" ? 0 : -1;
  }
}

let defaultBuffer: RingBuffer | null = null;

export function getLogBuffer(): RingBuffer {
  defaultBuffer ??= new RingBuffer();
  return defaultBuffer;
}

export function snapshotLogs(identityScope?: string): LogEntry[] {
  return getLogBuffer().snapshot(identityScope);
}

export function clearLogs(): void {
  getLogBuffer().clear();
}

export function encodedBytes(entry: LogEntry): number {
  return new TextEncoder().encode(JSON.stringify(entry)).byteLength;
}

function cloneEntry(entry: LogEntry): LogEntry {
  return {
    ...entry,
    args: entry.args ? entry.args.map(clonePreview) : undefined,
  };
}

function clonePreview(value: unknown): unknown {
  if (!value || typeof value !== "object") return value;
  if (Array.isArray(value)) return value.map(clonePreview);
  return { ...(value as Record<string, unknown>) };
}

/** Exposed for tests. */
export function _resetForTesting(): void {
  defaultBuffer = new RingBuffer();
}
