import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import {
  parseBlobReports,
  findBlobReportFiles,
  readShardMetadata,
  type ShardMetadata,
  type TimingAttachment,
  type TimingError,
  type TimingObservation,
  type TimingStatus,
} from "./e2e-timings";

export type RetryAttachment = TimingAttachment & {
  artifactName?: string;
  artifactUrl?: string;
};

/** Playwright's per-test verdict, as reported by `TestCase.outcome()`. */
export type TestOutcome = "expected" | "unexpected" | "flaky" | "skipped";

export type RetryTestSummary = {
  key: string;
  project: string;
  file: string;
  title: string;
  attempts: number;
  statuses: TimingStatus[];
  finalStatus: TimingStatus;
  outcome: TestOutcome;
  finalDurationSeconds: number;
  errorCategory: "none" | "failure" | "timeout" | "interrupted";
  errors: TimingError[];
  attachments: RetryAttachment[];
};

export type FlakeCounts = {
  /** Tests that failed at least once and still ended up expected. */
  flaky: number;
  /** Tests that never reached their expected status. */
  unexpected: number;
  /** Tests that produced a verdict at all, i.e. the flake-rate denominator. */
  executed: number;
  /** `flaky / executed * 1000`, rounded to two decimals. */
  ratePerThousand: number;
  /** Keys of the flaky tests, sorted, for per-test attribution. */
  flakyTests: string[];
};

export type RetrySummary = {
  generatedAt: string;
  counts: {
    passedFirstAttempt: number;
    passedAfterRetry: number;
    failed: number;
    timedOut: number;
    skipped: number;
  };
  flake: FlakeCounts;
  planning: {
    mode: "main" | "count-fallback" | "mixed" | "unknown";
    unknownUnits: number;
    warmUnits: number;
    staleUnits: number;
    targetSeconds: number;
  };
  shards: Array<{
    cohort: string;
    shard: number;
    shardCount: number;
    predictedSeconds: number;
    actualSeconds: number;
    deltaSeconds: number;
    exitCode: number;
    startedAt: string;
    finishedAt: string;
  }>;
  tests: RetryTestSummary[];
};

export type PlanSummaryInput = {
  profileMode: "main" | "count-fallback";
  unknownUnits: number;
  warmUnits: number;
  staleUnits: number;
  targetSeconds: number;
};

export type RetrySummaryOptions = {
  shardMetadata?: ShardMetadata[];
  planSummaries?: PlanSummaryInput[];
  attachmentArtifact?: {
    name: string;
    url: string;
    pathPrefix: string;
    availablePaths: ReadonlySet<string>;
  };
};

function retryAttachment(
  attachment: TimingAttachment,
  artifact: RetrySummaryOptions["attachmentArtifact"],
): RetryAttachment {
  const resourcePath = attachment.path?.replaceAll("\\", "/");
  if (!artifact || !resourcePath || !artifact.availablePaths.has(resourcePath)) {
    return attachment;
  }
  return {
    ...attachment,
    path: `${artifact.pathPrefix}/${path.posix.basename(resourcePath)}`,
    artifactName: artifact.name,
    artifactUrl: artifact.url,
  };
}

function planMode(planModes: PlanSummaryInput["profileMode"][]): RetrySummary["planning"]["mode"] {
  if (planModes.length === 0) return "unknown";
  if (planModes.length === 1) return planModes[0]!;
  return "mixed";
}

function errorCategory(
  status: TimingStatus,
  errors: TimingError[],
): RetryTestSummary["errorCategory"] {
  if (status === "timedOut" || errors.some((error) => /timeout/i.test(error.message ?? ""))) {
    return "timeout";
  }
  if (status === "interrupted") return "interrupted";
  if (status === "failed") return "failure";
  return "none";
}

/**
 * Mirror of Playwright's `computeTestCaseOutcome` (packages/playwright/src/isomorphic/teleReceiver.ts).
 * Keeping the algorithm identical is what lets the CI cross-check assert that our
 * flake count equals the merged report's `stats.flaky` for the same run.
 *
 * Playwright also tallies interrupted and did-not-run (skipped-but-expected-to-run)
 * results, but never reads those tallies when picking the outcome, so they are
 * skipped here rather than counted.
 */
export function computeOutcome(
  statuses: readonly TimingStatus[],
  expectedStatus: TimingStatus = "passed",
): TestOutcome {
  let skipped = 0;
  let expected = 0;
  let unexpected = 0;
  for (const status of statuses) {
    if (status === "interrupted") continue;
    if (status === "skipped") {
      if (expectedStatus === "skipped") skipped += 1;
      continue;
    }
    if (status === expectedStatus) expected += 1;
    else unexpected += 1;
  }
  if (expected === 0 && unexpected === 0) return "skipped";
  if (unexpected === 0) return "expected";
  if (expected === 0 && skipped === 0) return "unexpected";
  return "flaky";
}

