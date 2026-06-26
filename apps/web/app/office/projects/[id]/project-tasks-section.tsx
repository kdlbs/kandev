"use client";

import { useMemo } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { useOfficeAgentsData } from "@/hooks/domains/office/use-office-data";
import { officeTasksInfiniteQueryOptions } from "@/lib/query/query-options";
import { agentProfileId as toAgentProfileId } from "@/lib/types/ids";
import { TaskRow } from "../../tasks/task-row";

type ProjectTasksSectionProps = {
  projectId: string;
};

export function ProjectTasksSection({ projectId }: ProjectTasksSectionProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const agentProfiles = useOfficeAgentsData(workspaceId).data?.agents ?? [];
  const projectTasksQuery = useInfiniteQuery(
    officeTasksInfiniteQueryOptions(workspaceId ?? "", {
      project: projectId,
      limit: 100,
      sort: "updated_at",
      order: "desc",
    }),
  );

  const queriedTasks = useMemo(
    () => projectTasksQuery.data?.pages.flatMap((page) => page.tasks ?? []) ?? [],
    [projectTasksQuery.data],
  );

  const sorted = useMemo(
    () =>
      [...queriedTasks].sort(
        (a, b) => new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime(),
      ),
    [queriedTasks],
  );

  const agentNameById = useMemo(
    () => new Map(agentProfiles.map((a) => [a.id, a.name])),
    [agentProfiles],
  );

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-semibold">Tasks</h2>
        <span className="text-xs text-muted-foreground">
          {sorted.length} {sorted.length === 1 ? "task" : "tasks"}
        </span>
      </div>
      {sorted.length === 0 ? (
        <p className="text-xs text-muted-foreground">No tasks in this project yet.</p>
      ) : (
        <div className="border border-border rounded-md divide-y divide-border/60 overflow-hidden">
          {sorted.map((task) => (
            <TaskRow
              key={task.id}
              task={task}
              level={0}
              hasChildren={false}
              expanded={false}
              onToggleExpand={noop}
              agentName={
                task.assigneeAgentProfileId
                  ? agentNameById.get(toAgentProfileId(task.assigneeAgentProfileId))
                  : undefined
              }
            />
          ))}
        </div>
      )}
    </div>
  );
}

function noop() {
  // TaskRow requires an expand handler; flat lists never collapse, so
  // we hand it a no-op rather than forcing the prop to be optional and
  // touching every existing caller.
}
