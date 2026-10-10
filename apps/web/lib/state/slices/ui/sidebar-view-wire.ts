import type { SidebarViewApi, SidebarViewDraftApi } from "@/lib/types/http";
import type { SidebarView, SidebarViewDraft } from "./sidebar-view-types";
import { sidebarSortFromWireRaw, sidebarSortToWire } from "@/lib/sidebar/sidebar-sort-chain";
import {
  cloneSidebarTaskRowPresentation,
  normalizeSidebarTaskRowPresentation,
  type SidebarTaskRowPresentation,
} from "./sidebar-task-row-presentation";

function toApiTaskRow(value: SidebarTaskRowPresentation | undefined) {
  const normalized = cloneSidebarTaskRowPresentation(value);
  return {
    details_enabled: normalized.detailsEnabled,
    detail_order: normalized.detailOrder,
    visible_details: normalized.visibleDetails,
    trailing: normalized.trailing,
  };
}

function fromApiTaskRow(value: SidebarViewApi["task_row"]): SidebarTaskRowPresentation {
  if (!value || typeof value !== "object") return normalizeSidebarTaskRowPresentation(undefined);
  return normalizeSidebarTaskRowPresentation({
    detailsEnabled: value.details_enabled,
    detailOrder: value.detail_order,
    visibleDetails: value.visible_details,
    trailing: value.trailing,
  });
}

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

function toApiSort(sort: SidebarView["sort"]) {
  return sidebarSortToWire(sort);
}

function fromApiSort(sort: SidebarViewApi["sort"]): SidebarView["sort"] {
  return sidebarSortFromWireRaw(sort);
}

export function toApiSidebarView(view: SidebarView): SidebarViewApi {
  return {
    id: view.id,
    name: view.name,
    filters: view.filters.map(toApiClause),
    sort: toApiSort(view.sort),
    group: view.group,
    group_indent: view.groupIndent,
    collapsed_groups: view.collapsedGroups,
    task_row: toApiTaskRow(view.taskRow),
  };
}

export function fromApiSidebarView(api: SidebarViewApi): SidebarView {
  return {
    id: api.id,
    name: api.name,
    filters: api.filters.map(fromApiClause),
    sort: fromApiSort(api.sort),
    group: api.group as SidebarView["group"],
    groupIndent: typeof api.group_indent === "boolean" ? api.group_indent : true,
    collapsedGroups: api.collapsed_groups ?? [],
    taskRow: fromApiTaskRow(api.task_row),
  };
}

export function toApiSidebarDraft(draft: SidebarViewDraft): SidebarViewDraftApi {
  return {
    base_view_id: draft.baseViewId,
    filters: draft.filters.map(toApiClause),
    sort: toApiSort(draft.sort),
    group: draft.group,
    group_indent: draft.groupIndent,
    task_row: toApiTaskRow(draft.taskRow),
  };
}

export function fromApiSidebarDraft(api: SidebarViewDraftApi): SidebarViewDraft {
  return {
    baseViewId: api.base_view_id,
    filters: api.filters.map(fromApiClause),
    sort: fromApiSort(api.sort),
    group: api.group as SidebarView["group"],
    groupIndent: typeof api.group_indent === "boolean" ? api.group_indent : true,
    taskRow: fromApiTaskRow(api.task_row),
  };
}
