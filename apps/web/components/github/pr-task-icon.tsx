"use client";

import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useChangeRequestTaskTooltipState } from "@/components/integrations/use-change-request-task-tooltip-state";
import {
  CHANGE_REQUEST_STATUS_COLORS,
  CHANGE_REQUEST_STATUS_RANK,
  getChangeRequestAggregateStatusColor,
} from "@/components/integrations/change-request-task-status-color";
import {
  getTaskPRsForCurrentWorkspace,
  useTaskPRTooltipHydration,
} from "@/hooks/domains/github/use-task-pr-tooltip-hydration";
import type { TaskPR } from "@/lib/types/github";
import type { TaskCIAutomationOptions } from "@/lib/types/github";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { derivePRTaskStatusSummary } from "./pr-task-status-summary";
import {
  createPRTaskIconDisclosureProps,
  PRTaskIconDrawer,
  PRTaskIconTooltip,
} from "./pr-task-icon-disclosure";
import { getTaskPRAutomationSummary, type TaskPRInfo } from "./pr-task-automation";
import { buildPRTaskIconAriaLabel } from "./pr-task-icon-aria";
import { getTaskPRWorkflowAttention, isWorkflowApprovalRequired } from "./pr-workflow-attention";
import {
  compactWorkflowApprovalIsNewerThanFullPRs,
  getCompactPRStatusAccessibleLabels,
  getCompactStaleWorkflowPRs,
  getCompactWorkflowStatusSummaries,
  getProjectedPRRepository,
} from "./pr-task-workflow-projection";

export type { TaskPRInfo } from "./pr-task-automation";
export { getTaskPRAutomationSummary } from "./pr-task-automation";
export { AutomationIndicatorDots } from "./pr-task-icon-disclosure";

const MUTED_FOREGROUND = CHANGE_REQUEST_STATUS_COLORS.muted;
const PURPLE_500 = CHANGE_REQUEST_STATUS_COLORS.merged;
const RED_500 = CHANGE_REQUEST_STATUS_COLORS.danger;
const YELLOW_500 = CHANGE_REQUEST_STATUS_COLORS.warning;
const SKY_400 = CHANGE_REQUEST_STATUS_COLORS.review;
const EMERALD_400 = CHANGE_REQUEST_STATUS_COLORS.ready;
const QUEUED = CHANGE_REQUEST_STATUS_COLORS.queued;
const GREEN_500 = CHANGE_REQUEST_STATUS_COLORS.passing;
const EMPTY_PRS: TaskPR[] = [];

/** Maps the task-level PR projection to the same visual language as live PRs. */
export function getPRAggregateStatusColor(state: string | null | undefined): string {
  return getChangeRequestAggregateStatusColor(state);
}

// Higher = more attention-worthy. Drives the aggregated icon color when a
// task has multiple PRs (we surface the worst state).
const STATUS_RANK = CHANGE_REQUEST_STATUS_RANK;

function hasExplicitPRChecksPassed(pr: TaskPR): boolean {
  return pr.checks_state === "success";
}

export function hasPRChecksPassedForDisplay(pr: TaskPR): boolean {
  if (pr.checks_state === "success") return true;
  if (pr.checks_state !== "" || pr.checks_total <= 0) return false;
  return pr.checks_passing >= pr.checks_total;
}

export function hasPRChecksInProgressForDisplay(pr: TaskPR): boolean {
  if (pr.checks_state === "pending") return true;
  return pr.checks_state === "" && pr.checks_total > 0 && pr.checks_passing < pr.checks_total;
}

export function hasPRChecksPassedWithoutReviewWaitForDisplay(pr: TaskPR): boolean {
  if (!hasPRChecksPassedForDisplay(pr)) return false;
  if (pr.required_reviews != null && pr.review_count < pr.required_reviews) return false;
  if (pr.pending_review_count > 0) return false;
  return pr.review_state === "approved" || pr.review_state === "";
}

