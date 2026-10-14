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

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.16
// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.18
describe("manual acceptance policy", () => {
  it("accepts only optional manual jobs", () => {
    expect(bucketJob(makeJob({ status: "manual", allow_failure: true }))).toBe("passed");
    expect(bucketJob(makeJob({ status: "manual", allow_failure: false }))).toBe("in_progress");
  });
  it.each(["running", "pending", "created", "scheduled", "unknown"])(
    "keeps %s in progress with either allowance",
    (status) => {
      for (const allow_failure of [false, true]) {
        expect(bucketJob(makeJob({ status, allow_failure }))).toBe("in_progress");
      }
    },
  );
});

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.17
// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.19
describe("optional manual stage summaries", () => {
  it("accepts a successful test and optional deploy without changing raw jobs", () => {
    const jobs = [
      makeJob(),
      makeJob({
        id: 2,
        stage: "deploy",
        status: "manual",
        allow_failure: true,
        web_url: "https://ci/deploy",
      }),
    ];
    const original = structuredClone(jobs);
    expect(bucketJobCounts(jobs)).toEqual({ passed: 2, inProgress: 0, failed: 0 });
    expect(groupJobsByStage(jobs)[1]).toMatchObject({
      bucket: "passed",
      passed: 1,
      total: 1,
      webUrl: "https://ci/deploy",
    });
    expect(jobs).toEqual(original);
  });
  it.each([
    ["manual", false, "in_progress"],
    ["running", true, "in_progress"],
    ["pending", true, "in_progress"],
    ["failed", false, "failed"],
    ["canceled", false, "failed"],
    ["skipped", false, "passed"],
    ["failed", true, "passed"],
    ["canceled", true, "passed"],
  ] as const)(
    "preserves %s allowance %s beside optional manual",
    (status, allow_failure, bucket) => {
      const jobs = [
        makeJob({ status: "manual", allow_failure: true, web_url: "https://ci/optional" }),
        makeJob({ id: 2, status, allow_failure, web_url: "https://ci/dominant" }),
      ];
      const accepted = bucket === "passed";
      const counts = {
        passed: accepted ? 2 : 1,
        inProgress: bucket === "in_progress" ? 1 : 0,
        failed: bucket === "failed" ? 1 : 0,
      };
      expect(bucketJobCounts(jobs)).toEqual(counts);
      expect(groupJobsByStage(jobs)[0]).toMatchObject({
        ...counts,
        bucket,
        total: 2,
        webUrl: accepted ? "https://ci/optional" : "https://ci/dominant",
      });
    },
  );
});
