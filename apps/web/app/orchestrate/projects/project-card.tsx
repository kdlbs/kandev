"use client";

import Link from "next/link";
import { IconGitBranch } from "@tabler/icons-react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Badge } from "@kandev/ui/badge";
import { Progress } from "@kandev/ui/progress";
import type { Project } from "@/lib/state/slices/orchestrate/types";

const statusBadgeClasses: Record<string, string> = {
  active: "bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-300",
  completed: "bg-blue-100 text-blue-700 dark:bg-blue-900/50 dark:text-blue-300",
  on_hold: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/50 dark:text-yellow-300",
  archived: "bg-neutral-100 text-neutral-700 dark:bg-neutral-900/50 dark:text-neutral-300",
};

const statusLabels: Record<string, string> = {
  active: "Active",
  completed: "Completed",
  on_hold: "On Hold",
  archived: "Archived",
};

type ProjectCardProps = {
  project: Project;
  leadAgentName?: string;
};

export function ProjectCard({ project, leadAgentName }: ProjectCardProps) {
  const counts = project.taskCounts ?? { total: 0, in_progress: 0, done: 0, blocked: 0 };
  const repoCount = project.repositories?.length ?? 0;
  const progressPct = counts.total > 0 ? Math.round((counts.done / counts.total) * 100) : 0;

  return (
    <Link href={`/orchestrate/projects/${project.id}`} className="block cursor-pointer">
      <Card className="hover:bg-accent/50 transition-colors">
        <CardHeader className="pb-2">
          <div className="flex items-center gap-2">
            <span
              className="h-3 w-3 rounded-sm shrink-0"
              style={{ backgroundColor: project.color || "#6b7280" }}
            />
            <CardTitle className="text-sm font-medium truncate flex-1">{project.name}</CardTitle>
            <Badge className={statusBadgeClasses[project.status] ?? ""}>
              {statusLabels[project.status] ?? project.status}
            </Badge>
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex items-center gap-4 text-xs text-muted-foreground">
            <span className="flex items-center gap-1">
              <IconGitBranch className="h-3.5 w-3.5" />
              {repoCount} {repoCount === 1 ? "repo" : "repos"}
            </span>
            <span>{counts.total} tasks</span>
            {counts.in_progress > 0 && (
              <span className="text-yellow-600 dark:text-yellow-400">
                {counts.in_progress} in progress
              </span>
            )}
            <span className="text-green-600 dark:text-green-400">{counts.done} done</span>
          </div>

          {counts.total > 0 && (
            <div className="space-y-1">
              <Progress value={progressPct} className="h-1.5" />
              <p className="text-[10px] text-muted-foreground text-right">{progressPct}%</p>
            </div>
          )}

          {leadAgentName && <p className="text-xs text-muted-foreground">Lead: {leadAgentName}</p>}
        </CardContent>
      </Card>
    </Link>
  );
}
