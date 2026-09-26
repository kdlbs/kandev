/**
 * Turns `retry-summary.json` into a job-summary flake report and an append-only
 * flake-rate history, so the E2E flake rate is a trended number instead of a
 * fact you can only learn by downloading an artifact and reading it by hand.
 *
 * The history is carried between runs as the `e2e-flake-history` artifact: the
 * report job downloads the newest one published by `main`, appends this run's
 * entry, and re-uploads it. No external service is involved.
 */
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { parseArguments, type RetrySummary } from "./retry-summary";

export const FLAKE_HISTORY_VERSION = 1;
/** Roughly a month of `main` runs; keeps the artifact small and the table readable. */
export const MAX_HISTORY_ENTRIES = 50;
/** How many past runs the baseline, trend table, and repeat-offender table cover. */
export const TREND_WINDOW = 10;

export type FlakeHistoryEntry = {
  runId: string;
  runUrl?: string;
  branch: string;
  sha: string;
  recordedAt: string;
  flaky: number;
  unexpected: number;
  executed: number;
  ratePerThousand: number;
  flakyTests: string[];
};

export type FlakeHistory = {
  version: typeof FLAKE_HISTORY_VERSION;
  /** Newest first. */
  entries: FlakeHistoryEntry[];
};

export type RunSource = {
  runId: string;
  runUrl?: string;
  branch: string;
  sha: string;
};

export type PlaywrightStats = {
  expected: number;
  unexpected: number;
  flaky: number;
  skipped: number;
};

export type CrossCheck =
  | { status: "match"; ours: number; theirs: number }
  | { status: "mismatch"; ours: number; theirs: number }
  | { status: "unavailable"; ours: number; reason: string };

export function entryFromSummary(
  summary: RetrySummary,
  source: RunSource,
  recordedAt: string,
): FlakeHistoryEntry {
  return {
    runId: source.runId,
    ...(source.runUrl ? { runUrl: source.runUrl } : {}),
    branch: source.branch,
    sha: source.sha,
    recordedAt,
    flaky: summary.flake.flaky,
    unexpected: summary.flake.unexpected,
    executed: summary.flake.executed,
    ratePerThousand: summary.flake.ratePerThousand,
    flakyTests: summary.flake.flakyTests,
  };
}

export function appendHistoryEntry(
  previous: FlakeHistory | undefined,
  entry: FlakeHistoryEntry,
  limit = MAX_HISTORY_ENTRIES,
): FlakeHistory {
  const kept = (previous?.entries ?? []).filter((candidate) => candidate.runId !== entry.runId);
  return { version: FLAKE_HISTORY_VERSION, entries: [entry, ...kept].slice(0, limit) };
}

/**
 * Every field the baseline, trend table, and offender table read. A partial
 * entry is worse than a missing one: an absent `ratePerThousand` makes `median`
 * return `NaN`, which serializes to `null` and reads as a real measurement, and
 * an absent `sha` throws in the trend table.
 */
function isUsableEntry(value: unknown): value is FlakeHistoryEntry {
  if (typeof value !== "object" || value === null) return false;
  const entry = value as Partial<FlakeHistoryEntry>;
  return (
    typeof entry.runId === "string" &&
    typeof entry.sha === "string" &&
    typeof entry.flaky === "number" &&
    typeof entry.unexpected === "number" &&
    typeof entry.executed === "number" &&
    typeof entry.ratePerThousand === "number" &&
    Array.isArray(entry.flakyTests)
  );
}

export function readHistory(filePath: string | undefined): FlakeHistory | undefined {
  if (!filePath || !fs.existsSync(filePath)) return undefined;
  try {
    const parsed = JSON.parse(fs.readFileSync(filePath, "utf8")) as Partial<FlakeHistory>;
    if (parsed.version !== FLAKE_HISTORY_VERSION || !Array.isArray(parsed.entries))
      return undefined;
    return { version: FLAKE_HISTORY_VERSION, entries: parsed.entries.filter(isUsableEntry) };
  } catch {
    // A corrupt baseline must not take the reporting step down with it.
    return undefined;
  }
}

export function median(values: readonly number[]): number {
  if (values.length === 0) return 0;
  const sorted = [...values].sort((left, right) => left - right);
  const middle = Math.floor(sorted.length / 2);
  const value =
    sorted.length % 2 === 1 ? sorted[middle]! : (sorted[middle - 1]! + sorted[middle]!) / 2;
  return Math.round(value * 100) / 100;
}

export type Baseline = {
  runs: number;
  medianFlaky: number;
  medianRatePerThousand: number;
};

