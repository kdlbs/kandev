import type { AutomationRunCreatedPayload } from "@/lib/types/automation";

type Listener = (run: AutomationRunCreatedPayload) => void;

const listeners = new Set<Listener>();

/** Announces a run recorded in the active workspace to open run pages. */
export function emitAutomationRunRecorded(run: AutomationRunCreatedPayload): void {
  for (const listener of [...listeners]) listener(run);
}

export function subscribeAutomationRunRecorded(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
