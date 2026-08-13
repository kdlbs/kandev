import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { computeOutcome, summarizeObservations, type TestOutcome } from "./retry-summary";
import { parseBlobReports, type ShardMetadata } from "./e2e-timings";
import type { TimingObservation } from "./e2e-timings";

const parityFixtureDir = path.join(
  path.dirname(fileURLToPath(import.meta.url)),
  "__fixtures__/playwright-parity",
);

function observation(overrides: Partial<TimingObservation>): TimingObservation {
  return {
    key: "chromium::tests/example.spec.ts::works",
    project: "chromium",
    file: "tests/example.spec.ts",
    title: "works",
    retry: 0,
    status: "passed",
    durationSeconds: 1,
    errors: [],
    attachments: [],
    ...overrides,
  };
}

describe("retry summary", () => {
  it("distinguishes first-attempt passes, passed retries, and failures", () => {
    const summary = summarizeObservations(
      [
        observation({}),
        observation({
          key: "chromium::tests/flaky.spec.ts::flaky",
          title: "flaky",
          status: "failed",
        }),
        observation({
          key: "chromium::tests/flaky.spec.ts::flaky",
          title: "flaky",
          retry: 1,
          status: "passed",
          durationSeconds: 2,
          errors: [{ message: "temporary failure" }],
        }),
        observation({
          key: "chromium::tests/slow.spec.ts::slow",
          title: "slow",
          status: "timedOut",
          errors: [{ message: "Timeout 60000ms exceeded" }],
        }),
        observation({
          key: "chromium::tests/terminal.spec.ts::terminal",
          title: "terminal",
          status: "failed",
          errors: [{ message: "terminal failure" }],
          attachments: [{ name: "trace", path: "resources/trace.zip" }],
        }),
      ],
      "2026-08-10T10:02:00.000Z",
      {
        attachmentArtifact: {
          name: "e2e-timing-diagnostics",
          url: "https://github.com/kdlbs/kandev/actions/runs/42#artifacts",
          pathPrefix: "retry-artifacts",
          availablePaths: new Set(["resources/trace.zip"]),
        },
      },
    );

    expect(summary.counts).toEqual({
      passedFirstAttempt: 1,
      passedAfterRetry: 1,
      failed: 1,
      timedOut: 1,
      skipped: 0,
    });
    expect(summary.flake).toEqual({
      flaky: 1,
      unexpected: 2,
      executed: 4,
      ratePerThousand: 250,
      flakyTests: ["chromium::tests/flaky.spec.ts::flaky"],
    });
    expect(summary.tests).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ key: "chromium::tests/flaky.spec.ts::flaky", attempts: 2 }),
        expect.objectContaining({
          key: "chromium::tests/slow.spec.ts::slow",
          finalStatus: "timedOut",
        }),
        expect.objectContaining({
          key: "chromium::tests/terminal.spec.ts::terminal",
          finalStatus: "failed",
          errorCategory: "failure",
          attachments: [
            {
              name: "trace",
              path: "retry-artifacts/trace.zip",
              artifactName: "e2e-timing-diagnostics",
              artifactUrl: "https://github.com/kdlbs/kandev/actions/runs/42#artifacts",
            },
          ],
        }),
      ]),
    );
  });

  it("matches Playwright's outcome algorithm, including the cases a retry count gets wrong", () => {
    // A retried-then-passed test is the flake we want to count.
    expect(computeOutcome(["failed", "passed"])).toBe("flaky");
    expect(computeOutcome(["timedOut", "timedOut", "passed"])).toBe("flaky");
    // Playwright never counts an interrupted attempt as unexpected, so an
    // interrupted-then-passed test is expected, not flaky.
    expect(computeOutcome(["interrupted", "passed"])).toBe("expected");
    expect(computeOutcome(["passed"])).toBe("expected");
    expect(computeOutcome(["failed", "failed", "failed"])).toBe("unexpected");
    // A did-not-run attempt of a test expected to pass is neither expected nor
    // unexpected, so failing and then skipping stays unexpected.
    expect(computeOutcome(["failed", "skipped"])).toBe("unexpected");
    expect(computeOutcome(["skipped"])).toBe("skipped");
    expect(computeOutcome(["interrupted"])).toBe("skipped");
    // `test.fail()` inverts the expectation: failing is expected, passing is not.
    expect(computeOutcome(["failed"], "failed")).toBe("expected");
    expect(computeOutcome(["passed"], "failed")).toBe("unexpected");
    expect(computeOutcome(["passed", "failed"], "failed")).toBe("flaky");
  });

  it("reads the expected status recorded on each attempt", () => {
    const summary = summarizeObservations(
      [
        observation({
          key: "chromium::tests/expected-failure.spec.ts::fails on purpose",
          status: "failed",
          expectedStatus: "failed",
        }),
      ],
      "2026-08-10T10:02:00.000Z",
    );

    expect(summary.tests[0]?.outcome).toBe("expected");
    expect(summary.flake).toMatchObject({ flaky: 0, unexpected: 0, executed: 1 });
  });

  it("reports the flake rate per thousand executed tests, ignoring skipped ones", () => {
    const observations = Array.from({ length: 8 }, (_, index) =>
      observation({ key: `chromium::tests/bulk.spec.ts::case ${index}`, title: `case ${index}` }),
    );
    observations.push(
      observation({ key: "chromium::tests/skipped.spec.ts::off", status: "skipped" }),
      observation({ key: "chromium::tests/flaky.spec.ts::flaky", status: "failed" }),
      observation({ key: "chromium::tests/flaky.spec.ts::flaky", retry: 1, status: "passed" }),
    );

    const summary = summarizeObservations(observations, "2026-08-10T10:02:00.000Z");

    expect(summary.flake.executed).toBe(9);
    expect(summary.flake.ratePerThousand).toBe(111.11);
  });

  it("ignores a blob report that was parsed twice instead of inventing retries", () => {
    const attempt = observation({ key: "chromium::tests/a.spec.ts::a", status: "passed" });

    const summary = summarizeObservations([attempt, { ...attempt }], "2026-08-10T10:02:00.000Z");

    expect(summary.tests[0]?.attempts).toBe(1);
    expect(summary.tests[0]?.outcome).toBe("expected");
    expect(summary.flake).toMatchObject({ flaky: 0, executed: 1 });
  });

  // The whole point of the flake rate is that it is Playwright's number. This
  // parses a real blob report and asserts our totals against the `stats` block
  // Playwright itself produced from that same blob; see the fixture's README.
  it("reproduces Playwright's own per-outcome totals for a recorded run", () => {
    const summary = summarizeObservations(
      parseBlobReports(parityFixtureDir),
      "2026-08-13T10:00:00.000Z",
    );
    const tally = (outcome: TestOutcome) =>
      summary.tests.filter((test) => test.outcome === outcome).length;

    const playwrightStats = JSON.parse(
      fs.readFileSync(path.join(parityFixtureDir, "playwright-stats.json"), "utf8"),
    ) as Record<string, number>;

    expect({
      expected: tally("expected"),
      unexpected: tally("unexpected"),
      flaky: tally("flaky"),
      skipped: tally("skipped"),
    }).toEqual(playwrightStats);
    expect(summary.flake.flaky).toBe(playwrightStats.flaky);
    expect(summary.flake.flakyTests).toEqual([
      "chromium::tests/outcomes.spec.ts::flakes once then passes",
      "chromium::tests/outcomes.spec.ts::flakes twice then passes",
    ]);

    // Totals alone are not enough: misreading `expectedStatus` swaps the two
    // `test.fail()` tests between expected and unexpected and leaves every
    // total identical, so pin each test's verdict as well.
    expect(Object.fromEntries(summary.tests.map((test) => [test.title, test.outcome]))).toEqual({
      "always passes": "expected",
      "flakes once then passes": "flaky",
      "flakes twice then passes": "flaky",
      "always fails": "unexpected",
      "is skipped": "skipped",
      "is expected to fail and does": "expected",
      "is expected to fail but passes": "unexpected",
    });
  });

  it("compares planned and actual shard durations and keeps plan health counters", () => {
    const metadata: ShardMetadata[] = [
      {
        cohort: "normal",
        shard: 1,
        shardCount: 2,
        startedAt: "2026-08-10T10:00:00.000Z",
        finishedAt: "2026-08-10T10:02:00.000Z",
        exitCode: 0,
        predictedSeconds: 90,
        unitIds: ["chromium::tests/example.spec.ts"],
      },
    ];

    const summary = summarizeObservations([], "2026-08-10T10:02:00.000Z", {
      shardMetadata: metadata,
      planSummaries: [
        {
          profileMode: "count-fallback",
          unknownUnits: 3,
          warmUnits: 1,
          staleUnits: 2,
          targetSeconds: 120,
        },
      ],
    });

    expect(summary.planning).toEqual({
      mode: "count-fallback",
      unknownUnits: 3,
      warmUnits: 1,
      staleUnits: 2,
      targetSeconds: 120,
    });
    expect(summary.shards).toEqual([
      expect.objectContaining({
        cohort: "normal",
        shard: 1,
        predictedSeconds: 90,
        actualSeconds: 120,
        deltaSeconds: 30,
        exitCode: 0,
      }),
    ]);
  });
});
