"use client";

import { useSearchParams } from "@/lib/routing/client-router";
import type { QueueGroupKind } from "@/lib/coordinator/attention";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { CoordinatorRouteContent } from "./coordinator-route-content";
import { WhatItDid } from "./queue/what-it-did";
import { QueueGroup } from "./components/queue-group";

export type QueuePageClientProps = {
  workspaceId: string;
  coordinatorId: string | null;
};

// Display order (AC-COORDINATOR-NEEDS-YOU-004.1): Working, In review and
// Ready to merge expanded first, then Done and Other collapsed. This is
// screen layout, not the classification match order in `lib/coordinator/attention.ts`.
const QUEUE_DISPLAY_ORDER: QueueGroupKind[] = [
  "working",
  "in_review",
  "ready_to_merge",
  "done",
  "other",
];

function isQueueGroupKind(value: string | null): value is QueueGroupKind {
  return value !== null && (QUEUE_DISPLAY_ORDER as string[]).includes(value);
}

/**
 * The Queue screen: every other task grouped by Working / In review / Ready
 * to merge / Done / Other (docs/specs/coordinator/requirements/needs-you.md
 * REQ-COORDINATOR-NEEDS-YOU-004). `?group=<kind>` (from the count strip)
 * force-opens a collapsed-by-default group.
 */
export function QueuePageClient({ workspaceId, coordinatorId }: QueuePageClientProps) {
  const searchParams = useSearchParams();
  const rawGroup = searchParams.get("group");
  const linkedGroup = isQueueGroupKind(rawGroup) ? rawGroup : undefined;
  const phase2 = useFeature("coordinatorPhase2");

  return (
    <CoordinatorRouteContent workspaceId={workspaceId} coordinatorId={coordinatorId} view="queue">
      {({ attention, coordinator, canManage }) => (
        <div className="space-y-4" data-testid="queue-group-list">
          {QUEUE_DISPLAY_ORDER.map((group) => (
            <QueueGroup
              key={group}
              group={group}
              items={attention.classification.queue[group]}
              stepNameByTaskId={attention.stepNameByTaskId}
              prsByTaskId={attention.prsByTaskId}
              defaultOpen={linkedGroup === group ? true : undefined}
              phase2={phase2}
              canManage={canManage}
            />
          ))}
          {phase2 && (
            <WhatItDid
              key={coordinator.id}
              workspaceId={workspaceId}
              coordinatorId={coordinator.id}
              canManage={canManage}
              tasks={attention.tasks}
              tasksLoadedAt={attention.loadedAt}
              tasksError={attention.error}
              stepNameByWorkflowStep={attention.stepNameByWorkflowStep}
            />
          )}
        </div>
      )}
    </CoordinatorRouteContent>
  );
}
