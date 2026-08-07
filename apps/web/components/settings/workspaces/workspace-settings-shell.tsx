"use client";

import type { ComponentType, ReactNode } from "react";
import { useTranslation } from "react-i18next";
import {
  IconArrowsShuffle,
  IconBolt,
  IconFolder,
  IconGitBranch,
  IconKey,
  IconLayoutGrid,
  IconPlugConnected,
} from "@tabler/icons-react";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import Link from "@/components/routing/app-link";
import { useAppStore } from "@/components/state-provider";
import { useRouter } from "@/lib/routing/client-router";
import { WORKSPACES_SETTINGS_HREF } from "@/lib/settings-discovery/catalog/workspaces";
import { cn } from "@kandev/ui/lib/utils";

export function ActiveWorkspaceBadge() {
  const { t } = useTranslation();
  return (
    <span className="shrink-0 rounded-full border border-primary/35 bg-primary/10 px-1.5 py-0.5 text-[10px] font-medium leading-none text-primary">
      {t("sidebar:activeWorkspaceBadge")}
    </span>
  );
}

export type WorkspaceSettingsTab =
  | "overview"
  | "repositories"
  | "workflows"
  | "integrations"
  | "automations"
  | "secrets";

export function workspaceSettingsHref(workspaceId: string, tab: WorkspaceSettingsTab): string {
  const base = `${WORKSPACES_SETTINGS_HREF}/${encodeURIComponent(workspaceId)}`;
  return tab === "overview" ? base : `${base}/${tab}`;
}

type WorkspaceTabSpec = {
  tab: WorkspaceSettingsTab;
  labelKey: string;
  icon: ComponentType<{ className?: string }>;
};

/**
 * One entry per tab, carrying the name and mark of that section.
 *
 * The tab strip is not the only surface that names a section: each tab's page
 * heads itself with the same name and mark (`WorkspaceSectionHeader`). Reading
 * both from one table is what stops the strip and the page it opens from
 * disagreeing about what the section is called.
 */
const TAB_ORDER: WorkspaceTabSpec[] = [
  { tab: "overview", labelKey: "workspaces:overview", icon: IconLayoutGrid },
  { tab: "repositories", labelKey: "sidebar:repositories", icon: IconGitBranch },
  { tab: "workflows", labelKey: "workflows:workflows", icon: IconArrowsShuffle },
  { tab: "integrations", labelKey: "common:integrations", icon: IconPlugConnected },
  { tab: "automations", labelKey: "common:automations", icon: IconBolt },
  { tab: "secrets", labelKey: "settings:secrets", icon: IconKey },
];

/** The name and mark for a tab, for the page that tab opens. */
export function workspaceSettingsTabSpec(tab: WorkspaceSettingsTab): WorkspaceTabSpec {
  // Every member of the union has a row, so the fallback is unreachable; it
  // exists so a future tab cannot crash a page before its row is added.
  return TAB_ORDER.find((entry) => entry.tab === tab) ?? TAB_ORDER[0];
}

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
      <nav
        aria-label={t("common:workspace")}
        className="flex gap-1 overflow-x-auto border-b border-border"
        data-testid="workspace-settings-tabs"
      >
        {TAB_ORDER.map(({ tab, labelKey }) => (
          <Link
            key={tab}
            href={workspaceSettingsHref(workspaceId, tab)}
            aria-current={activeTab === tab ? "page" : undefined}
            className={cn(
              "whitespace-nowrap border-b-2 px-3 py-2 text-sm transition-colors",
              activeTab === tab
                ? "border-primary font-medium text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground",
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
