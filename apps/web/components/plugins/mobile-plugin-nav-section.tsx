"use client";

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { AppSidebarWorkspaceActions } from "@/components/app-sidebar/app-sidebar-workspace-actions";
import { MainTopBarPluginActions } from "@/components/kanban/main-top-bar-plugin-actions";
import { DestinationRows } from "@/components/navigation/destination-rows";
import { resolveDestinations } from "@/lib/navigation/resolve-destinations";
import { NO_WORKSPACE_CONTEXT } from "@/lib/navigation/surface-policy";
import { usePluginRegistry } from "@/lib/plugins/registry";
import type { TaskListingPage } from "@/lib/task-listing/view-navigation";
import { cn } from "@/lib/utils";

export type MobilePluginWorkspaceContext = {
  workspaceId?: string;
  workspaceLabel?: string;
  currentPage: TaskListingPage;
};

type MobilePluginNavSectionProps = {
  /** Closes the surrounding menu sheet once a plugin page is opened. */
  onNavigate: () => void;
  /** Page-scoped plugin status and controls for this menu. */
  actions?: ReactNode;
  /** Custom sidebar layouts already place plugin destinations themselves. */
  includeDestinations?: boolean;
  /** Phone app navigation groups workspace slots with page contributions. */
  workspaceContext?: MobilePluginWorkspaceContext;
};

/**
 * Mobile counterpart to the desktop sidebar's `<PluginNavItems/>`: the
 * hamburger-sheet surface for plugin-registered "main"-section nav items
 * (`registerNavItem({ section: "main" })`, the default). The desktop rail is
 * `hidden md:block`, so without this section a plugin's own page has no phone
 * entry point at all. "integrations"-section items keep rendering inside
 * `MobileIntegrationsSection`.
 */
export function MobilePluginNavSection({
  onNavigate,
  actions,
  includeDestinations = true,
  workspaceContext,
}: MobilePluginNavSectionProps) {
  const { t } = useTranslation();
  const registry = usePluginRegistry();
  const { hasMainActions, hasSidebarActions } = workspaceSlotAvailability(
    registry,
    workspaceContext,
  );
  const hasWorkspaceActions = hasMainActions || hasSidebarActions;
  // Resolved directly rather than through `useStaticDestinations`: this group's
  // hrefs are static plugin paths, so it needs neither workspace context nor the
  // availability subscription. Matches the desktop rail in `plugin-nav-items.tsx`.
  const destinations = includeDestinations
    ? resolveDestinations({
        surface: "mobileMenu",
        section: "plugins",
        ctx: NO_WORKSPACE_CONTEXT,
        pluginItems: registry.getNavRegistrations(),
      })
    : [];

  if (!actions && !hasWorkspaceActions && destinations.length === 0) return null;

  return (
    <section
      className={cn("space-y-3", workspaceContext && "border-t border-border pt-4")}
      data-testid="mobile-plugin-nav-section"
      aria-label={t("common:plugins")}
    >
      <h3 className="text-sm font-medium">{t("common:plugins")}</h3>
      {hasWorkspaceActions && workspaceContext && (
        <div className="space-y-2" data-testid="mobile-plugin-workspace-actions">
          {actions && <h4 className="text-xs text-muted-foreground">{t("common:workspace")}</h4>}
          <WorkspacePluginControls
            context={workspaceContext}
            hasMainActions={hasMainActions}
            hasSidebarActions={hasSidebarActions}
          />
        </div>
      )}
      {actions && (
        <div className="space-y-2" data-testid="mobile-plugin-page-actions">
          {hasWorkspaceActions && (
            <h4 className="text-xs text-muted-foreground">{t("common:task")}</h4>
          )}
          {actions}
        </div>
      )}
      {destinations.length > 0 && (
        <DestinationRows
          destinations={destinations}
          onNavigate={onNavigate}
          pluginTestIdPrefix="mobile-plugin-nav-item-"
        />
      )}
    </section>
  );
}

function WorkspacePluginControls({
  context,
  hasMainActions,
  hasSidebarActions,
}: {
  context: MobilePluginWorkspaceContext;
  hasMainActions: boolean;
  hasSidebarActions: boolean;
}) {
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-2">
      {hasMainActions && <MainTopBarPluginActions {...context} presentation="mobile" />}
      {hasSidebarActions && context.workspaceId && (
        <AppSidebarWorkspaceActions
          workspaceId={context.workspaceId}
          workspaceLabel={context.workspaceLabel}
          presentation="mobile"
        />
      )}
    </div>
  );
}

function workspaceSlotAvailability(
  registry: ReturnType<typeof usePluginRegistry>,
  context?: MobilePluginWorkspaceContext,
) {
  return {
    hasMainActions: !!context && registry.getSlotRegistrations("main-top-bar").length > 0,
    hasSidebarActions:
      !!context?.workspaceId &&
      registry.getSlotRegistrations("sidebar-workspace-actions").length > 0,
  };
}
