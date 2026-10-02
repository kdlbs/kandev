"use client";

import { useSyncExternalStore } from "react";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildResumeRequest } from "@/lib/services/session-launch-helpers";
import { isWebSocketRequestTimeoutError } from "@/lib/ws/request-error";

export type StallResumeEntry =
  | { phase: "sending" | "resuming" | "queued" }
  | { phase: "error"; unknown: boolean };

type Held = { entry: StallResumeEntry; seq: number };

// Module-level so a refetch or a remount of the card never resets the lock.
const held = new Map<string, Held>();
const listeners = new Set<() => void>();
let counter = 0;

function emit() {
  for (const listener of listeners) listener();
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

function isLocked(entry: StallResumeEntry | undefined): boolean {
  return entry !== undefined && entry.phase !== "error";
}

function settle(taskId: string, seq: number, entry: StallResumeEntry) {
  // A response for a superseded or dropped request is ignored.
  if (held.get(taskId)?.seq !== seq) return;
  held.set(taskId, { entry, seq });
  emit();
}

/** Starts a resume of the task's primary session; a second call while one is held sends nothing. */
export function resumeStalledTask(taskId: string, sessionId: string): void {
  if (isLocked(held.get(taskId)?.entry)) return;
  const seq = ++counter;
  held.set(taskId, { entry: { phase: "sending" }, seq });
  emit();
  launchSession(buildResumeRequest(taskId, sessionId).request)
    .then((response) => {
      if (!response.success || response.activation_disposition === "suppressed") {
        settle(taskId, seq, { phase: "error", unknown: false });
        return;
      }
      settle(taskId, seq, {
        phase: response.activation_disposition === "queued" ? "queued" : "resuming",
      });
    })
    .catch((error: unknown) => {
      settle(taskId, seq, { phase: "error", unknown: isWebSocketRequestTimeoutError(error) });
    });
}

/** Drops the held entries of every task not in `liveTaskIds` (the stall left the list). */
export function pruneStallResumes(liveTaskIds: ReadonlySet<string>): void {
  let changed = false;
  for (const taskId of [...held.keys()]) {
    if (!liveTaskIds.has(taskId)) {
      held.delete(taskId);
      changed = true;
    }
  }
  if (changed) emit();
}

export function resetStallResumesForTest(): void {
  held.clear();
  emit();
}

export function useStallResume(taskId: string): StallResumeEntry | undefined {
  return useSyncExternalStore(
    subscribe,
    () => held.get(taskId)?.entry,
    () => undefined,
  );
}
