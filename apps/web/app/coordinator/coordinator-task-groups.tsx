import { useTranslation } from "react-i18next";
import type { Task } from "@/lib/types/http";
import {
  COORDINATOR_GROUPS,
  type CoordinatorTaskGroup,
} from "@/lib/orchestration/coordinator-task-groups";
import type { CoordinatorWorkspace } from "@/hooks/domains/orchestration/use-coordinator-workspace";
import { CoordinatorTaskRow } from "./coordinator-task-row";

export function CoordinatorTaskGroups({
  rows,
  catalog,
  groupFilter,
}: {
  rows: { task: Task; group: CoordinatorTaskGroup }[];
  catalog: CoordinatorWorkspace;
  groupFilter: string;
}) {
  const { t } = useTranslation();
  return COORDINATOR_GROUPS.filter((group) => groupFilter === "all" || groupFilter === group).map(
    (group) => {
      const tasks = rows.filter((row) => row.group === group);
      if (groupFilter === "all" && tasks.length === 0) return null;
      return (
        <section
          key={group}
          data-testid={`coordinator-group-${group}`}
          aria-labelledby={`group-${group}`}
          className="space-y-2"
        >
          <h2 id={`group-${group}`} className="flex items-center gap-2 text-sm font-semibold">
            <span>{t(`orchestration:group_${group}`)}</span>
            <span className="rounded-full bg-muted px-2 text-xs tabular-nums">{tasks.length}</span>
          </h2>
          {tasks.map(({ task }) => (
            <CoordinatorTaskRow key={task.id} task={task} group={group} catalog={catalog} />
          ))}
        </section>
      );
    },
  );
}
