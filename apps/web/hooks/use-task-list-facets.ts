import { useEffect, useMemo, useState } from "react";
import { pluginTaskListFacetRegistrationKey, usePluginRegistry } from "@/lib/plugins/registry";
import type { PluginTaskListFacetRegistration } from "@/lib/plugins/registry";
import type { TaskListFacetValue } from "@/lib/plugins/types";
import type { Task } from "@/lib/types/http";
import { TASK_LIST_FACET_PREFIX } from "@/lib/tasks/tasks-list-options";

export type ResolvedTaskListFacet = PluginTaskListFacetRegistration & { key: string };
export type TaskFacetValues = Record<string, readonly TaskListFacetValue[]>;

export function useTaskListFacets(tasks: Task[], workspaceId: string | null) {
  const registry = usePluginRegistry();
  const registrations = registry.getTaskListFacets();
  const registryVersion = registry.getVersion();
  const facets = useMemo(
    () =>
      registrations.map((facet) => ({
        ...facet,
        key: `${TASK_LIST_FACET_PREFIX}${pluginTaskListFacetRegistrationKey(facet)}`,
      })),
    [registryVersion],
  );
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    const unsubscribers = facets.flatMap((facet) => {
      if (!facet.subscribe) return [];
      try {
        return [facet.subscribe(() => setRevision((value) => value + 1))];
      } catch (error) {
        console.error(
          `[plugins] task-list facet "${facet.pluginId}:${facet.id}" subscribe() threw`,
          error,
        );
        return [];
      }
    });
    return () =>
      unsubscribers.forEach((unsubscribe) => {
        try {
          unsubscribe();
        } catch (error) {
          console.error("[plugins] task-list facet unsubscribe() threw", error);
        }
      });
  }, [facets]);

  const values = useMemo(
    () => resolveTaskFacetValues(facets, tasks, workspaceId),
    [facets, tasks, workspaceId, revision],
  );
  return { facets, values };
}

export function resolveTaskFacetValues(
  facets: ResolvedTaskListFacet[],
  tasks: Task[],
  workspaceId: string | null,
): TaskFacetValues {
  if (!workspaceId) return {};
  const resolved: TaskFacetValues = {};
  for (const facet of facets) {
    for (const task of tasks) {
      try {
        const values = facet.getValues({ taskId: task.id, workspaceId });
        resolved[`${facet.key}:${task.id}`] = Array.isArray(values)
          ? values.filter(
              (value) =>
                value && typeof value.value === "string" && typeof value.label === "string",
            )
          : [];
      } catch (error) {
        console.error(
          `[plugins] task-list facet "${facet.pluginId}:${facet.id}" getValues() threw`,
          error,
        );
        resolved[`${facet.key}:${task.id}`] = [];
      }
    }
  }
  return resolved;
}
