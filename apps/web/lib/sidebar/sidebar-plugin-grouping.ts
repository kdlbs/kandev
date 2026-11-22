import { createDefaultSidebarView } from "@/lib/state/slices/ui/sidebar-view-builtins";
import { cloneSidebarTaskRowPresentation } from "@/lib/state/slices/ui/sidebar-task-row-presentation";
import type { SidebarView } from "@/lib/state/slices/ui/sidebar-view-types";

export type SidebarPluginGroupingChoice = {
  reference: `plugin:${string}:${string}`;
  label: string;
};

export function isSidebarPluginGrouping(
  value: string,
): value is SidebarPluginGroupingChoice["reference"] {
  return /^plugin:[a-z0-9][a-z0-9._-]{0,127}:[a-z0-9][a-z0-9._-]{0,127}$/.test(value);
}

export function findGeneratedSidebarPluginView(
  views: SidebarView[],
  grouping: SidebarPluginGroupingChoice,
): SidebarView | undefined {
  return views.find((view) => {
    const origin = view.generatedPluginGrouping;
    if (
      !origin ||
      origin.reference !== grouping.reference ||
      origin.name !== view.name ||
      view.group !== grouping.reference ||
      view.filters.length !== 0
    )
      return false;
    const defaults = createDefaultSidebarView(view.id, view.name);
    return (
      view.sort.key === defaults.sort.key &&
      view.sort.direction === defaults.sort.direction &&
      !view.sort.thenBy?.length &&
      !view.sort.color &&
      !view.sortWarningCount &&
      view.groupIndent === defaults.groupIndent &&
      JSON.stringify(cloneSidebarTaskRowPresentation(view.taskRow)) ===
        JSON.stringify(defaults.taskRow)
    );
  });
}

export function createSidebarPluginView(
  grouping: SidebarPluginGroupingChoice,
  id: string,
): SidebarView {
  return {
    ...createDefaultSidebarView(id, grouping.label),
    group: grouping.reference,
    generatedPluginGrouping: { reference: grouping.reference, name: grouping.label },
  };
}
