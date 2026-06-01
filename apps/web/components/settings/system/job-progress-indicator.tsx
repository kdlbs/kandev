"use client";

import { Badge } from "@kandev/ui/badge";
import { Spinner } from "@kandev/ui/spinner";
import { IconCheck, IconAlertTriangle } from "@tabler/icons-react";
import { useSystemJob, useSystemJobs } from "@/hooks/domains/system/use-system-jobs";
import type { SystemJob, SystemJobKind } from "@/lib/types/system";

type JobProgressIndicatorProps = {
  kind: SystemJobKind;
  /** When provided, only the matching job id is rendered (ignores all others). */
  jobId?: string;
  /** Label shown when the job has succeeded. */
  successLabel?: string;
  /** Testid suffix; default "system-job-<kind>". */
  testId?: string;
};

function pickJob(jobs: SystemJob[], jobId?: string): SystemJob | null {
  if (!jobs.length) return null;
  if (jobId) return jobs.find((j) => j.id === jobId) ?? null;
  // Latest by started_at (fall back to insertion order).
  const sorted = [...jobs].sort((a, b) => {
    const at = Date.parse(a.started_at) || 0;
    const bt = Date.parse(b.started_at) || 0;
    return bt - at;
  });
  return sorted[0] ?? null;
}

function badgeVariant(state: SystemJob["state"]): "destructive" | "secondary" | "outline" {
  if (state === "failed") return "destructive";
  if (state === "succeeded") return "secondary";
  return "outline";
}

function stateLabel(state: SystemJob["state"]): string {
  switch (state) {
    case "queued":
      return "Queued";
    case "running":
      return "Running";
    case "succeeded":
      return "Done";
    case "failed":
      return "Failed";
    default:
      return state;
  }
}

export function JobProgressIndicator({
  kind,
  jobId,
  successLabel,
  testId,
}: JobProgressIndicatorProps) {
  const pinnedJob = useSystemJob(jobId);
  const jobs = useSystemJobs(kind);
  const job = jobId ? (pinnedJob ?? pickJob(jobs, jobId)) : pickJob(jobs);
  if (!job) return null;

  const tid = testId ?? `system-job-${kind}`;
  const isActive = job.state === "queued" || job.state === "running";
  const isSuccess = job.state === "succeeded";
  const isFailed = job.state === "failed";

  return (
    <div
      className="inline-flex items-center gap-2 text-xs text-muted-foreground"
      data-testid={tid}
      data-state={job.state}
    >
      {isActive && <Spinner className="size-3.5" />}
      {isSuccess && <IconCheck className="size-3.5 text-emerald-500" />}
      {isFailed && <IconAlertTriangle className="size-3.5 text-red-500" />}
      <Badge variant={badgeVariant(job.state)} className="text-[10px]">
        {isSuccess && successLabel ? successLabel : stateLabel(job.state)}
      </Badge>
      {job.message && <span className="truncate max-w-[24rem]">{job.message}</span>}
    </div>
  );
}
