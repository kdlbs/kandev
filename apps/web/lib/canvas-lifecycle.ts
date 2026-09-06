import { useSyncExternalStore } from "react";
import type { CanvasLifecyclePayload } from "@/lib/types/backend";

type Listener = () => void;

export type CanvasLifecycleAction =
  | "canvas.created"
  | "canvas.release.activated"
  | "canvas.release.permission_required"
  | "canvas.promoted"
  | "canvas.archived"
  | "canvas.restored"
  | "canvas.removed";

export type CanvasLifecycleHint = {
  revision: number;
  action: CanvasLifecycleAction;
  payload: CanvasLifecyclePayload;
};

const MAX_LIFECYCLE_HINTS = 32;

let revision = 0;
const listeners = new Set<Listener>();
const hints: CanvasLifecycleHint[] = [];

/**
 * Canvas metadata is intentionally fetched from the authoritative HTTP API.
 * This small external store only tells visible projections that a committed
 * lifecycle event arrived over WebSocket and that they must refetch.
 */
export function recordCanvasLifecycle(
  action: CanvasLifecycleAction,
  payload: CanvasLifecyclePayload,
): void {
  revision += 1;
  hints.push({ revision, action, payload });
  if (hints.length > MAX_LIFECYCLE_HINTS) hints.shift();
  listeners.forEach((listener) => listener());
}

/** Notify HTTP-backed projections when no identity hint is available. */
export function invalidateCanvasLifecycle(): void {
  revision += 1;
  listeners.forEach((listener) => listener());
}

export function subscribeCanvasLifecycle(listener: Listener): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function getCanvasLifecycleRevision(): number {
  return revision;
}

/** Return the bounded set of recent lifecycle identity hints. */
export function getCanvasLifecycleHints(): readonly CanvasLifecycleHint[] {
  return hints;
}

export function useCanvasLifecycleRevision(): number {
  return useSyncExternalStore(
    subscribeCanvasLifecycle,
    getCanvasLifecycleRevision,
    getCanvasLifecycleRevision,
  );
}
