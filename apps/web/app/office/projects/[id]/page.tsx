"use client";

import { use } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { IconChevronRight, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { toast } from "sonner";
import { useAppStore } from "@/components/state-provider";
import { deleteProject } from "@/lib/api/domains/office-api";
import { officeQueryOptions } from "@/lib/query/query-options/office";
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
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const qc = useQueryClient();
  const {
    data: project,
    isPending,
    isError,
  } = useQuery({
    ...officeQueryOptions.projects(workspaceId ?? ""),
    enabled: !!workspaceId,
    select: (projects) => projects.find((p) => p.id === id),
  });

  const handleDelete = async () => {
    if (!project) return;
    try {
      await deleteProject(project.id);
      if (workspaceId) void qc.invalidateQueries({ queryKey: ["office", workspaceId, "projects"] });
      toast.success("Project deleted");
      router.push("/office/projects");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to delete project");
    }
  };

  if (isError) {
    return (
      <div className="p-6">
        <p className="text-muted-foreground">Failed to load project.</p>
      </div>
    );
  }

  if (isPending || !project) {
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
