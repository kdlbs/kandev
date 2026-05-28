"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAppStore } from "@/components/state-provider";
import { officeQueryOptions } from "@/lib/query/query-options/office";
import { agentProfileId as toAgentProfileId } from "@/lib/types/ids";
import { TaskRow } from "../../tasks/task-row";

type ProjectTasksSectionProps = {
  projectId: string;
};

export function ProjectTasksSection({ projectId }: ProjectTasksSectionProps) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  // TQ fetches tasks for this project directly — no global store merge needed.
  const { data: tasks = [] } = useQuery({
    ...officeQueryOptions.tasks(workspaceId ?? "", { projectIds: [projectId] }),
    enabled: !!workspaceId,
  });
  const { data: agentProfiles = [] } = useQuery({
    ...officeQueryOptions.agents(workspaceId ?? ""),
    enabled: !!workspaceId,
  });

  const sorted = useMemo(
    () =>
      [...tasks].sort((a, b) => new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime()),
    [tasks],
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
