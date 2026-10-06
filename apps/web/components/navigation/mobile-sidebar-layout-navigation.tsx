"use client";

import { useCallback, useMemo, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconInbox } from "@tabler/icons-react";
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
import { MobileSidebarCustomization } from "./mobile-sidebar-customization";
import { MobileNewTaskRow } from "./mobile-new-task-row";
import { DestinationRows } from "./destination-rows";
import { MobileAutomationsSection } from "./mobile-automations-section";
import { MobileCoordinatorsSection } from "./mobile-coordinators-section";
import { MobileCanvasesSection } from "./mobile-canvases-section";
import { MobileIntegrationsSection } from "@/components/integrations/integrations-menu";

type MobileSidebarLayoutNavigationProps = {
  quickActions?: ReactNode;
  homeCoversListings?: boolean;
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

function MobileRequiredRows({
  onNavigate,
  omitSections,
  omitDestinations,
  inboxKind = "none",
  coordinatorsWithAutomations = false,
}: {
  onNavigate: () => void;
  omitSections: Set<string>;
  omitDestinations: string[];
  inboxKind?: "office" | "needs-you" | "none";
  coordinatorsWithAutomations?: boolean;
}) {
  const primary = useStaticDestinations("mobileMenu", "primary");
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const coordinatorEnabled = useFeature("coordinator") && !coordinatorsWithAutomations;
  if (omitSections.has("primary")) return null;
  if (inboxKind !== "none") return <MobileInboxRow kind={inboxKind} onNavigate={onNavigate} />;
  const fixedDestinations = primary.filter(
    (destination) =>
      (destination.id === "tasks" || destination.id === "threads") &&
      !omitDestinations.includes(destination.id),
  );
  if (!fixedDestinations.length && !(coordinatorEnabled && workspaceId)) return null;
  return (
    <div className="flex flex-col gap-3" data-testid="mobile-sidebar-fixed-navigation">
      {fixedDestinations.length > 0 && (
        <DestinationRows
          destinations={fixedDestinations}
          onNavigate={onNavigate}
          className="h-11 gap-3 px-3 text-sm aria-[current=page]:bg-primary/10"
        />
      )}
      {coordinatorEnabled && workspaceId && <MobileCoordinatorsSection onNavigate={onNavigate} />}
    </div>
  );
}

function MobileInboxRow({
  kind,
  onNavigate,
}: {
  kind: "office" | "needs-you";
  onNavigate: () => void;
}) {
  const { t } = useTranslation();
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const mode = useOfficeModeState();
  const enabled = useFeature("needsYouInbox");
  const count = useAppStore(selectNeedsYouInboxCount);
  const hasMore = useAppStore(selectNeedsYouInboxHasMore);
  const officeCount = useAppStore(selectOfficeInboxCount);
  const office = kind === "office";
  if (office && mode !== "office") return null;
  if (!office && (!enabled || !workspaceId)) return null;
  const label = !office && mode === "office" ? t("sidebar:needsYouInbox") : t("sidebar:inbox");
  const badge = office ? officeCount : count;
  const suffix = !office && hasMore ? "+" : "";
  return (
    <Button
      asChild
      variant="outline"
      className="h-11 w-full cursor-pointer justify-start gap-3 px-3"
    >
      <Link href={office ? "/office/inbox" : NEEDS_YOU_INBOX_HREF} onClick={onNavigate}>
        <IconInbox className="h-4 w-4 shrink-0" />
        <span className="flex-1 text-left">{label}</span>
        {badge > 0 && <Badge>{`${badge}${suffix}`}</Badge>}
      </Link>
    </Button>
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
    <div className="space-y-2">
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

function MobileBuiltinNodeContent(props: MobileLayoutNodeProps) {
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
  if (node.destinationId === "inbox" || node.destinationId === "needs_you_inbox")
    return (
      <MobileRequiredRows
        inboxKind={node.destinationId === "inbox" ? "office" : "needs-you"}
        onNavigate={onNavigate}
        omitSections={omitSections}
        omitDestinations={omitDestinations}
      />
    );
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

function MobileBuiltinNode(props: MobileLayoutNodeProps) {
  if (props.node.destinationId !== "automations") return <MobileBuiltinNodeContent {...props} />;
  return (
    <>
      <MobileCoordinatorsSection onNavigate={props.onNavigate} />
      <MobileBuiltinNodeContent {...props} />
    </>
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
  // The phone projection keeps saved tools in order without rewriting desktop preferences.
  const visibleNodes = projection.nodes.filter((node) => node.visible);
  const hasVisibleHome =
    !omitSections.has("primary") &&
    !omitDestinations.includes("home") &&
    visibleNodes.some((node) => node.destinationId === "home");
  const homeDestination = primary.find((destination) => destination.id === "home");
  const canvasEntries = catalog.catalog.filter((entry) => entry.target.kind === "canvas");
  const phoneMain = homeCoversListings && workspaceMode === "kanban";
  const isPrimary = (node: ProjectedSidebarNode) =>
    node.kind === "builtin" && (node.destinationId === "home" || node.destinationId === "new_task");
  const primaryNodes = phoneMain
    ? [
        ...visibleNodes.filter((node) => isPrimary(node) && node.destinationId === "new_task"),
        ...visibleNodes.filter((node) => isPrimary(node) && node.destinationId === "home"),
      ]
    : visibleNodes;
  const toolNodes = phoneMain ? visibleNodes.filter((node) => !isPrimary(node)) : [];
  const coordinatorsWithAutomations = visibleNodes.some(
    (node) => node.destinationId === "automations",
  );
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
        {primaryNodes.map(renderNode)}
        <MobileRequiredRows
          onNavigate={onNavigate}
          omitSections={omitSections}
          omitDestinations={omitDestinations}
          coordinatorsWithAutomations={coordinatorsWithAutomations}
        />
      </div>
      <MobileSidebarCustomization
        nodes={projection.nodes}
        catalog={catalog.catalog}
        onNavigate={onNavigate}
      />
      {toolNodes.length > 0 && (
        <div className="flex min-w-0 flex-col gap-3">{toolNodes.map(renderNode)}</div>
      )}
    </div>
  );
}
