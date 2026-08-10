import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

export const TIMING_PROFILE_VERSION = 1;
export const MAX_SAMPLES_PER_TEST = 20;

export type TimingStatus = "passed" | "failed" | "timedOut" | "skipped" | "interrupted" | "unknown";

export type TimingError = {
  message?: string;
  value?: string;
  stack?: string;
};

export type TimingAttachment = {
  name: string;
  path?: string;
  body?: string;
};

export type TimingObservation = {
  key: string;
  project: string;
  file: string;
  title: string;
  retry: number;
  status: TimingStatus;
  durationSeconds: number;
  errors: TimingError[];
  attachments: TimingAttachment[];
};

export type TimingSample = {
  durationSeconds: number;
  sourceSha: string;
  runId: string;
  recordedAt: string;
};

export type TimingEntry = {
  key: string;
  project: string;
  file: string;
  title: string;
  fileHash: string;
  samples: TimingSample[];
  p50Seconds: number;
  p75Seconds: number;
  lastSeenSha: string;
  lastSeenRunId: string;
};

export type TimingProfile = {
  version: typeof TIMING_PROFILE_VERSION;
  generatedAt: string;
  source: {
    branch: string;
    sha: string;
    runId: string;
  };
  entries: TimingEntry[];
};

export type TimingProfileBuildMetadata = {
  branch: string;
  sha: string;
  runId: string;
  generatedAt: string;
  fileHashes: Map<string, string>;
  knownKeys?: Set<string>;
};

export type ShardMetadata = {
  cohort: string;
  shard: number;
  shardCount: number;
  startedAt: string;
  finishedAt: string;
  exitCode: number;
  predictedSeconds: number;
  unitIds: string[];
};

type JsonRecord = Record<string, unknown>;

type TestDescriptor = {
  key: string;
  project: string;
  file: string;
  title: string;
};

function asRecord(value: unknown): JsonRecord | undefined {
  return value && typeof value === "object" ? (value as JsonRecord) : undefined;
}

function asString(value: unknown, fallback = ""): string {
  return typeof value === "string" ? value : fallback;
}

