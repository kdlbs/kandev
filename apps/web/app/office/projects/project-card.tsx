"use client";

import Link from "@/components/routing/app-link";
import { IconGitBranch } from "@tabler/icons-react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Badge } from "@kandev/ui/badge";
import { Progress } from "@kandev/ui/progress";
import { useAppStore } from "@/components/state-provider";
import type { Project, ProjectStatus } from "@/lib/state/slices/office/types";
import { PROJECT_STATUS_LABEL_KEYS } from "../lib/label-keys";
import { normalizeRepos } from "./normalize-repos";
import { useTranslation } from "react-i18next";

const FALLBACK_BADGE_CLASSES: Record<string, string> = {
  active: "bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-300",
  completed: "bg-blue-100 text-blue-700 dark:bg-blue-900/50 dark:text-blue-300",
  on_hold: "bg-yellow-100 text-yellow-700 dark:bg-yellow-900/50 dark:text-yellow-300",
  archived: "bg-neutral-100 text-neutral-700 dark:bg-neutral-900/50 dark:text-neutral-300",
};

// Keys, not labels — module scope freezes a `t()` at the boot locale. The
// record keys are the wire project-status values. The workspace's own project
// status metadata wins when loaded, and those labels are workspace data.

type ProjectCardProps = {
  project: Project;
  leadAgentName?: string;
};

function useProjectStatusDisplay(status: string) {
  const { t } = useTranslation();
  const meta = useAppStore((s) => s.office.meta);
  const metaStatus = meta?.projectStatuses.find((s) => s.id === status);
  const fallbackKey = PROJECT_STATUS_LABEL_KEYS[status as ProjectStatus];
  return {
    badgeClass: metaStatus?.color ?? FALLBACK_BADGE_CLASSES[status] ?? "",
    // `?? status` keeps an unknown wire value visible rather than blank.
    label: metaStatus?.label ?? (fallbackKey ? t(fallbackKey) : status),
  };
}

function ProjectStats({ project }: { project: Project }) {
  const { t } = useTranslation();
  const counts = project.taskCounts ?? { total: 0, in_progress: 0, done: 0, blocked: 0 };
  const repoCount = normalizeRepos(project.repositories).length;
  const progressPct = counts.total > 0 ? Math.round((counts.done / counts.total) * 100) : 0;

  return (
    <>
      <div className="flex items-center gap-4 text-xs text-muted-foreground">
        <span className="flex items-center gap-1">
          <IconGitBranch className="h-3.5 w-3.5" />
          {t("office:repoCount", { count: repoCount })}
        </span>
        <span>{t("office:taskCount", { count: counts.total })}</span>
        {counts.in_progress > 0 && (
          <span className="text-yellow-600 dark:text-yellow-400">
            {t("office:countInProgress", { count: counts.in_progress })}
          </span>
        )}
        <span className="text-green-600 dark:text-green-400">
          {t("office:countDone", { count: counts.done })}
        </span>
      </div>
      {counts.total > 0 && (
        <div className="space-y-1">
          <Progress value={progressPct} className="h-1.5" />
          <p className="text-[10px] text-muted-foreground text-right">{progressPct}%</p>
        </div>
      )}
    </>
  );
}

export function ProjectCard({ project, leadAgentName }: ProjectCardProps) {
  const { t } = useTranslation();
  const { badgeClass, label: statusLabel } = useProjectStatusDisplay(project.status);

  return (
    <Link href={`/office/projects/${project.id}`} className="block cursor-pointer">
      <Card className="hover:bg-accent/50 transition-colors">
        <CardHeader className="pb-2">
          <div className="flex items-center gap-2">
            <span
              className="h-3 w-3 rounded-sm shrink-0"
              style={{ backgroundColor: project.color || "#6b7280" }}
            />
            <CardTitle className="text-sm font-medium truncate flex-1">{project.name}</CardTitle>
            <Badge className={badgeClass}>{statusLabel}</Badge>
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          <ProjectStats project={project} />
          {leadAgentName && (
            <p className="text-xs text-muted-foreground">
              {t("office:leadAgentName", { name: leadAgentName })}
            </p>
          )}
        </CardContent>
      </Card>
    </Link>
  );
}
