"use client";

import { IconGitPullRequest } from "@tabler/icons-react";
import { useCallback, type ReactNode, useState } from "react";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
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
  type TaskPRTooltipHydrationStatus,
} from "@/hooks/domains/github/use-task-pr-tooltip-hydration";
import type { TaskPR } from "@/lib/types/github";
import type { TaskCIAutomationOptions } from "@/lib/types/github";
import { automationForPR } from "./pr-status-automation-badges";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { derivePRTaskStatusSummary, PRTaskStatusSummary } from "./pr-task-status-summary";

const MUTED_FOREGROUND = CHANGE_REQUEST_STATUS_COLORS.muted;
const PURPLE_500 = CHANGE_REQUEST_STATUS_COLORS.merged;
const RED_500 = CHANGE_REQUEST_STATUS_COLORS.danger;
const YELLOW_500 = CHANGE_REQUEST_STATUS_COLORS.warning;
const SKY_400 = CHANGE_REQUEST_STATUS_COLORS.review;
const EMERALD_400 = CHANGE_REQUEST_STATUS_COLORS.ready;
const QUEUED = CHANGE_REQUEST_STATUS_COLORS.queued;
const GREEN_500 = CHANGE_REQUEST_STATUS_COLORS.passing;

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
  if (pr.review_state === "changes_requested" || pr.checks_state === "failure") {
    return RED_500;
  }
  if (isPRDraft(pr)) {
    return MUTED_FOREGROUND;
  }
  const blockerColor = openMergeBlockerColor(pr);
  if (blockerColor) return blockerColor;
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

export type TaskPRInfo = {
  number: number;
  state: string;
  aggregateState?: string;
  autoFixEnabled?: boolean;
  autoMergeEnabled?: boolean;
};

export type TaskPRAutomationDetail = {
  number: number;
  autoFixEnabled: boolean;
  autoMergeEnabled: boolean;
};

export type TaskPRAutomationSummary = {
  autoFixEnabled: boolean;
  autoMergeEnabled: boolean;
  details: TaskPRAutomationDetail[];
};

export function getTaskPRAutomationSummary(
  prs: TaskPR[],
  prInfo?: TaskPRInfo,
  options?: TaskCIAutomationOptions | null,
): TaskPRAutomationSummary {
  const activePRs = prs.filter((pr) => pr.state.trim().toLowerCase() === "open");
  // The bounded task-row projection is the source of truth until the full PR
  // list is hydrated. Keep its flags visible even when a previously hydrated
  // options response is present but the scoped PR records are not.
  if (!options || prs.length === 0) {
    return {
      autoFixEnabled: prInfo?.autoFixEnabled === true,
      autoMergeEnabled: prInfo?.autoMergeEnabled === true,
      details: [],
    };
  }
  const details = activePRs.map((pr) => {
    const automation = automationForPR(options, pr);
    return {
      number: pr.pr_number,
      autoFixEnabled: automation.autoFix,
      autoMergeEnabled: automation.autoMerge,
    };
  });
  return {
    autoFixEnabled: details.some((detail) => detail.autoFixEnabled),
    autoMergeEnabled: details.some((detail) => detail.autoMergeEnabled),
    details,
  };
}

export function AutomationIndicatorDots({
  autoFixEnabled,
  autoMergeEnabled,
}: Pick<TaskPRAutomationSummary, "autoFixEnabled" | "autoMergeEnabled">) {
  return (
    <>
      {autoFixEnabled && (
        <span
          data-testid="pr-task-automation-auto-fix"
          className="absolute left-0 top-0 h-1.5 w-1.5 rounded-full bg-yellow-400 ring-1 ring-background"
          aria-hidden="true"
        />
      )}
      {autoMergeEnabled && (
        <span
          data-testid="pr-task-automation-auto-merge"
          className="absolute right-0 top-0 h-1.5 w-1.5 rounded-full bg-purple-500 ring-1 ring-background"
          aria-hidden="true"
        />
      )}
    </>
  );
}

