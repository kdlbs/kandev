"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useSidebarWorkspaceGuard } from "@/hooks/domains/sidebar/use-sidebar-workspace-guard";
import { usePluginRegistry } from "@/lib/plugins/registry";
import { selectSidebarViews } from "@/lib/state/slices/ui/sidebar-workspace-state";
import { MAX_SIDEBAR_VIEWS } from "@/lib/state/slices/ui/sidebar-view-builtins";
import { findGeneratedSidebarPluginView } from "@/lib/sidebar/sidebar-plugin-grouping";
import type { SidebarPluginGroupingChoice } from "@/lib/sidebar/sidebar-plugin-grouping";
import { sidebarTaskPageScope } from "@/lib/sidebar/sidebar-task-page-cache";

export type SidebarGroupingChoice = SidebarPluginGroupingChoice & { disabledReason: string | null };
export type SidebarGroupingChoices = {
  groupings: readonly SidebarGroupingChoice[];
  workspaceId: string | null;
  selectGrouping: (choice: SidebarPluginGroupingChoice) => Promise<boolean> | undefined;
};

export function useSidebarGroupings(): SidebarGroupingChoices {
  const { t } = useTranslation();
  const providers = usePluginRegistry().getSidebarGroupings();
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const store = useAppStoreApi();
  const scope = useAppStore(sidebarTaskPageScope);
  const { views, draft, transitionPending } = useAppStore(selectSidebarViews);
  const selectView = useAppStore((state) => state.selectSidebarPluginGrouping);
  const guard = useSidebarWorkspaceGuard();
  const selectGrouping = useMemo(
    () =>
      guard((choice: SidebarPluginGroupingChoice) => {
        if (
          sidebarTaskPageScope(store.getState()) !== scope ||
          selectSidebarViews(store.getState()).transitionPending
        )
          return;
        return selectView(choice);
      }),
    [guard, selectView, store, scope],
  );
  const groupings = useMemo(
    () =>
      providers.map((provider) => {
        const label = provider.labelKey
          ? t(provider.labelKey, { defaultValue: provider.label })
          : provider.label;
        const choice = { reference: provider.reference, label };
        const full =
          views.length >= MAX_SIDEBAR_VIEWS && !findGeneratedSidebarPluginView(views, choice);
        let disabledReason: string | null = null;
        if (transitionPending) disabledReason = t("common:loadingIndicatorLabel");
        else if (draft) disabledReason = t("sidebar:saveDiscardFirst");
        else if (full) disabledReason = t("sidebar:maxViews", { count: MAX_SIDEBAR_VIEWS });
        return { ...choice, disabledReason };
      }),
    [providers, views, draft, transitionPending, t],
  );
  return { groupings, selectGrouping, workspaceId };
}
