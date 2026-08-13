import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { describe, expect, it } from "vitest";
import {
  appendHistoryEntry,
  baselineFrom,
  crossCheckFlakeCount,
  entryFromSummary,
  MAX_HISTORY_ENTRIES,
  median,
  readHistory,
  readPlaywrightStats,
  renderFlakeReport,
  repeatOffenders,
  type FlakeHistoryEntry,
} from "./flake-report";
import type { RetrySummary, RetryTestSummary } from "./retry-summary";

function entry(overrides: Partial<FlakeHistoryEntry> = {}): FlakeHistoryEntry {
  const runId = overrides.runId ?? "100";
  return {
    runId,
    runUrl: `https://github.com/kdlbs/kandev/actions/runs/${runId}`,
    branch: "main",
    sha: "abcdef1234567890",
    recordedAt: "2026-08-10T10:00:00.000Z",
    flaky: 3,
    unexpected: 0,
    executed: 2000,
    ratePerThousand: 1.5,
    flakyTests: ["chromium::tests/a.spec.ts::a"],
    ...overrides,
  };
}

function flakyTest(overrides: Partial<RetryTestSummary> = {}): RetryTestSummary {
  return {
    key: "chromium::tests/a.spec.ts::a",
    project: "chromium",
    file: "tests/a.spec.ts",
    title: "a",
    attempts: 2,
    statuses: ["failed", "passed"],
    finalStatus: "passed",
    outcome: "flaky",
    finalDurationSeconds: 1,
    errorCategory: "none",
    errors: [],
    attachments: [],
    ...overrides,
  };
}

function summary(overrides: Partial<RetrySummary["flake"]> = {}): RetrySummary {
  return {
    generatedAt: "2026-08-11T10:00:00.000Z",
    counts: {
      passedFirstAttempt: 1999,
      passedAfterRetry: 1,
      failed: 0,
      timedOut: 0,
      skipped: 0,
    },
    flake: {
      flaky: 1,
      unexpected: 0,
      executed: 2000,
      ratePerThousand: 0.5,
      flakyTests: ["chromium::tests/a.spec.ts::a"],
      ...overrides,
    },
    planning: { mode: "main", unknownUnits: 0, warmUnits: 0, staleUnits: 0, targetSeconds: 0 },
    shards: [],
    tests: [flakyTest()],
  };
}

describe("flake history", () => {
  it("prepends the newest entry, replaces a re-run of the same run, and caps the history", () => {
    const previous = {
      version: 1 as const,
      entries: Array.from({ length: MAX_HISTORY_ENTRIES }, (_, index) =>
        entry({ runId: `${index}` }),
      ),
    };

    const appended = appendHistoryEntry(previous, entry({ runId: "new", flaky: 7 }));

    expect(appended.entries).toHaveLength(MAX_HISTORY_ENTRIES);
    expect(appended.entries[0]).toMatchObject({ runId: "new", flaky: 7 });
    expect(appended.entries.at(-1)?.runId).toBe(`${MAX_HISTORY_ENTRIES - 2}`);

    const rerun = appendHistoryEntry(previous, entry({ runId: "5", flaky: 9 }));
    expect(rerun.entries.filter((candidate) => candidate.runId === "5")).toEqual([
      expect.objectContaining({ flaky: 9 }),
    ]);
  });

  it("takes the median of the trend window, not the mean and not the whole history", () => {
    expect(median([1, 2, 3])).toBe(2);
    expect(median([1, 2, 3, 10])).toBe(2.5);
    expect(median([])).toBe(0);

    const entries = [
      ...Array.from({ length: 10 }, () => entry({ flaky: 2, ratePerThousand: 1 })),
      entry({ flaky: 99, ratePerThousand: 50 }),
    ];

    expect(baselineFrom(entries)).toEqual({
      runs: 10,
      medianFlaky: 2,
      medianRatePerThousand: 1,
    });
  });

  it("counts how many recent runs each test flaked in, deduplicating within a run", () => {
    const offenders = repeatOffenders([
      entry({ runId: "3", flakyTests: ["a", "a", "b"] }),
      entry({ runId: "2", flakyTests: ["a"] }),
      entry({ runId: "1", flakyTests: ["c"] }),
    ]);

    expect(offenders).toEqual([
      { key: "a", runs: 2 },
      { key: "b", runs: 1 },
      { key: "c", runs: 1 },
    ]);
  });

  it("ignores a history file of an unknown version or unreadable shape", () => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-flake-history-"));
    const write = (name: string, content: string) => {
      const filePath = path.join(directory, name);
      fs.writeFileSync(filePath, content);
      return filePath;
    };

    expect(readHistory(write("bad.json", "{"))).toBeUndefined();
    expect(readHistory(write("v9.json", '{"version":9,"entries":[]}'))).toBeUndefined();
    expect(readHistory(path.join(directory, "missing.json"))).toBeUndefined();
    expect(readHistory(write("ok.json", '{"version":1,"entries":[]}'))).toEqual({
      version: 1,
      entries: [],
    });
  });
});

