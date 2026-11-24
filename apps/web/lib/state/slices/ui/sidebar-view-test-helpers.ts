import type {
  SidebarViewApi,
  SidebarViewUpdateApi,
  SidebarWorkspaceStateApi,
  UserSettingsUpdatePayload,
} from "@/lib/types/http-user-settings";

// Mock transport state for journal tests. Real CAS semantics are exercised in Go and browser tests.
export function applyViewUpdateForTest(
  view: SidebarViewApi,
  patch: SidebarViewUpdateApi,
): SidebarViewApi {
  const { disclosure, generated_plugin_grouping, ...fields } = patch;
  const changedGroup = patch.group !== undefined && patch.group !== view.group;
  const next: SidebarViewApi = {
    ...view,
    ...fields,
    collapsed_groups: changedGroup ? [] : view.collapsed_groups,
    ...(generated_plugin_grouping !== undefined
      ? { generated_plugin_grouping: generated_plugin_grouping ?? undefined }
      : {}),
  };
  if (disclosure?.group === next.group) {
    if (disclosure.replace) next.collapsed_groups = [...disclosure.replace];
    else {
      const remove = new Set(disclosure.remove);
      next.collapsed_groups = [
        ...new Set([
          ...next.collapsed_groups.filter((key) => !remove.has(key)),
          ...(disclosure.add ?? []).filter((key) => !remove.has(key)),
        ]),
      ];
    }
  }
  return next;
}

type SidebarPatch = NonNullable<UserSettingsUpdatePayload["sidebar_view_state"]>;

export function applyViewMutationsForTest(
  views: Map<string, SidebarViewApi>,
  patch: SidebarPatch,
): void {
  for (const id of patch.view_ids_delete ?? []) views.delete(id);
  for (const view of patch.views_create ?? []) if (!views.has(view.id)) views.set(view.id, view);
  for (const view of patch.views_upsert ?? []) views.set(view.id, view);
  for (const view of patch.views_update ?? [])
    if (views.has(view.id)) views.set(view.id, applyViewUpdateForTest(views.get(view.id)!, view));
}

function changedDraftOwner(
  current: SidebarWorkspaceStateApi["draft"],
  update: NonNullable<SidebarPatch["draft_update"]>,
) {
  return (
    current &&
    (current.base_view_id !== update.base_view_id || current.group !== update.expected_group)
  );
}

export function applyDraftMutationForTest(
  views: Map<string, SidebarViewApi>,
  active: string,
  current: SidebarWorkspaceStateApi["draft"],
  patch: SidebarPatch,
): SidebarWorkspaceStateApi["draft"] {
  if (!patch.draft_update) return "draft" in patch ? (patch.draft ?? null) : current;
  const { base_view_id, expected_group, clear, require_existing, ...fields } = patch.draft_update;
  if (changedDraftOwner(current, patch.draft_update)) return current;
  if (clear) return null;
  if (require_existing && !current) return current;
  const base = views.get(base_view_id);
  if (!base || active !== base_view_id || (!current && base.group !== expected_group))
    return current;
  const view = applyViewUpdateForTest(
    {
      ...base,
      ...current,
      collapsed_groups: current?.collapsed_groups ?? [],
    },
    { id: base_view_id, ...fields },
  );
  return {
    base_view_id,
    filters: view.filters,
    sort: view.sort,
    group: view.group,
    group_indent: view.group_indent,
    task_row: view.task_row,
    collapsed_groups: view.collapsed_groups,
  };
}

export function applyPluginSelectionForTest(
  state: SidebarWorkspaceStateApi,
  patch: SidebarPatch,
): SidebarWorkspaceStateApi {
  const candidate = patch.select_plugin_grouping;
  if (!candidate) return state;
  if (state.draft) throw new Error();
  const existing = state.views.find(
    (view) =>
      view.generated_plugin_grouping?.reference === candidate.group &&
      view.generated_plugin_grouping.name === view.name &&
      view.group === candidate.group &&
      !view.filters.length &&
      view.sort.key === "state" &&
      view.sort.direction === "asc" &&
      !view.sort.color &&
      !view.sort.then_by?.length &&
      (view.group_indent ?? true) &&
      JSON.stringify(view.task_row ?? candidate.task_row) === JSON.stringify(candidate.task_row),
  );
  if (existing) return { ...state, active_view_id: existing.id };
  if (state.views.length >= 50) throw new Error();
  const names = new Set(state.views.map((view) => view.name));
  let name = candidate.name;
  for (let suffix = 2; names.has(name); suffix++) name = `${candidate.name} ${suffix}`;
  const view = {
    ...candidate,
    name,
    generated_plugin_grouping: { ...candidate.generated_plugin_grouping!, name },
  };
  return { ...state, views: [...state.views, view], active_view_id: view.id };
}

export function applyDraftSaveForTest(
  state: SidebarWorkspaceStateApi,
  patch: SidebarPatch,
): SidebarWorkspaceStateApi {
  const save = patch.save_draft;
  if (!save) return state;
  const draft = state.draft;
  const base = state.views.find((view) => view.id === save.base_view_id);
  if (
    !base ||
    !draft ||
    state.active_view_id !== base.id ||
    draft.base_view_id !== base.id ||
    draft.group !== save.expected_group
  )
    throw new Error("Draft owner changed");
  const copy = save.target_view_id !== base.id;
  const view = {
    ...base,
    ...draft,
    id: save.target_view_id,
    name: copy ? save.name! : base.name,
    collapsed_groups: [
      ...(draft.group === base.group ? base.collapsed_groups : (draft.collapsed_groups ?? [])),
    ],
    generated_plugin_grouping: copy ? undefined : base.generated_plugin_grouping,
  };
  return {
    views: copy
      ? [...state.views, view]
      : state.views.map((row) => (row.id === base.id ? view : row)),
    active_view_id: view.id,
    draft: null,
  };
}
