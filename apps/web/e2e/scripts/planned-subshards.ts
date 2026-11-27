import type { Cohort, Shard } from "./plan-shards";

function checkedCount(count: number): number {
  if (!Number.isInteger(count) || count < 1 || count > 4) {
    throw new Error(`E2E process count must be an integer 1-4, got: ${count}`);
  }
  return count;
}

export function processCount(
  requested: string,
  memoryMaximum: string,
  cpus: number,
  cohort: Cohort,
): number {
  const count = checkedCount(Number(requested));
  const memory = Number(memoryMaximum);
  const memoryLimit = Number.isInteger(memory) && memory >= 1 && memory <= 4 ? memory : 1;
  const cpuLimit = Number.isFinite(cpus) ? Math.max(1, Math.floor(cpus) - (cpus >= 4 ? 0 : 1)) : 1;
  return cohort === "containers" ? 1 : Math.min(count, memoryLimit, cpuLimit);
}

export function partitionShard(shard: Shard, count: number): Shard[] {
  checkedCount(count);
  const files = shard.files
    .map((file) => {
      const units = shard.units
        .filter((unit) => unit.file === file)
        .sort((a, b) => a.id.localeCompare(b.id));
      return { file, units, seconds: units.reduce((sum, unit) => sum + unit.estimatedSeconds, 0) };
    })
    .sort((a, b) => b.seconds - a.seconds || a.file.localeCompare(b.file));
  const parts: Shard[] = Array.from({ length: Math.min(count, files.length) }, (_, index) => ({
    index: index + 1,
    files: [],
    units: [],
    predictedSeconds: 0,
  }));
  for (const file of files) {
    const target = parts.reduce((best, part) =>
      part.predictedSeconds < best.predictedSeconds ? part : best,
    );
    target.files.push(file.file);
    target.units.push(...file.units);
    target.predictedSeconds += file.seconds;
  }
  return parts;
}
