import type { SidebarViewApi, SidebarViewDraftApi } from "@/lib/types/http";
import type { SidebarView, SidebarViewDraft } from "./sidebar-view-types";

function toApiClause(c: SidebarView["filters"][number]) {
  return {
    id: c.id,
    dimension: c.dimension,
    op: c.op,
    value: c.value,
  };
}

function fromApiClause(c: SidebarViewApi["filters"][number]): SidebarView["filters"][number] {
  return {
    id: c.id,
    dimension: c.dimension as SidebarView["filters"][number]["dimension"],
    op: c.op as SidebarView["filters"][number]["op"],
    value: c.value as SidebarView["filters"][number]["value"],
  };
}

export function toApiSidebarView(view: SidebarView): SidebarViewApi {
  return {
    id: view.id,
    name: view.name,
    filters: view.filters.map(toApiClause),
    sort: { key: view.sort.key, direction: view.sort.direction },
    group: view.group,
    collapsed_groups: view.collapsedGroups,
  };
}

export function fromApiSidebarView(api: SidebarViewApi): SidebarView {
  return {
    id: api.id,
    name: api.name,
    filters: api.filters.map(fromApiClause),
    sort: {
      key: api.sort.key as SidebarView["sort"]["key"],
      direction: api.sort.direction as SidebarView["sort"]["direction"],
    },
    group: api.group as SidebarView["group"],
    collapsedGroups: api.collapsed_groups ?? [],
  };
}

export function toApiSidebarDraft(draft: SidebarViewDraft): SidebarViewDraftApi {
  return {
    base_view_id: draft.baseViewId,
    filters: draft.filters.map(toApiClause),
    sort: { key: draft.sort.key, direction: draft.sort.direction },
    group: draft.group,
  };
}

export function fromApiSidebarDraft(api: SidebarViewDraftApi): SidebarViewDraft {
  return {
    baseViewId: api.base_view_id,
    filters: api.filters.map(fromApiClause),
    sort: {
      key: api.sort.key as SidebarView["sort"]["key"],
      direction: api.sort.direction as SidebarView["sort"]["direction"],
    },
    group: api.group as SidebarView["group"],
  };
}