function flakeCounts(tests: RetryTestSummary[]): FlakeCounts {
  const flakyTests = tests.filter((test) => test.outcome === "flaky").map((test) => test.key);
  const unexpected = tests.filter((test) => test.outcome === "unexpected").length;
  const executed = tests.filter((test) => test.outcome !== "skipped").length;
  const rate = executed === 0 ? 0 : (flakyTests.length / executed) * 1000;
  return {
    flaky: flakyTests.length,
    unexpected,
    executed,
    ratePerThousand: Math.round(rate * 100) / 100,
    flakyTests,
  };
}

export function summarizeObservations(
  observations: TimingObservation[],
  generatedAt = new Date().toISOString(),
  options: RetrySummaryOptions = {},
): RetrySummary {
  // Keyed by retry index so a blob report that is present twice (`merge-reports`
  // unpacks `report.jsonl` next to the `report.zip` it read) cannot inflate the
  // attempt count or turn a clean pass into a phantom retry. A test only ever
  // produces one result per retry index.
  const grouped = new Map<string, Map<number, TimingObservation>>();
  for (const observation of observations) {
    const existing = grouped.get(observation.key) ?? new Map<number, TimingObservation>();
    existing.set(observation.retry, observation);
    grouped.set(observation.key, existing);
  }

  const tests = [...grouped.values()]
    .map((attempts) => {
      const ordered = [...attempts.values()].sort((left, right) => left.retry - right.retry);
      const finalAttempt = ordered.at(-1)!;
      const errors = ordered.flatMap((attempt) => attempt.errors);
      const statuses = ordered.map((attempt) => attempt.status);
      return {
        key: finalAttempt.key,
        project: finalAttempt.project,
        file: finalAttempt.file,
        title: finalAttempt.title,
        attempts: ordered.length,
        statuses,
        finalStatus: finalAttempt.status,
        outcome: computeOutcome(statuses, finalAttempt.expectedStatus ?? "passed"),
        finalDurationSeconds: finalAttempt.durationSeconds,
        errorCategory: errorCategory(finalAttempt.status, errors),
        errors,
        attachments: ordered.flatMap((attempt) =>
          attempt.attachments.map((attachment) =>
            retryAttachment(attachment, options.attachmentArtifact),
          ),
        ),
      } satisfies RetryTestSummary;
    })
    .sort((left, right) => left.key.localeCompare(right.key));

  const counts = {
    passedFirstAttempt: tests.filter((test) => test.finalStatus === "passed" && test.attempts === 1)
      .length,
    passedAfterRetry: tests.filter((test) => test.finalStatus === "passed" && test.attempts > 1)
      .length,
    failed: tests.filter((test) => test.finalStatus === "failed").length,
    timedOut: tests.filter((test) => test.finalStatus === "timedOut").length,
    skipped: tests.filter((test) => test.finalStatus === "skipped").length,
  };

  const planModes = [...new Set((options.planSummaries ?? []).map((plan) => plan.profileMode))];
  const planning = {
    mode: planMode(planModes),
    unknownUnits: (options.planSummaries ?? []).reduce(
      (total, plan) => total + plan.unknownUnits,
      0,
    ),
    warmUnits: (options.planSummaries ?? []).reduce((total, plan) => total + plan.warmUnits, 0),
    staleUnits: (options.planSummaries ?? []).reduce((total, plan) => total + plan.staleUnits, 0),
    targetSeconds: (options.planSummaries ?? []).reduce(
      (total, plan) => total + plan.targetSeconds,
      0,
    ),
  };

  const shards = (options.shardMetadata ?? []).map((metadata) => {
    const actualSeconds = Math.max(
      0,
      (Date.parse(metadata.finishedAt) - Date.parse(metadata.startedAt)) / 1000,
    );
    return {
      cohort: metadata.cohort,
      shard: metadata.shard,
      shardCount: metadata.shardCount,
      predictedSeconds: metadata.predictedSeconds,
      actualSeconds: Number.isFinite(actualSeconds) ? actualSeconds : 0,
      deltaSeconds: Number.isFinite(actualSeconds)
        ? actualSeconds - metadata.predictedSeconds
        : -metadata.predictedSeconds,
      exitCode: metadata.exitCode,
      startedAt: metadata.startedAt,
      finishedAt: metadata.finishedAt,
    };
  });

  return { generatedAt, counts, flake: flakeCounts(tests), planning, shards, tests };
}

