"use client";

import { useAppStore } from "@/components/state-provider";
import { useNavContext } from "@/hooks/use-app-destinations";
import { homeDestinationHref } from "@/lib/navigation/core-destinations";

export type HomeAffordance = {
  mode: "none" | "phone" | "always";
  href: string;
};

/**
 * The one rule for whether a page shows a home crumb and where it leads.
 *
 * - `none` for root-variant pages — the surface *is* home.
 * - `always` while the sidebar's settings takeover is active (expanded sidebar
 *   in settings mode): the sidebar's Home row is replaced by the settings tree,
 *   so desktop users would otherwise be left with only the unlabelled logo.
 * - `phone` otherwise — below `md` the sidebar is hidden entirely.
 *
 * The href comes from the navigation manifest's home entry, so it follows the
 * active workspace and lands on the office dashboard inside Office.
 */
export function useHomeAffordance(variant: "breadcrumb" | "root" = "breadcrumb"): HomeAffordance {
  const ctx = useNavContext();
  const settingsMode = useAppStore((s) => s.appSidebar.settingsMode);
  const collapsed = useAppStore((s) => s.appSidebar.collapsed);
  const href = homeDestinationHref(ctx);
  if (variant === "root") return { mode: "none", href };
  // The takeover only renders in the expanded sidebar (see AppSidebarNavigation),
  // so a collapsed sidebar still shows its icon rail and needs no extra crumb.
  return { mode: settingsMode && !collapsed ? "always" : "phone", href };
}
