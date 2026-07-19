"use client";

/**
 * TaskHeader — shared title / identifier / state pill / assignee row.
 *
 * Pure read; no domain branching. Both the kanban shell (/t/:id) and
 * the office shell (/office/tasks/:id) render this above the body.
 *
 * Designed to be source-agnostic: callers pass plain primitives, so the
 * component works for both the kanban Task DTO (lib/types/http) and the
 * office Task DTO (app/office/tasks/[id]/types).
 */

import { Badge } from "@kandev/ui/badge";
import type { ForegroundActivity } from "@/lib/types/http";

export type TaskHeaderProps = {
  identifier?: string | null;
  title: string;
  state?: string | null;
  /**
   * Pre-resolved assignee display name. Resolution (initials, agent
   * profile name) happens at the call site against the appropriate
   * store; this component is a pure renderer.
   */
  assigneeName?: string | null;
  /** Optional pill colour for the state badge. Falls back to outline. */
  stateBadgeVariant?: "default" | "secondary" | "outline" | "destructive";
  /**
   * Task-level MOST-ACTIVE-WINS activity aggregate (§spec:task-level-indicator).
   * When set it takes precedence over the coarse workflow state in the badge, so a
   * task still doing background work never reads as a done coarse state and stays
   * distinct from a generating task.
   */
  foregroundActivity?: ForegroundActivity | null;
  /**
   * Message-derived "needs me" flags (§spec:waiting-for-input-parity). When set
   * the badge reads the waiting variant distinctly ("Permission requested" /
   * "Waiting for input") instead of the raw coarse state, matching the sidebar.
   */
  hasPendingClarification?: boolean;
  hasPendingPermission?: boolean;
};

// The badge reflects the task-level activity aggregate ABOVE the coarse workflow
// state (§spec:task-level-indicator), mirroring getTaskStateIcon: background-running
// and generating each read distinctly and never fall back to a done coarse state.
// The waiting-for-input variants (§spec:waiting-for-input-parity) sit below live
// activity but above the coarse state, with pending-permission taking precedence
// over the generic waiting/clarification reading.
function resolveBadgeLabel(
  state: string | null | undefined,
  foregroundActivity: ForegroundActivity | null | undefined,
  hasPendingClarification: boolean,
  hasPendingPermission: boolean,
): string | null {
  if (foregroundActivity === "background") return "Background running";
  if (foregroundActivity === "generating") return "Generating";
  if (hasPendingPermission) return "Permission requested";
  if (hasPendingClarification || state === "WAITING_FOR_INPUT") return "Waiting for input";
  return state ?? null;
}

export function TaskHeader({
  identifier,
  title,
  state,
  assigneeName,
  stateBadgeVariant = "outline",
  foregroundActivity,
  hasPendingClarification = false,
  hasPendingPermission = false,
}: TaskHeaderProps) {
  const badgeLabel = resolveBadgeLabel(
    state,
    foregroundActivity,
    hasPendingClarification,
    hasPendingPermission,
  );
  return (
    <div className="flex items-center gap-3 min-w-0">
      {identifier && (
        <span className="text-xs font-mono text-muted-foreground shrink-0">{identifier}</span>
      )}
      <span className="text-sm font-medium truncate flex-1">{title}</span>
      {badgeLabel && (
        <Badge variant={stateBadgeVariant} className="shrink-0">
          {badgeLabel}
        </Badge>
      )}
      {assigneeName && (
        <span className="text-xs text-muted-foreground shrink-0">{assigneeName}</span>
      )}
    </div>
  );
}
