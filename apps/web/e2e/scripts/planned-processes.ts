import { spawn as spawnProcess, type ChildProcess } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { killProcessGroup } from "../fixtures/process-group";
import type { Shard, ShardManifest } from "./plan-shards";
import { partitionShard } from "./planned-subshards";

export function writeShardMetadata(
  manifest: ShardManifest,
  shard: Shard,
  startedAt: string,
  exitCode: number,
  options: { outputDir?: string; processCount?: number } = {},
) {
  const outputDir = options.outputDir ?? path.resolve("e2e/blob-report");
  fs.mkdirSync(outputDir, { recursive: true });
  const metadata = {
    cohort: manifest.cohort,
    shard: shard.index,
    shardCount: manifest.shardCount,
    startedAt,
    finishedAt: new Date().toISOString(),
    exitCode,
    predictedSeconds: shard.predictedSeconds,
    unitIds: shard.units.map((unit) => unit.id),
    processCount: options.processCount ?? 1,
  };
  fs.writeFileSync(
    path.join(outputDir, `shard-${manifest.cohort}-${shard.index}-metadata.json`),
    `${JSON.stringify(metadata, null, 2)}\n`,
  );
}

export function buildPlaywrightArgs(
  manifest: ShardManifest,
  shard: Shard,
  extraArgs: string[] = [],
): string[] {
  return [
    "playwright",
    "test",
    "--config",
    "e2e/playwright.config.ts",
    "--workers=1",
    ...manifest.projects.map((project) => `--project=${project}`),
    ...shard.files,
    ...extraArgs,
  ];
}

type ExecutionOptions = {
  spawn?: typeof spawnProcess;
  outputDir?: string;
  extraArgs?: string[];
};

function startPartition(
  manifest: ShardManifest,
  parent: Shard,
  part: Shard,
  options: Required<ExecutionOptions> & { cacheRoot: string },
  children: ChildProcess[],
): Promise<number> {
  const { spawn, outputDir, extraArgs } = options;
  return new Promise((resolve) => {
    const name = `${manifest.cohort}-${parent.index}-${part.index}`;
    try {
      const child = spawn(
        "pnpm",
        [
          "exec",
          ...buildPlaywrightArgs(manifest, part, extraArgs),
          `--output=${path.resolve("e2e/test-results", name)}`,
        ],
        {
          stdio: "inherit",
          detached: process.platform !== "win32",
          env: {
            ...process.env,
            E2E_PLANNED_SHARD: String(parent.index),
            E2E_PORT_OFFSET: String(part.index - 1),
            PWTEST_CACHE_DIR: path.join(options.cacheRoot, String(part.index)),
            PLAYWRIGHT_BLOB_OUTPUT_FILE: undefined,
            PLAYWRIGHT_BLOB_OUTPUT_DIR: path.join(outputDir, ".parts", String(part.index)),
            PLAYWRIGHT_BLOB_OUTPUT_NAME: `report-${name}.zip`,
          },
        },
      );
      children.push(child);
      let failed = false;
      child.once("error", (error) => {
        failed = true;
        console.error(error);
      });
      child.once("close", (code) => resolve(failed ? 1 : (code ?? 1)));
    } catch (error) {
      console.error(error);
      resolve(1);
    }
  });
}

function collectReports(
  manifest: ShardManifest,
  parent: Shard,
  parts: Shard[],
  outputDir: string,
): boolean {
  let complete = true;
  for (const part of parts) {
    const directory = path.join(outputDir, ".parts", String(part.index));
    const reports = fs.existsSync(directory)
      ? fs.readdirSync(directory).filter((file) => file.endsWith(".zip"))
      : [];
    if (process.env.CI && reports.length !== 1) {
      console.error(
        `Missing or ambiguous blob report for partition ${part.index}: ${reports.length}`,
      );
      complete = false;
    }
    for (const [index, file] of reports.entries()) {
      fs.renameSync(
        path.join(directory, file),
        path.join(
          outputDir,
          `report-${manifest.cohort}-${parent.index}-${part.index}-${index}.zip`,
        ),
      );
    }
  }
  return complete;
}

export async function executeSubshards(
  manifest: ShardManifest,
  shard: Shard,
  count: number,
  options: ExecutionOptions = {},
): Promise<number> {
  const execution = {
    spawn: options.spawn ?? spawnProcess,
    outputDir: options.outputDir ?? path.resolve("e2e/blob-report"),
    extraArgs: options.extraArgs ?? [],
  };
  const { outputDir } = execution;
  const parts = partitionShard(shard, count);
  const startedAt = new Date().toISOString();
  fs.rmSync(outputDir, { recursive: true, force: true });
  fs.mkdirSync(outputDir, { recursive: true });
  // Playwright's transform cache permits only one runner invocation per directory.
  const cacheRoot = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-e2e-transform-"));
  const children: ChildProcess[] = [];
  const cleanup: Promise<void>[] = [];
  let interrupted = 0;
  const interrupt = (signal: NodeJS.Signals) => {
    if (interrupted) return;
    interrupted = signal === "SIGINT" ? 130 : 143;
    for (const child of children) {
      if (child.exitCode !== null && child.exitCode !== undefined) continue;
      cleanup.push(killProcessGroup(child).catch((error) => console.error(error)));
    }
  };
  process.on("SIGINT", interrupt);
  process.on("SIGTERM", interrupt);
  try {
    const exits = await Promise.all(
      parts.map((part) =>
        startPartition(manifest, shard, part, { ...execution, cacheRoot }, children),
      ),
    );
    const complete = collectReports(manifest, shard, parts, outputDir);
    const exitCode = interrupted || exits.find((code) => code !== 0) || (complete ? 0 : 1);
    writeShardMetadata(manifest, shard, startedAt, exitCode, {
      outputDir,
      processCount: parts.length,
    });
    return exitCode;
  } finally {
    await Promise.all(cleanup);
    process.off("SIGINT", interrupt);
    process.off("SIGTERM", interrupt);
    fs.rmSync(cacheRoot, { recursive: true, force: true });
  }
}
