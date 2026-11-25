import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";

import { FrontendTimingReporter } from "./vitest-ci-reporter";

afterEach(() => vi.unstubAllEnvs());

function testCase(fullName: string, status: string, retryCount = 0, flaky = false) {
  return {
    fullName,
    name: fullName.split(" > ").at(-1),
    parent: { type: "suite", name: "suite", parent: { type: "module" } },
    options: { mode: "run" },
    result: () => ({ state: status }),
    diagnostic: () => ({ retryCount, flaky }),
  };
}

function moduleFixture(identity: string, tests: ReturnType<typeof testCase>[]) {
  const [project, relativeModuleId] = identity.split(":");
  return {
    project: { name: project },
    relativeModuleId,
    state: () => "passed",
    children: {
      *allTests() {
        yield* tests;
      },
    },
    diagnostic: () => ({
      environmentSetupDuration: 2,
      prepareDuration: 3,
      collectDuration: 5,
      setupDuration: 7,
      duration: 11,
      importDurations: { "./heavy-dependency": { duration: 90 } },
    }),
  };
}

describe("FrontendTimingReporter", () => {
  it("records public module diagnostics and native test outcomes without import double counting", async () => {
    const root = mkdtempSync(path.join(os.tmpdir(), "kandev-vite-ci-reporter-"));
    try {
      const assignmentPath = path.join(root, "assignment.json");
      const reportPath = path.join(root, "report.json");
      const assignment = {
        schema: "kandev.frontend-timing-assignment/v1",
        sourceSha: "b".repeat(40),
        runId: "12345",
        attempt: "2",
        shard: 1,
        shardCount: 2,
        snapshot: { digest: "c".repeat(64), runId: "12345", attempt: "1", profileProducer: null },
        mode: "fallback",
        planDigest: "d".repeat(64),
        assigned: [{ identity: "node:src/a.test.ts", contentHash: "e".repeat(64), shard: 1 }],
      };
      writeFileSync(assignmentPath, JSON.stringify(assignment));
      vi.stubEnv("KANDEV_FRONTEND_TIMINGS_ENABLED", "1");
      vi.stubEnv("FRONTEND_TIMING_ASSIGNMENT_FILE", assignmentPath);
      vi.stubEnv("FRONTEND_TIMING_REPORT_FILE", reportPath);
      vi.stubEnv("FRONTEND_TIMING_SOURCE_SHA", "b".repeat(40));
      vi.stubEnv("GITHUB_RUN_ID", "12345");
      vi.stubEnv("GITHUB_RUN_ATTEMPT", "2");

      const reporter = new FrontendTimingReporter();
      await reporter.onTestRunEnd(
        [
          moduleFixture("node:src/a.test.ts", [
            testCase("suite > passes", "passed"),
            testCase("suite > skipped", "skipped"),
            testCase("suite > retries", "passed", 1, true),
          ]),
        ] as never,
        [],
        "passed",
      );

      const report = JSON.parse(readFileSync(reportPath, "utf8"));
      expect(report.completed).toHaveLength(1);
      expect(report.completed[0].durationMs).toBe(28);
      expect(report.completed[0].tests.map((entry: { status: string }) => entry.status)).toEqual([
        "passed",
        "passed",
        "skipped",
      ]);
      expect(
        report.completed[0].tests.find((entry: { retryCount: number }) => entry.retryCount === 1)
          .flaky,
      ).toBe(true);
      expect(report.snapshot.digest).toBe("c".repeat(64));
      expect(report.reason).toBe("passed");
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });

  it("retains interrupted runs and incomplete module states for the gate to reject", async () => {
    const root = mkdtempSync(path.join(os.tmpdir(), "kandev-vite-ci-reporter-"));
    try {
      const assignmentPath = path.join(root, "assignment.json");
      const reportPath = path.join(root, "report.json");
      writeFileSync(
        assignmentPath,
        JSON.stringify({
          schema: "kandev.frontend-timing-assignment/v1",
          sourceSha: "b".repeat(40),
          runId: "12345",
          attempt: "1",
          shard: 2,
          shardCount: 2,
          snapshot: { digest: "c".repeat(64), runId: "12345", attempt: "1", profileProducer: null },
          mode: "fallback",
          planDigest: "d".repeat(64),
          assigned: [],
        }),
      );
      vi.stubEnv("KANDEV_FRONTEND_TIMINGS_ENABLED", "1");
      vi.stubEnv("FRONTEND_TIMING_ASSIGNMENT_FILE", assignmentPath);
      vi.stubEnv("FRONTEND_TIMING_REPORT_FILE", reportPath);
      vi.stubEnv("FRONTEND_TIMING_SOURCE_SHA", "b".repeat(40));
      vi.stubEnv("GITHUB_RUN_ID", "12345");
      vi.stubEnv("GITHUB_RUN_ATTEMPT", "1");

      const reporter = new FrontendTimingReporter();
      await reporter.onTestRunEnd([], [], "interrupted");

      const report = JSON.parse(readFileSync(reportPath, "utf8"));
      expect(report.reason).toBe("interrupted");
      expect(report.completed).toEqual([]);
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });
});
