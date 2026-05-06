"use client";

import { useCallback, useEffect, useState } from "react";
import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { useOfficeRefetch } from "@/hooks/use-office-refetch";
import { listProjects } from "@/lib/api/domains/office-api";
import type { Project } from "@/lib/state/slices/office/types";
import { ProjectCard } from "./project-card";
import { CreateProjectDialog } from "./create-project-dialog";
import { EmptyState } from "../components/shared/empty-state";

type ProjectsPageClientProps = {
  initialProjects: Project[];
};

export function ProjectsPageClient({ initialProjects }: ProjectsPageClientProps) {
  const projects = useAppStore((s) => s.office.projects);
  const agents = useAppStore((s) => s.office.agentProfiles);
  const setProjects = useAppStore((s) => s.setProjects);
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const [dialogOpen, setDialogOpen] = useState(false);

  // Hydrate from SSR; subsequent updates flow through the WS-driven
  // refetch below. Skipping the unconditional mount fetch removes a
  // redundant round-trip when SSR data is already in the store
  // (Stream G of office optimization).
  useEffect(() => {
    if (initialProjects.length > 0) {
      setProjects(initialProjects);
    }
  }, [initialProjects, setProjects]);

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

  useOfficeRefetch("projects", loadProjects);

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
                  ? agentNameMap.get(project.leadAgentProfileId)
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
