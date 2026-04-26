"use client";

import { useEffect, useState, useCallback } from "react";
import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { listProjects } from "@/lib/api/domains/orchestrate-api";
import { ProjectCard } from "./project-card";
import { CreateProjectDialog } from "./create-project-dialog";

export default function ProjectsPage() {
  const projects = useAppStore((s) => s.orchestrate.projects);
  const agents = useAppStore((s) => s.orchestrate.agentInstances);
  const setProjects = useAppStore((s) => s.setProjects);
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const [dialogOpen, setDialogOpen] = useState(false);

  const loadProjects = useCallback(async () => {
    if (!activeWorkspaceId) return;
    try {
      const res = await listProjects(activeWorkspaceId);
      if (res?.projects) {
        setProjects(res.projects);
      }
    } catch {
      // Silently handle fetch errors
    }
  }, [activeWorkspaceId, setProjects]);

  useEffect(() => {
    loadProjects();
  }, [loadProjects]);

  const agentNameMap = new Map(agents.map((a) => [a.id, a.name]));

  return (
    <div className="space-y-4 p-6">
      <div className="flex items-center justify-between">
        <h1 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
          Projects
        </h1>
        <Button
          size="sm"
          onClick={() => setDialogOpen(true)}
          className="cursor-pointer"
        >
          <IconPlus className="h-4 w-4 mr-1" />
          New Project
        </Button>
      </div>

      {projects.length === 0 ? (
        <div className="text-center py-12">
          <p className="text-muted-foreground mb-4">No projects yet</p>
          <Button
            variant="outline"
            onClick={() => setDialogOpen(true)}
            className="cursor-pointer"
          >
            <IconPlus className="h-4 w-4 mr-1" />
            Create your first project
          </Button>
        </div>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {projects.map((project) => (
            <ProjectCard
              key={project.id}
              project={project}
              leadAgentName={
                project.leadAgentInstanceId
                  ? agentNameMap.get(project.leadAgentInstanceId)
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
