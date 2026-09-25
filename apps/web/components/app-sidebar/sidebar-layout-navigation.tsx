"use client";

import { Fragment, useCallback, useMemo } from "react";
import { AppSidebarNavItem } from "./app-sidebar-nav-item";
import { AppSidebarFixedNav, AppSidebarHomeItem } from "./app-sidebar-primary-nav";
import { AppSidebarNewTaskItem } from "./app-sidebar-new-task-item";
import { AutomationsSection } from "./sections/automations-section";
import { CanvasesSection } from "./sections/canvases-section";
import { IntegrationsSection } from "./sections/integrations-section";
import { ShortcutSection } from "./shortcut-section";
import { useQuickChatLauncher } from "@/hooks/use-quick-chat-launcher";
import { useQuickTerminalLauncher } from "@/hooks/use-quick-terminal-launcher";
import { useSidebarLayoutNavigation } from "@/hooks/domains/sidebar/use-sidebar-layout-navigation";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { requestNewTaskCreation } from "@/lib/desktop/new-task-request";
import type { ProjectedSidebarNode, ProjectedShortcut } from "@/lib/sidebar/layout-projection";

type SidebarLayoutNavigationProps = {
  collapsed: boolean;
  inOffice: boolean;
};

function LayoutPluginItem({
  node,
  href,
  collapsed,
}: {
  node: ProjectedSidebarNode;
  href?: string;
  collapsed: boolean;
}) {
  return (
    <AppSidebarNavItem
      icon={node.icon}
      label={node.label}
      href={href}
      disabled={!node.destinationId || !href}
      collapsed={collapsed}
      testId={`plugin-nav-item-${node.destinationId ?? node.id}`}
    />
  );
}

function builtinNode(node: ProjectedSidebarNode, collapsed: boolean): React.ReactNode {
  switch (node.destinationId) {
    case "home":
      return <AppSidebarHomeItem collapsed={collapsed} />;
    case "new_task":
      return <AppSidebarNewTaskItem collapsed={collapsed} />;
    case "automations":
      return <AutomationsSection collapsed={collapsed} />;
    case "canvases":
      return <CanvasesSection collapsed={collapsed} />;
    case "integrations":
      return <IntegrationsSection collapsed={collapsed} includePluginItems={false} />;
    default:
      return null;
  }
}

export function SidebarLayoutNavigation({ collapsed, inOffice }: SidebarLayoutNavigationProps) {
  const { isMobile } = useResponsiveBreakpoint();
  const { workspaceId, catalog, projection, activity } = useSidebarLayoutNavigation({
    active: !isMobile,
  });
  const openQuickChat = useQuickChatLauncher(workspaceId);
  const openQuickTerminal = useQuickTerminalLauncher(workspaceId);
  const destinationHrefs = useMemo(
    () =>
      new Map(
        catalog.catalog
          .filter((entry) => entry.target.kind === "destination")
          .map((entry) => [entry.target.id, entry.href]),
      ),
    [catalog.catalog],
  );

  const activateShortcut = useCallback(
    (shortcut: ProjectedShortcut) => {
      if (shortcut.target.kind !== "host_action") return;
      if (shortcut.target.id === "new_task") requestNewTaskCreation();
      if (shortcut.target.id === "quick_chat") openQuickChat();
      if (shortcut.target.id === "quick_terminal") void openQuickTerminal();
    },
    [openQuickChat, openQuickTerminal],
  );

  const visibleNodes = projection.nodes.filter(
    (node) => node.visible && (!inOffice || !isKanbanOnlyNode(node)),
  );
  const hasVisibleNewTask = visibleNodes.some((node) => node.destinationId === "new_task");
  let fixedRendered = false;
  const items: React.ReactNode[] = [];

  for (const node of visibleNodes) {
    if (!fixedRendered && node.destinationId !== "home") {
      items.push(<AppSidebarFixedNav key="sidebar-fixed-navigation" collapsed={collapsed} />);
      fixedRendered = true;
    }
    if (node.kind === "builtin") {
      items.push(<Fragment key={node.id}>{builtinNode(node, collapsed)}</Fragment>);
      continue;
    }
    if (node.kind === "plugin") {
      items.push(
        <LayoutPluginItem
          key={node.id}
          node={node}
          href={node.destinationId ? destinationHrefs.get(node.destinationId) : undefined}
          collapsed={collapsed}
        />,
      );
      continue;
    }
    items.push(
      <ShortcutSection
        key={node.id}
        node={node}
        collapsed={collapsed}
        getActivity={activity.getActivity}
        onActivateShortcut={activateShortcut}
      />,
    );
  }

  if (!fixedRendered) {
    items.push(<AppSidebarFixedNav key="sidebar-fixed-navigation" collapsed={collapsed} />);
  }
  if (!hasVisibleNewTask) {
    items.push(
      <div key="sidebar-hidden-new-task-host" className="hidden" aria-hidden="true">
        <AppSidebarNewTaskItem collapsed={collapsed} />
      </div>,
    );
  }

  return <div className="flex flex-col gap-1">{items}</div>;
}

function isKanbanOnlyNode(node: ProjectedSidebarNode): boolean {
  return (
    node.destinationId === "automations" ||
    node.destinationId === "canvases" ||
    node.destinationId === "integrations"
  );
}
