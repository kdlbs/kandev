import type { GitLabPipelineJob } from "@/lib/types/gitlab";

export type JobBucket = "passed" | "in_progress" | "failed";

/**
 * Bucket a GitLabPipelineJob's status into Passed / In Progress / Failed.
 * Mirrors the backend's pipelineJobBucket (apps/backend/internal/gitlab/models_pipeline_jobs.go):
 * a job with allow_failure=true never buckets as failed, matching GitLab's
 * own pipeline-status rollup.
 */
export function bucketJob(job: GitLabPipelineJob): JobBucket {
  switch (job.status) {
    case "success":
    case "skipped":
      return "passed";
    case "failed":
    case "canceled":
      return job.allow_failure ? "passed" : "failed";
    default:
      // running, pending, created, manual, scheduled, ...
      return "in_progress";
  }
}

export type JobBucketCounts = {
  passed: number;
  inProgress: number;
  failed: number;
};

export function bucketJobCounts(jobs: GitLabPipelineJob[]): JobBucketCounts {
  const counts: JobBucketCounts = { passed: 0, inProgress: 0, failed: 0 };
  for (const job of jobs) {
    const bucket = bucketJob(job);
    if (bucket === "passed") counts.passed++;
    else if (bucket === "in_progress") counts.inProgress++;
    else counts.failed++;
  }
  return counts;
}

export type StageGroup = {
  stage: string;
  bucket: JobBucket;
  jobs: GitLabPipelineJob[];
  passed: number;
  inProgress: number;
  failed: number;
  total: number;
  /** First job matching the group's bucket — stage-row click target. */
  webUrl?: string;
};

/**
 * Group pipeline jobs by their GitLab-reported stage. Group bucket
 * priority: failed > in_progress > passed — a stage with any failing job
 * (that isn't allow_failure) reads as failed even if other jobs passed.
 */
export function groupJobsByStage(jobs: GitLabPipelineJob[]): StageGroup[] {
  const order: string[] = [];
  const map = new Map<string, StageGroup>();
  for (const job of jobs) {
    const stage = job.stage || "unstaged";
    let group = map.get(stage);
    if (!group) {
      group = { stage, bucket: "passed", jobs: [], passed: 0, inProgress: 0, failed: 0, total: 0 };
      map.set(stage, group);
      order.push(stage);
    }
    group.jobs.push(job);
    group.total++;
    const bucket = bucketJob(job);
    if (bucket === "passed") group.passed++;
    else if (bucket === "in_progress") group.inProgress++;
    else group.failed++;
  }
  for (const group of map.values()) {
    if (group.failed > 0) group.bucket = "failed";
    else if (group.inProgress > 0) group.bucket = "in_progress";
    else group.bucket = "passed";
    const target = group.jobs.find((job) => bucketJob(job) === group.bucket && job.web_url);
    group.webUrl = target?.web_url;
  }
  return order.map((stage) => map.get(stage)!);
}
