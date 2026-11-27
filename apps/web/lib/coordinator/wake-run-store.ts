import { getRun, type RunRead } from "@/lib/api/domains/coordinator-autonomy-api";

export type WakeRunEntry =
  | { status: "loading"; run: null }
  | { status: "loaded"; run: RunRead }
  | { status: "unavailable"; run: null };

type Meta = { workspaceId: string; coordinatorId: string; turnId: string; sequence: number };

const entries = new Map<string, WakeRunEntry>();
const metas = new Map<string, Meta>();
const listeners = new Set<() => void>();

export const wakeRunKey = (coordinatorId: string, turnId: string) => `${coordinatorId}:${turnId}`;

function emit(): void {
  listeners.forEach((listener) => listener());
}

export function subscribeWakeRuns(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function getWakeRun(key: string): WakeRunEntry | undefined {
  return entries.get(key);
}

export function resetWakeRunStore(): void {
  entries.clear();
  metas.clear();
  emit();
}

function read(meta: Meta, key: string): void {
  const sequence = ++meta.sequence;
  getRun(meta.workspaceId, meta.coordinatorId, meta.turnId)
    .then((run) => {
      if (sequence !== meta.sequence) return;
      entries.set(key, { status: "loaded", run });
      emit();
    })
    .catch(() => {
      if (sequence !== meta.sequence) return;
      // A failed re-read keeps what is already on screen; only a first read falls to unavailable.
      if (entries.get(key)?.status === "loaded") return;
      entries.set(key, { status: "unavailable", run: null });
      emit();
    });
}

/** Reads a turn's run once per coordinator and turn id; a known key is never re-read here. */
export function ensureWakeRun(workspaceId: string, coordinatorId: string, turnId: string): void {
  const key = wakeRunKey(coordinatorId, turnId);
  if (entries.has(key)) return;
  const meta: Meta = { workspaceId, coordinatorId, turnId, sequence: 0 };
  metas.set(key, meta);
  entries.set(key, { status: "loading", run: null });
  emit();
  read(meta, key);
}

/** Re-reads an entry that is still in flight or whose run has not finished. Settled runs are left alone. */
export function refreshOpenWakeRun(coordinatorId: string, turnId: string): void {
  const key = wakeRunKey(coordinatorId, turnId);
  const entry = entries.get(key);
  const meta = metas.get(key);
  if (!entry || !meta) return;
  const open =
    entry.status === "loading" || (entry.status === "loaded" && entry.run.outcome === null);
  if (open) read(meta, key);
}
