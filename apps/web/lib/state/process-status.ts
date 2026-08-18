import type { ProcessInfo } from "@/lib/types/http";
import type { ProcessStatusEntry } from "@/lib/state/slices";

/**
 * Map a `ProcessInfo` HTTP response onto the store's `ProcessStatusEntry`.
 *
 * Shared so every caller that seeds the store from a start/list response
 * produces the same shape as the `session.process.status` WS handler. Three
 * copies of this had already drifted apart.
 */
export function toProcessStatusEntry(process: ProcessInfo): ProcessStatusEntry {
  return {
    processId: process.id,
    sessionId: process.session_id,
    kind: process.kind,
    scriptName: process.script_name,
    status: process.status,
    command: process.command,
    workingDir: process.working_dir,
    exitCode: process.exit_code ?? null,
    startedAt: process.started_at,
    updatedAt: process.updated_at,
  };
}

/** Statuses in which a workspace process is considered live. */
export function isProcessLive(status: string | undefined): boolean {
  return status === "running" || status === "starting";
}
