"use client";

import { useTranslation } from "react-i18next";
import { usePathname } from "@/lib/routing/client-router";
import { IconSettings } from "@tabler/icons-react";
import { CollapseAllButton } from "./sections/settings/collapse-all-button";
import { SettingsTree } from "./sections/settings/settings-tree";

/**
 * Full-height settings takeover for the sidebar, shown while the footer gear is
 * active. Replaces the normal primary nav + sections with just the settings
 * tree, which fills the remaining height and scrolls internally.
 *
 * The header is a plain label. It used to double as an exit button, but it
 * only flipped the sidebar back without leaving the /settings route, stranding
 * the main area on a settings page next to kanban navigation. Exits are the
 * topbar home crumb (always visible in settings mode) and the footer gear.
 */
export function AppSidebarSettingsMode() {
  const { t } = useTranslation();
  const pathname = usePathname();

  return (
    <div
      className="flex-1 min-h-0 flex flex-col gap-1 sidebar-fade-in"
      data-testid="app-sidebar-settings-mode"
    >
      <div className="flex items-center gap-1.5 px-2 h-7 shrink-0 text-foreground/70">
        <IconSettings className="h-3.5 w-3.5" />
        <span className="text-[11px] font-semibold uppercase tracking-wider">
          {t("common:settings")}
        </span>
        <CollapseAllButton />
      </div>
      <div className="flex-1 min-h-0 overflow-y-auto flex flex-col gap-0.5">
        <SettingsTree pathname={pathname} />
      </div>
    </div>
  );
}
