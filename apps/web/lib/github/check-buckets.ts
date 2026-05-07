import type { CheckRun } from "@/lib/types/github";

export type CheckBucket = "passed" | "in_progress" | "failed";

/**
 * Bucket a CheckRun's status/conclusion into Passed / In Progress / Failed.
 * Returns null for skipped/stale checks — they're ignored entirely in the
 * popover (see Q11). Order of evaluation matters: status comes before
 * conclusion so a queued/in_progress run never falls through to the
 * conclusion mapping (which would be the *previous* run's conclusion when
 * GitHub re-runs a check).
 */
export function bucketCheck(check: CheckRun): CheckBucket | null {
  if (check.status === "queued" || check.status === "in_progress") {
    return "in_progress";
  }
  switch (check.conclusion) {
    case "success":
    case "neutral":
      return "passed";
    case "failure":
    case "cancelled":
    case "timed_out":
    case "action_required":
      return "failed";
    case "skipped":
    case "stale":
      return null;
    case "":
    case undefined:
      // Completed checks without a conclusion are unusual; treat them as
      // passed so we don't accidentally flag them as failed in the popover.
      return check.status === "completed" ? "passed" : "in_progress";
    default:
      return null;
  }
}

export type CheckBucketCounts = {
  passed: number;
  inProgress: number;
  failed: number;
};

export function bucketCheckCounts(checks: CheckRun[]): CheckBucketCounts {
  const counts: CheckBucketCounts = { passed: 0, inProgress: 0, failed: 0 };
  for (const c of checks) {
    const b = bucketCheck(c);
    if (b === "passed") counts.passed++;
    else if (b === "in_progress") counts.inProgress++;
    else if (b === "failed") counts.failed++;
  }
  return counts;
}

export type WorkflowGroup = {
  /** Workflow name (the part before " / " in CheckRun.name); falls back to
   *  the full name for status_context entries. */
  workflow: string;
  /** Aggregated bucket — matches bucketCheck applied to the worst job. */
  bucket: CheckBucket;
  /** All jobs that belong to this workflow, in input order. */
  jobs: CheckRun[];
  /** Count of completed jobs whose conclusion is success/neutral. */
  passed: number;
  /** Count of jobs currently running/queued. */
  inProgress: number;
  /** Count of jobs in the failed bucket. */
  failed: number;
  /** Count of jobs we counted toward total (excludes skipped/stale). */
  total: number;
  /** First jobs's html_url — workflow-row click uses this when no job-level
   *  rollup link is available. */
  htmlUrl: string;
};

/**
 * Group CheckRuns by workflow name. A name like "Test / unit" splits to
 * workflow="Test", job="unit". A name without " / " (e.g. legacy
 * status_context like "vercel") is treated as a single-row workflow.
 *
 * Group bucket priority: failed > in_progress > passed. A workflow with any
 * failing job is "failed" even if other jobs passed.
 */
export function groupChecksByWorkflow(checks: CheckRun[]): WorkflowGroup[] {
  const order: string[] = [];
  const map = new Map<string, WorkflowGroup>();
  for (const check of checks) {
    const sep = check.name.indexOf(" / ");
    const workflow = sep >= 0 ? check.name.slice(0, sep) : check.name;
    let group = map.get(workflow);
    if (!group) {
      group = {
        workflow,
        bucket: "passed",
        jobs: [],
        passed: 0,
        inProgress: 0,
        failed: 0,
        total: 0,
        htmlUrl: check.html_url,
      };
      map.set(workflow, group);
      order.push(workflow);
    }
    group.jobs.push(check);
    const bucket = bucketCheck(check);
    if (bucket === null) continue;
    group.total++;
    if (bucket === "passed") group.passed++;
    else if (bucket === "in_progress") group.inProgress++;
    else if (bucket === "failed") group.failed++;
  }
  for (const g of map.values()) {
    if (g.failed > 0) g.bucket = "failed";
    else if (g.inProgress > 0) g.bucket = "in_progress";
    else g.bucket = "passed";
    // Prefer the URL of the first job that matches the group's bucket so
    // clicking a "failed" workflow row navigates to a failing job, not an
    // earlier passing one. Falls back to the first job's URL when no
    // matching job has html_url set.
    const target = g.jobs.find((j) => bucketCheck(j) === g.bucket && j.html_url);
    if (target) g.htmlUrl = target.html_url;
  }
  return order.map((w) => map.get(w)!);
}