// Requires a positive CI signal so repos with no CI configured won't trigger
// ready-to-merge on mergeable_state=clean alone. Display surfaces may fall
// back to aggregate counts, but merge actions require GitHub's explicit
// success rollup because stored counts can be preserved across lightweight
// syncs that do not populate check details.
export function isPRReadyToMerge(pr: TaskPR): boolean {
  if (pr.state !== "open") return false;
  if (getTaskPRWorkflowAttention(pr)) return false;
  if (!hasExplicitPRChecksPassed(pr)) return false;
  if (pr.mergeable_state !== "clean") return false;
  // Guard against stale mergeable_state: enforce required_reviews to match GitHub's gate.
  if (pr.required_reviews != null && pr.review_count < pr.required_reviews) {
    return false;
  }
  if (pr.pending_review_count > 0) return false;
  if (pr.review_state === "approved") return true;
  // No review process: no requested reviewers and no submitted reviews. GitHub
  // sets mergeable_state=clean when branch protection is satisfied, so this
  // covers repos without required reviewers.
  return pr.review_state === "" && pr.pending_review_count === 0;
}

// GitHub overloads `blocked` for merge queues and unrelated repository rules.
// Keep readiness clean-only, but allow a neutral merge attempt so GitHub can
// authoritatively accept the PR into a queue or return the actual blocker.
export function canAttemptPRMerge(pr: TaskPR): boolean {
  if (pr.mergeable_state !== "clean" && pr.mergeable_state !== "blocked") return false;
  return isPRReadyToMerge({ ...pr, mergeable_state: "clean" });
}

export function isPRDraft(pr: TaskPR): boolean {
  return pr.state === "open" && pr.mergeable_state === "draft";
}

function getPRLifecycleAccessibleLabel(
  pr: TaskPR,
  t: ReturnType<typeof useTranslation>["t"],
): string | null {
  if (pr.state === "merged") return t("github:merged");
  if (pr.state === "closed") return t("github:closed");
  if (isPRDraft(pr)) return t("github:draft");
  if (pr.state === "open") return t("common:open");
  return null;
}

function getPRWorkflowAccessibleLabel(
  pr: TaskPR,
  t: ReturnType<typeof useTranslation>["t"],
): string | null {
  const workflowAttention = getTaskPRWorkflowAttention(pr);
  if (workflowAttention && isWorkflowApprovalRequired(workflowAttention)) {
    return t("github:workflowAwaitingApproval");
  }
  return null;
}

function getPRChecksAccessibleLabel(
  pr: TaskPR,
  t: ReturnType<typeof useTranslation>["t"],
): string | null {
  if (pr.checks_state === "failure") return t("github:checksFailed");
  if (pr.checks_state === "success") return t("github:checksPassed");
  if (!hasPRChecksInProgressForDisplay(pr)) return null;

  const pendingCount = pr.checks_total > 0 ? Math.max(1, pr.checks_total - pr.checks_passing) : 1;
  return t("github:checksPendingCount", { count: pendingCount });
}

function getPRMergeabilityAccessibleLabel(
  pr: TaskPR,
  t: ReturnType<typeof useTranslation>["t"],
): string | null {
  if (pr.mergeable_state === "behind") return t("github:behindBase");
  if (isPRWaitingOnBranchProtection(pr)) return t("github:blockedByBranchProtection");
  return null;
}

function getPRReviewAccessibleLabel(
  pr: TaskPR,
  t: ReturnType<typeof useTranslation>["t"],
): string | null {
  switch (pr.review_state) {
    case "changes_requested":
      return t("github:changesRequested");
    case "approved":
      return t("github:approved");
    case "pending":
      return t("github:pendingReview");
    default:
      return null;
  }
}

export function getPRStatusAccessibleLabels(
  pr: TaskPR,
  t: ReturnType<typeof useTranslation>["t"],
): string[] {
  const lifecycleLabel = getPRLifecycleAccessibleLabel(pr, t);
  if (pr.state !== "open") return lifecycleLabel ? [lifecycleLabel] : [];

  return [
    lifecycleLabel,
    getPRWorkflowAccessibleLabel(pr, t),
    getPRChecksAccessibleLabel(pr, t),
    getPRMergeabilityAccessibleLabel(pr, t),
    getPRReviewAccessibleLabel(pr, t),
  ].filter((label): label is string => label !== null);
}

function getTaskPRStatusAccessibleLabels(
  prs: TaskPR[],
  prInfo: TaskPRInfo | undefined,
  t: ReturnType<typeof useTranslation>["t"],
): string[] {
  if (prs.length > 0) {
    return [...new Set(prs.flatMap((pr) => getPRStatusAccessibleLabels(pr, t)))];
  }
  if (prInfo) return getCompactPRStatusAccessibleLabels(prInfo, t);
  return [];
}

export function hasPRMergeConflict(pr: TaskPR): boolean {
  if (pr.state !== "open") return false;
  return pr.has_merge_conflicts ?? pr.mergeable_state === "dirty";
}

