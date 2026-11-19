import type { SidebarView, SidebarViewDraft } from "./sidebar-view-types";
import { toApiSidebarView } from "./sidebar-view-wire";
import { cloneDraft, cloneView } from "./sidebar-view-clone";

export type SidebarSnapshot = {
  views: SidebarView[];
  activeViewId: string;
  draft: SidebarViewDraft | null;
  serverRevision?: number | null;
};

function mergeViewEdit(current: SidebarView, before: SidebarView, after: SidebarView) {
  const keys = new Set([...Object.keys(before), ...Object.keys(after)] as (keyof SidebarView)[]);
  const fields = [...keys].filter(
    (key) =>
      key !== "id" &&
      key !== "collapsedGroups" &&
      JSON.stringify(before[key]) !== JSON.stringify(after[key]),
  );
  if (
    fields.length === 0 &&
    before.collapsedGroups.length === after.collapsedGroups.length &&
    before.collapsedGroups.every((key, index) => key === after.collapsedGroups[index])
  )
    return current;
  const next = Object.assign(
    cloneView(current),
    Object.fromEntries(fields.map((key) => [key, after[key]])),
  );
  if (before.group !== after.group) next.collapsedGroups = [...after.collapsedGroups];
  else if (current.group === before.group) {
    const old = new Set(before.collapsedGroups);
    const wanted = new Set(after.collapsedGroups);
    const kept = current.collapsedGroups.filter((key) => !old.has(key) || wanted.has(key));
    next.collapsedGroups = [
      ...new Set([...kept, ...after.collapsedGroups.filter((key) => !old.has(key))]),
    ];
  }
  return next;
}

function rebaseViews(current: SidebarView[], before: SidebarView[], after: SidebarView[]) {
  const previous = new Map(before.map((view) => [view.id, view]));
  const wanted = new Map(after.map((view) => [view.id, view]));
  const views = new Map(
    current
      .filter((view) => !previous.has(view.id) || wanted.has(view.id))
      .map((view) => [view.id, view]),
  );
  for (const view of after) {
    const old = previous.get(view.id);
    const existing = views.get(view.id);
    if (!old && !existing) views.set(view.id, cloneView(view));
    else if (old && existing) views.set(view.id, mergeViewEdit(existing, old, view));
  }
  const beforeOrder = before.filter((view) => wanted.has(view.id)).map((view) => view.id);
  const afterOrder = after.filter((view) => previous.has(view.id)).map((view) => view.id);
  if (beforeOrder.every((id, index) => afterOrder[index] === id)) return [...views.values()];
  const reordered = afterOrder.filter((id) => views.has(id));
  const slots = new Set(reordered);
  let position = 0;
  return [...views.values()].map((view) =>
    slots.has(view.id) ? views.get(reordered[position++])! : view,
  );
}

function sameDraftOwner(left: SidebarViewDraft, right: SidebarViewDraft): boolean {
  return left.baseViewId === right.baseViewId && left.group === right.group;
}

function rebaseSidebarDraft(
  current: SidebarViewDraft | null,
  before: SidebarViewDraft | null,
  after: SidebarViewDraft | null,
): SidebarViewDraft | null {
  if (JSON.stringify(before) === JSON.stringify(after)) return current;
  if (!before || !after || !sameDraftOwner(before, after)) return after;
  if (!current || !sameDraftOwner(current, before)) return current;
  const fields = (
    ["filters", "sort", "sortWarningCount", "groupIndent", "taskRow"] as const
  ).filter((key) => JSON.stringify(before[key]) !== JSON.stringify(after[key]));
  const disclosureChanged =
    JSON.stringify(before.collapsedGroups) !== JSON.stringify(after.collapsedGroups);
  if (!fields.length && !disclosureChanged) return current;
  const next = Object.assign(
    cloneDraft(current)!,
    Object.fromEntries(fields.map((key) => [key, after[key]])),
  );
  if (disclosureChanged) {
    const old = new Set(before.collapsedGroups ?? []);
    const wanted = new Set(after.collapsedGroups ?? []);
    next.collapsedGroups = [
      ...new Set([
        ...(current.collapsedGroups ?? []).filter((key) => !old.has(key) || wanted.has(key)),
        ...(after.collapsedGroups ?? []).filter((key) => !old.has(key)),
      ]),
    ];
  }
  return next;
}

export function rebaseSidebarMutation(
  current: SidebarSnapshot,
  before: SidebarSnapshot,
  after: SidebarSnapshot,
): SidebarSnapshot {
  const views = rebaseViews(current.views, before.views, after.views);
  let activeViewId =
    before.activeViewId !== after.activeViewId ? after.activeViewId : current.activeViewId;
  if (!views.some((view) => view.id === activeViewId)) activeViewId = views[0]?.id ?? "";
  const draft = rebaseSidebarDraft(current.draft, before.draft, after.draft);
  return {
    views,
    activeViewId,
    draft: draft?.baseViewId === activeViewId ? cloneDraft(draft) : null,
    serverRevision: current.serverRevision,
  };
}

export function sidebarViewDelta(before: SidebarView[], after: SidebarView[]) {
  const previous = new Map(before.map((view) => [view.id, view]));
  const next = new Set(after.map((view) => view.id));
  const changed = after
    .filter((view) => {
      const old = previous.get(view.id);
      return (
        old !== view &&
        JSON.stringify(old && toApiSidebarView(old)) !== JSON.stringify(toApiSidebarView(view))
      );
    })
    .map(toApiSidebarView);
  const created = changed.filter((view) => !previous.has(view.id));
  const updated = changed.filter((view) => previous.has(view.id));
  const deleted = before.filter((view) => !next.has(view.id)).map((view) => view.id);
  const order = after.map((view) => view.id);
  const naturalOrder = [
    ...before.filter((view) => next.has(view.id)).map((view) => view.id),
    ...after.filter((view) => !previous.has(view.id)).map((view) => view.id),
  ];
  const reordered = naturalOrder.some((id, index) => id !== order[index]);
  return {
    ...(created.length ? { views_upsert: created } : {}),
    ...(updated.length ? { views_update: updated } : {}),
    ...(deleted.length ? { view_ids_delete: deleted } : {}),
    ...(reordered ? { view_order: order } : {}),
  };
}
