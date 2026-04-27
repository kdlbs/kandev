"use client";

import { useEffect, useState, use } from "react";
import { useRouter } from "next/navigation";
import { IconArrowLeft, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { toast } from "sonner";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { useAppStore } from "@/components/state-provider";
import { getProject, deleteProject } from "@/lib/api/domains/orchestrate-api";
import type { Project } from "@/lib/state/slices/orchestrate/types";
import { ProjectHeader } from "./project-header";
import { ProjectReposSection } from "./project-repos-section";
import { ProjectExecutorSection } from "./project-executor-section";

type PageProps = {
  params: Promise<{ id: string }>;
};

export default function ProjectDetailPage({ params }: PageProps) {
  const { id } = use(params);
  const router = useRouter();
  const removeProject = useAppStore((s) => s.removeProject);
  const storeProject = useAppStore((s) => s.orchestrate.projects.find((p) => p.id === id));
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
      router.push("/orchestrate/projects");
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
    <div className="p-6 max-w-3xl space-y-6">
      <div className="flex items-center gap-2">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => router.push("/orchestrate/projects")}
              className="cursor-pointer"
            >
              <IconArrowLeft className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Back to projects</TooltipContent>
        </Tooltip>
        <span className="text-sm text-muted-foreground">Projects</span>
      </div>

      <ProjectHeader project={project} />

      <Separator />

      <ProjectReposSection project={project} />

      <Separator />

      <ProjectExecutorSection project={project} />

      <Separator />

      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Tasks</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">Tasks coming in Wave 3</p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Budget</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">Budget coming in Wave 5</p>
        </CardContent>
      </Card>

      <Separator />

      <div className="flex justify-end">
        <Button variant="destructive" size="sm" onClick={handleDelete} className="cursor-pointer">
          <IconTrash className="h-4 w-4 mr-1" />
          Delete Project
        </Button>
      </div>
    </div>
  );
}
