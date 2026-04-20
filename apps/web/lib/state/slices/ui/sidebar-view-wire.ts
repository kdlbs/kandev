import type { SidebarViewApi } from "@/lib/types/http";
import type { SidebarView } from "./sidebar-view-types";

export function toApiSidebarView(view: SidebarView): SidebarViewApi {
  return {
    id: view.id,
    name: view.name,
    filters: view.filters.map((c) => ({
      id: c.id,
      dimension: c.dimension,
      op: c.op,
      value: c.value,
    })),
    sort: { key: view.sort.key, direction: view.sort.direction },
    group: view.group,
    collapsed_groups: view.collapsedGroups,
  };
}

export function fromApiSidebarView(api: SidebarViewApi): SidebarView {
  return {
    id: api.id,
    name: api.name,
    filters: api.filters.map((c) => ({
      id: c.id,
      dimension: c.dimension as SidebarView["filters"][number]["dimension"],
      op: c.op as SidebarView["filters"][number]["op"],
      value: c.value as SidebarView["filters"][number]["value"],
    })),
    sort: {
      key: api.sort.key as SidebarView["sort"]["key"],
      direction: api.sort.direction as SidebarView["sort"]["direction"],
    },
    group: api.group as SidebarView["group"],
    collapsedGroups: api.collapsed_groups ?? [],
  };
}
