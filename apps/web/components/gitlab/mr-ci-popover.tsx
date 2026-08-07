"use client";

import { useEffect, useMemo, useState, type ReactNode } from "react";
import {
  IconCheck,
  IconCircleDot,
  IconExternalLink,
  IconMessageCircle,
  IconPlus,
  IconUnlink,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useMRFeedback } from "@/hooks/domains/gitlab/use-mr-feedback";
import { bucketJobCounts, groupJobsByStage, type JobBucket } from "@/lib/gitlab/pipeline-buckets";
import type { GitLabPipelineJob, TaskMR } from "@/lib/types/gitlab";
import { MRAutomationControls } from "./mr-automation-controls";
import { MRMergeButton } from "./mr-merge-button";

/**
 * Gates useMRFeedback's fetch on `enabled` by nulling its identity args
 * rather than fetching and discarding — useMRFeedback (C12) has no
 * built-in enabled/lazy flag of its own.
 */
function useMRFeedbackGated(mr: TaskMR, enabled: boolean) {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  return useMRFeedback(
    enabled ? workspaceId : null,
    enabled ? mr.project_path : null,
    enabled ? mr.mr_iid : null,
    mr.host,
  );
}

function MRPopoverTitle({ mr, onOpenDetailPanel }: { mr: TaskMR; onOpenDetailPanel?: () => void }) {
  const titleClassName = "min-w-0 flex-1 text-left";
  const content = (
    <>
      <div className="truncate text-xs font-medium text-foreground">
        !{mr.mr_iid} {mr.mr_title}
      </div>
      <div className="truncate text-[11px] text-muted-foreground">{mr.project_path}</div>
    </>
  );
  if (!onOpenDetailPanel) return <div className={titleClassName}>{content}</div>;
  return (
    <button
      type="button"
      data-testid="mr-popover-title"
      className={`${titleClassName} cursor-pointer`}
      onClick={(e) => {
        e.stopPropagation();
        onOpenDetailPanel();
      }}
    >
      {content}
    </button>
  );
}

function MRPopoverHeader({
  mr,
  onOpenDetailPanel,
  onUnlink,
}: {
  mr: TaskMR;
  onOpenDetailPanel?: () => void;
  onUnlink: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-start justify-between gap-2 px-1">
      <MRPopoverTitle mr={mr} onOpenDetailPanel={onOpenDetailPanel} />
      <div className="flex shrink-0 items-center gap-0.5">
        <a
          data-testid="mr-popover-open-in-gitlab"
          href={mr.mr_url}
          target="_blank"
          rel="noopener noreferrer"
          className="flex h-6 w-6 items-center justify-center rounded text-muted-foreground hover:bg-muted hover:text-foreground cursor-pointer"
          aria-label={t("gitlab:mrPopoverOpenInGitlab", { mriid: mr.mr_iid })}
          onClick={(e) => e.stopPropagation()}
        >
          <IconExternalLink className="h-3.5 w-3.5" />
        </a>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-6 w-6 shrink-0 cursor-pointer text-muted-foreground hover:text-destructive"
          aria-label={t("gitlab:mrPopoverUnlink", { mriid: mr.mr_iid })}
          onClick={(e) => {
            e.stopPropagation();
            onUnlink();
          }}
        >
          <IconUnlink className="h-3.5 w-3.5" />
        </Button>
      </div>
    </div>
  );
}

function MRPopoverLinkAnother({ canLink, onLink }: { canLink: boolean; onLink: () => void }) {
  const { t } = useTranslation();
  if (!canLink) return null;
  return (
    <button
      type="button"
      data-testid="mr-popover-link-another"
      className="flex cursor-pointer items-center gap-1.5 rounded px-1 py-1 text-xs text-muted-foreground hover:bg-muted hover:text-foreground"
      onClick={(e) => {
        e.stopPropagation();
        onLink();
      }}
    >
      <IconPlus className="h-3.5 w-3.5" />
      {t("gitlab:linkAnotherMergeRequest")}
    </button>
  );
}

/**
 * Pass-rate bar: PipelineJobsPass / PipelineJobsTotal, a single green
 * segment — ports cleanly from GitHub's multi-segment bar since GitLab's
 * job buckets collapse to "does it count as passing" for this summary
 * (see plan §2). Renders nothing when there are no jobs to report (AC19
 * falls through to the empty-state row instead).
 */
