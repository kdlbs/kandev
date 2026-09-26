"use client";

import { useEffect, useState } from "react";
import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import {
  selectOfficeAgentProfiles,
  selectOfficeProjects,
} from "@/lib/state/slices/office/selectors";
import { agentProfileId as toAgentProfileId } from "@/lib/types/ids";
import type { Project } from "@/lib/state/slices/office/types";
import { ProjectCard } from "./project-card";
import { CreateProjectDialog } from "./create-project-dialog";
import { EmptyState } from "../components/shared/empty-state";
import { useTranslation } from "react-i18next";

type ProjectsPageClientProps = {
  initialProjects: Project[];
  initialWorkspaceId?: string | null;
};

export function ProjectsPageClient({
  initialProjects,
  initialWorkspaceId,
}: ProjectsPageClientProps) {
  const { t } = useTranslation();
  const projects = useAppStore(selectOfficeProjects);
  const agents = useAppStore(selectOfficeAgentProfiles);
  const setProjects = useAppStore((s) => s.setProjects);
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const [dialogOpen, setDialogOpen] = useState(false);

  // Hydrate from SSR; subsequent updates flow through the WS-driven
  // refetch below. Skipping the unconditional mount fetch removes a
  // redundant round-trip when SSR data is already in the store
  // (Stream G of office optimization).
  useEffect(() => {
    if (
      !activeWorkspaceId ||
      (initialWorkspaceId !== undefined && initialWorkspaceId !== activeWorkspaceId) ||
      initialProjects.length === 0
    ) {
      return;
    }
    setProjects(activeWorkspaceId, initialProjects);
  }, [activeWorkspaceId, initialProjects, initialWorkspaceId, setProjects]);

  const agentNameMap = new Map(agents.map((a) => [a.id, a.name]));

  return (
    <div className="space-y-4 p-6">
      <div className="flex justify-end">
        <Button size="sm" onClick={() => setDialogOpen(true)} className="cursor-pointer">
          <IconPlus className="h-4 w-4 mr-1" />
          {t("office:newProject")}
        </Button>
      </div>

      {projects.length === 0 ? (
        <EmptyState
          message={t("office:noProjectsYet")}
          description={t("office:projectsGroupRelatedTasksAndRepositories")}
          action={
            <Button
              variant="outline"
              onClick={() => setDialogOpen(true)}
              className="cursor-pointer"
            >
              <IconPlus className="h-4 w-4 mr-1" />
              {t("office:createYourFirstProject")}
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
