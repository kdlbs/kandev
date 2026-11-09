// i18n-exempt: CI-only sequencer metadata and schema values are machine-readable, not user-facing copy.
import { createHash } from "node:crypto";
import { mkdirSync, readFileSync, renameSync, writeFileSync } from "node:fs";
import path from "node:path";
import { BaseSequencer, type TestSpecification, type Vitest } from "vitest/node";

import {
  allocateFiles,
  parseIdentity,
  parseTimingSnapshot,
} from "../../../.github/scripts/frontend-timings.mjs";

interface ShardConfig {
  index: number;
  count: number;
}

interface SequencerContext extends Vitest {
  config: Vitest["config"] & { shard?: ShardConfig };
}

interface CurrentFile {
  identity: string;
  project: string;
  path: string;
  contentHash: string;
}

interface TimingAssignmentFile {
  schema: string;
  sourceSha: string;
  runId: string;
  attempt: string;
  shard: number;
  shardCount: number;
  snapshot: {
    digest: string;
    runId: string;
    attempt: string;
    profileProducer: { runId: string; attempt: string } | null;
  };
  mode: "profile" | "fallback";
  planDigest: string;
  assigned: Array<{
    identity: string;
    contentHash: string;
    estimateMs: number;
    estimateSource: string;
    shard: number;
  }>;
}

function sha256(value: Buffer): string {
  return createHash("sha256").update(value).digest("hex");
}

function getModulePath(root: string, moduleId: string): string {
  const withoutQuery = moduleId.split("?")[0];
  return path.resolve(root, withoutQuery);
}

function createCurrentFile(root: string, specification: TestSpecification): CurrentFile {
  const absolutePath = getModulePath(root, specification.moduleId);
  const relativePath = path.relative(root, absolutePath).split(path.sep).join("/");
  if (!relativePath || relativePath === ".." || relativePath.startsWith("../")) {
    throw new Error("Vitest discovered a test module outside the web root");
  }
  const project = specification.project.name;
  const identity = `${project}:${relativePath}`;
  if (!parseIdentity(identity)) throw new Error("Vitest discovered an invalid test identity");
  return {
    identity,
    project,
    path: relativePath,
    contentHash: sha256(readFileSync(absolutePath)),
  };
}

function writeAssignment(filePath: string, assignment: TimingAssignmentFile): void {
  mkdirSync(path.dirname(filePath), { recursive: true });
  const temporary = `${filePath}.${process.pid}.tmp`;
  writeFileSync(temporary, `${JSON.stringify(assignment)}\n`);
  renameSync(temporary, filePath);
}

function timingRequested(context: SequencerContext): boolean {
  return (
    process.env.CI === "true" &&
    process.env.KANDEV_FRONTEND_TIMINGS_ENABLED === "1" &&
    context.config.shard?.count === 2
  );
}

function requiredEnvironment(name: string): string {
  const value = process.env[name];
  if (!value) throw new Error(`Missing ${name}`);
  return value;
}

function contextForShard(context: SequencerContext, index: number): SequencerContext {
  const nativeContext = Object.create(context) as SequencerContext;
  const nativeConfig = Object.create(context.config) as SequencerContext["config"];
  Object.defineProperty(nativeConfig, "shard", { value: { index, count: 2 } });
  Object.defineProperty(nativeContext, "config", { value: nativeConfig });
  return nativeContext;
}

async function nativeShardPlan(
  context: SequencerContext,
  files: TestSpecification[],
  currentFiles: CurrentFile[],
): Promise<{
  selections: TestSpecification[][];
  assignments: Array<{ identity: string; shard: number }>;
}> {
  const identityBySpecification = new Map(
    files.map((specification, index) => [specification, currentFiles[index].identity]),
  );
  const selections: TestSpecification[][] = [];
  const assignments: Array<{ identity: string; shard: number }> = [];
  for (const index of [1, 2]) {
    const selected = await new BaseSequencer(contextForShard(context, index)).shard([...files]);
    selections.push(selected);
    for (const specification of selected) {
      const identity = identityBySpecification.get(specification);
      if (!identity) throw new Error("Vitest native shard returned an unknown test specification");
      assignments.push({ identity, shard: index });
    }
  }
  return { selections, assignments };
}

