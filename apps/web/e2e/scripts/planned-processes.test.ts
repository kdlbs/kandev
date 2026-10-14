import { EventEmitter } from "node:events";
import type { ChildProcess, spawn as Spawn } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import { executeSubshards } from "./planned-processes";
import type { ShardManifest } from "./plan-shards";

const directories: string[] = [];
afterEach(() => {
  for (const dir of directories.splice(0)) fs.rmSync(dir, { recursive: true, force: true });
  vi.unstubAllEnvs();
});

function fixture() {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-subshards-"));
  directories.push(dir);
  const units = ["one.spec.ts", "two.spec.ts"].map((file) => ({
    id: `chromium:${file}`,
    file,
    project: "chromium",
    fileHash: "hash",
    testCount: 1,
    estimatedSeconds: 1,
    classification: "profile" as const,
  }));
  const shard = { index: 7, files: units.map((unit) => unit.file), units, predictedSeconds: 2 };
  const manifest = {
    cohort: "normal",
    projects: ["chromium", "mobile-chrome"],
    shardCount: 14,
  } as ShardManifest;
  const children: EventEmitter[] = [];
  const spawn = vi.fn((_command: string, _args: string[], options: { env: NodeJS.ProcessEnv }) => {
    const child = new EventEmitter();
    children.push(child);
    const reportDir = options.env.PLAYWRIGHT_BLOB_OUTPUT_DIR!;
    fs.mkdirSync(reportDir, { recursive: true });
    fs.writeFileSync(path.join(reportDir, "report.zip"), `part-${children.length}`);
    return child as ChildProcess;
  });
  vi.stubEnv("CI", "1");
  return {
    dir,
    shard,
    manifest,
    children,
    spawn,
    start: () =>
      executeSubshards(manifest, shard, 2, {
        spawn: spawn as unknown as typeof Spawn,
        outputDir: dir,
      }),
  };
}

describe("planned shard process supervision", () => {
  it("starts every partition, joins a failed sibling, and preserves both reports", async () => {
    const current = fixture();
    const pending = current.start();
    expect(current.children).toHaveLength(2);
    const [first, second] = current.children;
    const metadataPath = path.join(current.dir, "shard-normal-7-metadata.json");
    first!.emit("close", 7);
    await Promise.resolve();
    expect(fs.existsSync(metadataPath)).toBe(false);
    second!.emit("close", 0);
    expect(await pending).toBe(7);
    expect(JSON.parse(fs.readFileSync(metadataPath, "utf8"))).toMatchObject({
      shard: 7,
      shardCount: 14,
      exitCode: 7,
      unitIds: current.shard.units.map((unit) => unit.id),
    });
    const reports = fs.readdirSync(current.dir).filter((file) => file.endsWith(".zip"));
    expect(reports).toHaveLength(2);
    expect(
      reports.map((file) => fs.readFileSync(path.join(current.dir, file), "utf8")).sort(),
    ).toEqual(["part-1", "part-2"]);
    const calls = current.spawn.mock.calls;
    expect(calls.map((call) => call[2].env.E2E_PORT_OFFSET)).toEqual(["0", "1"]);
    expect(new Set(calls.map((call) => call[2].env.PLAYWRIGHT_BLOB_OUTPUT_DIR)).size).toBe(2);
    for (const call of calls) {
      expect(call[1]).toContain("--workers=1");
      expect(call[1]).toContain("--project=chromium");
      expect(call[1]).toContain("--project=mobile-chrome");
    }
    expect(
      calls.flatMap((call) => call[1].filter((arg) => arg.endsWith(".spec.ts"))).sort(),
    ).toEqual([...current.shard.files].sort());
  });

  it("fails when a nominally successful child omits its blob report", async () => {
    const current = fixture();
    const pending = current.start();
    const missing = current.spawn.mock.calls[0]![2].env.PLAYWRIGHT_BLOB_OUTPUT_DIR!;
    fs.rmSync(missing, { recursive: true });
    for (const child of current.children) child.emit("close", 0);
    expect(await pending).toBe(1);
  });

  it("joins started siblings when another process cannot spawn", async () => {
    const current = fixture();
    current.spawn.mockImplementationOnce(() => {
      throw new Error("spawn unavailable");
    });
    const pending = current.start();
    expect(current.spawn).toHaveBeenCalledTimes(2);
    expect(current.children).toHaveLength(1);
    current.children[0]!.emit("close", 0);
    expect(await pending).toBe(1);
  });

  it("does not finish an errored child until its close event", async () => {
    const current = fixture();
    const pending = current.start();
    current.children[0]!.emit("error", new Error("launch failed"));
    current.children[1]!.emit("close", 0);
    await Promise.resolve();
    expect(fs.existsSync(path.join(current.dir, "shard-normal-7-metadata.json"))).toBe(false);
    current.children[0]!.emit("close", 0);
    expect(await pending).toBe(1);
  });
});
