"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { officeQueryOptions } from "@/lib/query/query-options/office";
import { agentProfileId as toAgentProfileId } from "@/lib/types/ids";
import { ProjectCard } from "./project-card";
import { CreateProjectDialog } from "./create-project-dialog";
import { EmptyState } from "../components/shared/empty-state";

export function ProjectsPageClient() {
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const [dialogOpen, setDialogOpen] = useState(false);
  const { data: projects = [] } = useQuery({
    ...officeQueryOptions.projects(activeWorkspaceId ?? ""),
    enabled: !!activeWorkspaceId,
  });
  const { data: agents = [] } = useQuery({
    ...officeQueryOptions.agents(activeWorkspaceId ?? ""),
    enabled: !!activeWorkspaceId,
  });

  const agentNameMap = new Map(agents.map((a) => [a.id, a.name]));

  return (
    <div className="space-y-4 p-6">
      <div className="flex justify-end">
        <Button size="sm" onClick={() => setDialogOpen(true)} className="cursor-pointer">
          <IconPlus className="h-4 w-4 mr-1" />
          New Project
        </Button>
      </div>

      {projects.length === 0 ? (
        <EmptyState
          message="No projects yet."
          description="Projects group related tasks and repositories together."
          action={
            <Button
              variant="outline"
              onClick={() => setDialogOpen(true)}
              className="cursor-pointer"
            >
              <IconPlus className="h-4 w-4 mr-1" />
              Create your first project
            </Button>
          }
        />
      ) : (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {projects.map((project) => (
            <ProjectCard
              key={project.id}
              project={project}
              leadAgentName={
                project.leadAgentProfileId
                  ? agentNameMap.get(toAgentProfileId(project.leadAgentProfileId))
                  : undefined
              }
            />
          ))}
        </div>
      )}

      {activeWorkspaceId && (
        <CreateProjectDialog
          open={dialogOpen}
          onOpenChange={setDialogOpen}
          workspaceId={activeWorkspaceId}
        />
      )}
    </div>
  );
}
