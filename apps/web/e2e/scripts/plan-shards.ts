import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import {
  hashSourceFile,
  normalizeReportPath,
  readTimingProfile,
  type TimingProfile,
} from "./e2e-timings";

export const SHARD_MANIFEST_VERSION = 1;
export const NORMAL_SHARD_COUNT = 14;
export const CONTAINER_SHARD_COUNT = 6;
export const DEFAULT_NORMAL_TEST_SECONDS = 10;
export const DEFAULT_CONTAINER_TEST_SECONDS = 20;
export const CHANGED_FILE_MULTIPLIER = 1.25;
export const DOMINANT_UNIT_SHARE = 0.5;

export type Cohort = "normal" | "containers";
export type EstimateClassification = "profile" | "warm" | "unknown";

export type CatalogUnit = {
  project: string;
  file: string;
  fileHash: string;
  testCount: number;
};

export type PlannedUnit = CatalogUnit & {
  id: string;
  estimatedSeconds: number;
  classification: EstimateClassification;
};

export type Shard = {
  index: number;
  predictedSeconds: number;
  files: string[];
  units: PlannedUnit[];
};

export type DominantUnitDiagnostic = {
  shard: number;
  id: string;
  estimatedSeconds: number;
  shareOfShard: number;
};

export type ShardManifest = {
  version: typeof SHARD_MANIFEST_VERSION;
  cohort: Cohort;
  shardCount: number;
  generatedAt: string;
  projects: string[];
  profile: {
    mode: "main" | "count-fallback";
    sourceSha: string;
    sourceRunId: string;
  };
  catalog: {
    checksum: string;
    unitCount: number;
  };
  summary: {
    predictedSeconds: number;
    unknownUnits: number;
    warmUnits: number;
    staleUnits: number;
    targetSeconds: number;
    dominantUnits: DominantUnitDiagnostic[];
  };
  shards: Shard[];
};

const SPEC_FILE = /\.spec\.ts$/;

function listFiles(root: string): string[] {
  if (!fs.existsSync(root)) return [];
  return fs.readdirSync(root, { withFileTypes: true }).flatMap((entry) => {
    const child = path.join(root, entry.name);
    return entry.isDirectory() ? listFiles(child) : [child];
  });
}

function relativeTestFile(webRoot: string, filePath: string): string {
  return normalizeReportPath(path.relative(webRoot, filePath));
}