export function hasAnyPRMergeConflict(prs: TaskPR[]): boolean {
  return prs.some(hasPRMergeConflict);
}

function taskPRHasMergeConflict(prs: TaskPR[], prInfo?: TaskPRInfo): boolean {
  return prs.length > 0 ? hasAnyPRMergeConflict(prs) : prInfo?.hasMergeConflicts === true;
}

export function isPRQueued(pr: TaskPR): boolean {
  return (
    pr.state === "open" &&
    typeof pr.merge_queue_state === "string" &&
    pr.merge_queue_state.trim() !== ""
  );
}

// CI passed but the PR is still waiting on human review (reviewers requested
// or pending review state). Distinct from yellow "CI running". An approved
// PR with extra reviewers still pending also counts — GitHub's
// review_state="approved" only means at least one reviewer approved, not
// that branch protection's required count is met.
export function isPRAwaitingReview(pr: TaskPR): boolean {
  if (pr.state !== "open") return false;
  if (!hasPRChecksPassedForDisplay(pr)) return false;
  // Shortfall is "awaiting review" even when no reviewer is currently requested.
  if (pr.required_reviews != null && pr.review_count < pr.required_reviews) {
    return true;
  }
  if (pr.review_state === "approved") return pr.pending_review_count > 0;
  return pr.review_state === "pending" || pr.pending_review_count > 0;
}

export function isPRWaitingOnBranchProtection(pr: TaskPR): boolean {
  if (pr.state !== "open") return false;
  if (pr.mergeable_state !== "blocked") return false;
  if (isPRReadyToMerge(pr)) return false;
  if (!hasPRChecksPassedForDisplay(pr)) return false;
  if (pr.review_state === "changes_requested") return false;
  return !isPRAwaitingReview(pr);
}

// Colour for the hard merge blockers that must beat ready/awaiting-review:
// conflicts ("dirty") are a hard stop, "behind" needs a base update first.
// Returns null for every other state so the caller falls through to its
// review/check-driven colours. ("blocked" is handled later, after
// awaiting-review, so an outstanding review still reads as sky.)
function openMergeBlockerColor(pr: TaskPR): string | null {
  if (pr.state !== "open") return null;
  if (pr.mergeable_state === "dirty") return RED_500;
  if (pr.mergeable_state === "behind") return YELLOW_500;
  return null;
}

export function getPRStatusColor(pr: TaskPR): string {
  if (pr.state === "merged") return PURPLE_500;
  if (pr.state === "closed") return RED_500;
  // An active queue entry is the authoritative non-terminal state. Queue
  // membership must remain visible while provider checks or mergeability
  // fields hydrate, even when those fields still describe an earlier state.
  if (isPRQueued(pr)) return QUEUED;
  if (isPRDraft(pr)) {
    return MUTED_FOREGROUND;
  }
  if (pr.review_state === "changes_requested" || pr.checks_state === "failure") {
    return RED_500;
  }
  const blockerColor = openMergeBlockerColor(pr);
  if (blockerColor) return blockerColor;
  if (getTaskPRWorkflowAttention(pr)) return YELLOW_500;
  if (isPRReadyToMerge(pr)) {
    return EMERALD_400;
  }
  // Check awaiting-review before the plain-green fallback so an approved PR
  // with pending reviewers (1 of N required) doesn't read as fully approved.
  if (isPRAwaitingReview(pr)) {
    return SKY_400;
  }
  // Branch protection can be a normal repository-rule wait after CI has passed.
  // Keep it muted so it doesn't read like a failure.
  if (isPRWaitingOnBranchProtection(pr)) {
    return MUTED_FOREGROUND;
  }
  if (hasPRChecksPassedWithoutReviewWaitForDisplay(pr)) {
    return GREEN_500;
  }
  if (hasPRChecksInProgressForDisplay(pr) || pr.review_state === "pending") {
    return YELLOW_500;
  }
  return MUTED_FOREGROUND;
}

/**
 * Picks the most attention-worthy color across N PRs. For multi-repo tasks one
 * red PR should dominate the visual even if the others are green. Terminal
 * (merged/closed) PRs are dropped when at least one PR is still open so a
 * task whose first PR landed and was followed by a new open PR surfaces the
 * live PR's status instead of the merged-purple from the closed one.
 */