function asNumber(value: unknown, fallback = 0): number {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

export function normalizeReportPath(file: string): string {
  const normalized = file.replaceAll("\\", "/").replace(/^\.\//, "");
  const testsIndex = normalized.indexOf("tests/");
  if (testsIndex >= 0) return normalized.slice(testsIndex);
  const e2eIndex = normalized.indexOf("e2e/");
  if (e2eIndex >= 0) return normalized.slice(e2eIndex + "e2e/".length);
  return normalized ? `tests/${normalized}` : normalized;
}

export function stableTestKey(project: string, file: string, title: string): string {
  return `${project}::${normalizeReportPath(file)}::${title}`;
}

function collectProjectTests(project: JsonRecord, descriptors: Map<string, TestDescriptor>): void {
  const projectName = asString(project.name, "unknown");

  const walkSuite = (suite: JsonRecord, parentTitles: string[]) => {
    const suiteTitle = asString(suite.title);
    const titles = suiteTitle ? [...parentTitles, suiteTitle] : parentTitles;
    const entries = Array.isArray(suite.entries) ? suite.entries : [];
    for (const rawEntry of entries) {
      const entry = asRecord(rawEntry);
      if (!entry) continue;

      const testId = asString(entry.testId);
      if (testId) {
        const location = asRecord(entry.location);
        const file = normalizeReportPath(asString(location?.file));
        const fileName = path.posix.basename(file);
        const title = [...titles.filter((value) => value !== fileName), asString(entry.title)]
          .filter(Boolean)
          .join(" › ");
        descriptors.set(testId, {
          key: stableTestKey(projectName, file, title),
          project: projectName,
          file,
          title,
        });
        continue;
      }

      walkSuite(entry, titles);
    }
  };

  const suites = Array.isArray(project.suites) ? project.suites : [];
  for (const rawSuite of suites) {
    const suite = asRecord(rawSuite);
    if (suite) walkSuite(suite, []);
  }
}

function parseJsonLines(content: string): JsonRecord[] {
  const events: JsonRecord[] = [];
  for (const line of content.split(/\r?\n/)) {
    if (!line.trim()) continue;
    try {
      const parsed = JSON.parse(line) as unknown;
      const event = asRecord(parsed);
      if (event) events.push(event);
    } catch {
      // A truncated report should not make all other shard timings unusable.
    }
  }
  return events;
}

type BlobParseState = {
  descriptors: Map<string, TestDescriptor>;
  retries: Map<string, number>;
  attachments: Map<string, TimingAttachment[]>;
  observations: TimingObservation[];
};

function parseTimingStatus(value: unknown): TimingStatus {
  switch (asString(value)) {
    case "passed":
    case "failed":
    case "timedOut":
    case "skipped":
    case "interrupted":
      return asString(value) as TimingStatus;
    default:
      return "unknown";
  }
}

function parseTimingErrors(value: unknown): TimingError[] {
  if (!Array.isArray(value)) return [];
  return value.flatMap((rawError) => {
    const error = asRecord(rawError);
    return error
      ? [
          {
            message: asString(error.message) || undefined,
            value: asString(error.value) || undefined,
            stack: asString(error.stack) || undefined,
          },
        ]
      : [];
  });
}

function parseTimingAttachment(value: unknown): TimingAttachment | undefined {
  const attachment = asRecord(value);
  if (!attachment) return undefined;
  return {
    name: asString(attachment.name, "attachment"),
    path: asString(attachment.path) || undefined,
    body: asString(attachment.base64) || undefined,
  };
}

function handleProjectEvent(state: BlobParseState, params: JsonRecord): void {
  const project = asRecord(params.project);
  if (project) collectProjectTests(project, state.descriptors);
}

function handleTestBeginEvent(state: BlobParseState, params: JsonRecord): void {
  const result = asRecord(params.result);
  const resultId = asString(result?.id);
  if (resultId) state.retries.set(resultId, asNumber(result?.retry));
}

function handleAttachEvent(state: BlobParseState, params: JsonRecord): void {
  const resultId = asString(params.resultId);
  if (!resultId) return;

  const existing = state.attachments.get(resultId) ?? [];
  const rawAttachments = Array.isArray(params.attachments) ? params.attachments : [];
  for (const rawAttachment of rawAttachments) {
    const attachment = parseTimingAttachment(rawAttachment);
    if (attachment) existing.push(attachment);
  }
  state.attachments.set(resultId, existing);
}

function parseTestEndEvent(
  state: BlobParseState,
  params: JsonRecord,
): TimingObservation | undefined {
  const test = asRecord(params.test);
  const result = asRecord(params.result);
  if (!test || !result) return undefined;

  const descriptor = state.descriptors.get(asString(test.testId));
  if (!descriptor) return undefined;

  const resultId = asString(result.id);
  return {
    ...descriptor,
    retry: state.retries.get(resultId) ?? 0,
    status: parseTimingStatus(result.status),
    durationSeconds: asNumber(result.duration) / 1000,
    errors: parseTimingErrors(result.errors),
    attachments: state.attachments.get(resultId) ?? [],
  };
}

export function parseBlobReportJsonl(content: string): TimingObservation[] {
  const state: BlobParseState = {
    descriptors: new Map(),
    retries: new Map(),
    attachments: new Map(),
    observations: [],
  };

  for (const event of parseJsonLines(content)) {
    const method = asString(event.method);
    const params = asRecord(event.params);
    if (!params) continue;

    switch (method) {
      case "onProject":
        handleProjectEvent(state, params);
        break;
      case "onTestBegin":
        handleTestBeginEvent(state, params);
        break;
      case "onAttach":
        handleAttachEvent(state, params);
        break;
      case "onTestEnd": {
        const observation = parseTestEndEvent(state, params);
        if (observation) state.observations.push(observation);
        break;
      }
      default:
        break;
    }
  }

  return state.observations;
}

export function collectEligibleSamples(observations: TimingObservation[]): TimingObservation[] {
  return observations.filter(
    (observation) =>
      observation.status === "passed" &&
      observation.retry === 0 &&
      Number.isFinite(observation.durationSeconds) &&
      observation.durationSeconds >= 0,
  );
}

export function quantile(values: number[], probability: number): number {
  if (values.length === 0) return 0;
  const sorted = [...values].sort((a, b) => a - b);
  const position = Math.min(1, Math.max(0, probability)) * (sorted.length - 1);
  const lower = Math.floor(position);
  const upper = Math.ceil(position);
  if (lower === upper) return sorted[lower] ?? 0;
  const fraction = position - lower;
  return (sorted[lower] ?? 0) + ((sorted[upper] ?? 0) - (sorted[lower] ?? 0)) * fraction;
}

export function buildTimingProfile(
  previous: TimingProfile | undefined,
  observations: TimingObservation[],
  metadata: TimingProfileBuildMetadata,
): TimingProfile {
  const entries = new Map<string, TimingEntry>();
  for (const entry of previous?.entries ?? []) {
    if (metadata.knownKeys && !metadata.knownKeys.has(entry.key)) continue;
    entries.set(entry.key, {
      ...entry,
      samples: entry.samples.slice(-MAX_SAMPLES_PER_TEST),
    });
  }

  for (const observation of collectEligibleSamples(observations)) {
    const previousEntry = entries.get(observation.key);
    const samples = [
      ...(previousEntry?.samples ?? []),
      {
        durationSeconds: observation.durationSeconds,
        sourceSha: metadata.sha,
        runId: metadata.runId,
        recordedAt: metadata.generatedAt,
      },
    ].slice(-MAX_SAMPLES_PER_TEST);
    const durations = samples.map((sample) => sample.durationSeconds);
    const fileHash = metadata.fileHashes.get(observation.file) ?? previousEntry?.fileHash ?? "";
    entries.set(observation.key, {
      key: observation.key,
      project: observation.project,
      file: observation.file,
      title: observation.title,
      fileHash,
      samples,
      p50Seconds: quantile(durations, 0.5),
      p75Seconds: quantile(durations, 0.75),
      lastSeenSha: metadata.sha,
      lastSeenRunId: metadata.runId,
    });
  }

  return {
    version: TIMING_PROFILE_VERSION,
    generatedAt: metadata.generatedAt,
    source: { branch: metadata.branch, sha: metadata.sha, runId: metadata.runId },
    entries: [...entries.values()].sort((left, right) => left.key.localeCompare(right.key)),
  };
}

export function readTimingProfile(filePath: string): TimingProfile | undefined {
  if (!fs.existsSync(filePath)) return undefined;
  const parsed = JSON.parse(fs.readFileSync(filePath, "utf8")) as Partial<TimingProfile>;
  if (parsed.version !== TIMING_PROFILE_VERSION || !Array.isArray(parsed.entries)) {
    throw new Error(`Unsupported timing profile version in ${filePath}`);
  }
  return parsed as TimingProfile;
}

export function hashSourceFile(webRoot: string, relativeFile: string): string {
  const normalized = normalizeReportPath(relativeFile);
  const candidates = [path.resolve(webRoot, normalized), path.resolve(webRoot, "e2e", normalized)];
  const filePath = candidates.find((candidate) => {
    try {
      return fs.statSync(candidate).isFile();
    } catch {
      return false;
    }
  });
  if (!filePath) return "";
  return createHash("sha256").update(fs.readFileSync(filePath)).digest("hex");
}

function listFiles(root: string): string[] {
  if (!fs.existsSync(root)) return [];
  const stats = fs.statSync(root);
  if (stats.isFile()) return [root];
  return fs.readdirSync(root, { withFileTypes: true }).flatMap((entry) => {
    const child = path.join(root, entry.name);
    return entry.isDirectory() ? listFiles(child) : [child];
  });
}

export function findBlobReportFiles(inputPath: string): string[] {
  return listFiles(inputPath)
    .filter((filePath) => filePath.endsWith(".zip") || filePath.endsWith(".jsonl"))
    .sort();
}

export function findShardMetadataFiles(inputPath: string): string[] {
  return listFiles(inputPath)
    .filter((filePath) => /shard-(normal|containers)-\d+-metadata\.json$/.test(filePath))
    .sort();
}

export function readShardMetadata(inputPath: string): ShardMetadata[] {
  return findShardMetadataFiles(inputPath).flatMap((filePath) => {
    try {
      const parsed = JSON.parse(fs.readFileSync(filePath, "utf8")) as Partial<ShardMetadata>;
      if (
        typeof parsed.cohort !== "string" ||
        typeof parsed.shard !== "number" ||
        typeof parsed.shardCount !== "number" ||
        typeof parsed.startedAt !== "string" ||
        typeof parsed.finishedAt !== "string" ||
        typeof parsed.exitCode !== "number" ||
        typeof parsed.predictedSeconds !== "number" ||
        !Array.isArray(parsed.unitIds)
      ) {
        return [];
      }
      return [
        {
          cohort: parsed.cohort,
          shard: parsed.shard,
          shardCount: parsed.shardCount,
          startedAt: parsed.startedAt,
          finishedAt: parsed.finishedAt,
          exitCode: parsed.exitCode,
          predictedSeconds: parsed.predictedSeconds,
          unitIds: parsed.unitIds.filter((unitId): unitId is string => typeof unitId === "string"),
        },
      ];
    } catch {
      return [];
    }
  });
}

export function readBlobReportJsonl(filePath: string): string {
  if (filePath.endsWith(".jsonl")) return fs.readFileSync(filePath, "utf8");
  const entries = execFileSync("unzip", ["-Z1", filePath], { encoding: "utf8" })
    .split(/\r?\n/)
    .filter(Boolean);
  const reportFile = entries.find((entry) => entry.endsWith("report.jsonl"));
  if (!reportFile) throw new Error(`Blob report ${filePath} does not contain report.jsonl`);
  return execFileSync("unzip", ["-p", filePath, reportFile], { encoding: "utf8" });
}

export function parseBlobReports(inputPath: string): TimingObservation[] {
  return findBlobReportFiles(inputPath).flatMap((filePath) =>
    parseBlobReportJsonl(readBlobReportJsonl(filePath)),
  );
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
  const input = args.input;
  const output = args.output;
  if (!input || !output) {
    throw new Error("Usage: e2e-timings.ts --input <blob-dir> --output <profile.json>");
  }

  const observations = parseBlobReports(input);
  const webRoot = args["web-root"] ?? process.cwd();
  const fileHashes = new Map(
    [...new Set(observations.map((observation) => observation.file))].map((file) => [
      file,
      hashSourceFile(webRoot, file),
    ]),
  );
  const previous = args.previous ? readTimingProfile(args.previous) : undefined;
  const profile = buildTimingProfile(previous, observations, {
    branch: args.branch ?? process.env.GITHUB_REF_NAME ?? "unknown",
    sha: args.sha ?? process.env.GITHUB_SHA ?? "unknown",
    runId: args["run-id"] ?? process.env.GITHUB_RUN_ID ?? "unknown",
    generatedAt: args.generated ?? new Date().toISOString(),
    fileHashes,
    knownKeys:
      observations.length > 0
        ? new Set(observations.map((observation) => observation.key))
        : undefined,
  });
  fs.mkdirSync(path.dirname(path.resolve(output)), { recursive: true });
  fs.writeFileSync(output, `${JSON.stringify(profile, null, 2)}\n`);
  console.log(
    `timing profile: ${profile.entries.length} entries, ${collectEligibleSamples(observations).length} new samples`,
  );
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  runCli();
}
