import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { describe, expect, it } from "vitest";
import {
  buildTimingProfile,
  collectEligibleSamples,
  hasCompleteShardSet,
  normalizeReportPath,
  parseBlobReportJsonl,
  quantile,
  readTimingProfile,
  type ShardMetadata,
  type TimingObservation,
  type TimingProfile,
} from "./e2e-timings";

function event(method: string, params: Record<string, unknown>): string {
  return JSON.stringify({ method, params });
}

function reportJsonl(): string {
  const project = {
    name: "chromium",
    suites: [
      {
        title: "",
        location: { file: "tests/chat/example.spec.ts", line: 1, column: 1 },
        entries: [
          {
            testId: "test-1",
            title: "shows the conversation",
            location: { file: "tests/chat/example.spec.ts", line: 10, column: 1 },
          },
        ],
      },
    ],
  };

  return [
    event("onBlobReportMetadata", { version: 2 }),
    event("onProject", { project }),
    event("onTestBegin", {
      testId: "test-1",
      result: { id: "result-1", retry: 0, workerIndex: 0, parallelIndex: 0, startTime: 1 },
    }),
    event("onTestEnd", {
      test: { testId: "test-1", expectedStatus: "passed", timeout: 60_000 },
      result: { id: "result-1", duration: 1200, status: "passed", errors: [] },
    }),
    event("onTestBegin", {
      testId: "test-1",
      result: { id: "result-2", retry: 1, workerIndex: 0, parallelIndex: 0, startTime: 2 },
    }),
    event("onTestEnd", {
      test: { testId: "test-1", expectedStatus: "passed", timeout: 60_000 },
      result: { id: "result-2", duration: 6000, status: "passed", errors: [] },
    }),
  ].join("\n");
}

