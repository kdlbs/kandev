import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { describe, expect, it } from "vitest";
import {
  createShardPlan,
  discoverTestCatalog,
  validateShardManifest,
  type CatalogUnit,
} from "./plan-shards";
import type { TimingProfile } from "./e2e-timings";

const profile: TimingProfile = {
  version: 1,
  generatedAt: "2026-08-10T00:00:00.000Z",
  source: { branch: "main", sha: "main-sha", runId: "12" },
  entries: [
    {
      key: "chromium::tests/a.spec.ts::a",
      project: "chromium",
      file: "tests/a.spec.ts",
      title: "a",
      fileHash: "hash-a",
      samples: [],
      p50Seconds: 8,
      p75Seconds: 10,
      lastSeenSha: "main-sha",
      lastSeenRunId: "12",
    },
    {
      key: "chromium::tests/b.spec.ts::b",
      project: "chromium",
      file: "tests/b.spec.ts",
      title: "b",
      fileHash: "hash-old-b",
      samples: [],
      p50Seconds: 20,
      p75Seconds: 20,
      lastSeenSha: "main-sha",
      lastSeenRunId: "12",
    },
  ],
};

const units: CatalogUnit[] = [
  { project: "chromium", file: "tests/a.spec.ts", fileHash: "hash-a", testCount: 1 },
  { project: "chromium", file: "tests/b.spec.ts", fileHash: "hash-new-b", testCount: 1 },
  { project: "chromium", file: "tests/c.spec.ts", fileHash: "hash-c", testCount: 3 },
  { project: "mobile-chrome", file: "tests/mobile-c.spec.ts", fileHash: "hash-m", testCount: 2 },
];

