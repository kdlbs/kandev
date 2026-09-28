"use client";

import { useTranslation } from "react-i18next";
import { PageShell } from "@/components/page-shell";
import { CoordinatorRouteContent } from "./coordinator-route-content";
import { NeedsYouItemCard } from "./components/needs-you-item-card";
import { EmptyNeedsYouState } from "./components/empty-needs-you-state";

export type NeedsYouPageClientProps = {
  workspaceId: string;
  coordinatorId: string | null;
};

/**
 * The Needs you screen: one item card per entry the coordinator classifies
 * as needing a human decision, or the empty state when there is none
 * (docs/specs/coordinator/requirements/needs-you.md REQ-COORDINATOR-NEEDS-YOU-001..003).
 */
export function NeedsYouPageClient({ workspaceId, coordinatorId }: NeedsYouPageClientProps) {
  const { t } = useTranslation();
  return (
    <PageShell title={t("coordinator:needsYouTitle")}>
      <CoordinatorRouteContent
        workspaceId={workspaceId}
        coordinatorId={coordinatorId}
        view="needs-you"
      >
        {({ coordinator, attention, canManage }) => {
          const items = attention.classification.needsYou;
          if (items.length === 0) {
            return (
              <EmptyNeedsYouState
                workingCount={attention.classification.queue.working.length}
                workspaceId={workspaceId}
                coordinatorId={coordinator.id}
              />
            );
          }
          return (
            <div className="space-y-3" data-testid="needs-you-item-list">
              {items.map((item) => (
                <NeedsYouItemCard
                  key={item.id}
                  item={item}
                  stepNameByTaskId={attention.stepNameByTaskId}
                  workflowNameById={attention.workflowNameById}
                  stepNameByWorkflowStep={attention.stepNameByWorkflowStep}
                  openTasksById={attention.openTasksById}
                  coordinatorName={coordinator.name}
                  coordinatorId={coordinator.id}
                  canManage={canManage}
                />
              ))}
            </div>
          );
        }}
      </CoordinatorRouteContent>
    </PageShell>
  );
}