export function aggregatePRStatusColor(prs: TaskPR[]): string {
  if (prs.length === 0) return MUTED_FOREGROUND;
  const open = prs.filter((p) => p.state === "open");
  const target = open.length > 0 ? open : prs;
  let bestColor: string = MUTED_FOREGROUND;
  let bestRank = -1;
  for (const pr of target) {
    const color = getPRStatusColor(pr);
    const rank = STATUS_RANK[color] ?? 0;
    if (rank > bestRank) {
      bestRank = rank;
      bestColor = color;
    }
  }
  return bestColor;
}

/**
 * True when at least one PR is open AND every open PR is ready to merge.
 * Terminal (merged/closed) siblings are ignored so they can't drag the result
 * to false. Extracted so the rule is testable without mounting MultiPRIcon.
 */
export function areAllOpenPRsReadyToMerge(prs: TaskPR[]): boolean {
  const openPRs = prs.filter((p) => p.state === "open");
  return openPRs.length > 0 && openPRs.every(isPRReadyToMerge);
}

/**
 * Attention rank for a single PR, reusing the same colour→rank table that
 * drives the aggregate icon. Terminal PRs (merged/closed) return -1 so they're
 * never the default focus when a task mixes open and finished PRs.
 */
export function prStatusRank(pr: TaskPR): number {
  if (pr.state !== "open") return -1;
  return STATUS_RANK[getPRStatusColor(pr)] ?? 0;
}

/**
 * Picks the most attention-worthy PR to focus first in a multi-PR popover —
 * the worst open status (failing > pending > awaiting-review > ready/passing).
 * Ties resolve to the first PR (creation order). Falls back to the first PR
 * when every PR is terminal so the popover always has something to show.
 */
export function pickDefaultPR(prs: TaskPR[]): TaskPR | null {
  if (prs.length === 0) return null;
  let best = prs[0];
  let bestRank = prStatusRank(prs[0]);
  for (let i = 1; i < prs.length; i++) {
    const rank = prStatusRank(prs[i]);
    if (rank > bestRank) {
      best = prs[i];
      bestRank = rank;
    }
  }
  return best;
}

export function PRTaskIcon({ taskId, prInfo }: { taskId: string; prInfo?: TaskPRInfo }) {
  const prs = useAppStore((state) => getTaskPRsForCurrentWorkspace(state, taskId));
  const hydration = useTaskPRTooltipHydration(taskId, { includeAutomation: true });
  const fullPRs = normalizeTaskPRs(prs);

  // Defensive: an upstream payload may briefly seed byTaskId[taskId] with a
  // non-array value (e.g. an empty object from a partial hydration). Bail
  // instead of falling through into a full-data summary, where for-of throws.
  if (fullPRs.length === 0 && !prInfo) return null;

  return (
    <PRTaskIconView
      taskId={taskId}
      prInfo={prInfo}
      prs={fullPRs}
      hydration={hydration}
      automationOptions={hydration.automationOptions}
    />
  );
}

export function normalizeTaskPRs(prs: unknown): TaskPR[] {
  return Array.isArray(prs) && prs.length > 0 ? prs : EMPTY_PRS;
}

type TaskPRIconPresentation = {
  hasFullData: boolean;
  singlePR: TaskPR | null;
  readyToMerge: boolean;
  allReadyToMerge: boolean;
  summaries: ReturnType<typeof derivePRTaskStatusSummary>[];
  iconColor: string;
  displayState: string | undefined;
  displayCount: number;
  hasWorkflowApprovalRequired: boolean;
};

function getTaskPRIconPresentation(
  prs: TaskPR[],
  prInfo?: TaskPRInfo,
  compactWorkflowApprovalAuthoritative = false,
): TaskPRIconPresentation {
  const hasFullData = prs.length > 0;
  const singlePR = prs.length === 1 ? prs[0] : null;
  const readyToMerge = singlePR ? isPRReadyToMerge(singlePR) : false;
  return {
    hasFullData,
    singlePR,
    readyToMerge,
    allReadyToMerge: areAllOpenPRsReadyToMerge(prs),
    summaries: prs.map((pr) => derivePRTaskStatusSummary(pr, isPRReadyToMerge(pr))),
    iconColor: getTaskPRIconStatusColor(prs, prInfo, compactWorkflowApprovalAuthoritative),
    displayState: getTaskPRIconDisplayState(
      hasFullData,
      singlePR,
      prInfo,
      compactWorkflowApprovalAuthoritative,
    ),
    displayCount: getTaskPRIconDisplayCount(
      prs,
      prInfo,
      hasFullData,
      compactWorkflowApprovalAuthoritative,
    ),
    hasWorkflowApprovalRequired: hasTaskWorkflowApproval(
      prs,
      prInfo,
      compactWorkflowApprovalAuthoritative,
    ),
  };
}

