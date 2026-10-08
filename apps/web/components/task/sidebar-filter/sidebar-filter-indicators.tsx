"use client";

import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useEffectiveSidebarView } from "@/hooks/domains/sidebar/use-effective-sidebar-view";
import { selectSidebarViews } from "@/lib/state/slices/ui/sidebar-workspace-state";

export function SidebarFilterIndicators() {
  const { t } = useTranslation();
  const view = useEffectiveSidebarView();
  const draft = useAppStore((state) => selectSidebarViews(state).draft);
  return (
    <>
      {view.filters.length > 0 && (
        <span
          data-testid="sidebar-active-filter-indicator"
          role="img"
          aria-label={t("sidebar:filtersActive")}
          className="absolute right-1 top-1 size-1.5 rounded-full bg-primary"
        />
      )}
      {draft?.baseViewId === view.id && (
        <span
          data-testid="sidebar-filter-gear-indicator"
          role="img"
          aria-label={t("sidebar:unsavedFilterChanges")}
          className="absolute bottom-1 right-1 size-1 rounded-full bg-amber-500"
        />
      )}
    </>
  );
}
