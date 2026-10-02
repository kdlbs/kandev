"use client";

import type { ComponentProps, ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useCoordinatorSection } from "@/hooks/domains/coordinator/use-coordinator-section";
import { useControlDraft } from "@/hooks/domains/coordinator/use-control-draft";
import { useCoordinatorPhase3Effective } from "@/hooks/domains/settings/use-coordinator-phase3-effective";
import { useCoordinatorPhase31Effective } from "@/hooks/domains/settings/use-coordinator-phase31-effective";
import { LearningSection } from "@/components/coordinators/learning/learning-section";
import type { ControlDraft } from "@/lib/coordinators/control-draft";
import type { WatchSet } from "@/lib/coordinator/watch-filter";
import { WatchesNoneNotice } from "@/app/coordinator/components/watches-none-notice";
import { AutonomySection } from "./autonomy-section";
import { ChangesWaiting } from "./changes-waiting";
import { GoalSection } from "./goal-section";
import { ControlError } from "./control-error";
import { MayDoSection } from "./may-do-section";
import { WatchesSection } from "./watches-section";
import { SectionsRow, type CoordinatorSectionEntry } from "./sections-row";
import { StandingOrdersSection } from "./standing-orders-section";

const BASE_SLUGS = ["identity", "watches", "may-do", "standing-orders", "goal"];

type CoordinatorSectionsProps = {
  workspaceId: string;
  coordinatorId: string;
  canManage: boolean;
  identity: ReactNode;
  /** Called with the stored context after a pending change was applied. */
  onContextApplied?: (context: string) => void;
};

type AutonomyBodyProps = Omit<CoordinatorSectionsProps, "identity">;

function AutonomyBody({
  workspaceId,
  coordinatorId,
  canManage,
  onContextApplied,
}: AutonomyBodyProps) {
  return (
    <div className="space-y-6">
      <AutonomySection
        workspaceId={workspaceId}
        coordinatorId={coordinatorId}
        canManage={canManage}
      />
      <ChangesWaiting
        workspaceId={workspaceId}
        coordinatorId={coordinatorId}
        canManage={canManage}
        onContextApplied={onContextApplied}
      />
    </div>
  );
}

function MayDoBody(props: ComponentProps<typeof MayDoSection>) {
  return (
    <>
      <MayDoSection {...props} />
      <ControlError error={props.control.fieldError} />
    </>
  );
}

function watchSetFromStored(stored: ControlDraft): WatchSet {
  const { scope, workflowIds } = stored.watches;
  const projects = stored.projects;
  if (projects?.scope !== "selected") return { scope, workflowIds };
  const ids = projects.entries.length === 0 ? [] : null;
  return {
    scope,
    workflowIds,
    projects: {
      scope: "selected",
      repositoryIds: ids,
      includeNoRepository: projects.includeNoRepository,
    },
  };
}

function sectionSlugs(phase3: boolean, phase31: boolean): string[] {
  return [...BASE_SLUGS, ...(phase3 ? ["autonomy"] : []), ...(phase31 ? ["learning"] : [])];
}

function learningEntry(
  t: ReturnType<typeof useTranslation>["t"],
  props: { workspaceId: string; coordinatorId: string; canManage: boolean },
): CoordinatorSectionEntry {
  return {
    slug: "learning",
    label: t("coordinator:sectionLearning"),
    help: t("coordinator:sectionLearningHelp"),
    render: () => <LearningSection {...props} />,
  };
}

/** The phase-2 coordinator page body: Identity holds the phase-1 fields unchanged. */
export function CoordinatorSections({
  workspaceId,
  coordinatorId,
  canManage,
  identity,
  onContextApplied,
}: CoordinatorSectionsProps) {
  const { t } = useTranslation();
  const control = useControlDraft({ workspaceId, coordinatorId, canManage });
  const phase3 = useCoordinatorPhase3Effective();
  const phase31 = useCoordinatorPhase31Effective();
  const { selectSection } = useCoordinatorSection(sectionSlugs(phase3, phase31));
  const stored = control.stored;
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
        <MayDoBody
          workspaceId={workspaceId}
          coordinatorId={coordinatorId}
          canManage={canManage}
          control={control}
          phase3={phase3}
        />
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
  if (phase3) {
    entries.push({
      slug: "autonomy",
      label: t("coordinator:sectionAutonomy"),
      help: t("coordinator:sectionAutonomyHelp"),
      render: () => (
        <AutonomyBody
          workspaceId={workspaceId}
          coordinatorId={coordinatorId}
          canManage={canManage}
          onContextApplied={onContextApplied}
        />
      ),
    });
  }
  if (phase31) {
    entries.push(learningEntry(t, { workspaceId, coordinatorId, canManage }));
  }
  return (
    <>
      <WatchesNoneNotice
        workspaceId={workspaceId}
        coordinatorId={coordinatorId}
        watchSet={stored ? watchSetFromStored(stored) : undefined}
        onChooseBoards={canManage ? () => selectSection("watches") : undefined}
      />
      <SectionsRow entries={entries} />
    </>
  );
}
