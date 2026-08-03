"use client";

import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
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

/**
 * `state` is a wire enum, so it travels as the key and only the label is copy.
 * The `default` branch echoes the raw token so a state from a newer backend
 * still renders something rather than blank.
 */
function stateLabel(state: SystemJob["state"], t: TFunction): string {
  switch (state) {
    case "queued":
      return t("system:jobStateQueued");
    case "running":
      return t("system:jobStateRunning");
    case "succeeded":
      return t("system:jobStateDone");
    case "failed":
      return t("system:jobStateFailed");
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
  const { t } = useTranslation();
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
        {isSuccess && successLabel ? successLabel : stateLabel(job.state, t)}
      </Badge>
      {job.message && <span className="truncate max-w-[24rem]">{job.message}</span>}
    </div>
  );
}