export function baselineFrom(entries: readonly FlakeHistoryEntry[]): Baseline {
  const window = entries.slice(0, TREND_WINDOW);
  return {
    runs: window.length,
    medianFlaky: median(window.map((entry) => entry.flaky)),
    medianRatePerThousand: median(window.map((entry) => entry.ratePerThousand)),
  };
}

export function repeatOffenders(
  entries: readonly FlakeHistoryEntry[],
): Array<{ key: string; runs: number }> {
  const counts = new Map<string, number>();
  for (const entry of entries.slice(0, TREND_WINDOW)) {
    for (const key of new Set(entry.flakyTests)) {
      counts.set(key, (counts.get(key) ?? 0) + 1);
    }
  }
  return [...counts.entries()]
    .map(([key, runs]) => ({ key, runs }))
    .sort((left, right) => right.runs - left.runs || left.key.localeCompare(right.key));
}

/**
 * Reads `stats` out of a Playwright JSON report without parsing the whole file.
 * The merged report for the full suite can be hundreds of megabytes, and
 * `stats` is the last key the JSON reporter writes, so a bounded tail read is
 * both sufficient and safe.
 */
export function readPlaywrightStats(filePath: string | undefined): PlaywrightStats | undefined {
  if (!filePath || !fs.existsSync(filePath)) return undefined;
  const size = fs.statSync(filePath).size;
  const length = Math.min(size, 8192);
  const buffer = Buffer.alloc(length);
  const descriptor = fs.openSync(filePath, "r");
  try {
    fs.readSync(descriptor, buffer, 0, length, size - length);
  } finally {
    fs.closeSync(descriptor);
  }
  const match = /"stats":\s*(\{[^{}]*\})\s*\}\s*$/.exec(buffer.toString("utf8"));
  if (!match) return undefined;
  try {
    const stats = JSON.parse(match[1]!) as Partial<PlaywrightStats>;
    if (typeof stats.flaky !== "number") return undefined;
    return {
      expected: stats.expected ?? 0,
      unexpected: stats.unexpected ?? 0,
      flaky: stats.flaky,
      skipped: stats.skipped ?? 0,
    };
  } catch {
    return undefined;
  }
}

export function crossCheckFlakeCount(ours: number, stats: PlaywrightStats | undefined): CrossCheck {
  if (!stats) {
    return { status: "unavailable", ours, reason: "no merged Playwright JSON report" };
  }
  return { status: stats.flaky === ours ? "match" : "mismatch", ours, theirs: stats.flaky };
}

function signed(value: number): string {
  return value > 0 ? `+${value}` : `${value}`;
}

function runLabel(entry: FlakeHistoryEntry): string {
  const label = `${entry.branch} #${entry.runId}`;
  return entry.runUrl ? `[${label}](${entry.runUrl})` : label;
}

/**
 * The workflow-log counterpart of the summary's cross-check line. Every status
 * gets a line, including a match: if only a mismatch spoke, a cross-check that
 * silently stopped running would be indistinguishable in the log from one that
 * passed, and an unverified flake rate is the failure this whole report exists
 * to prevent. A skip is annotated as a warning for the same reason.
 */
export function crossCheckLogLine(check: CrossCheck): string {
  if (check.status === "match") {
    return `E2E flake cross-check: match. retry-summary and the merged Playwright report both report ${check.ours} flaky.`;
  }
  if (check.status === "mismatch") {
    return (
      `::warning::E2E flake cross-check: MISMATCH. retry-summary reports ${check.ours} flaky, ` +
      `the merged Playwright report reports ${check.theirs}. The trended flake rate is not trustworthy for this run.`
    );
  }
  return (
    `::warning::E2E flake cross-check: SKIPPED (${check.reason}). ` +
    `The reported flake count of ${check.ours} was not verified against Playwright.`
  );
}

function crossCheckLine(check: CrossCheck): string {
  if (check.status === "match") {
    return `- Playwright cross-check: **match** (${check.ours} flaky in both retry-summary and the merged report)`;
  }
  if (check.status === "mismatch") {
    return `- Playwright cross-check: **MISMATCH** (retry-summary says ${check.ours}, merged Playwright report says ${check.theirs})`;
  }
  return `- Playwright cross-check: skipped (${check.reason})`;
}

