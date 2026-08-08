"use client";

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconFolder } from "@tabler/icons-react";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import Link from "@/components/routing/app-link";
import { useAppStore } from "@/components/state-provider";
import { useRouter } from "@/lib/routing/client-router";
import {
  WORKSPACE_SETTINGS_TABS,
  workspaceSettingsHref,
  type WorkspaceSettingsTab,
} from "@/lib/settings/workspace-settings-tabs";
import { ActiveWorkspaceBadge } from "@/components/settings/record-badges";
import { cn } from "@kandev/ui/lib/utils";

// The tab table and href builder are data — see `workspace-settings-tabs.ts`,
// which the settings menu's Workspaces branch reads too. Re-exported here so
// existing callers keep one import.
// The badge is shared with the settings menu and the workspace list — see
// `record-badges.tsx`. Re-exported so existing callers keep one import.
export { ActiveWorkspaceBadge } from "@/components/settings/record-badges";

export {
  workspaceSettingsHref,
  workspaceSettingsTabSpec,
  type WorkspaceSettingsTab,
} from "@/lib/settings/workspace-settings-tabs";

/**
 * The tabbed shell every workspace settings page renders through: the
 * workspace name doubles as a switcher (move between workspaces without going
 * back to the list — the breadcrumb's Workspaces crumb owns the way back),
 * plus one tab per section. The menu holds no workspace rows, so this is the
 * only workspace-level navigation surface.
 */
export function WorkspaceSettingsShell({
  workspaceId,
  activeTab,
  children,
}: {
  workspaceId: string;
  activeTab: WorkspaceSettingsTab;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const workspaces = useAppStore((s) => s.workspaces.items);
  const activeId = useAppStore((s) => s.workspaces.activeId);
  const workspace = workspaces.find((item) => item.id === workspaceId);

  return (
    <div className="space-y-6" data-testid="workspace-settings-shell">
      <div>
        <div className="flex min-w-0 items-center gap-3">
          <div className="p-2 bg-muted rounded-md">
            <IconFolder className="h-4 w-4" />
          </div>
          {workspace ? (
            <Select
              value={workspaceId}
              onValueChange={(nextId) => {
                if (nextId !== workspaceId) router.push(workspaceSettingsHref(nextId, activeTab));
              }}
            >
              {/* The workspace name is the switcher: one control, no separate dropdown. */}
              <SelectTrigger
                className="-ml-2 h-auto w-auto max-w-full gap-2 border-none bg-transparent px-2 py-1 text-2xl font-bold shadow-none hover:bg-accent/50 [&>span]:min-w-0"
                aria-label={t("sidebar:switchWorkspace")}
                data-testid="workspace-settings-switcher"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {workspaces.map((item) => (
                  <SelectItem key={item.id} value={item.id}>
                    <span className="flex min-w-0 items-center gap-2">
                      <span className="truncate">{item.name}</span>
                      {item.id === activeId && <ActiveWorkspaceBadge />}
                    </span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : (
            <h2 className="truncate text-2xl font-bold">{t("common:workspace")}</h2>
          )}
        </div>
      </div>
      {/* Pills on a phone, an underline rail from `md` up — the same boundary
          the sidebar uses, so the strip changes shape exactly when the sidebar
          that would otherwise carry this navigation disappears. Scrolls
          horizontally at both sizes; six sections outrun a phone's width, and
          wrapping them would push the page content below the fold. */}
      <nav
        aria-label={t("common:workspace")}
        className="flex gap-2 overflow-x-auto pb-1 scrollbar-hide md:gap-1 md:border-b md:border-border md:pb-0"
        data-testid="workspace-settings-tabs"
      >
        {WORKSPACE_SETTINGS_TABS.map(({ tab, labelKey }) => (
          <Link
            key={tab}
            href={workspaceSettingsHref(workspaceId, tab)}
            aria-current={activeTab === tab ? "page" : undefined}
            className={cn(
              "flex shrink-0 items-center whitespace-nowrap px-3 py-2 text-sm transition-colors",
              "rounded-full border",
              activeTab === tab
                ? "border-primary/40 bg-primary/10 font-medium text-primary"
                : "border-border/60 bg-muted/40 text-muted-foreground",
              // From `md`, drop the pill and restore the rail.
              "md:rounded-none md:border-0 md:border-b-2 md:bg-transparent",
              activeTab === tab
                ? "md:border-primary md:text-foreground"
                : "md:border-transparent md:text-muted-foreground md:hover:text-foreground",
            )}
          >
            {t(labelKey)}
          </Link>
        ))}
      </nav>
      <div>{children}</div>
    </div>
  );
}
