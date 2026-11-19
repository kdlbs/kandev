import type { SidebarView } from "./sidebar-view-types";
import { toApiSidebarView } from "./sidebar-view-wire";

export function sidebarViewDelta(before: SidebarView[], after: SidebarView[]) {
  const previous = new Map(before.map((view) => [view.id, JSON.stringify(toApiSidebarView(view))]));
  const next = new Set(after.map((view) => view.id));
  const upserts = after
    .map(toApiSidebarView)
    .filter((view) => previous.get(view.id) !== JSON.stringify(view));
  const deleted = before.filter((view) => !next.has(view.id)).map((view) => view.id);
  const order = after.map((view) => view.id);
  const reordered =
    before.length !== after.length || before.some((view, index) => view.id !== order[index]);
  return {
    ...(upserts.length ? { views_upsert: upserts } : {}),
    ...(deleted.length ? { view_ids_delete: deleted } : {}),
    ...(reordered ? { view_order: order } : {}),
  };
}
