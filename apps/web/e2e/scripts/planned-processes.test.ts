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

describe("planned runner transform cache ownership", () => {
  it("isolates four concurrent writers and retains their caches until every child closes", async () => {
    const current = fixture(4);
    const inherited = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-inherited-transform-"));
    directories.push(inherited);
    const moduleName = "compiled-module.js";
    fs.writeFileSync(path.join(inherited, moduleName), "ordinary runner");
    vi.stubEnv("PWTEST_CACHE_DIR", inherited);
    const spawn = current.spawn.getMockImplementation()!;
    current.spawn.mockImplementation((command, args, options) => {
      const child = spawn(command, args, options);
      const cache = options.env.PWTEST_CACHE_DIR ?? inherited;
      fs.mkdirSync(cache, { recursive: true });
      fs.writeFileSync(path.join(cache, moduleName), `partition-${current.children.length}`);
      return child;
    });

    const pending = current.start();
    const caches = current.spawn.mock.calls.map((call) => call[2].env.PWTEST_CACHE_DIR);
    expect(caches.every((cache) => cache && path.isAbsolute(cache))).toBe(true);
    expect(new Set(caches).size).toBe(4);
    for (const [index, cache] of caches.entries()) {
      expect(cache).not.toBe(inherited);
      expect(fs.readFileSync(path.join(cache!, moduleName), "utf8")).toBe(`partition-${index + 1}`);
    }
    current.children[0]!.emit("close", 9);
    for (const child of current.children.slice(1, 3)) child.emit("close", 0);
    await Promise.resolve();
    expect(caches.every((cache) => fs.existsSync(cache!))).toBe(true);
    current.children[3]!.emit("close", 0);
    expect(await pending).toBe(9);
    expect(caches.every((cache) => !fs.existsSync(cache!))).toBe(true);
    expect(fs.readFileSync(path.join(inherited, moduleName), "utf8")).toBe("ordinary runner");
    expect(process.env.PWTEST_CACHE_DIR).toBe(inherited);
  });

  it("keeps overlapping shard invocations independent when one cannot spawn a child", async () => {
    const first = fixture();
    const second = fixture();
    first.spawn.mockImplementationOnce(() => {
      throw new Error("spawn unavailable");
    });
    const firstPending = first.start();
    const secondPending = second.start();
    const firstCache = first.spawn.mock.calls[1]![2].env.PWTEST_CACHE_DIR;
    const secondCaches = second.spawn.mock.calls.map((call) => call[2].env.PWTEST_CACHE_DIR);
    expect(firstCache).toBeTruthy();
    expect(secondCaches.every((cache) => cache && cache !== firstCache)).toBe(true);
    expect(new Set(secondCaches).size).toBe(2);
    const firstRoot = path.dirname(firstCache!);
    const secondRoot = path.dirname(secondCaches[0]!);
    expect(firstRoot).not.toBe(secondRoot);
    first.children[0]!.emit("close", 0);
    expect(await firstPending).toBe(1);
    expect(fs.existsSync(firstRoot)).toBe(false);
    expect(fs.existsSync(secondRoot)).toBe(true);
    for (const child of second.children) child.emit("close", 0);
    expect(await secondPending).toBe(0);
    expect(fs.existsSync(secondRoot)).toBe(false);
  });
});

function fixture(count = 2) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-subshards-"));
  directories.push(dir);
  const units = Array.from({ length: count }, (_, index) => `file-${index}.spec.ts`).map(
    (file) => ({
      id: `chromium:${file}`,
      file,
      project: "chromium",
      fileHash: "hash",
      testCount: 1,
      estimatedSeconds: 1,
      classification: "profile" as const,
    }),
  );
  const shard = { index: 7, files: units.map((unit) => unit.file), units, predictedSeconds: count };
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
      executeSubshards(manifest, shard, count, {
        spawn: spawn as unknown as typeof Spawn,
        outputDir: dir,
      }),
  };
}

describe("planned shard process supervision", () => {
  it("starts all four partitions, joins failed siblings, and preserves every report", async () => {
    const current = fixture(4);
    const pending = current.start();
    expect(current.children).toHaveLength(4);
    const [first, second] = current.children;
    const metadataPath = path.join(current.dir, "shard-normal-7-metadata.json");
    first!.emit("close", 7);
    await Promise.resolve();
    expect(fs.existsSync(metadataPath)).toBe(false);
    second!.emit("close", 0);
    current.children[2]!.emit("close", 0);
    await Promise.resolve();
    expect(fs.existsSync(metadataPath)).toBe(false);
    current.children[3]!.emit("close", 0);
    expect(await pending).toBe(7);
    expect(JSON.parse(fs.readFileSync(metadataPath, "utf8"))).toMatchObject({
      shard: 7,
      shardCount: 14,
      exitCode: 7,
      processCount: 4,
      unitIds: current.shard.units.map((unit) => unit.id),
    });
    const reports = fs.readdirSync(current.dir).filter((file) => file.endsWith(".zip"));
    expect(reports).toHaveLength(4);
    expect(
      reports.map((file) => fs.readFileSync(path.join(current.dir, file), "utf8")).sort(),
    ).toEqual(["part-1", "part-2", "part-3", "part-4"]);
    const calls = current.spawn.mock.calls;
    expect(calls.map((call) => call[2].env.E2E_PORT_OFFSET)).toEqual(["0", "1", "2", "3"]);
    expect(new Set(calls.map((call) => call[2].env.PLAYWRIGHT_BLOB_OUTPUT_DIR)).size).toBe(4);
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
    const current = fixture(4);
    const pending = current.start();
    const missing = current.spawn.mock.calls[3]![2].env.PLAYWRIGHT_BLOB_OUTPUT_DIR!;
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
