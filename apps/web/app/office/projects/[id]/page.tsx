"use client";

import { useEffect, useState, use } from "react";
import { useRouter } from "@/lib/routing/client-router";
import { IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { toast } from "@/lib/toast/sonner";
import { useAppStore } from "@/components/state-provider";
import { selectOfficeProjects } from "@/lib/state/slices/office/selectors";
import { getProject, deleteProject } from "@/lib/api/domains/office-api";
import type { Project } from "@/lib/state/slices/office/types";
import { useOfficeTopbar } from "../../components/office-topbar-context";
import { ProjectHeader } from "./project-header";
import { ProjectReposSection } from "./project-repos-section";
import { ProjectExecutorSection } from "./project-executor-section";
import { ProjectTasksSection } from "./project-tasks-section";
import { useTranslation } from "react-i18next";

type PageProps = {
  params: Promise<{ id: string }>;
};

export default function ProjectDetailPage({ params }: PageProps) {
  const { t } = useTranslation();
  const { id } = use(params);
  const router = useRouter();
  const removeProject = useAppStore((s) => s.removeProject);
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const storeProject = useAppStore((s) => selectOfficeProjects(s).find((p) => p.id === id));
  const [fetchedProject, setFetchedProject] = useState<Project | null>(null);
  const project =
    storeProject ?? (fetchedProject?.workspaceId === activeWorkspaceId ? fetchedProject : null);

  useEffect(() => {
    setFetchedProject(null);
  }, [activeWorkspaceId, id]);

  useEffect(() => {
    if (!activeWorkspaceId || storeProject) return;
    let cancelled = false;
    getProject(id)
      .then((res) => {
        if (!cancelled && res.workspaceId === activeWorkspaceId) setFetchedProject(res);
      })
      .catch((err) => {
        if (!cancelled) {
          toast.error(err instanceof Error ? err.message : t("office:failedToLoadProject"));
        }
      });
    return () => {
      cancelled = true;
    };
  }, [activeWorkspaceId, id, storeProject, t]);

  useOfficeTopbar(
    project
      ? {
          title: project.name,
          parents: [{ label: t("office:projects"), href: "/office/projects" }],
        }
      : null,
  );

  const handleDelete = async () => {
    if (!project) return;
    try {
      await deleteProject(project.id);
      removeProject(project.workspaceId, project.id);
      toast.success(t("office:projectDeleted"));
      router.push("/office/projects");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("office:failedToDeleteProject"));
    }
  };

  if (!project) {
    return (
      <div className="p-6">
        <p className="text-muted-foreground">{t("office:loadingProject")}</p>
      </div>
    );
  }

  return (
    <div className="p-6 max-w-3xl space-y-6">
      <ProjectHeader project={project} />

      <Separator />

      <ProjectReposSection project={project} />

      <Separator />

      <ProjectExecutorSection project={project} />

      <Separator />

      <ProjectTasksSection projectId={project.id} workspaceId={project.workspaceId} />

      <Separator />

      <div className="flex justify-end">
        <Button variant="destructive" size="sm" onClick={handleDelete} className="cursor-pointer">
          <IconTrash className="h-4 w-4 mr-1" />
          {t("office:deleteProject")}
        </Button>
      </div>
    </div>
  );
}