function countTestDeclarations(contents: string): number {
  const matches = contents.match(/\btest(?:\.(?:only|skip|fixme|fail))?\s*\(\s*["'`]/g);
  return Math.max(1, matches?.length ?? 0);
}

function isMobileFile(file: string): boolean {
  return /(^|\/)mobile-[^/]+\.spec\.ts$/.test(file);
}

function isContainerFile(file: string): boolean {
  return /^(tests\/(docker|ssh)\/)/.test(file);
}

function isExcludedFromNormalChromium(file: string): boolean {
  return (
    isMobileFile(file) ||
    isContainerFile(file) ||
    /(^|\/)office-routing-[^/]+\.spec\.ts$/.test(file) ||
    /^tests\/auth\//.test(file)
  );
}

export function projectFilesForCohort(file: string, cohort: Cohort): string[] {
  if (cohort === "containers") return isContainerFile(file) ? ["containers"] : [];
  if (isMobileFile(file)) return ["mobile-chrome"];
  return isExcludedFromNormalChromium(file) ? [] : ["chromium"];
}

export function discoverTestCatalog(webRoot: string, cohort: Cohort): CatalogUnit[] {
  const testsRoot = path.join(webRoot, "e2e", "tests");
  return listFiles(testsRoot)
    .filter((filePath) => SPEC_FILE.test(filePath))
    .flatMap((filePath) => {
      const file = relativeTestFile(webRoot, filePath);
      const projects = projectFilesForCohort(file, cohort);
      if (projects.length === 0) return [];
      const testCount = countTestDeclarations(fs.readFileSync(filePath, "utf8"));
      const fileHash = hashSourceFile(webRoot, file);
      return projects.map((project) => ({ project, file, fileHash, testCount }));
    })
    .sort((left, right) =>
      `${left.project}::${left.file}`.localeCompare(`${right.project}::${right.file}`),
    );
}

function unitId(unit: Pick<CatalogUnit, "project" | "file">): string {
  return `${unit.project}::${normalizeReportPath(unit.file)}`;
}

function catalogChecksum(units: CatalogUnit[]): string {
  const value = units
    .map((unit) => `${unitId(unit)}:${unit.fileHash}:${unit.testCount}`)
    .sort()
    .join("\n");
  return createHash("sha256").update(value).digest("hex");
}

function fallbackSeconds(cohort: Cohort): number {
  return cohort === "containers" ? DEFAULT_CONTAINER_TEST_SECONDS : DEFAULT_NORMAL_TEST_SECONDS;
}

function countStaleProfileUnits(profile: TimingProfile | undefined, units: CatalogUnit[]): number {
  if (!profile) return 0;
  const currentUnits = new Set(units.map((unit) => unitId(unit)));
  const projects = new Set(units.map((unit) => unit.project));
  const profileUnits = new Set(
    profile.entries
      .filter((entry) => projects.has(entry.project))
      .map((entry) => unitId({ project: entry.project, file: entry.file })),
  );
  return [...profileUnits].filter((id) => !currentUnits.has(id)).length;
}

function estimateClassification(missingTestCount: number, warm: boolean): EstimateClassification {
  if (missingTestCount > 0) return "unknown";
  return warm ? "warm" : "profile";
}

function estimateUnit(
  unit: CatalogUnit,
  cohort: Cohort,
  profile: TimingProfile | undefined,
  changedFileMultiplier: number,
): PlannedUnit {
  const id = unitId(unit);
  const entries = profile?.entries.filter(
    (entry) => entry.project === unit.project && normalizeReportPath(entry.file) === unit.file,
  );
  if (!entries || entries.length === 0) {
    return {
      ...unit,
      id,
      estimatedSeconds: unit.testCount * fallbackSeconds(cohort),
      classification: "unknown",
    };
  }

  const missingTestCount = Math.max(0, unit.testCount - entries.length);
  const warm = entries.some((entry) => entry.fileHash !== unit.fileHash);
  const estimate =
    entries.reduce((total, entry) => total + Math.max(0, entry.p75Seconds), 0) +
    missingTestCount * fallbackSeconds(cohort);
  return {
    ...unit,
    id,
    estimatedSeconds: estimate * (warm ? changedFileMultiplier : 1),
    classification: estimateClassification(missingTestCount, warm),
  };
}

export function createShardPlan(
  units: CatalogUnit[],
  options: {
    cohort: Cohort;
    shardCount: number;
    profile?: TimingProfile;
    generatedAt: string;
    changedFileMultiplier?: number;
  },
): ShardManifest {
  if (!Number.isInteger(options.shardCount) || options.shardCount < 1) {
    throw new Error(`shardCount must be a positive integer, got ${options.shardCount}`);
  }
  const plannedUnits = units.map((unit) =>
    estimateUnit(
      unit,
      options.cohort,
      options.profile,
      options.changedFileMultiplier ?? CHANGED_FILE_MULTIPLIER,
    ),
  );
  const shards: Shard[] = Array.from({ length: options.shardCount }, (_, index) => ({
    index: index + 1,
    predictedSeconds: 0,
    files: [],
    units: [],
  }));

  const sortedUnits = [...plannedUnits].sort(
    (left, right) =>
      right.estimatedSeconds - left.estimatedSeconds || left.id.localeCompare(right.id),
  );
  for (const unit of sortedUnits) {
    const shard = shards.reduce((lightest, candidate) => {
      if (candidate.predictedSeconds < lightest.predictedSeconds) return candidate;
      return candidate.index < lightest.index ? candidate : lightest;
    }, shards[0]!);
    shard.units.push(unit);
    shard.predictedSeconds += unit.estimatedSeconds;
    if (!shard.files.includes(unit.file)) shard.files.push(unit.file);
  }
  for (const shard of shards) {
    shard.units.sort((left, right) => left.id.localeCompare(right.id));
    shard.files.sort();
  }

  const predictedSeconds = shards.reduce((total, shard) => total + shard.predictedSeconds, 0);
  const targetSeconds = options.shardCount > 0 ? predictedSeconds / options.shardCount : 0;
  const staleUnits = countStaleProfileUnits(options.profile, units);
  const dominantUnits = shards.flatMap((shard) => {
    const dominant = shard.units.reduce<PlannedUnit | undefined>(
      (current, unit) =>
        !current || unit.estimatedSeconds > current.estimatedSeconds ? unit : current,
      undefined,
    );
    if (!dominant || shard.predictedSeconds <= 0) return [];
    const shareOfShard = dominant.estimatedSeconds / shard.predictedSeconds;
    const isMaterialOutlier =
      dominant.estimatedSeconds > targetSeconds ||
      (shareOfShard >= DOMINANT_UNIT_SHARE &&
        dominant.estimatedSeconds >= targetSeconds * DOMINANT_UNIT_SHARE);
    if (!isMaterialOutlier) {
      return [];
    }
    return [
      {
        shard: shard.index,
        id: dominant.id,
        estimatedSeconds: dominant.estimatedSeconds,
        shareOfShard,
      },
    ];
  });
  return {
    version: SHARD_MANIFEST_VERSION,
    cohort: options.cohort,
    shardCount: options.shardCount,
    generatedAt: options.generatedAt,
    projects: [...new Set(units.map((unit) => unit.project))].sort(),
    profile: {
      mode: options.profile ? "main" : "count-fallback",
      sourceSha: options.profile?.source.sha ?? "",
      sourceRunId: options.profile?.source.runId ?? "",
    },
    catalog: { checksum: catalogChecksum(units), unitCount: units.length },
    summary: {
      predictedSeconds,
      unknownUnits: plannedUnits.filter((unit) => unit.classification === "unknown").length,
      warmUnits: plannedUnits.filter((unit) => unit.classification === "warm").length,
      staleUnits,
      targetSeconds,
      dominantUnits,
    },
    shards,
  };
}

export function validateShardManifest(manifest: ShardManifest, catalog: CatalogUnit[]): void {
  if (manifest.version !== SHARD_MANIFEST_VERSION) {
    throw new Error(`Unsupported shard manifest version: ${manifest.version}`);
  }
  if (manifest.shardCount !== manifest.shards.length) {
    throw new Error("Shard manifest count does not match its shard entries");
  }
  const expected = new Map(catalog.map((unit) => [unitId(unit), unit]));
  const seen = new Set<string>();
  for (const shard of manifest.shards) {
    for (const unit of shard.units) {
      if (!expected.has(unit.id)) throw new Error(`Unit is outside the catalog: ${unit.id}`);
      if (seen.has(unit.id)) throw new Error(`Duplicate unit assignment: ${unit.id}`);
      seen.add(unit.id);
    }
    const predicted = shard.units.reduce((total, unit) => total + unit.estimatedSeconds, 0);
    if (Math.abs(predicted - shard.predictedSeconds) > 0.000001) {
      throw new Error(`Shard ${shard.index} predicted duration does not match its units`);
    }
    const files = new Set(shard.units.map((unit) => unit.file));
    if (
      files.size !== shard.files.length ||
      [...files].some((file) => !shard.files.includes(file))
    ) {
      throw new Error(`Shard ${shard.index} file selections do not match its units`);
    }
  }
  const missing = [...expected.keys()].filter((id) => !seen.has(id));
  if (missing.length > 0) throw new Error(`Missing catalog units: ${missing.join(", ")}`);
  if (manifest.catalog.unitCount !== catalog.length) {
    throw new Error("Shard manifest catalog count does not match the discovered catalog");
  }
  if (manifest.catalog.checksum !== catalogChecksum(catalog)) {
    throw new Error("Shard manifest catalog checksum does not match the discovered catalog");
  }
}

function parseArguments(argv: string[]): Record<string, string> {
  const args: Record<string, string> = {};
  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    if (!argument?.startsWith("--")) continue;
    const name = argument.slice(2);
    const value = argv[index + 1];
    if (value && !value.startsWith("--")) {
      args[name] = value;
      index += 1;
    }
  }
  return args;
}

function writePlans(outputDir: string, plans: ShardManifest[]): void {
  fs.mkdirSync(outputDir, { recursive: true });
  for (const plan of plans) {
    const cohortDir = path.join(outputDir, plan.cohort);
    fs.mkdirSync(cohortDir, { recursive: true });
    for (const shard of plan.shards) {
      fs.writeFileSync(
        path.join(cohortDir, `${shard.index}.json`),
        `${JSON.stringify(plan, null, 2)}\n`,
      );
    }
    fs.writeFileSync(
      path.join(outputDir, `${plan.cohort}-summary.json`),
      `${JSON.stringify(plan, null, 2)}\n`,
    );
  }
}

function runCli(): void {
  const args = parseArguments(process.argv.slice(2));
  const webRoot = path.resolve(args["web-root"] ?? process.cwd());
  const outputDir = path.resolve(args["output-dir"] ?? path.join(webRoot, "e2e", "manifests"));
  const profile = args.profile ? readTimingProfile(path.resolve(args.profile)) : undefined;
  const generatedAt = args.generated ?? new Date().toISOString();
  const normal = createShardPlan(discoverTestCatalog(webRoot, "normal"), {
    cohort: "normal",
    shardCount: NORMAL_SHARD_COUNT,
    profile,
    generatedAt,
  });
  const containers = createShardPlan(discoverTestCatalog(webRoot, "containers"), {
    cohort: "containers",
    shardCount: CONTAINER_SHARD_COUNT,
    profile,
    generatedAt,
  });
  validateShardManifest(normal, discoverTestCatalog(webRoot, "normal"));
  validateShardManifest(containers, discoverTestCatalog(webRoot, "containers"));
  writePlans(outputDir, [normal, containers]);
  console.log(
    JSON.stringify(
      {
        outputDir,
        normal: normal.summary,
        containers: containers.summary,
      },
      null,
      2,
    ),
  );
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  runCli();
}
