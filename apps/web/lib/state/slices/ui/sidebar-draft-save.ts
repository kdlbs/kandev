import type { SidebarDraftSaveApi } from "@/lib/types/http-user-settings";
import type { SidebarSliceState } from "./sidebar-view-types";
import { sidebarViewWithDraft } from "./sidebar-view-disclosure";

export function prepareSidebarDraftSave(state: SidebarSliceState, targetId: string, name?: string) {
  const draft = state.draft;
  const base = state.views.find((view) => view.id === draft?.baseViewId);
  if (!draft || !base) return null;
  const copy = targetId !== base.id;
  const effective = sidebarViewWithDraft(base, draft);
  const view = {
    ...effective,
    id: targetId,
    name: copy ? name! : base.name,
    sortWarningCount: draft.sortWarningCount,
    collapsedGroups: [...effective.collapsedGroups],
    generatedPluginGrouping: copy ? undefined : base.generatedPluginGrouping,
  };
  const command: SidebarDraftSaveApi = {
    base_view_id: base.id,
    expected_group: draft.group,
    target_view_id: targetId,
    ...(copy ? { name } : {}),
  };
  return { view, command };
}
