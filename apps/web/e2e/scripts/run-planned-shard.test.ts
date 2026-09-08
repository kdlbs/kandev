import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { describe, expect, it, vi } from "vitest";
import {
  buildPlaywrightArgs,
  executeShard,
  readManifest,
  resolveShard,
  writeShardMetadata,
  type ShardManifest,
} from "./run-planned-shard";

function manifest(overrides: Partial<ShardManifest> = {}): ShardManifest {
  const shard = {
    index: 1,
    predictedSeconds: 0,
    files: [],
    units: [],
  };
  return {
    version: 1,
    cohort: "normal",
    shardCount: 1,
    generatedAt: "2026-08-10T00:00:00.000Z",
    projects: ["chromium"],
    profile: { mode: "count-fallback", sourceSha: "", sourceRunId: "" },
    catalog: { checksum: "", unitCount: 0 },
    summary: {
      predictedSeconds: 0,
      unknownUnits: 0,
      warmUnits: 0,
      staleUnits: 0,
      targetSeconds: 0,
      dominantUnits: [],
    },
    shards: [shard],
    ...overrides,
  };
}

describe("planned shard runner", () => {
  it("rejects an invalid manifest before catalog validation", () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-runner-"));
    const filePath = path.join(dir, "invalid.json");
    fs.writeFileSync(filePath, JSON.stringify({ version: 999, cohort: "normal", shards: [] }));

    expect(() => readManifest(filePath)).toThrow(/Invalid shard manifest/);
  });

  it("selects the requested shard from E2E_SHARD", () => {
    const selected = resolveShard(
      manifest({
        shardCount: 2,
        shards: [
          { index: 1, predictedSeconds: 1, files: ["tests/one.spec.ts"], units: [] },
          { index: 2, predictedSeconds: 2, files: ["tests/two.spec.ts"], units: [] },
        ],
      }),
      "2",
    );

    expect(selected.index).toBe(2);
  });

  it("writes empty-shard metadata and propagates the child exit code", () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-runner-"));
    const current = manifest({
      shards: [{ index: 1, predictedSeconds: 0, files: [], units: [] }],
    });
    writeShardMetadata(current, current.shards[0]!, "2026-08-10T00:00:00.000Z", 0, dir);
    expect(
      JSON.parse(fs.readFileSync(path.join(dir, "shard-normal-1-metadata.json"), "utf8")),
    ).toMatchObject({
      shard: 1,
      exitCode: 0,
    });

    const populated = manifest({
      projects: ["chromium", "mobile-chrome"],
      shards: [
        {
          index: 1,
          predictedSeconds: 4,
          files: ["tests/example.spec.ts"],
          units: [],
        },
      ],
    });
    const spawn = vi.fn(() => ({ status: 7 })) as never;
    const exitCode = executeShard(populated, populated.shards[0]!, spawn, dir);

    expect(exitCode).toBe(7);
    expect(spawn).toHaveBeenCalledWith(
      "pnpm",
      [
        "exec",
        "playwright",
        "test",
        "--config",
        "e2e/playwright.config.ts",
        "--workers=1",
        "--project=chromium",
        "--project=mobile-chrome",
        "tests/example.spec.ts",
      ],
      expect.objectContaining({
        env: expect.objectContaining({ E2E_PLANNED_SHARD: "1" }),
      }),
    );
  });

  it("builds the runner arguments from the manifest projects and files", () => {
    const current = manifest({
      projects: ["containers"],
      shards: [
        {
          index: 1,
          predictedSeconds: 1,
          files: ["tests/docker/example.spec.ts"],
          units: [],
        },
      ],
    });

    expect(buildPlaywrightArgs(current, current.shards[0]!)).toEqual([
      "playwright",
      "test",
      "--config",
      "e2e/playwright.config.ts",
      "--workers=1",
      "--project=containers",
      "tests/docker/example.spec.ts",
    ]);
  });
});
