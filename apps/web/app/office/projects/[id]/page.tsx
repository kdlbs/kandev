"use client";

import { use, useCallback, useEffect, useSyncExternalStore } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "@/components/routing/app-link";
import { useRouter } from "@/lib/routing/client-router";
import { IconChevronRight, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { toast } from "sonner";
import { useAppStore } from "@/components/state-provider";
import { deleteProject } from "@/lib/api/domains/office-api";
import { qk } from "@/lib/query/keys";
import { officeProjectQueryOptions } from "@/lib/query/query-options";
import type { Project } from "@/lib/state/slices/office/types";
import { OfficeTopbarPortal } from "../../components/office-topbar-portal";
import { ProjectHeader } from "./project-header";
import { ProjectReposSection } from "./project-repos-section";
import { ProjectExecutorSection } from "./project-executor-section";
import { ProjectTasksSection } from "./project-tasks-section";
import {
  readProjectFromListCache,
  removeProjectFromList,
  type OfficeProjectsCache,
} from "./project-query-cache";

type PageProps = {
  params: Promise<{ id: string }>;
};

export default function ProjectDetailPage({ params }: PageProps) {
  const { id } = use(params);
  const router = useRouter();
  const queryClient = useQueryClient();
  const projectQuery = useQuery(officeProjectQueryOptions(id));
  const cachedProject = useCachedProjectFromList(id);
  const project = projectQuery.data ?? cachedProject ?? null;

  useEffect(() => {
    if (!projectQuery.error) return;
    toast.error(
      projectQuery.error instanceof Error ? projectQuery.error.message : "Failed to load project",
    );
  }, [projectQuery.error]);

  const handleDelete = async () => {
    if (!project) return;
    try {
      await deleteProject(project.id);
      queryClient.removeQueries({ exact: true, queryKey: qk.office.project(project.id) });
      const projectsKey = qk.office.projects(project.workspaceId);
      queryClient.setQueryData<OfficeProjectsCache>(projectsKey, (current) =>
        removeProjectFromList(current, project.id),
      );
      queryClient.invalidateQueries({ exact: true, queryKey: projectsKey });
      toast.success("Project deleted");
      router.push("/office/projects");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete project");
    }
  };

  if (!project) {
    return (
      <div className="p-6">
        <p className="text-muted-foreground">Loading project...</p>
      </div>
    );
  }

  return (
    <>
      <OfficeTopbarPortal>
        <Link
          href="/office/projects"
          className="text-sm text-muted-foreground hover:text-foreground cursor-pointer"
        >
          Projects
        </Link>
        <IconChevronRight className="h-3.5 w-3.5 text-muted-foreground/60" />
        <span className="text-sm font-medium truncate">{project.name}</span>
      </OfficeTopbarPortal>

      <div className="p-6 max-w-3xl space-y-6">
        <ProjectHeader project={project} />

        <Separator />

        <ProjectReposSection project={project} />

        <Separator />

        <ProjectExecutorSection project={project} />

        <Separator />

        <ProjectTasksSection projectId={project.id} />

        <Separator />

        <div className="flex justify-end">
          <Button variant="destructive" size="sm" onClick={handleDelete} className="cursor-pointer">
            <IconTrash className="h-4 w-4 mr-1" />
            Delete Project
          </Button>
        </div>
      </div>
    </>
  );
}

function useCachedProjectFromList(projectId: string): Project | null {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const queryClient = useQueryClient();
  const subscribe = useCallback(
    (onStoreChange: () => void) => queryClient.getQueryCache().subscribe(onStoreChange),
    [queryClient],
  );
  const getSnapshot = useCallback(
    () => readProjectFromListCache(queryClient, workspaceId, projectId),
    [projectId, queryClient, workspaceId],
  );
  return useSyncExternalStore(subscribe, getSnapshot, () => null);
}