describe("e2e timing collection", () => {
  it("normalizes paths relative to Playwright's e2e test root", () => {
    expect(normalizeReportPath("task/example.spec.ts")).toBe("tests/task/example.spec.ts");
    expect(normalizeReportPath("e2e/tests/task/example.spec.ts")).toBe(
      "tests/task/example.spec.ts",
    );
  });

  it("parses project, file, title, retry, and duration from blob events", () => {
    const observations = parseBlobReportJsonl(reportJsonl());

    expect(observations).toHaveLength(2);
    expect(observations[0]).toMatchObject({
      project: "chromium",
      file: "tests/chat/example.spec.ts",
      title: "shows the conversation",
      retry: 0,
      status: "passed",
      durationSeconds: 1.2,
    });
    expect(observations[1]?.retry).toBe(1);
  });

  it("uses only first-attempt passing results as baseline samples", () => {
    const observations = parseBlobReportJsonl(reportJsonl());

    expect(collectEligibleSamples(observations)).toEqual([
      expect.objectContaining({ durationSeconds: 1.2, retry: 0 }),
    ]);
  });

  it("computes interpolated quantiles", () => {
    expect(quantile([1, 2, 3, 4], 0.5)).toBe(2.5);
    expect(quantile([1, 2, 3, 4], 0.75)).toBe(3.25);
    expect(quantile([], 0.75)).toBe(0);
  });

  it.each([
    ["missing percentile", { p75Seconds: undefined }],
    ["non-numeric percentile", { p75Seconds: "slow" }],
    ["non-finite sample", { samples: [{ durationSeconds: null }] }],
  ])("rejects a malformed timing profile with %s", (_name, override) => {
    const webRoot = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-profile-"));
    const filePath = path.join(webRoot, "timing-profile.json");
    const entry = { ...previousTimingEntry(), ...override };
    fs.writeFileSync(filePath, JSON.stringify({ ...previousTimingProfile(), entries: [entry] }));

    expect(() => readTimingProfile(filePath)).toThrow(/Unsupported timing profile version/);
  });

  it("only treats a complete normal and container shard set as a deletion catalog", () => {
    const metadata = (cohort: string, shard: number, shardCount: number): ShardMetadata => ({
      cohort,
      shard,
      shardCount,
      startedAt: "2026-08-10T10:00:00.000Z",
      finishedAt: "2026-08-10T10:00:01.000Z",
      exitCode: 0,
      predictedSeconds: 1,
      unitIds: [],
    });
    const complete = [
      ...Array.from({ length: 14 }, (_, index) => metadata("normal", index + 1, 14)),
      ...Array.from({ length: 6 }, (_, index) => metadata("containers", index + 1, 6)),
    ];

    expect(hasCompleteShardSet(complete)).toBe(true);
    expect(hasCompleteShardSet(complete.slice(0, -1))).toBe(false);
  });

  it("merges and bounds timing history", () => {
    const previous: TimingProfile = {
      version: 1,
      generatedAt: "2026-08-09T00:00:00.000Z",
      source: { branch: "main", sha: "old-sha", runId: "10" },
      entries: [
        {
          key: "chromium::tests/chat/example.spec.ts::shows the conversation",
          project: "chromium",
          file: "tests/chat/example.spec.ts",
          title: "shows the conversation",
          fileHash: "hash-old",
          samples: Array.from({ length: 20 }, (_, index) => ({
            durationSeconds: index + 1,
            sourceSha: "old-sha",
            runId: "10",
            recordedAt: "2026-08-09T00:00:00.000Z",
          })),
          p50Seconds: 10.5,
          p75Seconds: 15.25,
          lastSeenSha: "old-sha",
          lastSeenRunId: "10",
        },
      ],
    };
    const observations: TimingObservation[] = [
      {
        key: "chromium::tests/chat/example.spec.ts::shows the conversation",
        project: "chromium",
        file: "tests/chat/example.spec.ts",
        title: "shows the conversation",
        retry: 0,
        status: "passed",
        durationSeconds: 100,
        errors: [],
        attachments: [],
      },
    ];

    const profile = buildTimingProfile(previous, observations, {
      branch: "main",
      sha: "new-sha",
      runId: "11",
      generatedAt: "2026-08-10T00:00:00.000Z",
      fileHashes: new Map([["tests/chat/example.spec.ts", "hash-new"]]),
      knownKeys: new Set([observations[0]!.key]),
    });

    expect(profile.entries[0]?.samples).toHaveLength(20);
    expect(profile.entries[0]?.samples.at(-1)?.durationSeconds).toBe(100);
    expect(profile.entries[0]?.fileHash).toBe("hash-new");
    expect(profile.entries[0]?.p75Seconds).toBeGreaterThan(15);
  });

  it("compacts previous entries that are no longer in the current catalog", () => {
    const currentKey = "chromium::tests/chat/example.spec.ts::shows the conversation";
    const staleEntry = {
      ...previousTimingEntry(),
      key: "chromium::tests/removed.spec.ts::removed test",
      file: "tests/removed.spec.ts",
    };
    const profile = buildTimingProfile(
      { ...previousTimingProfile(), entries: [previousTimingEntry(), staleEntry] },
      [
        {
          key: currentKey,
          project: "chromium",
          file: "tests/chat/example.spec.ts",
          title: "shows the conversation",
          retry: 0,
          status: "passed",
          durationSeconds: 2,
          errors: [],
          attachments: [],
        },
      ],
      {
        branch: "main",
        sha: "new-sha",
        runId: "11",
        generatedAt: "2026-08-10T00:00:00.000Z",
        fileHashes: new Map([["tests/chat/example.spec.ts", "hash-new"]]),
        knownKeys: new Set([currentKey]),
      },
    );

    expect(profile.entries.map((entry) => entry.key)).toEqual([currentKey]);
  });
});

function previousTimingEntry(): TimingProfile["entries"][number] {
  return {
    key: "chromium::tests/chat/example.spec.ts::shows the conversation",
    project: "chromium",
    file: "tests/chat/example.spec.ts",
    title: "shows the conversation",
    fileHash: "hash-old",
    samples: [],
    p50Seconds: 1,
    p75Seconds: 1,
    lastSeenSha: "old-sha",
    lastSeenRunId: "10",
  };
}

function previousTimingProfile(): TimingProfile {
  return {
    version: 1,
    generatedAt: "2026-08-09T00:00:00.000Z",
    source: { branch: "main", sha: "old-sha", runId: "10" },
    entries: [previousTimingEntry()],
  };
}
