"use client";

import { useEffect, useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { listTasks } from "@/lib/api/domains/office-tasks-api";
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

  // Fetch tasks for this project once on mount. The list is merged into
  // the global store via appendTasks so other consumers (the Tasks page,
  // the inbox, etc.) keep seeing the union of every task they've loaded.
  useEffect(() => {
    if (!workspaceId) return;
    let cancelled = false;
    listTasks(workspaceId, { project: projectId })
      .then((res) => {
        if (cancelled || !res?.tasks?.length) return;
        appendTasks(res.tasks);
      })
      .catch(() => {
        // Failure is non-fatal — store-resident tasks still render.
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId, projectId, appendTasks]);

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
