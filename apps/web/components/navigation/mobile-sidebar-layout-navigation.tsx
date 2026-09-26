"use client";

import { useCallback, useMemo, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconInbox, IconSquarePlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import Link from "@/components/routing/app-link";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import { useQuickTerminalLauncher } from "@/hooks/use-quick-terminal-launcher";
import { useStaticDestinations } from "@/hooks/use-app-destinations";
import { useOfficeModeState } from "@/hooks/use-in-office";
import { useSidebarLayoutNavigation } from "@/hooks/domains/sidebar/use-sidebar-layout-navigation";
import { requestNewTaskCreation } from "@/lib/desktop/new-task-request";
import type { ProjectedSidebarNode, ProjectedShortcut } from "@/lib/sidebar/layout-projection";
import type { ResolvedDestination } from "@/lib/navigation/types";
import type { ShortcutCatalogEntry } from "@/lib/sidebar/shortcut-catalog";
import type { ShortcutActivityReader } from "@/hooks/domains/sidebar/use-shortcut-activity";
import { ShortcutSection } from "@/components/app-sidebar/shortcut-section";
import {
  ShortcutRows,
  type ShortcutActivation,
} from "@/components/app-sidebar/shortcut-section-actions";
import { workspaceSettingsHref } from "@/lib/settings/workspace-settings-tabs";
import {
  selectNeedsYouInboxCount,
  selectNeedsYouInboxHasMore,
} from "@/lib/state/slices/needs-you-inbox/selectors";
import { selectOfficeInboxCount } from "@/lib/state/slices/office/selectors";
import { NEEDS_YOU_INBOX_HREF } from "@/lib/navigation/needs-you-inbox-destination";
import { DestinationRows } from "./destination-rows";
import { MobileAutomationsSection } from "./mobile-automations-section";
import { MobileCanvasesSection } from "./mobile-canvases-section";
import { MobileIntegrationsSection } from "@/components/integrations/integrations-menu";

type MobileSidebarLayoutNavigationProps = {
  quickActions?: ReactNode;
  homeCoversListings?: boolean;
  afterPrimary?: ReactNode;
  onNavigate: () => void;
  omitSections: Set<string>;
  omitDestinations: string[];
};

type MobileLayoutNodeProps = {
  quickActions?: ReactNode;
  homeCoversListings?: boolean;
  node: ProjectedSidebarNode;
  homeDestination?: ResolvedDestination;
  destinationHrefs: Map<string, string | undefined>;
  workspaceId?: string;
  canvasEntries: ShortcutCatalogEntry[];
  catalog: ReturnType<typeof useSidebarLayoutNavigation>["catalog"];
  omitSections: Set<string>;
  omitDestinations: string[];
  getActivity: ShortcutActivityReader;
  refreshActivity: () => void;
  onActivateShortcut: ShortcutActivation;
  onNavigate: () => void;
};

function resourceShortcuts(
  node: ProjectedSidebarNode,
  entries: ReturnType<typeof useSidebarLayoutNavigation>["catalog"]["catalog"],
  omitDestinations: string[],
): ProjectedSidebarNode {
  const shortcuts: ProjectedShortcut[] = entries
    .filter(
      (entry) => entry.target.kind !== "destination" || !omitDestinations.includes(entry.target.id),
    )
    .map((entry, index) => ({
      id: `${node.id}:${entry.target.kind}:${entry.target.id}:${index}`,
      target: entry.target,
      label: entry.label,
      icon: entry.icon ?? node.icon,
      ...(entry.href ? { href: entry.href } : {}),
      source: entry.source ?? "builtin",
      available: entry.available,
    }));
  return { ...node, shortcuts };
}

function filterNodeShortcuts(
  node: ProjectedSidebarNode,
  omitDestinations: string[],
): ProjectedSidebarNode {
  return {
    ...node,
    shortcuts: node.shortcuts.filter(
      (shortcut) =>
        shortcut.target.kind !== "destination" || !omitDestinations.includes(shortcut.target.id),
    ),
  };
}

function MobilePluginRow({
  node,
  href,
  onNavigate,
}: {
  node: ProjectedSidebarNode;
  href?: string;
  onNavigate: () => void;
}) {
  const Icon = node.icon;
  const content = (
    <>
      <Icon className="h-4 w-4 shrink-0" />
      <span className="min-w-0 flex-1 truncate text-left">{node.label}</span>
    </>
  );
  if (!href) {
    return (
      <Button variant="outline" disabled className="h-11 w-full justify-start gap-3 px-3">
        {content}
      </Button>
    );
  }
  return (
    <Button
      asChild
      variant="outline"
      className="h-11 w-full cursor-pointer justify-start gap-3 px-3"
    >
      <Link href={href} onClick={onNavigate} data-testid={`mobile-sidebar-plugin-${node.id}`}>
        {content}
      </Link>
    </Button>
  );
}

function MobileNewTaskRow({ onNavigate }: { onNavigate: () => void }) {
  const { t } = useTranslation();
  return (
    <Button
      type="button"
      variant="outline"
      className="h-11 w-full cursor-pointer justify-start gap-3 px-3"
      onClick={() => {
        onNavigate();
        requestNewTaskCreation();
      }}
    >
      <IconSquarePlus className="h-4 w-4 shrink-0" />
      {t("sidebar:newTask")}
    </Button>
  );
}

function MobileRequiredRows({
  onNavigate,
  omitSections,
  omitDestinations,
}: {
  onNavigate: () => void;
  omitSections: Set<string>;
  omitDestinations: string[];
}) {
  const { t } = useTranslation();
  const primary = useStaticDestinations("mobileMenu", "primary");
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const mode = useOfficeModeState();
  const needsYouEnabled = useFeature("needsYouInbox");
  const needsYouCount = useAppStore(selectNeedsYouInboxCount);
  const needsYouHasMore = useAppStore(selectNeedsYouInboxHasMore);
  const officeInboxCount = useAppStore(selectOfficeInboxCount);
  if (omitSections.has("primary")) return null;
  const fixedDestinations = primary.filter(
    (destination) =>
      (destination.id === "tasks" || destination.id === "threads") &&
      !omitDestinations.includes(destination.id),
  );
  if (fixedDestinations.length === 0 && mode !== "office" && !(needsYouEnabled && workspaceId))
    return null;
  return (
    <div className="flex flex-col gap-3" data-testid="mobile-sidebar-fixed-navigation">
      {fixedDestinations.length > 0 && (
        <DestinationRows
          destinations={fixedDestinations}
          onNavigate={onNavigate}
          className="h-11 gap-3 px-3 text-sm aria-[current=page]:bg-primary/10"
        />
      )}
      {mode === "office" && (
        <Button
          asChild
          variant="outline"
          className="h-11 w-full cursor-pointer justify-start gap-3 px-3"
        >
          <Link href="/office/inbox" onClick={onNavigate}>
            <IconInbox className="h-4 w-4 shrink-0" />
            <span className="flex-1 text-left">{t("sidebar:inbox")}</span>
            {officeInboxCount > 0 && <Badge>{officeInboxCount}</Badge>}
          </Link>
        </Button>
      )}
      {needsYouEnabled && workspaceId && (
        <Button
          asChild
          variant="outline"
          className="h-11 w-full cursor-pointer justify-start gap-3 px-3"
        >
          <Link href={NEEDS_YOU_INBOX_HREF} onClick={onNavigate}>
            <IconInbox className="h-4 w-4 shrink-0" />
            <span className="flex-1 text-left">
              {mode === "office" ? t("sidebar:needsYouInbox") : t("sidebar:inbox")}
            </span>
            {needsYouCount > 0 && <Badge>{`${needsYouCount}${needsYouHasMore ? "+" : ""}`}</Badge>}
          </Link>
        </Button>
      )}
    </div>
  );
}

function SavedMobileAutomationRows({
  node,
  catalog,
  workspaceId,
  getActivity,
  refreshActivity,
  onNavigate,
}: MobileLayoutNodeProps) {
  const { t } = useTranslation();
  const entries = catalog.catalog.filter((entry) => entry.target.kind === "automation");
  const activityError = catalog.automations.some((item) => getActivity(item.id)?.error);
  return (
    <div id="mobile-automations-body" className="space-y-2">
      {catalog.loading && (
        <p role="status" className="text-sm text-muted-foreground">
          {t("common:loading")}
        </p>
      )}
      {(catalog.error || activityError) && (
        <div role="alert" className="space-y-2 text-sm">
          <p>
            {catalog.error
              ? t("automations:failedToLoadAutomations")
              : t("automations:failedToLoadAutomationActivity")}
          </p>
          <Button
            variant="outline"
            className="h-11 cursor-pointer"
            onClick={() => {
              catalog.refresh();
              refreshActivity();
            }}
          >
            {t("automations:tryAgain")}
          </Button>
        </div>
      )}
      {!catalog.loading && !catalog.error && (
        <ShortcutRows
          shortcuts={resourceShortcuts(node, entries, []).shortcuts}
          activity={getActivity}
          mobile
          onNavigate={onNavigate}
        />
      )}
      <Button asChild variant="outline" className="h-11 w-full cursor-pointer justify-start px-3">
        <Link href={workspaceSettingsHref(workspaceId!, "automations")} onClick={onNavigate}>
          {t("automations:setUpAnAutomation")}
        </Link>
      </Button>
    </div>
  );
}

function MobilePhoneResource(props: MobileLayoutNodeProps) {
  const { node, workspaceId, canvasEntries, catalog, omitDestinations, onNavigate } = props;
  switch (node.destinationId) {
    case "automations":
      return workspaceId ? (
        <MobileAutomationsSection workspaceId={workspaceId} onNavigate={onNavigate}>
          <SavedMobileAutomationRows {...props} />
        </MobileAutomationsSection>
      ) : null;
    case "canvases":
      return workspaceId ? (
        <MobileCanvasesSection
          workspaceId={workspaceId}
          entries={canvasEntries}
          loading={catalog.canvasLoading}
          error={catalog.canvasError}
          onRetry={catalog.refresh}
          onNavigate={onNavigate}
        />
      ) : null;
    case "integrations":
      return (
        <MobileIntegrationsSection
          onNavigate={onNavigate}
          showSetup
          collapsible
          includePlugins={false}
          omitDestinations={omitDestinations}
        />
      );
    default:
      return null;
  }
}

function MobileBuiltinNode(props: MobileLayoutNodeProps) {
  const {
    node,
    homeCoversListings,
    homeDestination,
    quickActions,
    omitSections,
    omitDestinations,
    catalog,
    getActivity,
    onActivateShortcut,
    onNavigate,
  } = props;
  if (node.destinationId === "home") {
    if (omitSections.has("primary") || omitDestinations.includes("home")) return null;
    return homeDestination ? (
      <>
        <DestinationRows
          destinations={[homeDestination]}
          onNavigate={onNavigate}
          homeCoversListings={homeCoversListings}
          className="h-11 gap-3 px-3 text-sm aria-[current=page]:bg-primary/10"
        />
        {quickActions}
      </>
    ) : null;
  }
  if (node.destinationId === "new_task")
    return omitDestinations.includes("new_task") ? null : (
      <MobileNewTaskRow onNavigate={onNavigate} />
    );
  if (node.destinationId === "integrations" && omitSections.has("integrations")) return null;
  if (homeCoversListings) return <MobilePhoneResource {...props} />;

  const entries = catalog.catalog.filter((entry) => {
    if (node.destinationId === "automations") return entry.target.kind === "automation";
    if (node.destinationId === "canvases") return entry.target.kind === "canvas";
    return (
      node.destinationId === "integrations" &&
      entry.section === "integrations" &&
      entry.source !== "plugin"
    );
  });
  return (
    <ShortcutSection
      node={resourceShortcuts(node, entries, omitDestinations)}
      mobile
      getActivity={getActivity}
      onActivateShortcut={onActivateShortcut}
      onNavigate={onNavigate}
    />
  );
}

function MobileLayoutNode(props: MobileLayoutNodeProps) {
  const { node, omitSections, omitDestinations, destinationHrefs, onNavigate } = props;
  if (node.kind === "plugin") {
    if (omitSections.has(node.pluginSection ?? "plugins")) return null;
    return (
      <MobilePluginRow
        node={node}
        href={node.destinationId ? destinationHrefs.get(node.destinationId) : undefined}
        onNavigate={onNavigate}
      />
    );
  }
  if (node.kind === "shortcuts") {
    return (
      <ShortcutSection
        node={filterNodeShortcuts(node, omitDestinations)}
        mobile
        getActivity={props.getActivity}
        onActivateShortcut={props.onActivateShortcut}
        onNavigate={onNavigate}
      />
    );
  }
  return <MobileBuiltinNode {...props} />;
}

export function MobileSidebarLayoutNavigation({
  quickActions,
  homeCoversListings,
  afterPrimary,
  onNavigate,
  omitSections,
  omitDestinations,
}: MobileSidebarLayoutNavigationProps) {
  const { workspaceId, catalog, projection, activity } = useSidebarLayoutNavigation({
    active: true,
  });
  const workspaceMode = useOfficeModeState();
  const openQuickChat = useQuickChatLauncher(workspaceId);
  const openQuickTerminal = useQuickTerminalLauncher(workspaceId);
  const primary = useStaticDestinations("mobileMenu", "primary");
  const destinationHrefs = useMemo(
    () =>
      new Map(
        catalog.catalog
          .filter((entry) => entry.target.kind === "destination")
          .map((entry) => [entry.target.id, entry.href]),
      ),
    [catalog.catalog],
  );
  const activateShortcut: ShortcutActivation = useCallback(
    (shortcut) => {
      if (shortcut.target.kind !== "host_action") return;
      onNavigate();
      if (shortcut.target.id === "new_task") requestNewTaskCreation();
      if (shortcut.target.id === "quick_chat") openQuickChat();
      if (shortcut.target.id === "quick_terminal") void openQuickTerminal();
    },
    [onNavigate, openQuickChat, openQuickTerminal],
  );
  // Phone workspace tools follow task navigation in their saved relative order.
  // This presentation never writes over the desktop layout preference.
  const visibleNodes = projection.nodes.filter(
    (node) =>
      node.visible &&
      !(
        homeCoversListings &&
        workspaceMode === "kanban" &&
        node.kind === "builtin" &&
        node.destinationId === "new_task"
      ),
  );
  const hasVisibleHome =
    !omitSections.has("primary") &&
    !omitDestinations.includes("home") &&
    visibleNodes.some((node) => node.destinationId === "home");
  const homeDestination = primary.find((destination) => destination.id === "home");
  const canvasEntries = catalog.catalog.filter((entry) => entry.target.kind === "canvas");
  const beforeTasks = homeCoversListings
    ? visibleNodes.filter((node) => node.destinationId === "home")
    : visibleNodes;
  const afterTasks = homeCoversListings
    ? visibleNodes.filter((node) => node.destinationId !== "home")
    : [];
  const renderNode = (node: ProjectedSidebarNode) => (
    <MobileLayoutNode
      key={node.id}
      node={node}
      homeDestination={homeDestination}
      homeCoversListings={homeCoversListings}
      quickActions={quickActions}
      destinationHrefs={destinationHrefs}
      workspaceId={workspaceId}
      canvasEntries={canvasEntries}
      catalog={catalog}
      omitSections={omitSections}
      omitDestinations={omitDestinations}
      getActivity={activity.getActivity}
      refreshActivity={activity.refresh}
      onActivateShortcut={activateShortcut}
      onNavigate={onNavigate}
    />
  );

  return (
    <div
      className="flex min-w-0 flex-col gap-4 md:gap-6"
      data-testid="mobile-sidebar-layout-navigation"
    >
      <div className="flex min-w-0 flex-col gap-2 md:gap-3">
        {!hasVisibleHome && quickActions}
        {beforeTasks.map(renderNode)}
        <MobileRequiredRows
          onNavigate={onNavigate}
          omitSections={omitSections}
          omitDestinations={omitDestinations}
        />
      </div>
      {afterPrimary}
      {afterTasks.length > 0 && (
        <div className="flex min-w-0 flex-col gap-3">{afterTasks.map(renderNode)}</div>
      )}
    </div>
  );
}