function getTaskPRIconStatusColor(
  prs: TaskPR[],
  prInfo: TaskPRInfo | undefined,
  compactWorkflowApprovalAuthoritative: boolean,
): string {
  if (compactWorkflowApprovalAuthoritative) {
    return getPRAggregateStatusColor(prInfo?.aggregateState ?? prInfo?.state);
  }
  return getTaskPRIconColor(prs, prInfo);
}

function getTaskPRIconDisplayState(
  hasFullData: boolean,
  singlePR: TaskPR | null,
  prInfo: TaskPRInfo | undefined,
  compactWorkflowApprovalAuthoritative: boolean,
): string | undefined {
  if (compactWorkflowApprovalAuthoritative) return prInfo?.state;
  return singlePR?.state ?? (hasFullData ? undefined : prInfo?.state);
}

function getTaskPRIconDisplayCount(
  prs: TaskPR[],
  prInfo: TaskPRInfo | undefined,
  hasFullData: boolean,
  compactWorkflowApprovalAuthoritative: boolean,
): number {
  if (compactWorkflowApprovalAuthoritative) return prInfo?.count ?? 1;
  return hasFullData ? prs.length : 1;
}

function hasTaskWorkflowApproval(
  prs: TaskPR[],
  prInfo: TaskPRInfo | undefined,
  compactWorkflowApprovalAuthoritative: boolean,
): boolean {
  if (compactWorkflowApprovalAuthoritative) return prInfo?.workflowApprovalRequired === true;
  if (prs.length === 0) return prInfo?.workflowApprovalRequired === true;
  return prs.some((pr) => {
    const attention = getTaskPRWorkflowAttention(pr);
    return attention !== null && isWorkflowApprovalRequired(attention);
  });
}

function getStaleWorkflowPRs(prs: TaskPR[]) {
  return prs.flatMap((pr) => {
    const attention = getTaskPRWorkflowAttention(pr);
    if (!attention?.stale) return [];
    return [
      {
        number: pr.pr_number,
        repository: pr.owner && pr.repo ? `${pr.owner}/${pr.repo}` : undefined,
      },
    ];
  });
}

function getTaskPRIconViewModel(
  prs: TaskPR[],
  prInfo: TaskPRInfo | undefined,
  automation: ReturnType<typeof getTaskPRAutomationSummary>,
  t: ReturnType<typeof useTranslation>["t"],
) {
  const compactWorkflowApprovalAuthoritative = compactWorkflowApprovalIsNewerThanFullPRs(
    prs,
    prInfo,
  );
  const presentation = getTaskPRIconPresentation(prs, prInfo, compactWorkflowApprovalAuthoritative);
  const staleWorkflowPRs =
    compactWorkflowApprovalAuthoritative && prInfo
      ? getCompactStaleWorkflowPRs(prInfo)
      : getStaleWorkflowPRs(prs);
  const compactDisclosureSummaries =
    compactWorkflowApprovalAuthoritative && prInfo
      ? getCompactWorkflowStatusSummaries(prInfo)
      : undefined;
  const disclosureSummaries = compactDisclosureSummaries ?? presentation.summaries;
  const projectedDisclosureIdentity =
    compactDisclosureSummaries?.length === 1
      ? {
          number: compactDisclosureSummaries[0].number,
          repository: getProjectedPRRepository(prInfo, compactDisclosureSummaries[0].number),
        }
      : undefined;
  const statusLabels = getTaskPRIconStatusLabels(
    prs,
    prInfo,
    t,
    compactWorkflowApprovalAuthoritative,
  );
  const statusNumber = getTaskPRIconStatusNumber(
    presentation.singlePR,
    prInfo,
    compactWorkflowApprovalAuthoritative,
  );
  const hasMergeConflicts = compactWorkflowApprovalAuthoritative
    ? prInfo?.hasMergeConflicts === true
    : taskPRHasMergeConflict(prs, prInfo);
  const ariaLabel = buildPRTaskIconAriaLabel({
    t,
    number: statusNumber,
    statusCount: prs.length > 1 && !compactWorkflowApprovalAuthoritative ? prs.length : undefined,
    statusLabels,
    hasConflicts: hasMergeConflicts,
    autoFixEnabled: automation.autoFixEnabled,
    autoMergeEnabled: automation.autoMergeEnabled,
  });
  return {
    ...presentation,
    compactWorkflowApprovalAuthoritative,
    staleWorkflowPRs,
    disclosureSummaries,
    disclosurePRNumber: projectedDisclosureIdentity?.number,
    disclosurePRRepository: projectedDisclosureIdentity?.repository,
    disclosurePRCount: compactDisclosureSummaries?.length,
    hasMergeConflicts,
    ariaLabel,
  };
}

