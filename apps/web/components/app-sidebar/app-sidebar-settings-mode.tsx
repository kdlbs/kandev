"use client";

import { usePathname } from "next/navigation";
import { IconSettings } from "@tabler/icons-react";
import { SettingsTree } from "./sections/settings/settings-tree";

/**
 * Full-height settings takeover for the sidebar, shown while the footer gear is
 * active. Replaces the normal primary nav + sections with just the settings
 * tree, which fills the remaining height and scrolls internally.
 */
export function AppSidebarSettingsMode() {
  const pathname = usePathname();

  return (
    <div className="flex-1 min-h-0 flex flex-col gap-1" data-testid="app-sidebar-settings-mode">
      <div className="flex items-center gap-1.5 px-2 h-7 shrink-0 text-foreground/70">
        <IconSettings className="h-3.5 w-3.5" />
        <span className="text-[11px] font-semibold uppercase tracking-wider">Settings</span>
      </div>
      <div className="flex-1 min-h-0 overflow-y-auto flex flex-col gap-0.5">
        <SettingsTree pathname={pathname} />
      </div>
    </div>
  );
}