describe("playwright cross-check", () => {
  function writeReport(stats: string): string {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-flake-report-"));
    const filePath = path.join(directory, "merged-report.json");
    // Mirrors the JSON reporter's layout: `stats` is the last key, preceded by a
    // `suites` array far larger than the tail we read.
    fs.writeFileSync(
      filePath,
      `{\n  "suites": [\n${'    { "title": "padding" },\n'.repeat(500)}    { "title": "last" }\n  ],\n  "errors": [],\n  "stats": ${stats}\n}`,
    );
    return filePath;
  }

  it("reads stats from the tail of a report larger than the bytes it reads", () => {
    const filePath = writeReport(
      '{\n    "startTime": "2026-08-11T10:00:00.000Z",\n    "duration": 12,\n    "expected": 1990,\n    "skipped": 6,\n    "unexpected": 0,\n    "flaky": 4\n  }',
    );

    expect(fs.statSync(filePath).size).toBeGreaterThan(8192);
    expect(readPlaywrightStats(filePath)).toEqual({
      expected: 1990,
      skipped: 6,
      unexpected: 0,
      flaky: 4,
    });
  });

  it("reports match, mismatch, and unavailable distinctly", () => {
    const stats = { expected: 10, unexpected: 0, flaky: 4, skipped: 0 };

    expect(crossCheckFlakeCount(4, stats)).toEqual({ status: "match", ours: 4, theirs: 4 });
    expect(crossCheckFlakeCount(3, stats)).toEqual({ status: "mismatch", ours: 3, theirs: 4 });
    expect(crossCheckFlakeCount(3, undefined)).toEqual({
      status: "unavailable",
      ours: 3,
      reason: "no merged Playwright JSON report",
    });
    expect(readPlaywrightStats(undefined)).toBeUndefined();
  });
});

describe("flake report markdown", () => {
  const source = {
    runId: "500",
    runUrl: "https://github.com/kdlbs/kandev/actions/runs/500",
    branch: "main",
    sha: "0123456789abcdef",
  };

  it("states the run's flake count, rate, baseline, and the delta against it", () => {
    const current = entryFromSummary(
      summary({ flaky: 5, executed: 2000, ratePerThousand: 2.5 }),
      source,
      "2026-08-11T10:00:00.000Z",
    );
    const history = [
      entry({ runId: "3", flaky: 3, ratePerThousand: 1.5 }),
      entry({ runId: "2", flaky: 2, ratePerThousand: 1 }),
      entry({ runId: "1", flaky: 2, ratePerThousand: 1 }),
    ];

    const report = renderFlakeReport(
      summary({ flaky: 5, executed: 2000, ratePerThousand: 2.5 }),
      current,
      history,
      { status: "match", ours: 5, theirs: 5 },
    );

    expect(report).toContain("- Flaky tests this run: **5** of 2000 executed (**2.5** per 1000)");
    expect(report).toContain("- Baseline (median of last 3 recorded runs): 2 flaky, 1 per 1000");
    expect(
      renderFlakeReport(summary(), current, history.slice(0, 1), {
        status: "match",
        ours: 5,
        theirs: 5,
      }),
    ).toContain("- Baseline (median of last 1 recorded run): 3 flaky, 1.5 per 1000");
    expect(report).toContain("- Change vs baseline: +3 flaky, +1.5 per 1000");
    expect(report).toContain(
      "- Playwright cross-check: **match** (5 flaky in both retry-summary and the merged report)",
    );
  });

  it("calls out a cross-check mismatch instead of quietly trusting our own count", () => {
    const report = renderFlakeReport(summary(), entry(), [], {
      status: "mismatch",
      ours: 3,
      theirs: 4,
    });

    expect(report).toContain(
      "- Playwright cross-check: **MISMATCH** (retry-summary says 3, merged Playwright report says 4)",
    );
  });

  it("attributes the flake to the test and shows how often it flaked recently", () => {
    const report = renderFlakeReport(
      summary(),
      entry({ runId: "500" }),
      [
        entry({ runId: "3", flakyTests: ["chromium::tests/a.spec.ts::a"] }),
        entry({ runId: "2", flakyTests: ["chromium::tests/a.spec.ts::a"] }),
        entry({ runId: "1", flakyTests: ["chromium::tests/b.spec.ts::b"] }),
      ],
      { status: "match", ours: 1, theirs: 1 },
    );

    expect(report).toContain("| `chromium::tests/a.spec.ts::a` | 2 | failed → passed | 2 |");
    expect(report).toContain("### Repeat offenders (last 10 recorded runs)");
    expect(report).toContain("| `chromium::tests/a.spec.ts::a` | 2 |");
    // Flaked in only one of the recorded runs, so it is not a repeat offender.
    expect(report).not.toContain("| `chromium::tests/b.spec.ts::b` | 1 |");
  });

  it("puts this run at the top of the trend table with a linked run label", () => {
    const report = renderFlakeReport(
      summary(),
      entry({ runId: "500", flaky: 5, ratePerThousand: 2.5, executed: 2000 }),
      [entry({ runId: "3", flaky: 3, ratePerThousand: 1.5, executed: 1998 })],
      { status: "unavailable", ours: 5, reason: "no merged Playwright JSON report" },
    );
    const rows = report.split("\n").filter((line) => line.startsWith("| [main #"));

    expect(rows).toEqual([
      "| [main #500](https://github.com/kdlbs/kandev/actions/runs/500) | `abcdef1` | 5 | 2.5 | 2000 |",
      "| [main #3](https://github.com/kdlbs/kandev/actions/runs/3) | `abcdef1` | 3 | 1.5 | 1998 |",
    ]);
  });

  it("says the trend is being seeded when there is no history yet", () => {
    const report = renderFlakeReport(summary(), entry(), [], {
      status: "unavailable",
      ours: 3,
      reason: "no merged Playwright JSON report",
    });

    expect(report).toContain("- Baseline: none recorded yet; this run seeds the trend.");
    expect(report).not.toContain("Change vs baseline");
  });
});