function headlineLines(entry: FlakeHistoryEntry, baseline: Baseline, check: CrossCheck): string[] {
  const lines = [
    "## E2E flake rate",
    "",
    `- Flaky tests this run: **${entry.flaky}** of ${entry.executed} executed (**${entry.ratePerThousand}** per 1000)`,
    `- Unexpected failures this run: ${entry.unexpected}`,
  ];
  if (baseline.runs > 0) {
    const deltaFlaky = entry.flaky - baseline.medianFlaky;
    const deltaRate =
      Math.round((entry.ratePerThousand - baseline.medianRatePerThousand) * 100) / 100;
    lines.push(
      `- Baseline (median of last ${baseline.runs} recorded ${baseline.runs === 1 ? "run" : "runs"}): ${baseline.medianFlaky} flaky, ${baseline.medianRatePerThousand} per 1000`,
      `- Change vs baseline: ${signed(deltaFlaky)} flaky, ${signed(deltaRate)} per 1000`,
    );
  } else {
    lines.push("- Baseline: none recorded yet; this run seeds the trend.");
  }
  lines.push(crossCheckLine(check));
  return lines;
}

function flakyTestTable(summary: RetrySummary, offenders: Map<string, number>): string[] {
  const flaky = summary.tests.filter((test) => test.outcome === "flaky");
  if (flaky.length === 0) return [];
  return [
    "",
    "### Flaky tests in this run",
    "",
    `| Test | Attempts | Statuses | Flaked in last ${TREND_WINDOW} recorded runs |`,
    "| --- | ---: | --- | ---: |",
    ...flaky.map(
      (test) =>
        `| \`${test.key}\` | ${test.attempts} | ${test.statuses.join(" → ")} | ${offenders.get(test.key) ?? 0} |`,
    ),
  ];
}

function trendTable(entries: readonly FlakeHistoryEntry[]): string[] {
  if (entries.length === 0) return [];
  return [
    "",
    "### Flake rate trend (most recent first)",
    "",
    "| Run | SHA | Flaky | Per 1000 | Executed |",
    "| --- | --- | ---: | ---: | ---: |",
    ...entries
      .slice(0, TREND_WINDOW)
      .map(
        (entry) =>
          `| ${runLabel(entry)} | \`${entry.sha.slice(0, 7)}\` | ${entry.flaky} | ${entry.ratePerThousand} | ${entry.executed} |`,
      ),
  ];
}

function offenderTable(offenders: Array<{ key: string; runs: number }>): string[] {
  const repeated = offenders.filter((offender) => offender.runs > 1);
  if (repeated.length === 0) return [];
  return [
    "",
    `### Repeat offenders (last ${TREND_WINDOW} recorded runs)`,
    "",
    "| Test | Runs flaked |",
    "| --- | ---: |",
    ...repeated.map((offender) => `| \`${offender.key}\` | ${offender.runs} |`),
  ];
}

/**
 * @param history entries recorded *before* this run, newest first.
 */
export function renderFlakeReport(
  summary: RetrySummary,
  entry: FlakeHistoryEntry,
  history: readonly FlakeHistoryEntry[],
  check: CrossCheck,
): string {
  const offenders = repeatOffenders(history);
  const offenderRuns = new Map(offenders.map((offender) => [offender.key, offender.runs]));
  return [
    ...headlineLines(entry, baselineFrom(history), check),
    ...flakyTestTable(summary, offenderRuns),
    ...trendTable([entry, ...history]),
    ...offenderTable(offenders),
    "",
  ].join("\n");
}

function runCli(): void {
  const args = parseArguments(process.argv.slice(2));
  if (!args.summary) {
    throw new Error(
      "Usage: flake-report.ts --summary <retry-summary.json> [--output <history.json>]",
    );
  }
  const summary = JSON.parse(fs.readFileSync(args.summary, "utf8")) as RetrySummary;
  const previous = readHistory(args.history);
  const entry = entryFromSummary(
    summary,
    {
      runId: args["run-id"] ?? "local",
      runUrl: args["run-url"],
      branch: args.branch ?? "unknown",
      sha: args.sha ?? "unknown",
    },
    new Date().toISOString(),
  );
  const check = crossCheckFlakeCount(entry.flaky, readPlaywrightStats(args["playwright-json"]));
  const report = renderFlakeReport(summary, entry, previous?.entries ?? [], check);

  const stepSummary = args["step-summary"] ?? process.env.GITHUB_STEP_SUMMARY;
  if (stepSummary) fs.appendFileSync(stepSummary, report);
  else console.log(report);

  if (args.output) {
    const outputPath = path.resolve(args.output);
    fs.mkdirSync(path.dirname(outputPath), { recursive: true });
    fs.writeFileSync(
      outputPath,
      `${JSON.stringify(appendHistoryEntry(previous, entry), null, 2)}\n`,
    );
  }

  console.log(crossCheckLogLine(check));
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  runCli();
}
