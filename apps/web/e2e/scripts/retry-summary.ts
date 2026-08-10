import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import {
  parseBlobReports,
  readShardMetadata,
  type ShardMetadata,
  type TimingAttachment,
  type TimingError,
  type TimingObservation,
  type TimingStatus,
} from "./e2e-timings";

export type RetryTestSummary = {
  key: string;
  project: string;
  file: string;
  title: string;
  attempts: number;
  statuses: TimingStatus[];
  finalStatus: TimingStatus;
  finalDurationSeconds: number;
  errorCategory: "none" | "failure" | "timeout" | "interrupted";
  errors: TimingError[];
  attachments: TimingAttachment[];
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
};

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

export function summarizeObservations(
  observations: TimingObservation[],
  generatedAt = new Date().toISOString(),
  options: RetrySummaryOptions = {},
): RetrySummary {
  const grouped = new Map<string, TimingObservation[]>();
  for (const observation of observations) {
    const existing = grouped.get(observation.key) ?? [];
    existing.push(observation);
    grouped.set(observation.key, existing);
  }

  const tests = [...grouped.values()]
    .map((attempts) => {
      const ordered = [...attempts].sort((left, right) => left.retry - right.retry);
      const finalAttempt = ordered.at(-1)!;
      const errors = ordered.flatMap((attempt) => attempt.errors);
      return {
        key: finalAttempt.key,
        project: finalAttempt.project,
        file: finalAttempt.file,
        title: finalAttempt.title,
        attempts: ordered.length,
        statuses: ordered.map((attempt) => attempt.status),
        finalStatus: finalAttempt.status,
        finalDurationSeconds: finalAttempt.durationSeconds,
        errorCategory: errorCategory(finalAttempt.status, errors),
        errors,
        attachments: ordered.flatMap((attempt) => attempt.attachments),
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

  return { generatedAt, counts, planning, shards, tests };
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

function parseArguments(argv: string[]): Record<string, string> {
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

function runCli(): void {
  const args = parseArguments(process.argv.slice(2));
  if (!args.input || !args.output) {
    throw new Error("Usage: retry-summary.ts --input <blob-dir> --output <summary.json>");
  }
  const summary = summarizeObservations(parseBlobReports(args.input), new Date().toISOString(), {
    shardMetadata: readShardMetadata(args.input),
    planSummaries: args["manifest-dir"] ? readPlanSummaries(args["manifest-dir"]!) : [],
  });
  fs.mkdirSync(path.dirname(path.resolve(args.output)), { recursive: true });
  fs.writeFileSync(args.output, `${JSON.stringify(summary, null, 2)}\n`);
  console.log(
    `retry summary: ${summary.counts.passedAfterRetry} passed after retry, ${summary.counts.failed} failed`,
  );
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  runCli();
}
