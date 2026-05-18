"use client";

import { useEffect, useState, use } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { IconChevronRight, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { toast } from "sonner";
import { useAppStore } from "@/components/state-provider";
import { getProject, deleteProject } from "@/lib/api/domains/office-api";
import type { Project } from "@/lib/state/slices/office/types";
import { OfficeTopbarPortal } from "../../components/office-topbar-portal";
import { ProjectHeader } from "./project-header";
import { ProjectReposSection } from "./project-repos-section";
import { ProjectExecutorSection } from "./project-executor-section";
import { ProjectTasksSection } from "./project-tasks-section";

type PageProps = {
  params: Promise<{ id: string }>;
};

export default function ProjectDetailPage({ params }: PageProps) {
  const { id } = use(params);
  const router = useRouter();
  const removeProject = useAppStore((s) => s.removeProject);
  const storeProject = useAppStore((s) => s.office.projects.find((p) => p.id === id));
  const [fetchedProject, setFetchedProject] = useState<Project | null>(null);
  const project = storeProject ?? fetchedProject;

  useEffect(() => {
    if (storeProject) return;
    let cancelled = false;
    getProject(id)
      .then((res) => {
        if (!cancelled && res) setFetchedProject(res as unknown as Project);
      })
      .catch((err) => {
        if (!cancelled) {
          toast.error(err instanceof Error ? err.message : "Failed to load project");
        }
      });
    return () => {
      cancelled = true;
    };
  }, [id, storeProject]);

  const handleDelete = async () => {
    if (!project) return;
    try {
      await deleteProject(project.id);
      removeProject(project.id);
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
