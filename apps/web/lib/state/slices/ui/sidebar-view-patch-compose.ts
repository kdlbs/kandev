import type {
  SidebarDraftUpdateApi,
  SidebarViewApi,
  SidebarViewUpdateApi,
  UserSettingsUpdatePayload,
} from "@/lib/types/http-user-settings";

type Patch = Omit<NonNullable<UserSettingsUpdatePayload["sidebar_view_state"]>, "workspace_id">;
type Disclosure = NonNullable<SidebarViewUpdateApi["disclosure"]>;

function applyDisclosureKeys(current: string[], update: Disclosure): string[] {
  const keys = new Set(current);
  for (const key of update.add ?? []) keys.add(key);
  for (const key of update.remove ?? []) keys.delete(key);
  return [...keys];
}

function mergeDisclosure(
  previous: Disclosure | undefined,
  next: Disclosure | undefined,
): Disclosure | undefined {
  if (!next) return previous;
  if (!previous || previous.group !== next.group || next.replace) return next;
  if (previous.replace)
    return { group: next.group, replace: applyDisclosureKeys(previous.replace, next) };
  const add = new Set(previous.add);
  const remove = new Set(previous.remove);
  for (const key of next.add ?? []) {
    remove.delete(key);
    add.add(key);
  }
  for (const key of next.remove ?? []) {
    add.delete(key);
    remove.add(key);
  }
  return {
    group: next.group,
    ...(add.size ? { add: [...add] } : {}),
    ...(remove.size ? { remove: [...remove] } : {}),
    ...(previous.canonicalize || next.canonicalize ? { canonicalize: true } : {}),
  };
}

function mergeDraft(
  previous: SidebarDraftUpdateApi | undefined,
  next: SidebarDraftUpdateApi,
): SidebarDraftUpdateApi {
  if (
    !previous ||
    previous.base_view_id !== next.base_view_id ||
    previous.expected_group !== next.expected_group ||
    next.clear ||
    previous.clear
  )
    return next;
  return {
    ...previous,
    ...next,
    disclosure: mergeDisclosure(previous.disclosure, next.disclosure),
  };
}

function mergeViewMutations(
  patch: Patch,
  creates: Map<string, SidebarViewApi>,
  updates: Map<string, SidebarViewUpdateApi>,
  deleted: Set<string>,
) {
  for (const id of patch.view_ids_delete ?? []) {
    deleted.add(id);
    creates.delete(id);
    updates.delete(id);
  }
  for (const view of patch.views_create ?? []) {
    deleted.delete(view.id);
    if (!creates.has(view.id)) creates.set(view.id, view);
  }
  for (const update of patch.views_update ?? []) {
    if (deleted.has(update.id)) continue;
    const previous = updates.get(update.id);
    updates.set(update.id, {
      ...previous,
      ...update,
      disclosure: mergeDisclosure(previous?.disclosure, update.disclosure),
    });
  }
}

// Preserve explicit final assignments even when an earlier response was lost after commit.
export function composeSidebarViewPatches(patches: Patch[]): Patch {
  const creates = new Map<string, SidebarViewApi>();
  const updates = new Map<string, SidebarViewUpdateApi>();
  const deleted = new Set<string>();
  const result: Patch = {};
  for (const patch of patches) {
    mergeViewMutations(patch, creates, updates, deleted);
    if (patch.view_order) result.view_order = patch.view_order;
    if (patch.active_view_id !== undefined) result.active_view_id = patch.active_view_id;
    if (patch.draft !== undefined) {
      result.draft = patch.draft;
      delete result.draft_update;
    }
    if (patch.draft_update)
      result.draft_update = mergeDraft(result.draft_update, patch.draft_update);
  }
  if (creates.size) result.views_create = [...creates.values()];
  if (updates.size) result.views_update = [...updates.values()];
  if (deleted.size) result.view_ids_delete = [...deleted];
  return result;
}
