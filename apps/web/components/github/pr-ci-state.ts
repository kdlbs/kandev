import type { PRCIState, TaskPR } from "@/lib/types/github";
import { getTaskPRWorkflowAttention, isWorkflowApprovalRequired } from "./pr-workflow-attention";

const CI_STATE_RANK: Record<PRCIState, number> = {
  none: 0,
  in_progress: 1,
  attention: 2,
  approval_required: 3,
  failed: 4,
};

export type TaskPRCIIdentity = {
  number: number;
  repository?: string;
};

export type TaskPRCIObservation = {
  state: PRCIState;
  identity?: TaskPRCIIdentity;
  stale: boolean;
};

function isOpenPR(pr: TaskPR): boolean {
  return pr.state === "open";
}

function prRepository(pr: TaskPR): string | undefined {
  const owner = pr.owner?.trim();
  const repo = pr.repo?.trim();
  return owner && repo ? `${owner}/${repo}` : undefined;
}

/**
 * Display-only CI classification for one pull request. Independent of review
 * state, draft status, queue membership, and mergeability. `none` means no
 * bubble; it does not certify that checks passed.
 */
export function getPRCIState(pr: TaskPR): PRCIState {
  if (!isOpenPR(pr)) return "none";
  if (pr.checks_state === "failure") return "failed";
  const attention = getTaskPRWorkflowAttention(pr);
  if (attention && isWorkflowApprovalRequired(attention)) return "approval_required";
  if (attention?.state === "action_required" || pr.checks_state === "unstable") return "attention";
  if (pr.checks_state === "pending") return "in_progress";
  return "none";
}

function betterCIObservation(
  candidate: TaskPRCIObservation,
  current: TaskPRCIObservation,
): boolean {
  const candidateRank = CI_STATE_RANK[candidate.state];
  const currentRank = CI_STATE_RANK[current.state];
  if (candidateRank !== currentRank) return candidateRank > currentRank;
  if (candidate.stale !== current.stale) return current.stale;
  const candidateNumber = candidate.identity?.number ?? 0;
  const currentNumber = current.identity?.number ?? 0;
  if (candidateNumber !== currentNumber) return candidateNumber < currentNumber;
  return (candidate.identity?.repository ?? "") < (current.identity?.repository ?? "");
}

/** Aggregate the highest-priority CI state over open pull requests only. */
export function aggregatePRCIState(prs: TaskPR[]): TaskPRCIObservation {
  let best: TaskPRCIObservation = { state: "none", stale: false };
  for (const pr of prs) {
    if (!isOpenPR(pr)) continue;
    const attention = getTaskPRWorkflowAttention(pr);
    const candidate: TaskPRCIObservation = {
      state: getPRCIState(pr),
      identity: {
        number: pr.pr_number,
        ...(prRepository(pr) ? { repository: prRepository(pr) } : {}),
      },
      stale: attention?.stale === true,
    };
    if (candidate.state !== "none" && betterCIObservation(candidate, best)) {
      best = candidate;
    }
  }
  return best;
}

/**
 * True when a persisted selected-check snapshot describes a nonempty
 * all-skipped observation for the PR's current head.
 */
export function isAllSkippedCheckSummary(pr: TaskPR): boolean {
  const summary = pr.check_summary;
  if (!summary || !pr.head_sha || summary.head_sha !== pr.head_sha) return false;
  if (summary.observed <= 0 || summary.skipped !== summary.observed) return false;
  return true;
}

/** Number of completed skipped checks in the PR's current-head snapshot. */
export function skippedCheckCount(pr: TaskPR): number {
  const summary = pr.check_summary;
  if (!summary || !pr.head_sha || summary.head_sha !== pr.head_sha) return 0;
  if (summary.skipped <= 0 || summary.skipped > summary.observed) return 0;
  return summary.skipped;
}