describe("duration-aware shard planning", () => {
  it("discovers specs below the web e2e test root and hashes their source", () => {
    const webRoot = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-planner-"));
    const testDir = path.join(webRoot, "e2e", "tests", "chat");
    fs.mkdirSync(testDir, { recursive: true });
    fs.symlinkSync(path.resolve("node_modules"), path.join(webRoot, "node_modules"), "dir");
    fs.writeFileSync(
      path.join(webRoot, "e2e", "playwright.config.ts"),
      [
        'import { defineConfig } from "@playwright/test";',
        "export default defineConfig({",
        '  testDir: "./tests",',
        "  projects: [",
        '    { name: "chromium", testIgnore: [/mobile-.*\\.spec\\.ts$/, /tests\\/(docker|ssh)\\//] },',
        '    { name: "mobile-chrome", testMatch: /mobile-.*\\.spec\\.ts$/ },',
        '    { name: "containers", testMatch: /tests\\/(docker|ssh)\\/.*\\.spec\\.ts$/ },',
        "  ],",
        "});",
      ].join("\n"),
    );
    fs.writeFileSync(
      path.join(testDir, "example.spec.ts"),
      [
        'import { test } from "@playwright/test";',
        'test.describe("suite", () => {',
        "  test.beforeEach(async () => {});",
        '  test("works", async () => {});',
        '  test.skip("skipped", async () => {});',
        '  for (const name of ["generated one", "generated two"]) {',
        "    test(name, async () => {});",
        "  }",
        "});",
      ].join("\n"),
    );

    expect(discoverTestCatalog(webRoot, "normal")).toEqual([
      expect.objectContaining({
        project: "chromium",
        file: "tests/chat/example.spec.ts",
        testCount: 4,
        fileHash: expect.stringMatching(/^[a-f0-9]{64}$/),
      }),
    ]);
  });

  it("assigns weighted units with stable longest-processing-time packing", () => {
    const first = createShardPlan(units, {
      cohort: "normal",
      shardCount: 2,
      profile,
      generatedAt: "2026-08-10T00:00:00.000Z",
    });
    const second = createShardPlan(units, {
      cohort: "normal",
      shardCount: 2,
      profile,
      generatedAt: "2026-08-10T00:00:00.000Z",
    });

    expect(first).toEqual(second);
    expect(first.shards).toHaveLength(2);
    expect(first.shards.flatMap((shard) => shard.units)).toHaveLength(units.length);
    expect(first.summary.warmUnits).toBe(1);
    expect(first.summary.unknownUnits).toBe(2);
    expect(first.shards[0]?.predictedSeconds).toBeGreaterThan(0);
  });

  it("validates exact coverage and rejects duplicate assignments", () => {
    const plan = createShardPlan(units, {
      cohort: "containers",
      shardCount: 2,
      generatedAt: "2026-08-10T00:00:00.000Z",
    });
    expect(() => validateShardManifest(plan, units)).not.toThrow();

    const duplicate = structuredClone(plan);
    duplicate.shards[1]?.units.push(plan.shards[0]!.units[0]!);
    expect(() => validateShardManifest(duplicate, units)).toThrow(/duplicate/i);
  });

  it("rejects units whose immutable catalog fields were changed", () => {
    const plan = createShardPlan(units, {
      cohort: "normal",
      shardCount: 2,
      generatedAt: "2026-08-10T00:00:00.000Z",
    });
    const tampered = structuredClone(plan);
    const unit = tampered.shards.find((shard) => shard.units.length > 0)?.units[0];
    expect(unit).toBeDefined();
    unit!.fileHash = "tampered";

    expect(() => validateShardManifest(tampered, units)).toThrow(/catalog fields/i);
  });

  it("rejects duplicate and out-of-range shard indexes", () => {
    const plan = createShardPlan(units, {
      cohort: "normal",
      shardCount: 2,
      generatedAt: "2026-08-10T00:00:00.000Z",
    });
    const duplicate = structuredClone(plan);
    duplicate.shards[1]!.index = duplicate.shards[0]!.index;
    expect(() => validateShardManifest(duplicate, units)).toThrow(/shard index/i);

    const outOfRange = structuredClone(plan);
    outOfRange.shards[0]!.index = plan.shardCount + 1;
    expect(() => validateShardManifest(outOfRange, units)).toThrow(/shard index/i);
  });

  it("uses count-based estimates when no profile exists", () => {
    const plan = createShardPlan(units.slice(0, 2), {
      cohort: "containers",
      shardCount: 1,
      generatedAt: "2026-08-10T00:00:00.000Z",
    });

    expect(plan.profile.mode).toBe("count-fallback");
    expect(plan.summary.unknownUnits).toBe(2);
    expect(plan.shards[0]?.predictedSeconds).toBe(40);
  });

  it("budgets unknown Kubernetes specs for real cluster lifecycle time", () => {
    const plan = createShardPlan(
      [
        {
          project: "containers",
          file: "tests/kubernetes/kubernetes-executor.spec.ts",
          fileHash: "hash-kubernetes",
          testCount: 2,
        },
      ],
      {
        cohort: "containers",
        shardCount: 1,
        generatedAt: "2026-08-10T00:00:00.000Z",
      },
    );

    expect(plan.shards[0]?.predictedSeconds).toBe(360);
  });

  it("uses the Kubernetes fallback for tests missing from a timing profile", () => {
    const kubernetesUnit: CatalogUnit = {
      project: "containers",
      file: "tests/kubernetes/kubernetes-executor.spec.ts",
      fileHash: "hash-kubernetes",
      testCount: 2,
    };
    const kubernetesProfile: TimingProfile = {
      ...profile,
      entries: [
        {
          ...profile.entries[0]!,
          key: "containers::tests/kubernetes/kubernetes-executor.spec.ts::launches",
          project: "containers",
          file: kubernetesUnit.file,
          fileHash: kubernetesUnit.fileHash,
          p50Seconds: 120,
          p75Seconds: 140,
        },
      ],
    };

    const plan = createShardPlan([kubernetesUnit], {
      cohort: "containers",
      shardCount: 1,
      profile: kubernetesProfile,
      generatedAt: "2026-08-10T00:00:00.000Z",
    });

    expect(plan.shards[0]?.predictedSeconds).toBe(320);
    expect(plan.shards[0]?.units[0]?.classification).toBe("unknown");
  });

  it("adds a fallback estimate for new tests in a profiled file", () => {
    const plan = createShardPlan(
      [{ project: "chromium", file: "tests/a.spec.ts", fileHash: "hash-a", testCount: 2 }],
      {
        cohort: "normal",
        shardCount: 1,
        profile,
        generatedAt: "2026-08-10T00:00:00.000Z",
      },
    );

    expect(plan.summary.unknownUnits).toBe(1);
    expect(plan.shards[0]?.units[0]?.classification).toBe("unknown");
    expect(plan.shards[0]?.predictedSeconds).toBe(20);
  });

  it("reports a file-level unit that dominates its shard", () => {
    const plan = createShardPlan(
      [
        { project: "chromium", file: "tests/heavy.spec.ts", fileHash: "heavy", testCount: 100 },
        { project: "chromium", file: "tests/light.spec.ts", fileHash: "light", testCount: 1 },
      ],
      {
        cohort: "normal",
        shardCount: 2,
        generatedAt: "2026-08-10T00:00:00.000Z",
      },
    );

    expect(plan.summary.dominantUnits).toEqual([
      expect.objectContaining({
        id: "chromium::tests/heavy.spec.ts",
        shard: 1,
        shareOfShard: 1,
      }),
    ]);
  });

  it("reports profiled files that are no longer in the catalog", () => {
    const plan = createShardPlan(units.slice(0, 1), {
      cohort: "normal",
      shardCount: 1,
      profile: {
        ...profile,
        entries: [
          ...profile.entries,
          {
            ...profile.entries[0]!,
            key: "chromium::tests/removed.spec.ts::removed",
            file: "tests/removed.spec.ts",
          },
        ],
      },
      generatedAt: "2026-08-10T00:00:00.000Z",
    });

    expect(plan.summary.staleUnits).toBe(2);
  });
});
