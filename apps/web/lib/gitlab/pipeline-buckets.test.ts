import { describe, expect, it } from "vitest";
import { bucketJob, bucketJobCounts, groupJobsByStage } from "./pipeline-buckets";
import type { GitLabPipelineJob } from "@/lib/types/gitlab";

const TEST_STAGE = "test";
const FAILED_JOB_URL = "https://ci/2";

function makeJob(overrides: Partial<GitLabPipelineJob> = {}): GitLabPipelineJob {
  return {
    id: 1,
    name: "unit",
    stage: TEST_STAGE,
    status: "success",
    allow_failure: false,
    ...overrides,
  };
}

describe("bucketJob", () => {
  it("buckets success and skipped as passed", () => {
    expect(bucketJob(makeJob({ status: "success" }))).toBe("passed");
    expect(bucketJob(makeJob({ status: "skipped" }))).toBe("passed");
  });

  it("buckets failed and canceled as failed unless allow_failure", () => {
    expect(bucketJob(makeJob({ status: "failed" }))).toBe("failed");
    expect(bucketJob(makeJob({ status: "canceled" }))).toBe("failed");
    expect(bucketJob(makeJob({ status: "failed", allow_failure: true }))).toBe("passed");
    expect(bucketJob(makeJob({ status: "canceled", allow_failure: true }))).toBe("passed");
  });

  it("buckets everything else as in_progress", () => {
    expect(bucketJob(makeJob({ status: "running" }))).toBe("in_progress");
    expect(bucketJob(makeJob({ status: "pending" }))).toBe("in_progress");
    expect(bucketJob(makeJob({ status: "manual" }))).toBe("in_progress");
  });
});

describe("bucketJobCounts", () => {
  it("counts jobs into each bucket", () => {
    const jobs = [
      makeJob({ status: "success" }),
      makeJob({ status: "failed" }),
      makeJob({ status: "failed", allow_failure: true }),
      makeJob({ status: "running" }),
    ];
    expect(bucketJobCounts(jobs)).toEqual({ passed: 2, inProgress: 1, failed: 1 });
  });
});

describe("groupJobsByStage", () => {
  it("groups by stage and rolls up the worst bucket", () => {
    const jobs = [
      makeJob({ id: 1, stage: "build", status: "success" }),
      makeJob({ id: 2, stage: TEST_STAGE, status: "failed", web_url: FAILED_JOB_URL }),
      makeJob({ id: 3, stage: TEST_STAGE, status: "success" }),
    ];
    const groups = groupJobsByStage(jobs);
    expect(groups.map((g) => g.stage)).toEqual(["build", TEST_STAGE]);
    expect(groups[0].bucket).toBe("passed");
    expect(groups[1].bucket).toBe("failed");
    expect(groups[1].passed).toBe(1);
    expect(groups[1].failed).toBe(1);
    expect(groups[1].webUrl).toBe(FAILED_JOB_URL);
  });

  it("prefers a failed job's URL for a failed group", () => {
    const jobs = [
      makeJob({ id: 1, stage: TEST_STAGE, status: "success", web_url: "https://ci/1" }),
      makeJob({ id: 2, stage: TEST_STAGE, status: "failed", web_url: FAILED_JOB_URL }),
    ];
    const [group] = groupJobsByStage(jobs);
    expect(group.webUrl).toBe(FAILED_JOB_URL);
  });

  it("defaults an empty stage name to 'unstaged'", () => {
    const jobs = [makeJob({ stage: "" })];
    const [group] = groupJobsByStage(jobs);
    expect(group.stage).toBe("unstaged");
  });
});
