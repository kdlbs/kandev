import type { RunState, WorkloadKind, WorkloadRunObservation } from "@/lib/types/background-work";
import { hasReadableOutput } from "@/lib/background-work/capabilities";

/** i18n key for a run state. Callers translate it; the helper never owns copy. */
export function workloadStateLabelKey(state: RunState): string {
  switch (state) {
    case "running":
      return "task:backgroundStateRunning";
    case "waiting":
      return "task:backgroundStateWaiting";
    case "unknown":
      return "task:backgroundStateUnknown";
    case "completed":
      return "task:backgroundStateCompleted";
    case "failed":
      return "task:backgroundStateFailed";
    case "interrupted":
      return "task:backgroundStateInterrupted";
    case "ended":
      return "task:backgroundStateEnded";
    default:
      return "task:backgroundStateUnknown";
  }
}

/**
 * Badge styling for a run state. Only verified-live states use the primary
 * treatment; unknown activity stays visually distinct from running, and a
 * failure is destructive.
 */
export function workloadStateBadgeVariant(state: RunState): "default" | "destructive" | "outline" {
  if (state === "failed") return "destructive";
  if (state === "running" || state === "waiting") return "default";
  return "outline";
}

/** i18n key for a workload kind. */
export function workloadKindLabelKey(kind: WorkloadKind): string {
  switch (kind) {
    case "shell":
      return "task:backgroundKindShell";
    case "subagent":
      return "task:backgroundKindSubagent";
    case "monitor":
      return "task:backgroundKindMonitor";
    case "custom":
      return "task:backgroundKindCustom";
    default:
      return "task:backgroundKindUnknown";
  }
}

/**
 * The label to show when no output is displayed. A readable surface with
 * nothing to show (for example after a local clear) uses the live-unavailable
 * copy; an inspect-only surface names the limitation.
 */
export function workloadOutputUnavailableKey(workload: WorkloadRunObservation): string {
  if (!hasReadableOutput(workload.capabilities)) return "task:backgroundOutputUnavailable";
  return "task:liveOutputUnavailable";
}

/** Exit-status summary key, or null when the provider reported no exit code. */
export function workloadExitStatusKey(workload: WorkloadRunObservation): string | null {
  if (workload.exit_code === undefined || workload.exit_code === null) return null;
  return "task:backgroundExitStatus";
}