export function PRTaskIcon({ taskId, prInfo }: { taskId: string; prInfo?: TaskPRInfo }) {
  const prs = useAppStore((state) => getTaskPRsForCurrentWorkspace(state, taskId));
  const hydration = useTaskPRTooltipHydration(taskId, { includeAutomation: true });
  const fullPRs = Array.isArray(prs) && prs.length > 0 ? prs : [];

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

type TaskPRIconPresentation = {
  hasFullData: boolean;
  singlePR: TaskPR | null;
  readyToMerge: boolean;
  allReadyToMerge: boolean;
  summaries: ReturnType<typeof derivePRTaskStatusSummary>[];
  iconColor: string;
  displayState: string | undefined;
  displayCount: number;
};

function getTaskPRIconPresentation(prs: TaskPR[], prInfo?: TaskPRInfo): TaskPRIconPresentation {
  const hasFullData = prs.length > 0;
  const singlePR = prs.length === 1 ? prs[0] : null;
  const readyToMerge = singlePR ? isPRReadyToMerge(singlePR) : false;
  return {
    hasFullData,
    singlePR,
    readyToMerge,
    allReadyToMerge: areAllOpenPRsReadyToMerge(prs),
    summaries: prs.map((pr) => derivePRTaskStatusSummary(pr, isPRReadyToMerge(pr))),
    iconColor: getTaskPRIconColor(prs, prInfo),
    displayState: singlePR?.state ?? (hasFullData ? undefined : prInfo?.state),
    displayCount: hasFullData ? prs.length : 1,
  };
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
  const needsHydration =
    !hasFullData ||
    ((automation.autoFixEnabled || automation.autoMergeEnabled) && !automationOptions);
  const hydrateOnDisclosure = useCallback(() => {
    if (needsHydration) void hydrate();
  }, [hydrate, needsHydration]);
  const tooltip = useChangeRequestTaskTooltipState(hydrateOnDisclosure);
  const {
    singlePR,
    readyToMerge,
    allReadyToMerge,
    summaries,
    iconColor,
    displayState,
    displayCount,
  } = getTaskPRIconPresentation(prs, prInfo);

  const statusAriaLabel =
    prs.length > 1
      ? t("github:pullRequestStatuses", { count: prs.length })
      : t("github:pullRequestStatus", { number: singlePR?.pr_number ?? prInfo?.number });
  const automationAria = [
    automation.autoFixEnabled ? t("github:autoFixEnabledAria") : null,
    automation.autoMergeEnabled ? t("github:autoMergeEnabledAria") : null,
  ]
    .filter(Boolean)
    .join(", ");
  const ariaLabel = automationAria ? `${statusAriaLabel}, ${automationAria}` : statusAriaLabel;

  const disclosureProps: PRTaskIconDisclosureProps = {
    taskId,
    prInfo,
    prs,
    hasFullData,
    singlePR,
    readyToMerge,
    allReadyToMerge,
    displayState,
    displayCount,
    iconColor,
    ariaLabel,
    icon: <PRTaskIconGlyph automation={automation} />,
    content: hasFullData ? (
      <>
        <PRTaskStatusSummary summaries={summaries} />
        <TaskPRAutomationDetails summary={automation} />
      </>
    ) : (
      <>
        <CompactPRTooltipContent status={hydration.status} />
        <TaskPRAutomationDetails summary={automation} status={hydration.status} />
      </>
    ),
  };

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

type PRTaskIconDisclosureProps = {
  taskId: string;
  prInfo?: TaskPRInfo;
  prs: TaskPR[];
  hasFullData: boolean;
  singlePR: TaskPR | null;
  readyToMerge: boolean;
  allReadyToMerge: boolean;
  displayState: string | undefined;
  displayCount: number;
  iconColor: string;
  ariaLabel: string;
  icon: ReactNode;
  content: ReactNode;
};

function PRTaskIconGlyph({ automation }: { automation: TaskPRAutomationSummary }) {
  return (
    <span className="relative inline-flex h-3.5 w-3.5 shrink-0">
      <IconGitPullRequest aria-hidden="true" className="h-3.5 w-3.5" />
      <AutomationIndicatorDots
        autoFixEnabled={automation.autoFixEnabled}
        autoMergeEnabled={automation.autoMergeEnabled}
      />
    </span>
  );
}

function PRTaskIconDrawer({
  open,
  onOpenChange,
  t,
  ...props
}: PRTaskIconDisclosureProps & {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  t: ReturnType<typeof useTranslation>["t"];
}) {
  const { prs, prInfo, singlePR, content } = props;
  return (
    <Drawer open={open} onOpenChange={onOpenChange}>
      <DrawerTrigger asChild>
        <PRTaskIconTrigger
          {...props}
          touch
          open={open}
          onClick={(event) => event.stopPropagation()}
        />
      </DrawerTrigger>
      <DrawerContent
        data-testid={`pr-task-automation-drawer-${props.taskId}`}
        className="max-h-[80dvh] flex flex-col"
      >
        <DrawerHeader className="shrink-0 border-b py-2">
          <DrawerTitle className="text-sm">
            {prs.length > 1
              ? t("github:pullRequestCount", { count: prs.length })
              : t("github:pullRequestStatus", { number: singlePR?.pr_number ?? prInfo?.number })}
          </DrawerTitle>
          <DrawerDescription className="sr-only">
            {t("github:pullRequestCiStatusReviewsAnd")}
          </DrawerDescription>
        </DrawerHeader>
        <div className="min-h-0 flex-1 overflow-y-auto p-3" data-vaul-no-drag>
          {content}
        </div>
      </DrawerContent>
    </Drawer>
  );
}

function PRTaskIconTooltip({
  tooltip,
  ...props
}: PRTaskIconDisclosureProps & {
  tooltip: ReturnType<typeof useChangeRequestTaskTooltipState>;
}) {
  return (
    <Tooltip open={tooltip.open}>
      <TooltipTrigger asChild>
        <PRTaskIconTrigger
          {...props}
          onPointerEnter={tooltip.onPointerEnter}
          onPointerLeave={tooltip.onPointerLeave}
          onFocus={tooltip.onFocus}
          onBlur={tooltip.onBlur}
        />
      </TooltipTrigger>
      <TooltipContent
        sideOffset={6}
        onEscapeKeyDown={tooltip.onEscapeKeyDown}
        className="w-80 max-w-[calc(100vw-1rem)] p-3"
      >
        {props.content}
      </TooltipContent>
    </Tooltip>
  );
}

function PRTaskIconTrigger({
  touch = false,
  open = false,
  onClick,
  ...props
}: PRTaskIconDisclosureProps & {
  touch?: boolean;
  open?: boolean;
  onClick?: React.MouseEventHandler<HTMLButtonElement>;
  onPointerEnter?: React.PointerEventHandler<HTMLSpanElement>;
  onPointerLeave?: React.PointerEventHandler<HTMLSpanElement>;
  onFocus?: React.FocusEventHandler<HTMLSpanElement>;
  onBlur?: React.FocusEventHandler<HTMLSpanElement>;
}) {
  const { prs, icon, taskId, hasFullData, readyToMerge, allReadyToMerge } = props;
  const commonAttributes = {
    "data-testid": `pr-task-icon-${taskId}`,
    "data-pr-state": props.displayState,
    "data-pr-count": props.displayCount,
    "data-pr-ready-to-merge": hasFullData
      ? String(prs.length === 1 ? readyToMerge : allReadyToMerge)
      : undefined,
    "aria-label": props.ariaLabel,
  };
  const contents = (
    <span className="inline-flex items-center gap-0.5">
      {icon}
      {prs.length > 1 ? (
        <span className="text-[9px] font-semibold leading-none tabular-nums">{prs.length}</span>
      ) : null}
    </span>
  );
  if (touch) {
    return (
      <button
        type="button"
        {...commonAttributes}
        aria-haspopup="dialog"
        aria-expanded={open}
        className={cn(
          "inline-flex h-11 w-11 items-center justify-center shrink-0",
          props.iconColor,
        )}
        onPointerDown={(event) => event.stopPropagation()}
        onClick={onClick}
      >
        {contents}
      </button>
    );
  }
  return (
    <span
      {...commonAttributes}
      role="img"
      tabIndex={0}
      className={cn(
        "inline-flex items-center shrink-0",
        prs.length > 1 && "gap-0.5",
        props.iconColor,
      )}
      onPointerEnter={props.onPointerEnter}
      onPointerLeave={props.onPointerLeave}
      onFocus={props.onFocus}
      onBlur={props.onBlur}
    >
      {contents}
    </span>
  );
}

function TaskPRAutomationDetails({
  summary,
  status,
}: {
  summary: TaskPRAutomationSummary;
  status?: TaskPRTooltipHydrationStatus;
}) {
  const { t } = useTranslation();
  const hasAutomation = summary.autoFixEnabled || summary.autoMergeEnabled;
  if (!hasAutomation && summary.details.length === 0) return null;
  const detailContent =
    summary.details.length > 0 ? (
      <div className="mt-1 space-y-1">
        {summary.details.map((detail, index) => (
          <div
            key={`${detail.number}-${index}`}
            className="flex flex-wrap items-center gap-x-2 gap-y-1"
          >
            <span className="font-medium">
              {t("github:prTaskStatusNumber", { number: detail.number })}
            </span>
            {detail.autoFixEnabled && (
              <span className="text-yellow-500">{t("github:autoFix")}</span>
            )}
            {detail.autoMergeEnabled && (
              <span className="text-purple-400">{t("github:autoMerge")}</span>
            )}
          </div>
        ))}
      </div>
    ) : null;
  const loadingContent =
    status === "loading" || status === "idle" ? (
      <p className="mt-1 text-muted-foreground">{t("github:taskPrDetailsLoading")}</p>
    ) : null;
  return (
    <section
      data-testid="pr-task-automation-details"
      className="mt-2 border-t border-border/60 pt-2 text-xs"
    >
      <h4 className="font-medium text-foreground">{t("github:automation")}</h4>
      {detailContent ?? loadingContent}
    </section>
  );
}

function getTaskPRIconColor(prs: TaskPR[], prInfo?: TaskPRInfo): string {
  if (prs.length === 1) return getPRStatusColor(prs[0]);
  if (prs.length > 1) return aggregatePRStatusColor(prs);
  return getPRAggregateStatusColor(prInfo?.aggregateState ?? prInfo?.state);
}

function CompactPRTooltipContent({ status }: { status: TaskPRTooltipHydrationStatus }) {
  const { t } = useTranslation();
  if (status === "loading" || status === "idle") {
    return (
      <span data-testid="pr-task-tooltip-loading" className="text-sm text-muted-foreground">
        {t("github:taskPrDetailsLoading")}
      </span>
    );
  }
  if (status === "unavailable") {
    return (
      <span data-testid="pr-task-tooltip-unavailable" className="text-sm text-muted-foreground">
        {t("github:taskPrDetailsUnavailable")}
      </span>
    );
  }
  return null;
}
