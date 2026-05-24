"use client";

import { memo } from "react";
import Link from "next/link";
import { IconBrandGitlab, IconCheck, IconClock, IconGitMerge, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTaskMRs } from "@/hooks/domains/gitlab/use-task-mr";
import { useAppStore } from "@/components/state-provider";
import type { TaskMR } from "@/lib/types/gitlab";

/**
 * Icon + colour for an MR's combined state. Mirrors github's pr-task-icon
 * priority order so a merged MR reads the same as a merged PR: terminal
 * states first, then pipeline failures / changes-requested, then ready-to-
 * merge, then awaiting-something, then pipeline-running.
 */
function MRStatusIcon({ mr }: { mr: TaskMR }) {
  if (mr.state === "merged") return <IconCheck className="h-3 w-3 text-purple-500" />;
  if (mr.state === "closed") return <IconX className="h-3 w-3 text-muted-foreground" />;
  if (mr.pipeline_state === "failure") return <IconX className="h-3 w-3 text-red-500" />;
  if (mr.approval_state === "approved" && mr.pipeline_state === "success" && !mr.draft) {
    return <IconCheck className="h-3 w-3 text-emerald-400" />;
  }
  if (mr.approval_state === "pending") return <IconClock className="h-3 w-3 text-sky-400" />;
  if (mr.pipeline_state === "pending") return <IconClock className="h-3 w-3 text-yellow-500" />;
  return null;
}

function statusTextColor(mr: TaskMR): string {
  if (mr.state === "merged") return "text-purple-500";
  if (mr.state === "closed") return "text-muted-foreground";
  if (mr.pipeline_state === "failure") return "text-red-500";
  if (mr.approval_state === "approved") return "text-emerald-400";
  if (mr.approval_state === "pending") return "text-sky-400";
  return "text-muted-foreground";
}

function MRSingleButton({ mr }: { mr: TaskMR }) {
  const tooltip = `${mr.project_path} !${mr.mr_iid} — ${mr.mr_title}`;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          asChild
          data-testid="mr-topbar-button"
          data-mr-iid={mr.mr_iid}
          data-mr-state={mr.state}
          size="sm"
          variant="outline"
          className="cursor-pointer gap-1.5 px-2"
        >
          <Link href={mr.mr_url} target="_blank" rel="noopener noreferrer">
            <IconGitMerge className={`h-4 w-4 ${statusTextColor(mr)}`} />
            <span className="text-xs font-medium">!{mr.mr_iid}</span>
            <MRStatusIcon mr={mr} />
          </Link>
        </Button>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

function MRMultiButton({ mrs }: { mrs: TaskMR[] }) {
  // 2+ MRs collapse into a single GitLab icon with a count. Detail panel
  // / dropdown for fan-out lands with phase 4.
  const failed = mrs.filter((m) => m.pipeline_state === "failure").length;
  const merged = mrs.filter((m) => m.state === "merged").length;
  const open = mrs.filter((m) => m.state === "open" || m.state === "opened").length;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button size="sm" variant="outline" className="cursor-pointer gap-1.5 px-2">
          <IconBrandGitlab className="h-4 w-4 text-orange-500" />
          <span className="text-xs font-medium">{mrs.length} MRs</span>
        </Button>
      </TooltipTrigger>
      <TooltipContent>
        {open > 0 && <div>{open} open</div>}
        {failed > 0 && <div className="text-red-500">{failed} failing</div>}
        {merged > 0 && <div className="text-purple-500">{merged} merged</div>}
      </TooltipContent>
    </Tooltip>
  );
}

export const MRTopbarButton = memo(function MRTopbarButton() {
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const mrs = useTaskMRs(activeTaskId);

  if (mrs.length === 0) return null;
  if (mrs.length === 1) return <MRSingleButton mr={mrs[0]} />;
  return <MRMultiButton mrs={mrs} />;
});
