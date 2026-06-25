"use client";

import { useEffect, useMemo } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { officeTasksInfiniteQueryOptions } from "@/lib/query/query-options";
import { agentProfileId as toAgentProfileId } from "@/lib/types/ids";
import { TaskRow } from "../../tasks/task-row";

type ProjectTasksSectionProps = {
  projectId: string;
};

export function ProjectTasksSection({ projectId }: ProjectTasksSectionProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const appendTasks = useAppStore((s) => s.appendTasks);
  // Select stable references; derive the filtered list and the agent-name
  // lookup via useMemo. Returning a freshly `.filter()`'d array or a
  // `new Map(...)` straight from the selector tripped React's
  // getSnapshot caching guard because every render produced a new
  // reference.
  const allTasks = useAppStore((s) => s.office.tasks.items);
  const agentProfiles = useAppStore((s) => s.office.agentProfiles);
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

  // Mirror query-loaded tasks into the global store so existing consumers
  // keep seeing the union of every task they've loaded.
  useEffect(() => {
    if (queriedTasks.length > 0) appendTasks(queriedTasks);
  }, [queriedTasks, appendTasks]);

  const sorted = useMemo(
    () =>
      allTasks
        .filter((t) => t.projectId === projectId)
        .sort((a, b) => new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime()),
    [allTasks, projectId],
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
