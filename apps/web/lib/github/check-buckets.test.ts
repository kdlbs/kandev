import { describe, it, expect } from "vitest";
import { bucketCheck, bucketCheckCounts, groupChecksByWorkflow } from "./check-buckets";
import type { CheckRun } from "@/lib/types/github";

function makeCheck(overrides: Partial<CheckRun> = {}): CheckRun {
  return {
    name: "Test / unit",
    source: "check_run",
    status: "completed",
    conclusion: "success",
    html_url: "https://example.com/checks/1",
    output: "",
    started_at: null,
    completed_at: null,
    ...overrides,
  };
}

describe("bucketCheck", () => {
  it("buckets success and neutral as Passed", () => {
    expect(bucketCheck(makeCheck({ conclusion: "success" }))).toBe("passed");
    expect(bucketCheck(makeCheck({ conclusion: "neutral" }))).toBe("passed");
  });

  it("buckets failure / cancelled / timed_out / action_required as Failed", () => {
    expect(bucketCheck(makeCheck({ conclusion: "failure" }))).toBe("failed");
    expect(bucketCheck(makeCheck({ conclusion: "cancelled" }))).toBe("failed");
    expect(bucketCheck(makeCheck({ conclusion: "timed_out" }))).toBe("failed");
    expect(bucketCheck(makeCheck({ conclusion: "action_required" }))).toBe("failed");
  });

  it("buckets queued / in_progress as In Progress (status takes priority)", () => {
    expect(bucketCheck(makeCheck({ status: "queued", conclusion: "" }))).toBe("in_progress");
    expect(bucketCheck(makeCheck({ status: "in_progress", conclusion: "" }))).toBe("in_progress");
    // Even if a stale conclusion is present, status overrides:
    expect(bucketCheck(makeCheck({ status: "in_progress", conclusion: "failure" }))).toBe(
      "in_progress",
    );
  });

  it("ignores skipped and stale", () => {
    expect(bucketCheck(makeCheck({ conclusion: "skipped" }))).toBeNull();
    expect(bucketCheck(makeCheck({ conclusion: "stale" }))).toBeNull();
  });
});

describe("bucketCheckCounts", () => {
  it("counts each bucket independently and ignores skipped/stale", () => {
    const checks = [
      makeCheck({ conclusion: "success" }),
      makeCheck({ conclusion: "neutral" }),
      makeCheck({ conclusion: "cancelled" }),
      makeCheck({ conclusion: "timed_out" }),
      makeCheck({ conclusion: "action_required" }),
      makeCheck({ status: "in_progress", conclusion: "" }),
      makeCheck({ conclusion: "skipped" }),
      makeCheck({ conclusion: "stale" }),
    ];
    expect(bucketCheckCounts(checks)).toEqual({ passed: 2, inProgress: 1, failed: 3 });
  });
});

describe("groupChecksByWorkflow", () => {
  it("groups by the part before ' / '", () => {
    const checks = [
      makeCheck({ name: "Test / unit", conclusion: "success" }),
      makeCheck({ name: "Test / e2e", conclusion: "failure" }),
      makeCheck({ name: "Lint / check", conclusion: "failure" }),
    ];
    const groups = groupChecksByWorkflow(checks);
    expect(groups.map((g) => g.workflow)).toEqual(["Test", "Lint"]);
    const test = groups.find((g) => g.workflow === "Test")!;
    expect(test.passed).toBe(1);
    expect(test.failed).toBe(1);
    expect(test.bucket).toBe("failed");
  });

  it("treats a name without ' / ' as a single-row workflow", () => {
    const checks = [makeCheck({ name: "vercel", source: "status_context", conclusion: "success" })];
    const groups = groupChecksByWorkflow(checks);
    expect(groups).toHaveLength(1);
    expect(groups[0].workflow).toBe("vercel");
    expect(groups[0].bucket).toBe("passed");
  });

  it("group bucket prioritizes failed > in_progress > passed", () => {
    const checks = [
      makeCheck({ name: "Test / a", conclusion: "success" }),
      makeCheck({ name: "Test / b", status: "in_progress", conclusion: "" }),
      makeCheck({ name: "Test / c", conclusion: "failure" }),
    ];
    const [group] = groupChecksByWorkflow(checks);
    expect(group.bucket).toBe("failed");
    expect(group.passed).toBe(1);
    expect(group.inProgress).toBe(1);
    expect(group.failed).toBe(1);
    expect(group.total).toBe(3);
  });

  it("computes (N/M passed) badge components correctly when no failures", () => {
    const checks = [
      makeCheck({ name: "Build / linux", status: "in_progress", conclusion: "" }),
      makeCheck({ name: "Build / mac", conclusion: "success" }),
    ];
    const [group] = groupChecksByWorkflow(checks);
    expect(group.bucket).toBe("in_progress");
    expect(group.passed).toBe(1);
    expect(group.inProgress).toBe(1);
    expect(group.total).toBe(2);
  });

  it("htmlUrl points to first job matching the group's bucket", () => {
    // Regression for the URL-selection fix in check-buckets.ts: a failed
    // workflow row should link to a failing job's log, not the first
    // (potentially passing) job's URL.
    const checks = [
      makeCheck({ name: "Build / a", conclusion: "success", html_url: "https://ok" }),
      makeCheck({ name: "Build / b", conclusion: "failure", html_url: "https://bad" }),
      makeCheck({ name: "Build / c", conclusion: "failure", html_url: "https://bad-2" }),
    ];
    const [group] = groupChecksByWorkflow(checks);
    expect(group.bucket).toBe("failed");
    // Picks the first failing job's URL.
    expect(group.htmlUrl).toBe("https://bad");
  });

  it("htmlUrl falls back to first job's URL when no bucket-match has html_url", () => {
    const checks = [
      makeCheck({ name: "Build / a", conclusion: "success", html_url: "https://ok-1" }),
      makeCheck({ name: "Build / b", conclusion: "success", html_url: "https://ok-2" }),
    ];
    const [group] = groupChecksByWorkflow(checks);
    expect(group.bucket).toBe("passed");
    expect(group.htmlUrl).toBe("https://ok-1");
  });

  it("ignores skipped jobs in totals", () => {
    const checks = [
      makeCheck({ name: "Test / a", conclusion: "success" }),
      makeCheck({ name: "Test / b", conclusion: "skipped" }),
    ];
    const [group] = groupChecksByWorkflow(checks);
    expect(group.total).toBe(1);
    expect(group.passed).toBe(1);
    expect(group.bucket).toBe("passed");
  });
});
