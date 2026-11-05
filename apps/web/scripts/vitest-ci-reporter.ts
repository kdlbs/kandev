// i18n-exempt: CI-only report fields and machine-readable test outcomes are not user-facing copy.
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import type {
  Reporter,
  SerializedError,
  TestCase,
  TestModule,
  TestRunEndReason,
} from "vitest/node";

import { parseIdentity } from "../../../.github/scripts/frontend-timings.mjs";

interface AssignedFile {
  identity: string;
  contentHash: string;
  estimateMs: number;
  estimateSource: string;
  shard: number;
}

interface AssignmentEvidence {
  schema: string;
  sourceSha: string;
  runId: string;
  attempt: string;
  shard: number;
  shardCount: number;
  snapshot: {
    digest: string;
    runId: string;
    attempt: string;
    profileProducer: { runId: string; attempt: string } | null;
  };
  mode: "profile" | "fallback";
  planDigest: string;
  assigned: AssignedFile[];
}

interface ReportTest {
  fullName: string;
  status: string;
  retryCount: number;
  flaky: boolean;
}

function requiredEnvironment(name: string): string {
  const value = process.env[name];
  if (!value) throw new Error(`Missing ${name}`);
  return value;
}

function testStatus(testCase: TestCase): string {
  if (testCase.options.mode === "todo") return "todo";
  return testCase.result().state;
}

function nativeFullName(testCase: TestCase): string {
  const ancestors: string[] = [];
  let parent = testCase.parent;
  while (parent.type === "suite") {
    ancestors.unshift(parent.name);
    parent = parent.parent;
  }
  return [...ancestors, testCase.name].join(" ");
}

function testEvidence(testCase: TestCase): ReportTest {
  const diagnostic = testCase.diagnostic();
  return {
    fullName: nativeFullName(testCase),
    status: testStatus(testCase),
    retryCount: diagnostic?.retryCount ?? 0,
    flaky: diagnostic?.flaky ?? false,
  };
}

function moduleDuration(module: TestModule): number {
  const diagnostic = module.diagnostic();
  return [
    diagnostic.environmentSetupDuration,
    diagnostic.prepareDuration,
    diagnostic.collectDuration,
    diagnostic.setupDuration,
    diagnostic.duration,
  ].reduce((total, duration) => {
    if (!Number.isFinite(duration) || duration < 0)
      throw new Error("Invalid Vitest module diagnostic");
    return total + duration;
  }, 0);
}

function moduleIdentity(module: TestModule): string {
  const identity = `${module.project.name}:${module.relativeModuleId.replaceAll("\\", "/")}`;
  if (!parseIdentity(identity)) throw new Error("Vitest produced an invalid module identity");
  return identity;
}

function atomicWrite(filePath: string, text: string): void {
  const absolutePath = path.resolve(filePath);
  mkdirSync(path.dirname(absolutePath), { recursive: true });
  writeFileSync(absolutePath, `${text}\n`);
}

export class FrontendTimingReporter implements Reporter {
  async onTestRunEnd(
    testModules: ReadonlyArray<TestModule>,
    unhandledErrors: ReadonlyArray<SerializedError>,
    reason: TestRunEndReason,
  ): Promise<void> {
    if (process.env.KANDEV_FRONTEND_TIMINGS_ENABLED !== "1") return;

    const assignment = JSON.parse(
      readFileSync(requiredEnvironment("FRONTEND_TIMING_ASSIGNMENT_FILE"), "utf8"),
    ) as AssignmentEvidence;
    const assigned = new Map(assignment.assigned.map((entry) => [entry.identity, entry]));
    const completed = testModules
      .map((module) => {
        const identity = moduleIdentity(module);
        const assignmentEntry = assigned.get(identity);
        if (!assignmentEntry)
          throw new Error("Vitest completed a module outside its shard assignment");
        const tests = [...module.children.allTests()]
          .map(testEvidence)
          .sort(
            (left, right) =>
              left.fullName.localeCompare(right.fullName) ||
              left.status.localeCompare(right.status),
          );
        return {
          identity,
          contentHash: assignmentEntry.contentHash,
          state: module.state(),
          durationMs: moduleDuration(module),
          tests,
        };
      })
      .sort((left, right) => left.identity.localeCompare(right.identity));
    const report = {
      schema: "kandev.frontend-timing-report/v1",
      sourceSha: requiredEnvironment("FRONTEND_TIMING_SOURCE_SHA"),
      runId: requiredEnvironment("GITHUB_RUN_ID"),
      attempt: requiredEnvironment("GITHUB_RUN_ATTEMPT"),
      shard: assignment.shard,
      shardCount: assignment.shardCount,
      reason,
      snapshot: assignment.snapshot,
      allocation: {
        mode: assignment.mode,
        planDigest: assignment.planDigest,
        assigned: assignment.assigned,
      },
      completed,
      unhandledErrorCount: unhandledErrors.length,
    };
    atomicWrite(requiredEnvironment("FRONTEND_TIMING_REPORT_FILE"), JSON.stringify(report));
  }
}

export default FrontendTimingReporter;
