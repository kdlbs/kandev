import {
  coveredTaskOverviews,
  type SidebarInventory,
} from "@/lib/state/slices/task-overview-coverage";
import type { SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";
import { sidebarSortRules } from "./sidebar-sort-chain";

export function sidebarTaskSource(
  state: SidebarInventory,
  workspaceId: string | null,
  view: SidebarView,
) {
  const rules = sidebarSortRules(view.sort);
  if (
    rules.some(
      (rule) =>
        ![
          "state",
          "updatedAt",
          "lastActivityAt",
          "createdAt",
          "title",
          "running",
          "color",
          "custom",
        ].includes(rule.key),
    ) ||
    rules.some((rule) => !["asc", "desc"].includes(rule.direction)) ||
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
