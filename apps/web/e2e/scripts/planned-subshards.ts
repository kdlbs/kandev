import type { Cohort, PlannedUnit, Shard } from "./plan-shards";

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
  const projects = new Map<string, Map<string, PlannedUnit[]>>();
  for (const unit of shard.units) {
    const files = projects.get(unit.project) ?? new Map<string, PlannedUnit[]>();
    const units = files.get(unit.file) ?? [];
    units.push(unit);
    files.set(unit.file, units);
    projects.set(unit.project, files);
  }

  const groups = [...projects.entries()]
    .map(([project, files]) => {
      const units = [...files.entries()]
        .map(([file, entries]) => ({
          file,
          units: entries.sort((a, b) => a.id.localeCompare(b.id)),
          seconds: entries.reduce((sum, unit) => sum + unit.estimatedSeconds, 0),
        }))
        .sort((a, b) => b.seconds - a.seconds || a.file.localeCompare(b.file));
      return {
        project,
        files: units,
        totalSeconds: units.reduce((sum, file) => sum + file.seconds, 0),
        partCount: 1,
      };
    })
    .sort((a, b) => a.project.localeCompare(b.project));
  const totalFiles = groups.reduce((sum, project) => sum + project.files.length, 0);
  const targetParts = Math.min(totalFiles, Math.max(count, groups.length));

  while (groups.reduce((sum, project) => sum + project.partCount, 0) < targetParts) {
    const target = groups
      .filter((project) => project.partCount < project.files.length)
      .sort(
        (a, b) =>
          b.totalSeconds / b.partCount - a.totalSeconds / a.partCount ||
          a.project.localeCompare(b.project),
      )[0];
    if (!target) break;
    target.partCount += 1;
  }

  const parts: Shard[] = [];
  for (const project of groups) {
    const projectParts: Shard[] = Array.from({ length: project.partCount }, () => ({
      index: 0,
      files: [],
      units: [],
      predictedSeconds: 0,
    }));
    for (const file of project.files) {
      const target = projectParts.reduce((best, part) =>
        part.predictedSeconds < best.predictedSeconds ? part : best,
      );
      target.files.push(file.file);
      target.units.push(...file.units);
      target.predictedSeconds += file.seconds;
    }
    parts.push(...projectParts);
  }
  parts.forEach((part, index) => {
    part.index = index + 1;
  });
  return parts;
}
