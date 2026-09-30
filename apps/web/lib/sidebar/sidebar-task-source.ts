import {
  coveredTaskOverviews,
  type SidebarInventory,
} from "@/lib/state/slices/task-overview-coverage";
import type { SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";

export function sidebarTaskSource(
  state: SidebarInventory,
  workspaceId: string | null,
  view: SidebarView,
) {
  if (
    !["state", "updatedAt", "lastActivityAt", "createdAt", "title", "custom"].includes(
      view.sort.key,
    ) ||
    !["asc", "desc"].includes(view.sort.direction) ||
    !["none", "repository", "workflow", "workflowStep", "executorType", "state"].includes(
      view.group,
    )
  )
    return null;
  const dimensions = new Set([
    "archived",
    "state",
    "workflow",
    "workflowStep",
    "executorType",
    "repository",
    "hasDiff",
    "hasPR",
    "isPRReview",
    "isIssueWatch",
    "titleMatch",
  ]);
  const operators = new Set(["is", "is_not", "in", "not_in", "matches", "not_matches"]);
  if (view.filters.some((filter) => !dimensions.has(filter.dimension) || !operators.has(filter.op)))
    return null;
  return coveredTaskOverviews(state, workspaceId, view);
}
