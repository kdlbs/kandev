import type { SidebarViewUpdateApi } from "@/lib/types/http-user-settings";
import type { SidebarView } from "./sidebar-view-types";
import { toApiSidebarView } from "./sidebar-view-wire";

function disclosureUpdate(
  before: SidebarView,
  after: SidebarView,
): SidebarViewUpdateApi["disclosure"] {
  if (before.group !== after.group) return { group: after.group, replace: after.collapsedGroups };
  if (
    before.collapsedGroups.length === after.collapsedGroups.length &&
    before.collapsedGroups.every((key, index) => key === after.collapsedGroups[index])
  )
    return;
  const previous = new Set(before.collapsedGroups);
  const wanted = new Set(after.collapsedGroups);
  const add = after.collapsedGroups.filter((key) => !previous.has(key));
  const remove = before.collapsedGroups.filter((key) => !wanted.has(key));
  if (add.length || remove.length)
    return {
      group: before.group,
      ...(add.length ? { add } : {}),
      ...(remove.length ? { remove } : {}),
    };
}

export function toSidebarViewUpdate(
  before: SidebarView,
  after: SidebarView,
): SidebarViewUpdateApi | null {
  if (before === after) return null;
  const previous = toApiSidebarView(before);
  const next = toApiSidebarView(after);
  const fields = (
    [
      "name",
      "filters",
      "sort",
      "group",
      "group_indent",
      "task_row",
      "generated_plugin_grouping",
    ] as const
  ).filter((key) => JSON.stringify(previous[key]) !== JSON.stringify(next[key]));
  const disclosure = disclosureUpdate(before, after);
  if (!fields.length && !disclosure) return null;
  return {
    id: after.id,
    ...Object.fromEntries(fields.map((key) => [key, next[key] ?? null])),
    ...(disclosure ? { disclosure } : {}),
  };
}