function readPlanSummaries(manifestDir: string): PlanSummaryInput[] {
  return ["normal-summary.json", "containers-summary.json"].flatMap((name) => {
    const filePath = path.join(manifestDir, name);
    if (!fs.existsSync(filePath)) return [];
    try {
      const parsed = JSON.parse(fs.readFileSync(filePath, "utf8")) as {
        profile?: { mode?: PlanSummaryInput["profileMode"] };
        summary?: {
          unknownUnits?: number;
          warmUnits?: number;
          staleUnits?: number;
          targetSeconds?: number;
        };
      };
      if (!parsed.profile?.mode || !parsed.summary) return [];
      return [
        {
          profileMode: parsed.profile.mode,
          unknownUnits: parsed.summary.unknownUnits ?? 0,
          warmUnits: parsed.summary.warmUnits ?? 0,
          staleUnits: parsed.summary.staleUnits ?? 0,
          targetSeconds: parsed.summary.targetSeconds ?? 0,
        },
      ];
    } catch {
      return [];
    }
  });
}

export function parseArguments(argv: string[]): Record<string, string> {
  const args: Record<string, string> = {};
  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    if (!argument?.startsWith("--")) continue;
    const name = argument.slice(2);
    const value = argv[index + 1];
    if (value && !value.startsWith("--")) {
      args[name] = value;
      index += 1;
    }
  }
  return args;
}

function blobResourceLocations(inputPath: string): Map<string, string> {
  const locations = new Map<string, string>();
  for (const filePath of findBlobReportFiles(inputPath).filter((file) => file.endsWith(".zip"))) {
    const entries = execFileSync("unzip", ["-Z1", filePath], { encoding: "utf8" })
      .split(/\r?\n/)
      .filter((entry) => /^resources\/[A-Za-z0-9._-]+$/.test(entry));
    for (const entry of entries) {
      if (!locations.has(entry)) locations.set(entry, filePath);
    }
  }
  return locations;
}

export function materializeRetryArtifacts(
  inputPath: string,
  outputDir: string,
  resourcePaths: Iterable<string>,
): Set<string> {
  const locations = blobResourceLocations(inputPath);
  const available = new Set<string>();
  for (const resourcePath of new Set(resourcePaths)) {
    const sourceZip = locations.get(resourcePath);
    if (!sourceZip) continue;
    fs.mkdirSync(outputDir, { recursive: true });
    const outputPath = path.join(outputDir, path.posix.basename(resourcePath));
    const outputFd = fs.openSync(outputPath, "w");
    try {
      execFileSync("unzip", ["-p", sourceZip, resourcePath], {
        stdio: ["ignore", outputFd, "pipe"],
      });
      available.add(resourcePath);
    } finally {
      fs.closeSync(outputFd);
    }
  }
  return available;
}

function runCli(): void {
  const args = parseArguments(process.argv.slice(2));
  if (!args.input || !args.output) {
    throw new Error("Usage: retry-summary.ts --input <blob-dir> --output <summary.json>");
  }
  const observations = parseBlobReports(args.input);
  const attachmentOutput = args["attachment-output"];
  const availablePaths = attachmentOutput
    ? materializeRetryArtifacts(
        args.input,
        path.resolve(attachmentOutput),
        observations.flatMap((observation) =>
          observation.attachments.flatMap((attachment) => attachment.path ?? []),
        ),
      )
    : new Set<string>();
  const attachmentArtifact =
    args["artifact-name"] && args["artifact-url"] && attachmentOutput
      ? {
          name: args["artifact-name"],
          url: args["artifact-url"],
          pathPrefix: path.basename(path.resolve(attachmentOutput)),
          availablePaths,
        }
      : undefined;
  const summary = summarizeObservations(observations, new Date().toISOString(), {
    shardMetadata: readShardMetadata(args.input),
    planSummaries: args["manifest-dir"] ? readPlanSummaries(args["manifest-dir"]!) : [],
    attachmentArtifact,
  });
  fs.mkdirSync(path.dirname(path.resolve(args.output)), { recursive: true });
  fs.writeFileSync(args.output, `${JSON.stringify(summary, null, 2)}\n`);
  console.log(
    `retry summary: ${summary.flake.flaky} flaky of ${summary.flake.executed} executed ` +
      `(${summary.flake.ratePerThousand} per 1000), ${summary.flake.unexpected} unexpected`,
  );
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  runCli();
}
