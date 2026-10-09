"use client";

import { useTranslation } from "react-i18next";
import { IconChevronDown, IconChevronRight, IconListDetails } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import Link from "@/components/routing/app-link";
import { useAppStore } from "@/components/state-provider";
import { APP_SIDEBAR_SECTION_IDS } from "@/components/app-sidebar/app-sidebar-constants";
import { useCoordinatorSidebarEntries } from "@/app/coordinator/use-coordinator-sidebar-entries";
import { useFeature } from "@/hooks/domains/features/use-feature";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import { CoordinatorIcon } from "@/lib/coordinator/icon";
import {
  linkToCoordinator,
  linkToCoordinatorNeedsYou,
  linkToCoordinatorSettingsList,
} from "@/lib/coordinator/links";

const BODY_ID = "mobile-coordinators-body";

function CoordinatorRows({
  workspaceId,
  coordinators,
  badgeByCoordinatorId,
  onNavigate,
}: {
  workspaceId: string;
  coordinators: Coordinator[];
  badgeByCoordinatorId: Map<string, number>;
  onNavigate: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div id={BODY_ID} className="flex flex-col gap-2">
      {coordinators.length === 0 ? (
        <Button asChild variant="outline" className="h-11 cursor-pointer justify-start px-3">
          <Link
            href={linkToCoordinatorSettingsList(workspaceId)}
            onClick={onNavigate}
            data-testid="mobile-sidebar-coordinators-empty"
          >
            {t("coordinator:setUpCoordinator")}
          </Link>
        </Button>
      ) : (
        coordinators.map((coordinator) => {
          const badge = badgeByCoordinatorId.get(coordinator.id) ?? 0;
          return (
            <Button
              key={coordinator.id}
              asChild
              variant="outline"
              className="h-11 w-full cursor-pointer justify-start gap-3 px-3"
            >
              <Link
                href={linkToCoordinatorNeedsYou(workspaceId, coordinator.id)}
                onClick={onNavigate}
                data-testid={`mobile-sidebar-coordinator-${coordinator.id}`}
              >
                <CoordinatorIcon className="h-4 w-4 shrink-0" />
                <span className="flex-1 truncate text-left">{coordinator.name}</span>
                {badge > 0 && <Badge>{badge}</Badge>}
              </Link>
            </Button>
          );
        })
      )}
    </div>
  );
}

function MobileCoordinatorsSectionBody({
  workspaceId,
  onNavigate,
}: {
  workspaceId: string;
  onNavigate: () => void;
}) {
  const { t } = useTranslation();
  const expanded = useAppStore(
    (s) => s.appSidebar.sectionExpanded[APP_SIDEBAR_SECTION_IDS.coordinators] ?? true,
  );
  const toggleSection = useAppStore((s) => s.toggleAppSidebarSection);
  const { coordinators, badgeByCoordinatorId } = useCoordinatorSidebarEntries(workspaceId);
  if (!coordinators) return null;

  let badgeSum = 0;
  for (const coordinator of coordinators) badgeSum += badgeByCoordinatorId.get(coordinator.id) ?? 0;
  const summary = badgeSum > 0 ? badgeSum : coordinators.length;

  return (
    <section
      className="flex flex-col gap-2 border-t border-border pt-2"
      data-testid="mobile-coordinators-section"
    >
      <div className="flex items-center gap-2">
        <Button
          variant="ghost"
          className="cursor-pointer h-11 min-w-11 flex-1 justify-start gap-2 px-0 text-sm font-medium hover:bg-transparent aria-expanded:bg-transparent"
          aria-expanded={expanded}
          aria-controls={BODY_ID}
          onClick={() => toggleSection(APP_SIDEBAR_SECTION_IDS.coordinators, true)}
        >
          {t("coordinator:sidebarSectionLabel")}
          {!expanded && summary > 0 && (
            <span
              className="text-xs font-normal tabular-nums text-muted-foreground"
              data-testid="mobile-coordinators-collapsed-summary"
            >
              {summary}
            </span>
          )}
          {expanded ? (
            <IconChevronDown className="size-3.5 text-muted-foreground" />
          ) : (
            <IconChevronRight className="size-3.5 text-muted-foreground" />
          )}
        </Button>
        <Button
          asChild
          variant="ghost"
          className="cursor-pointer size-11 shrink-0 text-muted-foreground"
        >
          <Link
            href={linkToCoordinator(workspaceId)}
            onClick={onNavigate}
            aria-label={t("coordinator:openCoordinators")}
            data-testid="mobile-coordinators-open-list"
          >
            <IconListDetails className="size-4" />
          </Link>
        </Button>
      </div>
      {expanded && (
        <CoordinatorRows
          workspaceId={workspaceId}
          coordinators={coordinators}
          badgeByCoordinatorId={badgeByCoordinatorId}
          onNavigate={onNavigate}
        />
      )}
    </section>
  );
}

/**
 * The coordinator phone navigation section (AC-COORDINATOR-NEEDS-YOU-006.1):
 * the mobile counterpart of the desktop `CoordinatorsSection`. Renders nothing
 * until the coordinator list has loaded.
 */
export function MobileCoordinatorsSection({ onNavigate }: { onNavigate: () => void }) {
  const enabled = useFeature("coordinator");
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  if (!enabled || !workspaceId) return null;
  return (
    <MobileCoordinatorsSectionBody
      key={workspaceId}
      workspaceId={workspaceId}
      onNavigate={onNavigate}
    />
  );
}
