import type { SidebarView, SidebarViewDraft } from "./sidebar-view-types";
import { cloneSidebarTaskRowPresentation } from "./sidebar-task-row-presentation";

export function sidebarViewDisclosure(view: SidebarView, draft: SidebarViewDraft | null): string[] {
  if (draft?.baseViewId === view.id && draft.group !== view.group) {
    return draft.collapsedGroups ?? [];
  }
  return view.collapsedGroups;
}

export function sidebarViewWithDraft(
  view: SidebarView,
  draft: SidebarViewDraft | null,
): SidebarView {
  if (!draft || draft.baseViewId !== view.id) return view;
  return {
    ...view,
    filters: draft.filters,
    sort: draft.sort,
    group: draft.group,
    groupIndent: draft.groupIndent,
    collapsedGroups: sidebarViewDisclosure(view, draft),
    taskRow: cloneSidebarTaskRowPresentation(draft.taskRow ?? view.taskRow),
  };
}
