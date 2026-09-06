"use client";

import { useCallback, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconFolder } from "@tabler/icons-react";
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "@kandev/ui/dropdown-menu";
import Link from "@/components/routing/app-link";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useRouter } from "@/lib/routing/client-router";
import {
  getWorkspaceSettingsTabs,
  workspaceSettingsHref,
  type WorkspaceSettingsTab,
} from "@/lib/settings/workspace-settings-tabs";
import {
  WorkspacePickerContent,
  WorkspaceTrigger,
  type WorkspaceItem,
} from "@/components/workspaces/workspace-picker-content";
import { cn } from "@kandev/ui/lib/utils";

// The tab table and href builder are data — see `workspace-settings-tabs.ts`,
// which the settings menu's Workspaces branch reads too. Re-exported here so
// existing callers keep one import.
// The badge is shared with the settings menu and the workspace list — see
// `record-badges.tsx`. Re-exported so existing callers keep one import.
import { ActiveWorkspaceBadge } from "@/components/settings/record-badges";

export { ActiveWorkspaceBadge };

export {
  workspaceSettingsHref,
  workspaceSettingsTabSpec,
  type WorkspaceSettingsTab,
} from "@/lib/settings/workspace-settings-tabs";

/**
 * The workspace switcher this page is headed by — the same control the sidebar
 * header draws, so the two read as one design (see
 * `workspace-picker-content.tsx`). It is a page-to-page navigator, not an
 * activation control: picking a workspace opens that workspace's copy of the
 * tab you are on and leaves the active workspace alone. The "Active" pill in
 * the list is what tells the two apart.
 */
function WorkspaceSettingsSwitcher({
  workspaceName,
  workspaceId,
  activeTab,
}: {
  workspaceName: string;
  workspaceId: string;
  activeTab: WorkspaceSettingsTab;
}) {
  const router = useRouter();
  const officeEnabled = useFeature("office");
  const workspaces = useAppStore((s) => s.workspaces.items);
  const activeId = useAppStore((s) => s.workspaces.activeId);
  const [open, setOpen] = useState(false);

  const handleSelect = useCallback(
    (next: WorkspaceItem) => {
      if (next.id !== workspaceId) {
        router.push(workspaceSettingsHref(next.id, activeTab));
      }
      setOpen(false);
    },
    [router, workspaceId, activeTab],
  );

  const handleNavigate = useCallback(
    (href: string) => {
      router.push(href);
      setOpen(false);
    },
    [router],
  );

  return (
    <>
      <DropdownMenu open={open} onOpenChange={setOpen}>
        <DropdownMenuTrigger asChild>
          <WorkspaceTrigger
            activeName={workspaceName}
            chevronTestId="workspace-settings-switcher-chevron"
            data-testid="workspace-settings-switcher"
            // Same control, sized for a page heading rather than a sidebar
            // row, at a fixed 240px: long names truncate, short ones leave
            // the chevron anchored instead of the header jumping per page.
            className="h-9 w-60 flex-none gap-2 px-3 text-base font-semibold"
          />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="w-72">
          <WorkspacePickerContent
            workspaces={workspaces}
            activeId={activeId}
            itemTestIdPrefix="workspace-settings-switcher-item"
            officeEnabled={officeEnabled}
            onWorkspaceSelect={handleSelect}
            onNavigate={handleNavigate}
          />
        </DropdownMenuContent>
      </DropdownMenu>
      {/* The page-level mark: the closed picker names the workspace being
          edited, so when that is also the active one, say so without opening
          the menu. */}
      {workspaceId === activeId && (
        <span data-testid="workspace-settings-active-badge" className="shrink-0">
          <ActiveWorkspaceBadge />
        </span>
      )}
    </>
  );
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
  const workspaces = useAppStore((s) => s.workspaces.items);
  const canvasesEnabled = useFeature("canvases");
  const workspace = workspaces.find((item) => item.id === workspaceId);
  const tabs = getWorkspaceSettingsTabs(canvasesEnabled);
  const tabsRef = useRef<HTMLElement | null>(null);

  // Each tab is its own route, so navigating remounts this shell and the
  // strip snaps back to its start — on a phone the pill you just tapped
  // scrolls out of view. Centre the active pill before paint instead; when
  // the strip fits (desktop rail), there is nothing to scroll and this is a
  // no-op.
  useLayoutEffect(() => {
    const nav = tabsRef.current;
    if (!nav || nav.scrollWidth <= nav.clientWidth) return;
    const pill = nav.querySelector<HTMLElement>('[aria-current="page"]');
    if (!pill) return;
    const navRect = nav.getBoundingClientRect();
    const pillRect = pill.getBoundingClientRect();
    nav.scrollLeft += pillRect.left - navRect.left - (nav.clientWidth - pillRect.width) / 2;
  }, [activeTab]);

  return (
    <div className="space-y-6" data-testid="workspace-settings-shell">
      <div>
        <div className="flex min-w-0 items-center gap-3">
          <div className="p-2 bg-muted rounded-md">
            <IconFolder className="h-4 w-4" />
          </div>
          {workspace ? (
            <WorkspaceSettingsSwitcher
              workspaceName={workspace.name}
              workspaceId={workspaceId}
              activeTab={activeTab}
            />
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
        ref={tabsRef}
        aria-label={t("common:workspace")}
        className="flex gap-2 overflow-x-auto pb-1 scrollbar-hide md:gap-1 md:border-b md:border-border md:pb-0"
        data-testid="workspace-settings-tabs"
      >
        {tabs.map(({ tab, labelKey }) => (
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
