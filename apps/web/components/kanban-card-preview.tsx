"use client";

import { useMemo } from "react";
import { Card, CardContent } from "@kandev/ui/card";
import { KanbanCardBody } from "@/components/kanban-card-content";
import {
  resolveTaskRepositoryChips,
  type RepositoryChip,
  type Task,
} from "@/components/kanban-card";
import { useAllCachedRepositories } from "@/hooks/domains/workspace/use-repository-cache";

function KanbanCardPreviewLayout({
  task,
  repositoryChips,
}: {
  task: Task;
  repositoryChips: RepositoryChip[];
}) {
  return (
    <Card
      size="sm"
      className="w-full py-0 cursor-grabbing shadow-lg ring-0 pointer-events-none border border-border"
    >
      <CardContent className="px-2 py-1">
        <KanbanCardBody task={task} repositoryChips={repositoryChips} />
      </CardContent>
    </Card>
  );
}

export function KanbanCardPreview({ task }: { task: Task }) {
  const repositories = useAllCachedRepositories();
  const repositoryChips = useMemo(
    () => resolveTaskRepositoryChips(task, repositories),
    [repositories, task],
  );

  return <KanbanCardPreviewLayout task={task} repositoryChips={repositoryChips} />;
}
