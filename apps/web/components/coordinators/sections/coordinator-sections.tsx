"use client";

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useCoordinatorSection } from "@/hooks/domains/coordinator/use-coordinator-section";
import { useControlDraft } from "@/hooks/domains/coordinator/use-control-draft";
import { WatchesNoneNotice } from "@/app/coordinator/components/watches-none-notice";
import { GoalSection } from "./goal-section";
import { ControlError } from "./control-error";
import { MayDoSection } from "./may-do-section";
import { WatchesSection } from "./watches-section";
import { SectionsRow, type CoordinatorSectionEntry } from "./sections-row";
import { StandingOrdersSection } from "./standing-orders-section";

const SLUGS = ["identity", "watches", "may-do", "standing-orders", "goal"];

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
  const control = useControlDraft({ workspaceId, coordinatorId, canManage });
  const { selectSection } = useCoordinatorSection(SLUGS);
  const stored = control.stored?.watches;
  const entries: CoordinatorSectionEntry[] = [
    {
      slug: "identity",
      label: t("coordinator:sectionIdentity"),
      help: t("coordinator:sectionIdentityHelp"),
      render: () => identity,
    },
    {
      slug: "watches",
      label: t("coordinator:sectionWatches"),
      help: t("coordinator:sectionWatchesHelp"),
      render: () => (
        <>
          <WatchesSection workspaceId={workspaceId} canManage={canManage} control={control} />
          <ControlError error={control.fieldError} />
        </>
      ),
    },
    {
      slug: "may-do",
      label: t("coordinator:sectionMayDo"),
      help: t("coordinator:sectionMayDoHelp"),
      render: () => (
        <>
          <MayDoSection
            workspaceId={workspaceId}
            coordinatorId={coordinatorId}
            canManage={canManage}
            control={control}
          />
          <ControlError error={control.fieldError} />
        </>
      ),
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
  return (
    <>
      <WatchesNoneNotice
        workspaceId={workspaceId}
        coordinatorId={coordinatorId}
        watchSet={stored && { scope: stored.scope, workflowIds: stored.workflowIds }}
        onChooseBoards={canManage ? () => selectSection("watches") : undefined}
      />
      <SectionsRow entries={entries} />
    </>
  );
}
