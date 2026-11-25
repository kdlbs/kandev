import { t } from "@/lib/i18n";
import { moveSidebarView, type SidebarViewMove } from "./sidebar-view-move";
import { composeSidebarViewPatches } from "./sidebar-view-patch-compose";
import type { SidebarView, SidebarViewDraft } from "./sidebar-view-types";
import { toApiSidebarView } from "./sidebar-view-wire";
import { cloneDraft, cloneView } from "./sidebar-view-clone";
import { toSidebarViewUpdate } from "./sidebar-view-update";
import type {
  SidebarViewApi,
  SidebarViewUpdateApi,
  UserSettingsUpdatePayload,
} from "@/lib/types/http-user-settings";
import { sidebarDraftDelta } from "./sidebar-draft-update";

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

function rebaseViews(
  current: SidebarView[],
  before: SidebarView[],
  after: SidebarView[],
  move?: SidebarViewMove,
) {
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
  if (move) {
    const reordered = moveSidebarView([...views.values()], move);
    if (!reordered) throw new Error(t("sidebar:failedToSyncSidebarViews"));
    return reordered;
  }
  return [...views.values()];
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
  move?: SidebarViewMove,
): SidebarSnapshot {
  const views = rebaseViews(current.views, before.views, after.views, move);
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
  const created: SidebarViewApi[] = [];
  const updated: SidebarViewUpdateApi[] = [];
  for (const view of after) {
    const old = previous.get(view.id);
    if (!old) created.push(toApiSidebarView(view));
    else {
      const update = toSidebarViewUpdate(old, view);
      if (update) updated.push(update);
    }
  }
  const deleted = before.filter((view) => !next.has(view.id)).map((view) => view.id);
  const order = after.map((view) => view.id);
  const naturalOrder = [
    ...before.filter((view) => next.has(view.id)).map((view) => view.id),
    ...after.filter((view) => !previous.has(view.id)).map((view) => view.id),
  ];
  const reordered = naturalOrder.some((id, index) => id !== order[index]);
  return {
    ...(created.length ? { views_create: created } : {}),
    ...(updated.length ? { views_update: updated } : {}),
    ...(deleted.length ? { view_ids_delete: deleted } : {}),
    ...(reordered ? { view_order: order } : {}),
  };
}

function sidebarSettingsDelta(before: SidebarSnapshot, after: SidebarSnapshot) {
  return {
    ...sidebarViewDelta(before.views, after.views),
    ...(before.activeViewId !== after.activeViewId ? { active_view_id: after.activeViewId } : {}),
    ...sidebarDraftDelta(before, after),
  };
}

export function sidebarWriteDelta(
  before: SidebarSnapshot,
  after: SidebarSnapshot,
  failed: ReadonlyArray<{ before: SidebarSnapshot; after: SidebarSnapshot }> = [],
  desired: SidebarSnapshot = after,
): Omit<NonNullable<UserSettingsUpdatePayload["sidebar_view_state"]>, "workspace_id"> {
  const patch = failed.length
    ? composeSidebarViewPatches([
        ...failed.map((mutation) => sidebarSettingsDelta(mutation.before, mutation.after)),
        sidebarSettingsDelta(before, after),
      ])
    : sidebarSettingsDelta(before, after);
  if (patch.view_order) patch.view_order = desired.views.map((view) => view.id);
  return patch;
}
