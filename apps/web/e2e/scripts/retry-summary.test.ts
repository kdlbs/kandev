import { describe, expect, it } from "vitest";
import { summarizeObservations } from "./retry-summary";
import type { ShardMetadata } from "./e2e-timings";
import type { TimingObservation } from "./e2e-timings";

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
    const summary = summarizeObservations([
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
        attachments: [{ name: "trace", path: "test-results/trace.zip" }],
      }),
    ]);

    expect(summary.counts).toEqual({
      passedFirstAttempt: 1,
      passedAfterRetry: 1,
      failed: 1,
      timedOut: 1,
      skipped: 0,
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
          attachments: [{ name: "trace", path: "test-results/trace.zip" }],
        }),
      ]),
    );
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
