"use client";

import { useTranslation } from "react-i18next";
import { CoordinatorIcon } from "@/lib/coordinator/icon";
import { linkToCoordinator, linkToCoordinatorNeedsYou } from "@/lib/coordinator/links";
import { useCoordinatorSidebarEntries } from "@/app/coordinator/use-coordinator-sidebar-entries";
import { AppSidebarNavItem } from "./app-sidebar-nav-item";

/**
 * The coordinator sidebar entries (AC-COORDINATOR-NEEDS-YOU-006.1/.2/.3): one
 * row per coordinator with its open-proposal badge, in list order, or one
 * generic "Coordinator" row with no badge when the workspace has none. Rows
 * render only once the list has loaded, to avoid flashing between the two
 * states (docs/specs/coordinator/system-design/needs-you.md
 * #routes-and-sidebar).
 */
export function AppSidebarCoordinatorRows({
  workspaceId,
  collapsed,
}: {
  workspaceId: string;
  collapsed: boolean;
}) {
  const { t } = useTranslation();
  const { coordinators, badgeByCoordinatorId } = useCoordinatorSidebarEntries(workspaceId);

  if (!coordinators) return null;

  if (coordinators.length === 0) {
    return (
      <AppSidebarNavItem
        icon={CoordinatorIcon}
        label={t("coordinator:sidebarGenericEntry")}
        href={linkToCoordinator(workspaceId)}
        collapsed={collapsed}
        testId="sidebar-coordinator-generic"
      />
    );
  }

  return (
    <>
      {coordinators.map((coordinator) => (
        <AppSidebarNavItem
          key={coordinator.id}
          icon={CoordinatorIcon}
          label={coordinator.name}
          href={linkToCoordinatorNeedsYou(workspaceId, coordinator.id)}
          badge={badgeByCoordinatorId.get(coordinator.id)}
          collapsed={collapsed}
          testId={`sidebar-coordinator-${coordinator.id}`}
        />
      ))}
    </>
  );
}
