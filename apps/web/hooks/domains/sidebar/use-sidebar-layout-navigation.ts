"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useSidebarShortcutCatalog } from "./use-sidebar-shortcut-catalog";
import { useShortcutActivity } from "./use-shortcut-activity";
import { fromApiSidebarLayout } from "@/lib/sidebar/layout-types";
import { projectSidebarLayout } from "@/lib/sidebar/layout-projection";

const BUILTIN_LABEL_KEYS: Record<string, string> = {
  home: "sidebar:home",
  new_task: "sidebar:newTask",
  automations: "common:automations",
  canvases: "canvases:canvases",
  integrations: "common:integrations",
};

export function useHasSavedSidebarLayout(): boolean {
  return useAppStore((state) => {
    const workspaceId = state.workspaces.activeId;
    // The boot payload contains a projected default for every accessible
    // workspace. Revision zero means the user has never selected a custom
    // layout, so keep the legacy navigation path for that workspace.
    return Boolean(
      workspaceId && state.userSettings.sidebarLayoutsByWorkspace?.[workspaceId]?.revision > 0,
    );
  });
}

export function useSidebarLayoutNavigation({ active = true }: { active?: boolean } = {}) {
  const { t } = useTranslation();
  const workspaceId = useAppStore((state) => state.workspaces.activeId ?? undefined);
  const savedLayout = useAppStore((state) => {
    const activeWorkspaceId = state.workspaces.activeId;
    return activeWorkspaceId
      ? state.userSettings.sidebarLayoutsByWorkspace?.[activeWorkspaceId]
      : undefined;
  });
  const catalog = useSidebarShortcutCatalog({ active });
  const layout = useMemo(() => fromApiSidebarLayout(savedLayout), [savedLayout]);
  const projection = useMemo(
    () =>
      projectSidebarLayout(layout, catalog.catalog, {
        unavailableLabel: t("common:unavailable"),
        builtinLabels: Object.fromEntries(
          Object.entries(BUILTIN_LABEL_KEYS).map(([id, key]) => [id, t(key)]),
        ),
      }),
    [catalog.catalog, layout, t],
  );
  const automationIds = useMemo(() => {
    const ids = new Set<string>();
    for (const node of projection.nodes) {
      if (!node.visible) continue;
      if (node.destinationId === "automations") {
        for (const automation of catalog.automations) ids.add(automation.id);
      }
      for (const shortcut of node.shortcuts) {
        if (shortcut.target.kind === "automation") ids.add(shortcut.target.id);
      }
    }
    return [...ids];
  }, [catalog.automations, projection.nodes]);
  const activity = useShortcutActivity({
    workspaceId,
    automations: catalog.automations,
    automationIds,
    definitionsLoading: catalog.definitionsLoading,
    definitionsError: catalog.definitionsError,
    active,
  });

  return {
    workspaceId,
    catalog,
    layout,
    projection,
    activity,
  };
}
