"use client";

import { useMemo } from "react";
import { Card, CardContent } from "@kandev/ui/card";
import { KanbanCardBody } from "@/components/kanban-card-content";
import { resolveTaskRepositoryNames, type Task } from "@/components/kanban-card";
import { useAppStore } from "@/components/state-provider";
import type { Repository } from "@/lib/types/http";

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
  const repositoriesByWorkspace = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const repositoryNames = useMemo(
    () =>
      resolveTaskRepositoryNames(
        task,
        Object.values(repositoriesByWorkspace).flat() as Repository[],
      ),
    [repositoriesByWorkspace, task],
  );

  return <KanbanCardPreviewLayout task={task} repositoryNames={repositoryNames} />;
}
