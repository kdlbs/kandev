import type { SidebarView, SidebarViewDraft } from "./sidebar-view-types";

export function sidebarViewDisclosure(view: SidebarView, draft: SidebarViewDraft | null): string[] {
  if (draft?.baseViewId === view.id && draft.group !== view.group) {
    return draft.collapsedGroups ?? [];
  }
  return view.collapsedGroups;
}
