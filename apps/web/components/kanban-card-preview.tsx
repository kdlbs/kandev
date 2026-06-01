"use client";

import { useMemo } from "react";
import { Card, CardContent } from "@kandev/ui/card";
import { KanbanCardBody } from "@/components/kanban-card-content";
import { resolveTaskRepositoryNames, type Task } from "@/components/kanban-card";
import { useAllRepositories } from "@/hooks/domains/workspace/use-all-repositories";

function KanbanCardPreviewLayout({
  task,
  repositoryNames,
}: {
  task: Task;
  repositoryNames: string[];
}) {
  return (
    <Card
      size="sm"
      className="w-full py-0 cursor-grabbing shadow-lg ring-0 pointer-events-none border border-border"
    >
      <CardContent className="px-2 py-1">
        <KanbanCardBody task={task} repoNames={repositoryNames} />
      </CardContent>
    </Card>
  );
}

export function KanbanCardPreview({ task }: { task: Task }) {
  const { repositories } = useAllRepositories(false);
  const repositoryNames = useMemo(
    () => resolveTaskRepositoryNames(task, repositories),
    [repositories, task],
  );

  return <KanbanCardPreviewLayout task={task} repositoryNames={repositoryNames} />;
}
