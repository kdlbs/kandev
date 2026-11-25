import type { SidebarView } from "./sidebar-view-types";

export type SidebarViewMove = { id: string; overId: string; after: boolean };

export function moveSidebarView(views: SidebarView[], move: SidebarViewMove): SidebarView[] | null {
  const index = views.findIndex((view) => view.id === move.id);
  const destination = views.findIndex((view) => view.id === move.overId);
  if (index < 0 || destination < 0 || index === destination) return null;
  const next = [...views];
  const [moved] = next.splice(index, 1);
  const over = next.findIndex((view) => view.id === move.overId);
  next.splice(over + (move.after ? 1 : 0), 0, moved);
  return next;
}