export class FrontendTimingSequencer extends BaseSequencer {
  protected override ctx: SequencerContext;

  constructor(context: Vitest) {
    super(context);
    this.ctx = context as SequencerContext;
  }

  override async shard(files: TestSpecification[]): Promise<TestSpecification[]> {
    const shard = this.ctx.config.shard;
    if (!shard) return files;
    if (!timingRequested(this.ctx)) {
      return super.shard(files);
    }
    if (!Number.isInteger(shard.index) || shard.index < 1 || shard.index > 2) {
      throw new Error(`Unsupported frontend CI shard index: ${shard.index}`);
    }

    const root = this.ctx.config.root;
    const currentFiles = files.map((specification) => createCurrentFile(root, specification));
    const inputPath = requiredEnvironment("FRONTEND_TIMING_INPUT_FILE");
    const assignmentPath = requiredEnvironment("FRONTEND_TIMING_ASSIGNMENT_FILE");
    const sourceSha = requiredEnvironment("FRONTEND_TIMING_SOURCE_SHA");
    const compatibilityHash = requiredEnvironment("FRONTEND_TIMING_COMPATIBILITY_HASH");
    const expectedDigest = requiredEnvironment("FRONTEND_TIMING_SNAPSHOT_DIGEST");
    const snapshotRunId = requiredEnvironment("FRONTEND_TIMING_SNAPSHOT_RUN_ID");
    const snapshotAttempt = requiredEnvironment("FRONTEND_TIMING_SNAPSHOT_ATTEMPT");
    const runId = requiredEnvironment("GITHUB_RUN_ID");
    const runAttempt = requiredEnvironment("GITHUB_RUN_ATTEMPT");
    if (!/^[1-9][0-9]*$/.test(runId) || !/^[1-9][0-9]*$/.test(runAttempt)) {
      throw new Error("Invalid GitHub workflow run identity for frontend timing");
    }
    let rawSnapshot: string;
    try {
      rawSnapshot = readFileSync(inputPath, "utf8");
    } catch (error) {
      throw new Error("Cannot read required frontend timing snapshot", { cause: error });
    }
    const parsed = parseTimingSnapshot(rawSnapshot, {
      sourceSha,
      runId: snapshotRunId,
      attempt: snapshotAttempt,
      compatibilityHash,
    });
    if (!parsed.valid)
      throw new Error(`Invalid required frontend timing snapshot: ${parsed.reason}`);
    if (parsed.snapshot.digest !== expectedDigest) {
      throw new Error("Frontend timing snapshot does not match the expected digest");
    }
    if (parsed.snapshot.runId !== runId) {
      throw new Error("Frontend timing snapshot belongs to a different workflow run");
    }

    let allocation;
    let selectedFiles: TestSpecification[];
    if (parsed.snapshot.mode === "fallback") {
      const native = await nativeShardPlan(this.ctx, files, currentFiles);
      allocation = allocateFiles(currentFiles, parsed, 2, native.assignments);
      selectedFiles = native.selections[shard.index - 1];
    } else {
      allocation = allocateFiles(currentFiles, parsed, 2);
      const assigned = new Set(
        allocation.assignments
          .filter((entry: TimingAssignmentFile["assigned"][number]) => entry.shard === shard.index)
          .map((entry: TimingAssignmentFile["assigned"][number]) => entry.identity),
      );
      selectedFiles = files.filter((_specification, index) =>
        assigned.has(currentFiles[index].identity),
      );
    }

    const assignment: TimingAssignmentFile = {
      schema: "kandev.frontend-timing-assignment/v1",
      sourceSha,
      runId,
      attempt: runAttempt,
      shard: shard.index,
      shardCount: 2,
      snapshot: {
        digest: parsed.snapshot.digest,
        runId: parsed.snapshot.runId,
        attempt: parsed.snapshot.attempt,
        profileProducer: parsed.snapshot.profileProducer,
      },
      mode: allocation.mode === "profile" ? "profile" : "fallback",
      planDigest: allocation.planDigest,
      assigned: allocation.assignments.filter(
        (entry: TimingAssignmentFile["assigned"][number]) => entry.shard === shard.index,
      ),
    };
    writeAssignment(assignmentPath, assignment);
    return selectedFiles;
  }
}