function getTaskPRIconStatusLabels(
  prs: TaskPR[],
  prInfo: TaskPRInfo | undefined,
  t: ReturnType<typeof useTranslation>["t"],
  compactWorkflowApprovalAuthoritative: boolean,
) {
  if (compactWorkflowApprovalAuthoritative && prInfo) {
    return getCompactPRStatusAccessibleLabels(prInfo, t);
  }
  return getTaskPRStatusAccessibleLabels(prs, prInfo, t);
}

function getTaskPRIconStatusNumber(
  singlePR: TaskPR | null,
  prInfo: TaskPRInfo | undefined,
  compactWorkflowApprovalAuthoritative: boolean,
): number | undefined {
  if (compactWorkflowApprovalAuthoritative && prInfo) {
    return prInfo.workflowApprovalPRNumber ?? prInfo.number;
  }
  return singlePR?.pr_number ?? prInfo?.number;
}

function taskPRIconNeedsHydration(
  hasFullData: boolean,
  automation: ReturnType<typeof getTaskPRAutomationSummary>,
  automationOptions: TaskCIAutomationOptions | null,
): boolean {
  if (!hasFullData) return true;
  return (automation.autoFixEnabled || automation.autoMergeEnabled) && !automationOptions;
}

function PRTaskIconView({
  taskId,
  prInfo,
  prs,
  hydration,
  automationOptions,
}: {
  taskId: string;
  prInfo?: TaskPRInfo;
  prs: TaskPR[];
  hydration: ReturnType<typeof useTaskPRTooltipHydration>;
  automationOptions: TaskCIAutomationOptions | null;
}) {
  const { t } = useTranslation();
  const { hydrate } = hydration;
  const hasFullData = prs.length > 0;
  const usesTouchDrawer = useTouchDrawer();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const automation = getTaskPRAutomationSummary(prs, prInfo, automationOptions);
  const viewModel = getTaskPRIconViewModel(prs, prInfo, automation, t);
  const needsHydration = taskPRIconNeedsHydration(hasFullData, automation, automationOptions);
  const hydrateOnDisclosure = useCallback(() => {
    if (needsHydration) void hydrate();
  }, [hydrate, needsHydration]);
  const tooltip = useChangeRequestTaskTooltipState(hydrateOnDisclosure);
  const {
    singlePR,
    readyToMerge,
    allReadyToMerge,
    iconColor,
    displayState,
    displayCount,
    hasWorkflowApprovalRequired,
    staleWorkflowPRs,
    disclosureSummaries,
    disclosurePRNumber,
    disclosurePRRepository,
    disclosurePRCount,
    hasMergeConflicts,
    ariaLabel,
  } = viewModel;
  const disclosureProps = createPRTaskIconDisclosureProps({
    taskId,
    prInfo,
    disclosurePRNumber,
    disclosurePRRepository,
    disclosurePRCount,
    prs,
    hasFullData,
    singlePR,
    readyToMerge,
    allReadyToMerge,
    displayState,
    displayCount,
    iconColor,
    ariaLabel,
    automation,
    hasMergeConflicts,
    hasWorkflowApprovalRequired,
    summaries: disclosureSummaries,
    staleWorkflowPRs,
    hydrationStatus: hydration.status,
  });

  return usesTouchDrawer ? (
    <PRTaskIconDrawer
      {...disclosureProps}
      open={drawerOpen}
      onOpenChange={(nextOpen) => {
        setDrawerOpen(nextOpen);
        if (nextOpen) hydrateOnDisclosure();
      }}
      t={t}
    />
  ) : (
    <PRTaskIconTooltip {...disclosureProps} tooltip={tooltip} />
  );
}

function getTaskPRIconColor(prs: TaskPR[], prInfo?: TaskPRInfo): string {
  if (prs.length === 1) return getPRStatusColor(prs[0]);
  if (prs.length > 1) return aggregatePRStatusColor(prs);
  return getPRAggregateStatusColor(prInfo?.aggregateState ?? prInfo?.state);
}
