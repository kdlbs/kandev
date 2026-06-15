"use client";

import { usePathname } from "@/lib/routing/client-router";
import { IconSettings, IconChevronLeft } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { SettingsTree } from "./sections/settings/settings-tree";

/**
 * Full-height settings takeover for the sidebar, shown while the footer gear is
 * active. Replaces the normal primary nav + sections with just the settings
 * tree, which fills the remaining height and scrolls internally.
 *
 * The header doubles as the exit affordance: the footer gear sits at the bottom
 * of the rail, far from the tree, so clicking the "Settings" header at the top
 * closes the takeover and returns to the normal sidebar — regardless of which
 * group is currently expanded.
 */
export function AppSidebarSettingsMode() {
  const pathname = usePathname();
  const toggleSettingsMode = useAppStore((s) => s.toggleAppSidebarSettingsMode);

  return (
    <div
      className="flex-1 min-h-0 flex flex-col gap-1 sidebar-fade-in"
      data-testid="app-sidebar-settings-mode"
    >
      <button
        type="button"
        onClick={toggleSettingsMode}
        aria-label="Close settings"
        data-testid="app-sidebar-settings-mode-close"
        className="group/close flex items-center gap-1.5 px-2 h-7 shrink-0 rounded-md text-foreground/70 hover:bg-muted/60 hover:text-foreground cursor-pointer transition-colors"
      >
        <IconSettings className="h-3.5 w-3.5 group-hover/close:hidden" />
        <IconChevronLeft className="h-3.5 w-3.5 hidden group-hover/close:block" />
        <span className="text-[11px] font-semibold uppercase tracking-wider">Settings</span>
      </button>
      <div className="flex-1 min-h-0 overflow-y-auto flex flex-col gap-0.5">
        <SettingsTree pathname={pathname} />
      </div>
    </div>
  );
}
