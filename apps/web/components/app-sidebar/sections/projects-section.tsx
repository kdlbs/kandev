"use client";

import { useRouter } from "next/navigation";
import { IconBoxMultiple, IconPlus } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { useInOffice } from "@/hooks/use-in-office";
import { cn } from "@/lib/utils";
import { APP_SIDEBAR_SECTION_IDS } from "../app-sidebar-constants";
import { AppSidebarSection } from "../app-sidebar-section";

type ProjectsSectionProps = {
  collapsed: boolean;
};

export function ProjectsSection({ collapsed }: ProjectsSectionProps) {
  const router = useRouter();
  const inOffice = useInOffice();
  const projects = useAppStore((s) => s.office.projects);
  const activeProjects = projects.filter((p) => p.status !== "archived");

  if (!inOffice) return null;

  const headerAction = (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="h-5 w-5 cursor-pointer"
          onClick={() => router.push("/office/projects")}
        >
          <IconPlus className="h-3 w-3 text-muted-foreground/60" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>Add project</TooltipContent>
    </Tooltip>
  );

  return (
    <AppSidebarSection
      id={APP_SIDEBAR_SECTION_IDS.projects}
      label="Projects"
      collapsed={collapsed}
      icon={IconBoxMultiple}
      headerAction={headerAction}
    >
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
                "flex items-center gap-2.5 px-2.5 py-1.5 text-[13px] font-medium rounded-md",
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
    </AppSidebarSection>
  );
}
