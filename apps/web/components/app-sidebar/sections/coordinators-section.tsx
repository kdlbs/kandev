"use client";

import { useTranslation } from "react-i18next";
import { IconListDetails } from "@tabler/icons-react";
import Link from "@/components/routing/app-link";
import { Badge } from "@kandev/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { useCoordinatorSidebarEntries } from "@/app/coordinator/use-coordinator-sidebar-entries";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { CoordinatorIcon } from "@/lib/coordinator/icon";
import {
  linkToCoordinator,
  linkToCoordinatorNeedsYou,
  linkToCoordinatorSettingsList,
} from "@/lib/coordinator/links";
import { usePathname } from "@/lib/routing/client-router";
import { cn } from "@/lib/utils";
import {
  APP_SIDEBAR_SECTION_IDS,
  SIDEBAR_ITEM_ACTIVE,
  SIDEBAR_ITEM_INACTIVE,
} from "../app-sidebar-constants";
import { AppSidebarSection } from "../app-sidebar-section";

function OpenListShortcut({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Link
          href={linkToCoordinator(workspaceId)}
          aria-label={t("coordinator:openCoordinators")}
          data-testid="coordinators-open-list"
          className="flex h-5 w-5 items-center justify-center rounded text-muted-foreground/70 hover:bg-muted/60 hover:text-foreground cursor-pointer transition-colors"
        >
          <IconListDetails className="h-3.5 w-3.5" />
        </Link>
      </TooltipTrigger>
      <TooltipContent side="right">{t("coordinator:openCoordinators")}</TooltipContent>
    </Tooltip>
  );
}

function CoordinatorRowLink({
  href,
  id,
  name,
  badge,
  active,
}: {
  href: string;
  id: string;
  name: string;
  badge: number;
  active: boolean;
}) {
  return (
    <Link
      href={href}
      data-testid={`sidebar-coordinator-${id}`}
      className={cn(
        "flex items-center gap-2.5 px-2.5 py-1.5 text-[13px] font-medium rounded-md cursor-pointer",
        active ? SIDEBAR_ITEM_ACTIVE : SIDEBAR_ITEM_INACTIVE,
      )}
    >
      <span className="min-w-0 flex-1 truncate">{name}</span>
      {badge > 0 && (
        <Badge className="rounded-full bg-primary px-1.5 py-0.5 text-xs text-primary-foreground">
          {badge}
        </Badge>
      )}
    </Link>
  );
}

function EmptyRow({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  return (
    <Link
      href={linkToCoordinatorSettingsList(workspaceId)}
      data-testid="sidebar-coordinators-empty"
      className="px-2.5 py-1.5 text-[13px] rounded-md cursor-pointer text-muted-foreground hover:bg-muted/60 hover:text-foreground"
    >
      {t("coordinator:setUpCoordinator")}
    </Link>
  );
}

function CoordinatorsSectionBody({
  workspaceId,
  collapsed,
}: {
  workspaceId: string;
  collapsed: boolean;
}) {
  const { t } = useTranslation();
  const pathname = usePathname();
  const { coordinators, badgeByCoordinatorId } = useCoordinatorSidebarEntries(workspaceId);

  if (!coordinators) return null;

  let badgeSum = 0;
  for (const coordinator of coordinators) badgeSum += badgeByCoordinatorId.get(coordinator.id) ?? 0;
  const summary = badgeSum > 0 ? badgeSum : coordinators.length;

  return (
    <AppSidebarSection
      id={APP_SIDEBAR_SECTION_IDS.coordinators}
      label={t("coordinator:sidebarSectionLabel")}
      collapsed={collapsed}
      icon={CoordinatorIcon}
      headerAction={<OpenListShortcut workspaceId={workspaceId} />}
      headerActionVisibility="always"
      defaultExpanded={true}
      collapsedSummary={summary > 0 ? summary : undefined}
    >
      {coordinators.length === 0 ? (
        <EmptyRow workspaceId={workspaceId} />
      ) : (
        coordinators.map((coordinator) => {
          const href = linkToCoordinatorNeedsYou(workspaceId, coordinator.id);
          return (
            <CoordinatorRowLink
              key={coordinator.id}
              id={coordinator.id}
              href={href}
              name={coordinator.name}
              badge={badgeByCoordinatorId.get(coordinator.id) ?? 0}
              active={pathname === href || pathname.startsWith(`${href}/`)}
            />
          );
        })
      )}
    </AppSidebarSection>
  );
}

/**
 * The coordinator sidebar entries (AC-COORDINATOR-NEEDS-YOU-006.1/.2/.3): an
 * expandable section with one row per coordinator and its open-proposal badge,
 * or an empty row when the workspace has none. Renders only once the list has
 * loaded, to avoid flashing between the two states.
 */
export function CoordinatorsSection({ collapsed }: { collapsed: boolean }) {
  const enabled = useFeature("coordinator");
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  if (!enabled || !workspaceId) return null;
  return <CoordinatorsSectionBody workspaceId={workspaceId} collapsed={collapsed} />;
}
