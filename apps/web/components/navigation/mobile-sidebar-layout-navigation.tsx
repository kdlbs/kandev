"use client";

import { useCallback, useMemo } from "react";
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
import type { ShortcutActivation } from "@/components/app-sidebar/shortcut-section-actions";
import {
  selectNeedsYouInboxCount,
  selectNeedsYouInboxHasMore,
} from "@/lib/state/slices/needs-you-inbox/selectors";
import { selectOfficeInboxCount } from "@/lib/state/slices/office/selectors";
import { NEEDS_YOU_INBOX_HREF } from "@/lib/navigation/needs-you-inbox-destination";
import { DestinationRows } from "./destination-rows";

const INTEGRATION_DESTINATION_IDS = new Set(["azure-devops", "github", "gitlab", "jira", "linear"]);

type MobileSidebarLayoutNavigationProps = {
  onNavigate: () => void;
  omitSections: Set<string>;
  omitDestinations: string[];
};

type MobileLayoutNodeProps = {
  node: ProjectedSidebarNode;
  homeDestination?: ResolvedDestination;
  destinationHrefs: Map<string, string | undefined>;
  integrationEntries: ShortcutCatalogEntry[];
  canvasEntries: ShortcutCatalogEntry[];
  automationEntries: ShortcutCatalogEntry[];
  omitSections: Set<string>;
  omitDestinations: string[];
  getActivity: ShortcutActivityReader;
  onActivateShortcut: ShortcutActivation;
  onNavigate: () => void;
};

function resourceShortcuts(
  node: ProjectedSidebarNode,
  entries: ReturnType<typeof useSidebarLayoutNavigation>["catalog"]["catalog"],
): ProjectedSidebarNode {
  const shortcuts: ProjectedShortcut[] = entries.map((entry, index) => ({
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
  return (
    <div className="flex flex-col gap-3" data-testid="mobile-sidebar-fixed-navigation">
      {fixedDestinations.length > 0 && (
        <DestinationRows
          destinations={fixedDestinations}
          onNavigate={onNavigate}
          className="h-11 gap-3 px-3 text-sm"
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

function MobileLayoutNode({
  node,
  homeDestination,
  destinationHrefs,
  integrationEntries,
  canvasEntries,
  automationEntries,
  omitSections,
  omitDestinations,
  getActivity,
  onActivateShortcut,
  onNavigate,
}: MobileLayoutNodeProps) {
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
        node={node}
        mobile
        getActivity={getActivity}
        onActivateShortcut={onActivateShortcut}
        onNavigate={onNavigate}
      />
    );
  }
  switch (node.destinationId) {
    case "home":
      if (omitSections.has("primary") || omitDestinations.includes("home")) return null;
      return homeDestination ? (
        <DestinationRows
          destinations={[homeDestination]}
          onNavigate={onNavigate}
          className="h-11 gap-3 px-3 text-sm"
        />
      ) : null;
    case "new_task":
      return omitDestinations.includes("new_task") ? null : (
        <MobileNewTaskRow onNavigate={onNavigate} />
      );
    case "automations":
      return (
        <ShortcutSection
          node={resourceShortcuts(node, automationEntries)}
          mobile
          getActivity={getActivity}
          onActivateShortcut={onActivateShortcut}
          onNavigate={onNavigate}
        />
      );
    case "canvases":
      return (
        <ShortcutSection
          node={resourceShortcuts(node, canvasEntries)}
          mobile
          getActivity={getActivity}
          onActivateShortcut={onActivateShortcut}
          onNavigate={onNavigate}
        />
      );
    case "integrations":
      return (
        <ShortcutSection
          node={resourceShortcuts(node, integrationEntries)}
          mobile
          getActivity={getActivity}
          onActivateShortcut={onActivateShortcut}
          onNavigate={onNavigate}
        />
      );
    default:
      return null;
  }
}

export function MobileSidebarLayoutNavigation({
  onNavigate,
  omitSections,
  omitDestinations,
}: MobileSidebarLayoutNavigationProps) {
  const { workspaceId, catalog, projection, activity } = useSidebarLayoutNavigation({
    active: true,
  });
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
  // The phone menu keeps the same destinations in Office and kanban modes.
  // Office-specific required rows are added below, while user-selected
  // visibility and order still come from the layout projection.
  const visibleNodes = projection.nodes.filter((node) => node.visible);
  const homeDestination = primary.find((destination) => destination.id === "home");
  const integrationEntries = catalog.catalog.filter(
    (entry) =>
      entry.target.kind === "destination" &&
      entry.source !== "plugin" &&
      INTEGRATION_DESTINATION_IDS.has(entry.target.id),
  );
  const canvasEntries = catalog.catalog.filter((entry) => entry.target.kind === "canvas");
  const automationEntries = catalog.catalog.filter((entry) => entry.target.kind === "automation");

  return (
    <div className="flex min-w-0 flex-col gap-3" data-testid="mobile-sidebar-layout-navigation">
      {visibleNodes.map((node) => (
        <MobileLayoutNode
          key={node.id}
          node={node}
          homeDestination={homeDestination}
          destinationHrefs={destinationHrefs}
          integrationEntries={integrationEntries}
          canvasEntries={canvasEntries}
          automationEntries={automationEntries}
          omitSections={omitSections}
          omitDestinations={omitDestinations}
          getActivity={activity.getActivity}
          onActivateShortcut={activateShortcut}
          onNavigate={onNavigate}
        />
      ))}
      <MobileRequiredRows
        onNavigate={onNavigate}
        omitSections={omitSections}
        omitDestinations={omitDestinations}
      />
    </div>
  );
}
