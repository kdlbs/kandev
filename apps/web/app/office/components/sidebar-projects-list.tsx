"use client";

import { useRouter } from "next/navigation";
import { Badge } from "@kandev/ui/badge";
import { useAppStore } from "@/components/state-provider";
import { SidebarCollapsibleSection } from "./sidebar-collapsible-section";
import { cn } from "@/lib/utils";

export function SidebarProjectsList() {
  const router = useRouter();
  const projects = useAppStore((s) => s.office.projects);
  const activeProjects = projects.filter((p) => p.status !== "archived");

  return (
    <SidebarCollapsibleSection label="Projects" onAdd={() => router.push("/office/projects")}>
      {activeProjects.length === 0 ? (
        <p className="px-3 py-2 text-xs text-muted-foreground">No projects yet</p>
      ) : (
        activeProjects.map((project) => {
          const taskCount = project.taskCounts?.total ?? 0;
          return (
            <button
              key={project.id}
              type="button"
              onClick={() => router.push(`/office/projects/${project.id}`)}
              className={cn(
                "flex items-center gap-2.5 px-3 py-2 text-[13px] font-medium rounded-md",
                "cursor-pointer w-full text-left",
                "text-foreground/80 hover:bg-muted/60",
              )}
            >
              <span
                className="h-3 w-3 rounded-sm shrink-0"
                style={{ backgroundColor: project.color || "#6b7280" }}
              />
              <span className="flex-1 truncate">{project.name}</span>
              {taskCount > 0 && (
                <Badge
                  variant="secondary"
                  className="rounded-full px-1.5 py-0 text-[10px] font-normal"
                >
                  {taskCount}
                </Badge>
              )}
            </button>
          );
        })
      )}
    </SidebarCollapsibleSection>
  );
}
