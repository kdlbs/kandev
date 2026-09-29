"use client";

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { GoalSection } from "./goal-section";
import { SectionsRow, type CoordinatorSectionEntry } from "./sections-row";
import { StandingOrdersSection } from "./standing-orders-section";

type CoordinatorSectionsProps = {
  workspaceId: string;
  coordinatorId: string;
  canManage: boolean;
  identity: ReactNode;
};

/** The phase-2 coordinator page body: Identity holds the phase-1 fields unchanged. */
export function CoordinatorSections({
  workspaceId,
  coordinatorId,
  canManage,
  identity,
}: CoordinatorSectionsProps) {
  const { t } = useTranslation();
  const entries: CoordinatorSectionEntry[] = [
    {
      slug: "identity",
      label: t("coordinator:sectionIdentity"),
      help: t("coordinator:sectionIdentityHelp"),
      render: () => identity,
    },
    {
      slug: "standing-orders",
      label: t("coordinator:sectionStandingOrders"),
      help: t("coordinator:sectionStandingOrdersHelp"),
      render: () => (
        <StandingOrdersSection
          workspaceId={workspaceId}
          coordinatorId={coordinatorId}
          canManage={canManage}
        />
      ),
    },
    {
      slug: "goal",
      label: t("coordinator:sectionGoal"),
      help: t("coordinator:sectionGoalHelp"),
      render: () => (
        <GoalSection
          workspaceId={workspaceId}
          coordinatorId={coordinatorId}
          canManage={canManage}
        />
      ),
    },
  ];
  return <SectionsRow entries={entries} />;
}
