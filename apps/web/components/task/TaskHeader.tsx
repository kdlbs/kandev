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
};

export function TaskHeader({
  identifier,
  title,
  state,
  assigneeName,
  stateBadgeVariant = "outline",
}: TaskHeaderProps) {
  return (
    <div className="flex items-center gap-3 min-w-0">
      {identifier && (
        <span className="text-xs font-mono text-muted-foreground shrink-0">{identifier}</span>
      )}
      <span className="text-sm font-medium truncate flex-1">{title}</span>
      {state && (
        <Badge variant={stateBadgeVariant} className="shrink-0">
          {state}
        </Badge>
      )}
      {assigneeName && (
        <span className="text-xs text-muted-foreground shrink-0">{assigneeName}</span>
      )}
    </div>
  );
}