function MRPipelineProgressBar({ passed, total }: { passed: number; total: number }) {
  const { t } = useTranslation();
  if (total <= 0) return null;
  const pct = Math.round((Math.min(passed, total) / total) * 100);
  return (
    <div data-testid="mr-pipeline-progress" className="flex flex-col gap-1.5 px-1 pt-1 pb-1.5">
      <div className="flex items-center justify-between text-xs">
        <span className="font-medium text-foreground">{t("gitlab:mrPopoverPassRate")}</span>
        <span className="tabular-nums text-muted-foreground">
          {passed}/{total} ({pct}%)
        </span>
      </div>
      <div className="flex h-1.5 w-full overflow-hidden rounded-full bg-muted/70">
        <div
          data-segment="passed"
          className="h-full bg-green-500 transition-[width]"
          style={{ width: `${pct}%` }}
        />
      </div>
    </div>
  );
}

function stageGroupIcon(bucket: JobBucket) {
  if (bucket === "failed") return <IconCircleDot className="h-3.5 w-3.5 text-red-500" />;
  if (bucket === "in_progress") return <IconCircleDot className="h-3.5 w-3.5 text-yellow-500" />;
  return <IconCheck className="h-3.5 w-3.5 text-emerald-500" />;
}

function MRPipelineStageGroups({ jobs }: { jobs: GitLabPipelineJob[] }) {
  const groups = useMemo(() => groupJobsByStage(jobs), [jobs]);
  if (groups.length === 0) return null;
  return (
    <div data-testid="mr-pipeline-stage-groups" className="flex flex-col gap-0.5">
      {groups.map((group) => (
        <a
          key={group.stage}
          href={group.webUrl}
          target="_blank"
          rel="noopener noreferrer"
          className="flex items-center justify-between gap-2 rounded px-1 py-1 text-xs hover:bg-muted/50"
          onClick={(e) => {
            if (!group.webUrl) e.preventDefault();
          }}
        >
          <div className="flex min-w-0 items-center gap-1.5">
            {stageGroupIcon(group.bucket)}
            <span className="truncate">{group.stage}</span>
          </div>
          <span className="shrink-0 tabular-nums text-muted-foreground">
            {group.passed}/{group.total}
          </span>
        </a>
      ))}
    </div>
  );
}

function MRPipelineEmptyState() {
  const { t } = useTranslation();
  return (
    <div data-testid="mr-pipeline-empty" className="px-1 py-2 text-xs text-muted-foreground">
      {t("gitlab:mrPopoverNoPipeline")}
    </div>
  );
}

/**
 * Approval row: reads TaskMR's own persisted aggregate fields directly
 * (approval_count / required_approvals / unapproved_reviewers), all
 * populated for free by GetMRStatus on every poll (Step 3) — no live
 * fetch needed, unlike the pipeline jobs section below. Mirrors GitHub's
 * PRReviewRow, adapted to Q2's decision (approvals + unapproved count
 * suffix rather than a three-way review-state label).
 */
function MRApprovalRow({ mr }: { mr: TaskMR }) {
  const { t } = useTranslation();
  const required = mr.required_approvals;
  const approved = mr.approval_count;
  // required === 0 still counts as approved once someone has approved —
  // matches the backend's summarizeApprovals, which returns "approved" for
  // required==0 && have>0. Requiring `required > 0` here contradicted that
  // and mislabeled optional-review projects as "Awaiting review".
  const isApproved = required > 0 ? approved >= required : approved > 0;
  const icon = isApproved ? (
    <IconCheck className="h-3.5 w-3.5 text-emerald-500" />
  ) : (
    <IconCircleDot className="h-3.5 w-3.5 text-muted-foreground" />
  );
  const label = isApproved ? t("gitlab:mrPopoverApproved") : t("gitlab:mrPopoverAwaitingReview");
  let countText = required > 0 ? `${approved} / ${required}` : `${approved}`;
  if (mr.unapproved_reviewers > 0) {
    countText += ` · ${t("gitlab:mrPopoverAwaitingCount", { count: mr.unapproved_reviewers })}`;
  }
  return (
    <div
      data-testid="mr-approval-row"
      className="flex items-center justify-between gap-2 px-1 py-1 text-xs"
    >
      <div className="flex min-w-0 items-center gap-1.5">
        {icon}
        <span className="truncate">{label}</span>
      </div>
      <span className="shrink-0 tabular-nums text-muted-foreground">{countText}</span>
    </div>
  );
}

