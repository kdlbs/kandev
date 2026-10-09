"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useOfficeModeState } from "@/hooks/use-in-office";
import { useAppStore } from "@/components/state-provider";
import { useSidebarShortcutCatalog } from "./use-sidebar-shortcut-catalog";
import { useShortcutActivity } from "./use-shortcut-activity";
import { fromApiSidebarLayout } from "@/lib/sidebar/layout-types";
import { projectSidebarLayout } from "@/lib/sidebar/layout-projection";
import {
  BUILTIN_LAYOUT_NODES,
  builtinLayoutNodeLabelKey,
  type BuiltinLayoutContext,
} from "@/lib/sidebar/builtin-layout-nodes";

export function useHasSavedSidebarLayout(): boolean {
  return useAppStore((state) => {
    const workspaceId = state.workspaces.activeId;
    return Boolean(workspaceId);
  });
}

export function useSidebarLayoutNavigation({ active = true }: { active?: boolean } = {}) {
  const { t } = useTranslation();
  const inOffice = useOfficeModeState() === "office";
  const needsYouInbox = useFeature("needsYouInbox");
  const canvasesEnabled = useFeature("canvases");
  const coordinatorEnabled = useFeature("coordinator");
  const workspaceId = useAppStore((state) => state.workspaces.activeId ?? undefined);
  const savedLayout = useAppStore((state) => {
    const activeWorkspaceId = state.workspaces.activeId;
    return activeWorkspaceId
      ? state.userSettings.sidebarLayoutsByWorkspace?.[activeWorkspaceId]
      : undefined;
  });
  const catalog = useSidebarShortcutCatalog({ active });
  const layout = useMemo(() => fromApiSidebarLayout(savedLayout), [savedLayout]);
  const builtinContext = useMemo<BuiltinLayoutContext>(
    () => ({
      hasWorkspace: Boolean(workspaceId),
      inOffice,
      features: {
        coordinator: coordinatorEnabled,
        canvases: canvasesEnabled,
        needsYouInbox,
      },
    }),
    [canvasesEnabled, coordinatorEnabled, inOffice, needsYouInbox, workspaceId],
  );
  const builtinLabels = useMemo(
    () =>
      Object.fromEntries(
        BUILTIN_LAYOUT_NODES.map((node) => [
          node.destinationId,
          t(builtinLayoutNodeLabelKey(node, builtinContext)),
        ]),
      ),
    [builtinContext, t],
  );
  const rawProjection = useMemo(
    () =>
      projectSidebarLayout(layout, catalog.catalog, {
        unavailableLabel: t("common:unavailable"),
        builtinLabels,
        builtinContext,
      }),
    [builtinContext, builtinLabels, catalog.catalog, layout, t],
  );
  const projection = useMemo(
    () => ({
      ...rawProjection,
      nodes: rawProjection.nodes.filter((node) => node.available !== false),
    }),
    [rawProjection],
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
