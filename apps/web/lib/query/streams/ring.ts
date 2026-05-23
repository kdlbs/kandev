import { useSyncExternalStore } from "react";

/**
 * Module-level ring-buffer registry for high-frequency streams.
 *
 * Shell output, process output, and terminal output MUST NOT flow through
 * the TanStack Query cache — TQ's per-chunk notify + structural sharing
 * is a performance cliff at thousands of chunks/sec.
 *
 * Instead, each session/process stream gets its own fixed-size ring buffer.
 * Components subscribe via useSyncExternalStore through useShellRingBuffer().
 *
 * Performance contract:
 * - Snapshot returns the same array reference until a write happens (React sees no change).
 * - A per-key version counter is bumped on every write; snapshot is re-computed only then.
 * - Listeners are notified synchronously after each append.
 */

const DEFAULT_CAPACITY = 10_000;

interface RingBufferEntry {
  lines: string[];
  head: number; // next write position (wraps around)
  size: number; // number of live entries (≤ capacity)
  capacity: number;
  version: number;
  snapshot: string[] | null; // cached snapshot; null = stale
  listeners: Set<() => void>;
}

const registry = new Map<string, RingBufferEntry>();

function getOrCreate(key: string, capacity: number): RingBufferEntry {
  let entry = registry.get(key);
  if (!entry) {
    entry = {
      lines: new Array<string>(capacity),
      head: 0,
      size: 0,
      capacity,
      version: 0,
      snapshot: null,
      listeners: new Set(),
    };
    registry.set(key, entry);
  }
  return entry;
}

function notifyListeners(entry: RingBufferEntry): void {
  for (const listener of entry.listeners) {
    listener();
  }
}

function buildSnapshot(entry: RingBufferEntry): string[] {
  if (entry.snapshot !== null) {
    return entry.snapshot;
  }
  const result: string[] = new Array(entry.size);
  const start = entry.size < entry.capacity ? 0 : entry.head;
  for (let i = 0; i < entry.size; i++) {
    result[i] = entry.lines[(start + i) % entry.capacity];
  }
  entry.snapshot = result;
  return result;
}

/**
 * Returns (or lazily creates) the ring buffer for the given key.
 * Capacity is only applied when a new buffer is created; subsequent
 * calls with a different capacity do not resize an existing buffer.
 */
export function getRingBuffer(key: string, capacity = DEFAULT_CAPACITY): RingBufferEntry {
  return getOrCreate(key, capacity);
}

/**
 * Appends a line to the ring buffer identified by key.
 * If the buffer is full the oldest line is overwritten.
 * Notifies all subscribed listeners after writing.
 */
export function appendToRing(key: string, line: string, capacity = DEFAULT_CAPACITY): void {
  const entry = getOrCreate(key, capacity);
  entry.lines[entry.head] = line;
  entry.head = (entry.head + 1) % entry.capacity;
  if (entry.size < entry.capacity) {
    entry.size++;
  }
  entry.version++;
  entry.snapshot = null; // invalidate cached snapshot
  notifyListeners(entry);
}

/**
 * Clears all lines from the ring buffer for the given key.
 * Notifies subscribers so they can re-render with an empty list.
 * Call this during session teardown.
 */
export function clearRing(key: string): void {
  const entry = registry.get(key);
  if (!entry) return;
  entry.head = 0;
  entry.size = 0;
  entry.version++;
  entry.snapshot = null;
  notifyListeners(entry);
}

/**
 * React hook: subscribes to a ring buffer and returns a snapshot of its lines.
 *
 * Uses useSyncExternalStore for concurrent-safe external subscriptions.
 * The snapshot array reference is stable until a write happens, so React
 * will skip re-renders when nothing changed.
 *
 * @param key - The ring buffer key (typically a sessionId or similar).
 * @param capacity - Buffer capacity; only applied when the buffer is first created.
 */
export function useShellRingBuffer(key: string, capacity = DEFAULT_CAPACITY): string[] {
  const entry = getOrCreate(key, capacity);

  return useSyncExternalStore(
    (listener) => {
      entry.listeners.add(listener);
      return () => {
        entry.listeners.delete(listener);
      };
    },
    () => buildSnapshot(entry),
    () => [], // server snapshot — streams are client-only
  );
}
