import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import os from "node:os";
import path from "node:path";
import { Writable } from "node:stream";
import { BaseSequencer, startVitest, type TestSpecification, type Vitest } from "vitest/node";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  createTimingSnapshot,
  reconcileShardReports,
} from "../../../.github/scripts/frontend-timings.mjs";
import { FrontendTimingSequencer } from "./vitest-ci-sequencer";
import { FrontendTimingReporter } from "./vitest-ci-reporter";

const NODE_PROJECT = "node";
const BROWSER_PROJECT = "browser";
const BROWSER_LOCALES_PROJECT = "browser-locales";
const SOURCE_SHA = "b".repeat(40);
const COMPATIBILITY_HASH = "a".repeat(64);
const RUN_ID = "12345";
const ATTEMPT = "1";
afterEach(() => vi.unstubAllEnvs());

async function withFixture(callback: (root: string) => Promise<void> | void) {
  const root = mkdtempSync(path.join(os.tmpdir(), "kandev-vite-ci-sequencer-"));
  try {
    for (const file of ["a.test.ts", "b.test.ts", "c.test.ts", "d.test.ts"]) {
      writeFileSync(path.join(root, file), `export const ${file[0]} = true;\n`);
    }
    await callback(root);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

function specifications(root: string): TestSpecification[] {
  const projectFiles = [
    ["a.test.ts", NODE_PROJECT],
    ["b.test.ts", BROWSER_PROJECT],
    ["c.test.ts", BROWSER_LOCALES_PROJECT],
    ["d.test.ts", NODE_PROJECT],
  ] as const;
  return projectFiles.map(
    ([file, project]) =>
      ({
        moduleId: path.join(root, file),
        project: { name: project },
      }) as TestSpecification,
  );
}

function context(root: string, index: number): Vitest {
  return {
    config: { root, shard: { index, count: 2 } },
    cache: { getFileTestResults: () => undefined, getFileStats: () => undefined },
  } as unknown as Vitest;
}

function installInput(root: string, snapshotText: string, expectedDigest: string) {
  const inputPath = path.join(root, "snapshot.json");
  const assignmentPath = path.join(root, "assignment.json");
  writeFileSync(inputPath, snapshotText);
  vi.stubEnv("CI", "true");
  vi.stubEnv("KANDEV_FRONTEND_TIMINGS_ENABLED", "1");
  vi.stubEnv("FRONTEND_TIMING_INPUT_FILE", inputPath);
  vi.stubEnv("FRONTEND_TIMING_ASSIGNMENT_FILE", assignmentPath);
  vi.stubEnv("FRONTEND_TIMING_SOURCE_SHA", SOURCE_SHA);
  vi.stubEnv("FRONTEND_TIMING_COMPATIBILITY_HASH", COMPATIBILITY_HASH);
  vi.stubEnv("FRONTEND_TIMING_SNAPSHOT_DIGEST", expectedDigest);
  vi.stubEnv("FRONTEND_TIMING_SNAPSHOT_RUN_ID", RUN_ID);
  vi.stubEnv("FRONTEND_TIMING_SNAPSHOT_ATTEMPT", ATTEMPT);
  vi.stubEnv("GITHUB_RUN_ID", RUN_ID);
  vi.stubEnv("GITHUB_RUN_ATTEMPT", ATTEMPT);
  return assignmentPath;
}

function baseSnapshot(overrides: Record<string, unknown> = {}) {
  return createTimingSnapshot({
    profile: null,
    sourceSha: SOURCE_SHA,
    runId: RUN_ID,
    attempt: ATTEMPT,
    compatibilityHash: COMPATIBILITY_HASH,
    ...overrides,
  });
}

async function expectSnapshotRejection(
  root: string,
  snapshot: ReturnType<typeof baseSnapshot>,
  message: string,
  digest = snapshot.digest,
) {
  const assignmentPath = installInput(root, JSON.stringify(snapshot), digest);
  await expect(
    new FrontendTimingSequencer(context(root, 1)).shard(specifications(root)),
  ).rejects.toThrow(message);
  expect(existsSync(assignmentPath)).toBe(false);
}

const WEB_ROOT = path.resolve(import.meta.dirname, "..");

function writeRunnerFixture(root: string): void {
  for (const [project, name, title] of [
    [NODE_PROJECT, "a", "one"],
    [NODE_PROJECT, "d", "four"],
    [BROWSER_PROJECT, "b", "two"],
    ["locales", "c", "three"],
  ]) {
    const directory = path.join(root, project);
    mkdirSync(directory, { recursive: true });
    writeFileSync(
      path.join(directory, `${name}.test.ts`),
      `import { describe, expect, it } from "vitest";\n\ndescribe("${title} suite", () => {\n  it("passes", () => expect(1).toBe(1));\n  it.skip("stays skipped", () => expect(1).toBe(2));\n});\n`,
    );
  }
}

function createRunnerSnapshot() {
  return createTimingSnapshot({
    profile: {
      schema: "kandev.frontend-timing-profile/v1",
      compatibilityHash: COMPATIBILITY_HASH,
      producer: { runId: "12300", attempt: "1" },
      entries: [{ identity: "node:a.test.ts", contentHash: "f".repeat(64), durationMs: 10 }],
    },
    sourceSha: SOURCE_SHA,
    runId: RUN_ID,
    attempt: ATTEMPT,
    compatibilityHash: COMPATIBILITY_HASH,
  });
}

function setRunnerEnvironment(
  root: string,
  snapshot: ReturnType<typeof createRunnerSnapshot>,
): void {
  vi.stubEnv("CI", "true");
  vi.stubEnv("KANDEV_FRONTEND_TIMINGS_ENABLED", "1");
  vi.stubEnv("FRONTEND_TIMING_INPUT_FILE", path.join(root, "snapshot.json"));
  vi.stubEnv("FRONTEND_TIMING_SOURCE_SHA", SOURCE_SHA);
  vi.stubEnv("FRONTEND_TIMING_COMPATIBILITY_HASH", COMPATIBILITY_HASH);
  vi.stubEnv("FRONTEND_TIMING_SNAPSHOT_DIGEST", snapshot.digest);
  vi.stubEnv("FRONTEND_TIMING_SNAPSHOT_RUN_ID", RUN_ID);
  vi.stubEnv("FRONTEND_TIMING_SNAPSHOT_ATTEMPT", ATTEMPT);
  vi.stubEnv("GITHUB_RUN_ID", RUN_ID);
}

async function runFixtureShard(root: string, shard: number) {
  vi.stubEnv("GITHUB_RUN_ATTEMPT", String(shard));
  const assignmentPath = path.join(root, `assignment-${shard}.json`);
  const timingPath = path.join(root, `frontend-timing-${shard}.json`);
  vi.stubEnv("FRONTEND_TIMING_ASSIGNMENT_FILE", assignmentPath);
  vi.stubEnv("FRONTEND_TIMING_REPORT_FILE", timingPath);
  const output = new Writable({
    write(_chunk, _encoding, callback) {
      callback();
    },
  });
  const vitest = await startVitest(
    "test",
    [],
    { run: true, root, config: false, shard: `${shard}/2`, silent: true },
    {
      test: {
        projects: [
          {
            extends: true,
            test: { name: NODE_PROJECT, include: ["node/**/*.test.ts"], environment: "node" },
          },
          {
            extends: true,
            test: {
              name: BROWSER_PROJECT,
              include: ["browser/**/*.test.ts"],
              environment: "happy-dom",
            },
          },
          {
            extends: true,
            test: {
              name: BROWSER_LOCALES_PROJECT,
              include: ["locales/**/*.test.ts"],
              environment: "happy-dom",
            },
          },
        ],
        pool: "threads",
        maxWorkers: 1,
        sequence: { sequencer: FrontendTimingSequencer },
        reporters: ["json", new FrontendTimingReporter()],
        outputFile: { json: path.join(root, "native.json") },
        passWithNoTests: false,
      },
    },
    { stdout: output, stderr: output },
  );
  expect(vitest.state.getCountOfFailedTests()).toBe(0);
  return {
    timing: JSON.parse(readFileSync(timingPath, "utf8")),
    native: JSON.parse(readFileSync(path.join(root, "native.json"), "utf8")),
  };
}

describe("FrontendTimingSequencer native behavior", () => {
  it("keeps native Vitest sharding when timing is disabled", async () => {
    vi.stubEnv("CI", "false");
    vi.stubEnv("KANDEV_FRONTEND_TIMINGS_ENABLED", "0");
    await withFixture(async (root) => {
      const files = specifications(root);
      const ctx = context(root, 1);
      const native = await new BaseSequencer(ctx).shard([...files]);
      const actual = await new FrontendTimingSequencer(ctx).shard([...files]);
      expect(actual.map((file) => file.moduleId)).toEqual(native.map((file) => file.moduleId));
    });
  });

  it("keeps unsharded runs native when timing metadata is absent", async () => {
    await withFixture(async (root) => {
      const files = specifications(root);
      const ctx = context(root, 1) as {
        config: { root: string; shard?: { index: number; count: number } };
      };
      delete ctx.config.shard;
      vi.stubEnv("CI", "true");
      vi.stubEnv("KANDEV_FRONTEND_TIMINGS_ENABLED", "1");
      vi.stubEnv("FRONTEND_TIMING_INPUT_FILE", "");

      const actual = await new FrontendTimingSequencer(ctx as Vitest).shard([...files]);
      expect(actual.map((file) => file.moduleId)).toEqual(files.map((file) => file.moduleId));
    });
  });
});

describe("FrontendTimingSequencer count fallback", () => {
  it("uses Vitest native SHA-1 placement for a valid fallback snapshot across all projects", async () => {
    await withFixture(async (root) => {
      const snapshot = createTimingSnapshot({
        profile: null,
        sourceSha: SOURCE_SHA,
        runId: RUN_ID,
        attempt: ATTEMPT,
        compatibilityHash: COMPATIBILITY_HASH,
      });
      const assignmentPath = installInput(root, JSON.stringify(snapshot), snapshot.digest);
      const files = specifications(root);
      const nativeSelections = await Promise.all(
        [1, 2].map((index) => new BaseSequencer(context(root, index)).shard([...files])),
      );
      const actualSelections: TestSpecification[][] = [];
      const assignments: Array<{
        mode: string;
        planDigest: string;
        assigned: Array<{ identity: string; estimateSource: string; shard: number }>;
      }> = [];
      for (const index of [1, 2]) {
        actualSelections.push(
          await new FrontendTimingSequencer(context(root, index)).shard([...files]),
        );
        assignments.push(JSON.parse(readFileSync(assignmentPath, "utf8")));
      }

      expect(new Set(files.map((file) => file.project.name))).toEqual(
        new Set([NODE_PROJECT, BROWSER_PROJECT, BROWSER_LOCALES_PROJECT]),
      );
      for (const index of [0, 1]) {
        expect(actualSelections[index].map((file) => file.moduleId)).toEqual(
          nativeSelections[index].map((file) => file.moduleId),
        );
        expect(assignments[index].mode).toBe("fallback");
        expect(
          assignments[index].assigned.every((entry) => entry.estimateSource === "native"),
        ).toBe(true);
      }
      expect(assignments[0].planDigest).toBe(assignments[1].planDigest);
      const assigned = assignments.flatMap((assignment) => assignment.assigned);
      expect(new Set(assigned.map((entry) => entry.identity)).size).toBe(files.length);
      expect(assigned.map((entry) => entry.identity).sort()).toEqual(
        files.map((file) => `${file.project.name}:${path.basename(file.moduleId)}`).sort(),
      );
      expect(assigned.map((entry) => entry.shard).sort()).toEqual([1, 1, 2, 2]);
    });
  });
});

describe("FrontendTimingSequencer required snapshot", () => {
  it("fails before writing an assignment when a required snapshot is malformed or unreadable", async () => {
    await withFixture(async (root) => {
      const assignmentPath = installInput(root, "{broken", "d".repeat(64));
      await expect(
        new FrontendTimingSequencer(context(root, 1)).shard(specifications(root)),
      ).rejects.toThrow("invalid snapshot JSON");
      expect(existsSync(assignmentPath)).toBe(false);

      vi.stubEnv("FRONTEND_TIMING_INPUT_FILE", path.join(root, "missing-snapshot.json"));
      await expect(
        new FrontendTimingSequencer(context(root, 1)).shard(specifications(root)),
      ).rejects.toThrow("Cannot read required frontend timing snapshot");
      expect(existsSync(assignmentPath)).toBe(false);
    });
  });

  it("fails before writing an assignment when the snapshot digest or provenance is inconsistent", async () => {
    await withFixture(async (root) => {
      const snapshot = baseSnapshot();
      await expectSnapshotRejection(
        root,
        snapshot,
        "does not match the expected digest",
        "d".repeat(64),
      );
      await expectSnapshotRejection(
        root,
        baseSnapshot({ sourceSha: "c".repeat(40) }),
        "foreign snapshot source",
      );
      await expectSnapshotRejection(
        root,
        baseSnapshot({ runId: "54321" }),
        "foreign snapshot producer",
      );
      await expectSnapshotRejection(
        root,
        baseSnapshot({ attempt: "2" }),
        "foreign snapshot attempt",
      );
      await expectSnapshotRejection(
        root,
        baseSnapshot({ compatibilityHash: "e".repeat(64) }),
        "incompatible snapshot",
      );

      const assignmentPath = installInput(root, JSON.stringify(snapshot), snapshot.digest);
      vi.stubEnv("GITHUB_RUN_ID", "54321");
      await expect(
        new FrontendTimingSequencer(context(root, 1)).shard(specifications(root)),
      ).rejects.toThrow("different workflow run");
      expect(existsSync(assignmentPath)).toBe(false);
    });
  });

  it("fails when a required timing environment value is missing", async () => {
    await withFixture(async (root) => {
      const snapshot = createTimingSnapshot({
        profile: null,
        sourceSha: SOURCE_SHA,
        runId: RUN_ID,
        attempt: ATTEMPT,
        compatibilityHash: COMPATIBILITY_HASH,
      });
      const assignmentPath = installInput(root, JSON.stringify(snapshot), snapshot.digest);
      vi.stubEnv("FRONTEND_TIMING_SNAPSHOT_DIGEST", "");

      await expect(
        new FrontendTimingSequencer(context(root, 1)).shard(specifications(root)),
      ).rejects.toThrow("Missing FRONTEND_TIMING_SNAPSHOT_DIGEST");
      expect(existsSync(assignmentPath)).toBe(false);
    });
  });
});

describe("FrontendTimingSequencer runner integration", () => {
  it("preserves native identities and outcomes through the public Vitest runner API", async () => {
    const root = mkdtempSync(path.join(os.tmpdir(), "kandev-frontend-timing-runner-"));
    const previousExitCode = process.exitCode;
    try {
      symlinkSync(path.join(WEB_ROOT, "node_modules"), path.join(root, "node_modules"), "dir");
      writeRunnerFixture(root);
      const snapshot = createRunnerSnapshot();
      writeFileSync(path.join(root, "snapshot.json"), JSON.stringify(snapshot));
      setRunnerEnvironment(root, snapshot);
      const reports = [await runFixtureShard(root, 1), await runFixtureShard(root, 2)];

      const result = reconcileShardReports(reports, {
        sourceSha: SOURCE_SHA,
        runId: RUN_ID,
        snapshotDigest: snapshot.digest,
        snapshotRunId: RUN_ID,
        snapshotAttempt: ATTEMPT,
        webRoot: root,
      });
      expect(result.valid, result.failures.join("; ")).toBe(true);
      const files: string[] = result.files ?? [];
      expect(files).toHaveLength(4);
      expect(new Set(files.map((identity) => identity.split(":")[0]))).toEqual(
        new Set([NODE_PROJECT, BROWSER_PROJECT, BROWSER_LOCALES_PROJECT]),
      );
      expect(result.tests).toBe(8);
      expect(reports.map((report) => report.timing.attempt)).toEqual(["1", "2"]);
      expect(
        reports.every((report) => report.timing.snapshot.profileProducer.runId === "12300"),
      ).toBe(true);
      expect(
        reports.every((report) => report.timing.snapshot.profileProducer.attempt === "1"),
      ).toBe(true);
    } finally {
      process.exitCode = previousExitCode;
      rmSync(root, { recursive: true, force: true });
    }
  });
});
