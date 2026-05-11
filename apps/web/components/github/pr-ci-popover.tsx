"use client";

import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
import {
  IconCheck,
  IconCircleCheck,
  IconCircleDot,
  IconCircleX,
  IconExternalLink,
  IconGitPullRequest,
  IconMessageCircle,
  IconPlus,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import { useCommentsStore } from "@/lib/state/slices/comments";
import type { PRFeedbackComment } from "@/lib/state/slices/comments";
import { useGitHubStatus } from "@/hooks/domains/github/use-github-status";
import { usePRCIPopover } from "@/hooks/domains/github/use-pr-ci-popover";
import {
  bucketCheck,
  bucketCheckCounts,
  groupChecksByWorkflow,
  type CheckBucket,
  type WorkflowGroup,
} from "@/lib/github/check-buckets";
import type { CheckRun, TaskPR } from "@/lib/types/github";

type CountsView = {
  passed: number;
  inProgress: number;
  failed: number;
};

const CHECK_GROUP_ORDER: CheckBucket[] = ["passed", "in_progress", "failed"];

function countForBucket(counts: CountsView, kind: CheckBucket): number {
  if (kind === "passed") return counts.passed;
  if (kind === "in_progress") return counts.inProgress;
  return counts.failed;
}

function deriveAggregateCounts(pr: TaskPR): CountsView {
  // Pre-load: if checks_total is populated, derive a coarse split using
  // checks_state to attribute the "non-passing" remainder. This is good
  // enough for the instant render — the lazy PRFeedback fetch refines.
  const total = pr.checks_total;
  const passing = pr.checks_passing;
  const remaining = Math.max(0, total - passing);
  if (pr.checks_state === "failure") {
    return { passed: passing, failed: remaining, inProgress: 0 };
  }
  if (pr.checks_state === "pending") {
    return { passed: passing, failed: 0, inProgress: remaining };
  }
  if (pr.checks_state === "success") {
    return { passed: total, failed: 0, inProgress: 0 };
  }
  return { passed: passing, failed: 0, inProgress: remaining };
}

function PRCIPopoverHeader({ pr }: { pr: TaskPR }) {
  const checksUrl = `${pr.pr_url}/checks`;
  return (
    <div
      data-testid="pr-popover-header"
      className="flex items-center justify-between gap-2 border-b border-border/50 pb-2"
    >
      <span className="text-sm font-medium">CI status</span>
      <a
        data-testid="pr-popover-external-link"
        href={checksUrl}
        target="_blank"
        rel="noopener noreferrer"
        className="cursor-pointer text-muted-foreground hover:text-foreground"
        aria-label="View all checks on GitHub"
        onClick={(e) => e.stopPropagation()}
      >
        <IconExternalLink className="h-3.5 w-3.5" />
      </a>
    </div>
  );
}

function CheckGroupIcon({ kind }: { kind: CheckBucket }) {
  if (kind === "passed") return <IconCircleCheck className="h-3.5 w-3.5 text-emerald-500" />;
  if (kind === "in_progress")
    return <IconCircleDot className="h-3.5 w-3.5 text-yellow-500 animate-pulse" />;
  return <IconCircleX className="h-3.5 w-3.5 text-red-500" />;
}

function bucketLabel(kind: CheckBucket): string {
  if (kind === "passed") return "Passed";
  if (kind === "in_progress") return "In progress";
  return "Failed";
}

function CheckGroupHeader({ kind, count }: { kind: CheckBucket; count: number }) {
  const label = bucketLabel(kind);
  return (
    <div className="flex items-center justify-between gap-2 px-1 py-1">
      <div className="flex items-center gap-1.5">
        <CheckGroupIcon kind={kind} />
        <span className="text-xs font-medium">{label}</span>
      </div>
      <span
        data-testid="pr-check-group-count"
        className="text-xs tabular-nums text-muted-foreground"
      >
        {count}
      </span>
    </div>
  );
}

function PRWorkflowRow({
  group,
  onAddAsContext,
}: {
  group: WorkflowGroup;
  onAddAsContext: ((message: string) => void) | null;
}) {
  // For an in-progress workflow, "0/1 ran" reads as "nothing finished"
  // and confuses people: did it start? It's clearer to just report how
  // many jobs are running. (Failed workflows go to a different bucket,
  // so an in_progress group never has failed jobs.)
  const badge =
    group.bucket === "in_progress"
      ? `${group.inProgress} running`
      : `${group.passed}/${group.total} passed`;
  return (
    <div
      data-testid="pr-workflow-row"
      data-workflow={group.workflow}
      data-bucket={group.bucket}
      className="flex items-center gap-2 px-2 py-1 rounded-sm hover:bg-accent/50 cursor-pointer"
      onClick={() => {
        if (group.htmlUrl) window.open(group.htmlUrl, "_blank", "noopener,noreferrer");
      }}
    >
      <span className="text-xs font-medium truncate min-w-0 flex-1" title={group.workflow}>
        {group.workflow}
      </span>
      <span className="text-[10px] text-muted-foreground shrink-0">{badge}</span>
      {group.htmlUrl && (
        <Button
          data-testid="pr-workflow-open"
          size="sm"
          variant="ghost"
          className="h-5 w-5 p-0 cursor-pointer"
          onClick={(e) => {
            e.stopPropagation();
            window.open(group.htmlUrl, "_blank", "noopener,noreferrer");
          }}
          aria-label={`Open ${group.workflow} on GitHub`}
        >
          <IconExternalLink className="h-3 w-3" />
        </Button>
      )}
      {group.bucket === "failed" && onAddAsContext && (
        <Button
          data-testid="pr-workflow-add-context"
          size="sm"
          variant="ghost"
          className="h-5 w-5 p-0 cursor-pointer"
          onClick={(e) => {
            e.stopPropagation();
            onAddAsContext(buildWorkflowMessage(group));
          }}
          aria-label={`Add ${group.workflow} failures to chat context`}
        >
          <IconPlus className="h-3 w-3" />
        </Button>
      )}
    </div>
  );
}

function buildWorkflowMessage(group: WorkflowGroup): string {
  const failed = group.jobs.filter((j) => bucketCheck(j) === "failed");
  const lines: string[] = [
    `### Workflow **${group.workflow}** has ${failed.length} failing job${failed.length !== 1 ? "s" : ""}.`,
    "",
  ];
  for (const job of failed) {
    lines.push(`- **${job.name}** — ${job.conclusion || job.status}`);
    if (job.output) lines.push(`  ${job.output}`);
    if (job.html_url) lines.push(`  ${job.html_url}`);
  }
  lines.push("", "Please investigate and fix.");
  return lines.join("\n");
}

function PRCheckGroup({
  kind,
  count,
  workflows,
  isLoading,
  onAddAsContext,
}: {
  kind: CheckBucket;
  count: number;
  workflows?: WorkflowGroup[];
  isLoading: boolean;
  onAddAsContext: ((message: string) => void) | null;
}) {
  if (count <= 0 && (!workflows || workflows.length === 0)) return null;
  const showRows = kind !== "passed";
  const hasWorkflows = !!workflows && workflows.length > 0;
  return (
    <div data-testid="pr-check-group" data-kind={kind} className="flex flex-col">
      <CheckGroupHeader kind={kind} count={count} />
      {showRows && (
        <div className="flex flex-col pl-5">
          {hasWorkflows &&
            workflows!.map((g) => (
              <PRWorkflowRow
                key={`${g.workflow}-${g.bucket}`}
                group={g}
                onAddAsContext={kind === "failed" ? onAddAsContext : null}
              />
            ))}
          {!hasWorkflows && isLoading
            ? Array.from({ length: Math.min(2, count || 1) }).map((_, i) => (
                <div
                  key={`skel-${i}`}
                  data-testid="pr-workflow-row-skeleton"
                  className="h-5 my-0.5 rounded-sm bg-muted animate-pulse"
                />
              ))
            : null}
        </div>
      )}
    </div>
  );
}

/**
 * Stacked progress bar mirroring TodoIndicator's bar but split into three
 * segments — green (passed), yellow (in_progress), red (failed) — each
 * sized proportionally to its count. The leading "pass rate" label gives
 * the same at-a-glance number TodoIndicator shows ("6/10 (60%)").
 *
 * Skipped/cancelled-but-not-failed jobs aren't counted toward the total
 * (they never enter any bucket), so the bar always reflects the actionable
 * picture: how close are we to "all green".
 */
function PRChecksProgressBar({ counts }: { counts: CountsView }) {
  const total = counts.passed + counts.inProgress + counts.failed;
  if (total === 0) return null;
  const pct = (n: number) => (n / total) * 100;
  const passedPct = pct(counts.passed);
  const inProgressPct = pct(counts.inProgress);
  const failedPct = pct(counts.failed);
  const completePct = Math.round(passedPct);
  return (
    <div data-testid="pr-checks-progress" className="flex flex-col gap-1.5 px-1 pt-1 pb-1.5">
      <div className="flex items-center justify-between text-xs">
        <span className="font-medium text-foreground">Pass rate</span>
        <span className="text-muted-foreground tabular-nums">
          {counts.passed}/{total} ({completePct}%)
        </span>
      </div>
      <div className="flex h-1.5 w-full overflow-hidden rounded-full bg-muted/70">
        {passedPct > 0 && (
          <div
            data-segment="passed"
            className="h-full bg-green-500 transition-[width]"
            style={{ width: `${passedPct}%` }}
          />
        )}
        {inProgressPct > 0 && (
          <div
            data-segment="in_progress"
            className="h-full bg-yellow-500 transition-[width]"
            style={{ width: `${inProgressPct}%` }}
          />
        )}
        {failedPct > 0 && (
          <div
            data-segment="failed"
            className="h-full bg-red-500 transition-[width]"
            style={{ width: `${failedPct}%` }}
          />
        )}
      </div>
    </div>
  );
}

function PRChecksSection({
  pr,
  feedback,
  isFetching,
  onAddAsContext,
}: {
  pr: TaskPR;
  feedback: { checks?: CheckRun[] } | null;
  isFetching: boolean;
  onAddAsContext: ((message: string) => void) | null;
}) {
  const aggregateCounts = useMemo(() => deriveAggregateCounts(pr), [pr]);

  const { precise, byBucket } = useMemo(() => {
    // Treat empty `feedback.checks` the same as "feedback not loaded yet" so
    // we keep showing the aggregate counts. Some mock paths return empty
    // arrays without errors, and we don't want the popover to flash 0/0/0.
    if (!feedback?.checks || feedback.checks.length === 0) {
      return { precise: null as CountsView | null, byBucket: null };
    }
    const counts = bucketCheckCounts(feedback.checks);
    const precise: CountsView = {
      passed: counts.passed,
      inProgress: counts.inProgress,
      failed: counts.failed,
    };
    const groups = groupChecksByWorkflow(feedback.checks);
    const byBucket: Record<CheckBucket, WorkflowGroup[]> = {
      passed: groups.filter((g) => g.bucket === "passed"),
      in_progress: groups.filter((g) => g.bucket === "in_progress"),
      failed: groups.filter((g) => g.bucket === "failed"),
    };
    return { precise, byBucket };
  }, [feedback]);

  const counts = precise ?? aggregateCounts;
  // "No checks at all": the lazy fetch has settled (not isFetching) and there
  // are no checks anywhere — neither in PRFeedback nor in the aggregate.
  const noChecksAtAll =
    !isFetching && pr.checks_total === 0 && (!feedback || (feedback.checks?.length ?? 0) === 0);
  if (noChecksAtAll) {
    return (
      <div data-testid="pr-checks-section" className="flex flex-col">
        <div data-testid="pr-checks-empty" className="px-1 py-2 text-xs text-muted-foreground">
          No checks have started
        </div>
      </div>
    );
  }
  return (
    <div data-testid="pr-checks-section" className="flex flex-col gap-1">
      <PRChecksProgressBar counts={counts} />
      {CHECK_GROUP_ORDER.map((kind) => {
        const value = countForBucket(counts, kind);
        return (
          <PRCheckGroup
            key={kind}
            kind={kind}
            count={value}
            workflows={byBucket?.[kind]}
            isLoading={isFetching && !byBucket}
            onAddAsContext={kind === "failed" ? onAddAsContext : null}
          />
        );
      })}
    </div>
  );
}

function PRReviewRow({ pr }: { pr: TaskPR }) {
  const required = pr.required_reviews ?? null;
  const approved = pr.review_count;
  const requested = pr.pending_review_count;

  let label: string;
  let icon: ReactNode;
  if (pr.review_state === "approved") {
    icon = <IconCheck className="h-3.5 w-3.5 text-emerald-500" />;
    label = "Approved";
  } else if (pr.review_state === "changes_requested") {
    icon = <IconCircleX className="h-3.5 w-3.5 text-red-500" />;
    label = "Changes requested";
  } else {
    // Default branch covers "no reviews yet", "review pending", and any
    // unknown state. Always rendered (no early-return) so a fresh PR
    // still surfaces its review status.
    icon = <IconCircleDot className="h-3.5 w-3.5 text-muted-foreground" />;
    label = "Awaiting review";
  }
  // Counts live on the right edge, mirroring the check-group rows.
  // Use "K / M" when a required-minimum is known, otherwise just the
  // approved count. The "(K requested)" suffix slides in when reviewers
  // have been pinged but haven't submitted a review.
  let countText: string;
  if (required != null) countText = `${approved} / ${required}`;
  else countText = `${approved}`;
  if (requested > 0) countText += ` · ${requested} requested`;
  return (
    <div
      data-testid="pr-review-row"
      className="flex items-center justify-between gap-2 px-1 py-1 text-xs"
    >
      <div className="flex items-center gap-1.5 min-w-0">
        {icon}
        <span className="truncate">{label}</span>
      </div>
      <span className="shrink-0 text-muted-foreground tabular-nums">{countText}</span>
    </div>
  );
}

function PRCommentsRow({ pr }: { pr: TaskPR }) {
  if (!pr.unresolved_review_threads || pr.unresolved_review_threads <= 0) return null;
  return (
    <div data-testid="pr-comments-row" className="flex items-center gap-1.5 px-1 py-1 text-xs">
      <IconMessageCircle className="h-3.5 w-3.5 text-muted-foreground" />
      <span>
        {pr.unresolved_review_threads} unresolved comment
        {pr.unresolved_review_threads === 1 ? "" : "s"}
      </span>
    </div>
  );
}

function elapsedLabel(elapsed: number | null): string {
  if (elapsed == null) return "";
  if (elapsed === 0) return "updated just now";
  return `updated ${formatElapsedShort(elapsed)} ago`;
}

function PRPopoverFooter({ lastUpdatedAt }: { lastUpdatedAt: number | null }) {
  // Capture a "now" snapshot only inside the setInterval callback (an event
  // handler — no impurity in render, no setState during commit). When no
  // tick has fired yet, fall back to lastUpdatedAt so the rendered elapsed
  // is 0 ("just now").
  const [now, setNow] = useState<number | null>(null);
  useEffect(() => {
    if (lastUpdatedAt == null) return;
    const id = setInterval(() => setNow(Date.now()), 10_000);
    return () => clearInterval(id);
  }, [lastUpdatedAt]);
  const elapsed =
    lastUpdatedAt == null
      ? null
      : Math.max(0, Math.floor(((now ?? lastUpdatedAt) - lastUpdatedAt) / 1000));
  if (lastUpdatedAt == null) return null;
  return (
    <div
      data-testid="pr-popover-footer"
      className="flex items-center justify-end border-t border-border/50 pt-1.5"
    >
      <span
        data-testid="pr-popover-updated-at"
        className="text-[10px] text-muted-foreground tabular-nums"
      >
        {elapsedLabel(elapsed)}
      </span>
    </div>
  );
}

function formatElapsedShort(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  return `${hours}h`;
}

function ReconnectGitHubBlock() {
  return (
    <div
      data-testid="pr-popover-auth-error"
      className="flex flex-col items-start gap-1 px-1 py-2 text-xs"
    >
      <span className="text-foreground">GitHub authentication lost.</span>
      <a
        data-testid="pr-popover-reconnect-link"
        href="/settings#github"
        className="cursor-pointer text-primary hover:underline"
      >
        Reconnect GitHub
      </a>
    </div>
  );
}

export function PRCIPopover({
  pr,
  enabled,
  onOpenDetailPanel,
}: {
  pr: TaskPR;
  enabled: boolean;
  onOpenDetailPanel?: () => void;
}) {
  const ghStatus = useAppStore((s) => s.githubStatus.status);
  const authLost = ghStatus !== null && !ghStatus.authenticated;
  // Trigger an initial status load from the same hook the rest of the app uses.
  useGitHubStatus();
  const { feedback, isFetching, lastUpdatedAt } = usePRCIPopover(pr, enabled && !authLost);
  const onAddAsContext = useAddCheckToContext(pr);

  return (
    <div
      data-testid="pr-topbar-popover-inner"
      className="flex flex-col gap-2"
      onClick={(e) => e.stopPropagation()}
    >
      <PRCIPopoverHeader pr={pr} />
      {authLost ? (
        <ReconnectGitHubBlock />
      ) : (
        <>
          <PRChecksSection
            pr={pr}
            feedback={feedback}
            isFetching={isFetching}
            onAddAsContext={onAddAsContext}
          />
          <div className="flex flex-col gap-0">
            <PRReviewRow pr={pr} />
            <PRCommentsRow pr={pr} />
          </div>
        </>
      )}
      {onOpenDetailPanel && (
        <Button
          data-testid="pr-popover-open-detail"
          size="sm"
          variant="ghost"
          className="cursor-pointer justify-start gap-1.5 px-2"
          onClick={onOpenDetailPanel}
        >
          <IconGitPullRequest className="h-3.5 w-3.5" />
          <span>Open PR details</span>
        </Button>
      )}
      <PRPopoverFooter lastUpdatedAt={lastUpdatedAt} />
    </div>
  );
}

// --- Add-to-context wiring (mirrors pr-detail-panel.tsx for failed checks) ---
function useAddCheckToContext(pr: TaskPR): ((message: string) => void) | null {
  const sessionId = useAppStore((s) => s.tasks.activeSessionId);
  const addComment = useCommentsStore((s) => s.addComment);
  const { toast } = useToast();
  const prNumber = pr.pr_number;
  // Always call useCallback (rules-of-hooks) before bailing out, so the
  // returned callback identity is stable across renders unless its inputs
  // change. Without memoization the popover would create a new function
  // every parent render, defeating PRWorkflowRow's cheap reference equality.
  const handler = useCallback(
    (message: string) => {
      if (!sessionId) return;
      const comment: PRFeedbackComment = {
        id: `pr-feedback-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
        sessionId,
        text: message,
        createdAt: new Date().toISOString(),
        status: "pending",
        source: "pr-feedback",
        prNumber,
        feedbackType: "check",
        content: message,
      };
      addComment(comment);
      toast({ description: "Added to chat context" });
    },
    [sessionId, prNumber, addComment, toast],
  );
  return sessionId ? handler : null;
}

// Multi-PR aggregate popover deferred — multi-PR tasks currently keep the
// existing DropdownMenu trigger (no hover popover). The aggregate component
// will land alongside that hover variant when it ships, see follow-up.
