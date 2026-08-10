import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import {
  CONTAINER_SHARD_COUNT,
  discoverTestCatalog,
  NORMAL_SHARD_COUNT,
  validateShardManifest,
  type Shard,
  type ShardManifest,
} from "./plan-shards";

function readManifest(filePath: string): ShardManifest {
  if (!fs.existsSync(filePath)) throw new Error(`Shard manifest does not exist: ${filePath}`);
  const manifest = JSON.parse(fs.readFileSync(filePath, "utf8")) as ShardManifest;
  if (
    manifest.version !== 1 ||
    !Array.isArray(manifest.shards) ||
    (manifest.cohort !== "normal" && manifest.cohort !== "containers")
  ) {
    throw new Error(`Invalid shard manifest: ${filePath}`);
  }
  const expectedShardCount =
    manifest.cohort === "containers" ? CONTAINER_SHARD_COUNT : NORMAL_SHARD_COUNT;
  if (manifest.shardCount !== expectedShardCount) {
    throw new Error(
      `Invalid ${manifest.cohort} shard count: expected ${expectedShardCount}, got ${manifest.shardCount}`,
    );
  }
  validateShardManifest(manifest, discoverTestCatalog(process.cwd(), manifest.cohort));
  return manifest;
}

function resolveShard(manifest: ShardManifest): Shard {
  const requested = Number(process.env.E2E_SHARD ?? path.basename(process.cwd()));
  if (Number.isInteger(requested) && requested > 0) {
    const shard = manifest.shards.find((candidate) => candidate.index === requested);
    if (shard) return shard;
  }
  if (manifest.shards.length === 1) return manifest.shards[0]!;
  throw new Error("E2E_SHARD must identify one shard in a multi-shard manifest");
}

function writeShardMetadata(
  manifest: ShardManifest,
  shard: Shard,
  startedAt: string,
  exitCode: number,
) {
  const outputDir = path.resolve("e2e/blob-report");
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
  };
  fs.writeFileSync(
    path.join(outputDir, `shard-${manifest.cohort}-${shard.index}-metadata.json`),
    `${JSON.stringify(metadata, null, 2)}\n`,
  );
}

function run(): void {
  const manifestPath = process.argv[process.argv.indexOf("--manifest") + 1] ?? process.argv[2];
  if (!manifestPath) throw new Error("Usage: run-planned-shard.ts --manifest <manifest.json>");
  const manifest = readManifest(path.resolve(manifestPath));
  const shard = resolveShard(manifest);
  const startedAt = new Date().toISOString();

  if (shard.files.length === 0) {
    writeShardMetadata(manifest, shard, startedAt, 0);
    console.log(`planned shard ${shard.index}: no tests assigned`);
    return;
  }

  const args = [
    "playwright",
    "test",
    "--config",
    "e2e/playwright.config.ts",
    ...manifest.projects.map((project) => `--project=${project}`),
    ...shard.files,
  ];
  const result = spawnSync("pnpm", ["exec", ...args], {
    stdio: "inherit",
    env: { ...process.env, E2E_PLANNED_SHARD: String(shard.index) },
  });
  const exitCode = result.status ?? 1;
  writeShardMetadata(manifest, shard, startedAt, exitCode);
  process.exitCode = exitCode;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  run();
}
