import type { SidebarView, SidebarViewDraft } from "./sidebar-view-types";
import { cloneSidebarTaskRowPresentation } from "./sidebar-task-row-presentation";

export function cloneView(v: SidebarView): SidebarView {
  return {
    id: v.id,
    name: v.name,
    filters: v.filters.map((f) => ({
      ...f,
      value: Array.isArray(f.value) ? [...f.value] : f.value,
    })),
    sort: { ...v.sort, thenBy: v.sort.thenBy?.map((rule) => ({ ...rule })) },
    ...(v.sortWarningCount !== undefined ? { sortWarningCount: v.sortWarningCount } : {}),
    groupIndent: typeof v.groupIndent === "boolean" ? v.groupIndent : true,
    group: v.group,
    collapsedGroups: [...v.collapsedGroups],
    taskRow: v.taskRow ? cloneSidebarTaskRowPresentation(v.taskRow) : undefined,
    ...(v.generatedPluginGrouping
      ? { generatedPluginGrouping: { ...v.generatedPluginGrouping } }
      : {}),
  };
}

export function cloneDraft(draft: SidebarViewDraft | null): SidebarViewDraft | null {
  if (!draft) return null;
  return {
    baseViewId: draft.baseViewId,
    filters: draft.filters.map((filter) => ({
      ...filter,
      value: Array.isArray(filter.value) ? [...filter.value] : filter.value,
    })),
    sort: { ...draft.sort, thenBy: draft.sort.thenBy?.map((rule) => ({ ...rule })) },
    ...(draft.sortWarningCount !== undefined ? { sortWarningCount: draft.sortWarningCount } : {}),
    group: draft.group,
    groupIndent: typeof draft.groupIndent === "boolean" ? draft.groupIndent : true,
    ...(draft.collapsedGroups ? { collapsedGroups: [...draft.collapsedGroups] } : {}),
    taskRow: draft.taskRow ? cloneSidebarTaskRowPresentation(draft.taskRow) : undefined,
  };
}
