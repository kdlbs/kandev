import { listProjects } from "@/lib/api/domains/office-api";
import { getActiveWorkspaceId } from "../lib/get-active-workspace";
import { ProjectsPageClient } from "./projects-page-client";
import type { Project } from "@/lib/state/slices/office/types";

export default async function ProjectsPage() {
  const workspaceId = await getActiveWorkspaceId();

  let projects: Project[] = [];
  if (workspaceId) {
    const res = await listProjects(workspaceId, { cache: "no-store" }).catch(() => ({
      projects: [],
    }));
    projects = res.projects ?? [];
  }

  return <ProjectsPageClient initialProjects={projects} />;
}