/**
 * Discussions row: uses the live discussion fetch (unlike TaskMR's own
 * unresolved_discussions field, which is only populated for
 * automation-subscribed MRs — see Step 4's design note). Every linked MR
 * gets this summary regardless of automation state, so it must come from
 * the popover's own fetch.
 */
function MRDiscussionsRow({ unresolvedCount }: { unresolvedCount: number }) {
  const { t } = useTranslation();
  if (unresolvedCount <= 0) return null;
  return (
    <div data-testid="mr-discussions-row" className="flex items-center gap-1.5 px-1 py-1 text-xs">
      <IconMessageCircle className="h-3.5 w-3.5 text-muted-foreground" />
      <span>{t("gitlab:mrPopoverUnresolvedComments", { count: unresolvedCount })}</span>
    </div>
  );
}

function elapsedShort(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  return `${Math.floor(minutes / 60)}h`;
}

function MRPopoverFooter({ lastUpdatedAt }: { lastUpdatedAt: number | null }) {
  const { t } = useTranslation();
  const [now, setNow] = useState<number | null>(null);
  useEffect(() => {
    if (lastUpdatedAt == null) return;
    const id = setInterval(() => setNow(Date.now()), 10_000);
    return () => clearInterval(id);
  }, [lastUpdatedAt]);
  if (lastUpdatedAt == null) return null;
  const elapsed = Math.max(0, Math.floor(((now ?? lastUpdatedAt) - lastUpdatedAt) / 1000));
  const label =
    elapsed === 0
      ? t("gitlab:mrPopoverUpdatedJustNow")
      : t("gitlab:mrPopoverUpdatedAgo", { elapsed: elapsedShort(elapsed) });
  return (
    <div
      data-testid="mr-popover-footer"
      className="flex items-center justify-end border-t border-border/50 pt-1.5"
    >
      <span
        data-testid="mr-popover-updated-at"
        className="tabular-nums text-[10px] text-muted-foreground"
      >
        {label}
      </span>
    </div>
  );
}

export function MRCIPopover({
  mr,
  taskId,
  enabled,
  canLink,
  onOpenDetailPanel,
  onLink,
  onUnlink,
}: {
  mr: TaskMR;
  taskId: string;
  enabled: boolean;
  canLink: boolean;
  onOpenDetailPanel?: () => void;
  onLink: () => void;
  onUnlink: () => void;
}): ReactNode {
  const { feedback, loading, revision } = useMRFeedbackGated(mr, enabled);
  const [lastUpdatedAt, setLastUpdatedAt] = useState<number | null>(null);
  useEffect(() => {
    if (feedback && !loading) setLastUpdatedAt(Date.now());
    // Only the feedback identity/revision should re-stamp "updated at" —
    // a re-render from an unrelated prop change must not reset the clock.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [revision, loading]);

  const latestPipeline = feedback?.pipelines?.[0];
  const jobs = latestPipeline?.jobs ?? [];
  const precise = jobs.length > 0 ? bucketJobCounts(jobs) : null;
  const total = precise
    ? precise.passed + precise.inProgress + precise.failed
    : mr.pipeline_jobs_total;
  const passed = precise ? precise.passed : mr.pipeline_jobs_pass;
  const unresolvedDiscussions =
    feedback?.discussions?.filter((d) => d.resolvable && !d.resolved).length ?? 0;

  return (
    <div
      data-testid="mr-topbar-popover-inner"
      className="flex flex-col gap-2"
      onClick={(e) => e.stopPropagation()}
    >
      <MRPopoverHeader mr={mr} onOpenDetailPanel={onOpenDetailPanel} onUnlink={onUnlink} />
      {total > 0 ? (
        <>
          <MRPipelineProgressBar passed={passed} total={total} />
          <MRPipelineStageGroups jobs={jobs} />
        </>
      ) : (
        <MRPipelineEmptyState />
      )}
      <div className="flex flex-col gap-0">
        <MRApprovalRow mr={mr} />
        <MRDiscussionsRow unresolvedCount={unresolvedDiscussions} />
      </div>
      <MRAutomationControls taskId={taskId} mr={mr} />
      <MRMergeButton mr={mr} />
      <MRPopoverLinkAnother canLink={canLink} onLink={onLink} />
      <MRPopoverFooter lastUpdatedAt={lastUpdatedAt} />
    </div>
  );
}
