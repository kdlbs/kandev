import { createHash } from "node:crypto";
import { existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

export const PROFILE_SCHEMA = "kandev.frontend-timing-profile/v1";
export const SNAPSHOT_SCHEMA = "kandev.frontend-timing-snapshot/v1";
export const REPORT_SCHEMA = "kandev.frontend-timing-report/v1";
export const MAX_DOCUMENT_BYTES = 10 * 1024 * 1024;
export const MAX_ENTRIES = 20_000;

const PROJECTS = new Set(["node", "browser", "browser-locales"]);
const SHA256 = /^[a-f0-9]{64}$/;
const SOURCE_SHA = /^[a-f0-9]{40,64}$/i;
const RUN_ID = /^[1-9][0-9]*$/;

function sha256(value) {
  return createHash("sha256").update(value).digest("hex");
}

function canonicalize(value) {
  if (Array.isArray(value)) return value.map(canonicalize);
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.keys(value).sort().map((key) => [key, canonicalize(value[key])]),
    );
  }
  return value;
}

function canonicalJson(value) {
  return JSON.stringify(canonicalize(value));
}

function isSafePath(relativePath) {
  if (typeof relativePath !== "string" || relativePath.length === 0) return false;
  if (relativePath.startsWith("/") || relativePath.includes("\\") || relativePath.includes("\0")) {
    return false;
  }
  const segments = relativePath.split("/");
  return segments.every((segment) => segment.length > 0 && segment !== "." && segment !== "..");
}

export function parseIdentity(identity) {
  if (typeof identity !== "string") return null;
  const separator = identity.indexOf(":");
  if (separator < 1) return null;
  const project = identity.slice(0, separator);
  const relativePath = identity.slice(separator + 1);
  if (!PROJECTS.has(project) || !isSafePath(relativePath)) return null;
  return { project, path: relativePath };
}

function validProducer(producer) {
  return Boolean(
    producer &&
      typeof producer === "object" &&
      typeof producer.runId === "string" &&
      RUN_ID.test(producer.runId) &&
      typeof producer.attempt === "string" &&
      RUN_ID.test(producer.attempt),
  );
}

export function validateTimingProfile(profile, { compatibilityHash } = {}) {
  try {
    const bytes = Buffer.byteLength(JSON.stringify(profile));
    if (bytes > MAX_DOCUMENT_BYTES) return { valid: false, reason: "oversize profile" };
    if (!profile || typeof profile !== "object" || Array.isArray(profile)) {
      return { valid: false, reason: "profile is not an object" };
    }
    if (profile.schema !== PROFILE_SCHEMA) return { valid: false, reason: "profile schema mismatch" };
    if (!SHA256.test(profile.compatibilityHash)) {
      return { valid: false, reason: "invalid compatibility hash" };
    }
    if (compatibilityHash && profile.compatibilityHash !== compatibilityHash) {
      return { valid: false, reason: "incompatible profile" };
    }
    if (!validProducer(profile.producer)) return { valid: false, reason: "invalid profile producer" };
    if (!Array.isArray(profile.entries) || profile.entries.length > MAX_ENTRIES) {
      return { valid: false, reason: "profile entry limit exceeded" };
    }
    const seen = new Set();
    for (const entry of profile.entries) {
      const parsed = parseIdentity(entry?.identity);
      if (!parsed) return { valid: false, reason: "invalid profile identity" };
      if (seen.has(entry.identity)) return { valid: false, reason: "duplicate profile identity" };
      seen.add(entry.identity);
      if (!SHA256.test(entry.contentHash)) return { valid: false, reason: "invalid content hash" };
      if (!Number.isFinite(entry.durationMs) || entry.durationMs < 0) {
        return { valid: false, reason: "invalid duration" };
      }
    }
    return { valid: true, profile };
  } catch {
    return { valid: false, reason: "profile is not serializable" };
  }
}

export function createTimingProfile({ compatibilityHash, producer, entries }) {
  const profile = {
    schema: PROFILE_SCHEMA,
    compatibilityHash,
    producer: { runId: String(producer.runId), attempt: String(producer.attempt) },
    entries: [...entries]
      .map((entry) => ({
        identity: entry.identity,
        contentHash: entry.contentHash,
        durationMs: entry.durationMs,
      }))
      .sort((left, right) => left.identity.localeCompare(right.identity)),
  };
  const validation = validateTimingProfile(profile, { compatibilityHash });
  if (!validation.valid) throw new Error(`Cannot create timing profile: ${validation.reason}`);
  return profile;
}

function snapshotPayload(snapshot) {
  const { digest: _digest, ...payload } = snapshot;
  return payload;
}

export function createTimingSnapshot({ profile, sourceSha, runId, attempt, compatibilityHash }) {
  if (!SOURCE_SHA.test(sourceSha)) throw new Error("Invalid source SHA");
  if (!RUN_ID.test(String(runId)) || !RUN_ID.test(String(attempt))) {
    throw new Error("Invalid timing snapshot producer");
  }
  if (!SHA256.test(compatibilityHash)) throw new Error("Invalid compatibility hash");
  const validation = validateTimingProfile(profile, { compatibilityHash });
  const accepted = validation.valid ? validation.profile : null;
  const snapshot = {
    schema: SNAPSHOT_SCHEMA,
    sourceSha,
    runId: String(runId),
    attempt: String(attempt),
    compatibilityHash,
    mode: accepted ? "profile" : "fallback",
    profile: accepted,
    profileProducer: accepted ? accepted.producer : null,
  };
  return { ...snapshot, digest: sha256(canonicalJson(snapshotPayload(snapshot))) };
}

export function parseTimingSnapshot(text, expected = {}) {
  const rawText = String(text);
  const rawDigest = sha256(rawText);
  if (Buffer.byteLength(rawText) > MAX_DOCUMENT_BYTES) {
    return { valid: false, reason: "oversize snapshot", digest: rawDigest };
  }
  let snapshot;
  try {
    snapshot = JSON.parse(rawText);
  } catch {
    return { valid: false, reason: "invalid snapshot JSON", digest: rawDigest };
  }
  if (!snapshot || typeof snapshot !== "object" || Array.isArray(snapshot)) {
    return { valid: false, reason: "snapshot is not an object", digest: rawDigest };
  }
  if (snapshot.schema !== SNAPSHOT_SCHEMA) return { valid: false, reason: "snapshot schema mismatch", digest: rawDigest };
  if (!SOURCE_SHA.test(snapshot.sourceSha) || !RUN_ID.test(snapshot.runId) || !RUN_ID.test(snapshot.attempt)) {
    return { valid: false, reason: "invalid snapshot identity", digest: rawDigest };
  }
  if (!SHA256.test(snapshot.compatibilityHash)) {
    return { valid: false, reason: "invalid snapshot compatibility", digest: rawDigest };
  }
  if (snapshot.mode !== "profile" && snapshot.mode !== "fallback") {
    return { valid: false, reason: "invalid snapshot mode", digest: rawDigest };
  }
  if (typeof snapshot.digest !== "string" || !SHA256.test(snapshot.digest)) {
    return { valid: false, reason: "invalid snapshot digest", digest: rawDigest };
  }
  if (sha256(canonicalJson(snapshotPayload(snapshot))) !== snapshot.digest) {
    return { valid: false, reason: "snapshot digest mismatch", digest: rawDigest };
  }
  if (expected.sourceSha && snapshot.sourceSha !== expected.sourceSha) {
    return { valid: false, reason: "foreign snapshot source", digest: rawDigest };
  }
  if (expected.runId && snapshot.runId !== String(expected.runId)) {
    return { valid: false, reason: "foreign snapshot producer", digest: rawDigest };
  }
  if (expected.attempt && snapshot.attempt !== String(expected.attempt)) {
    return { valid: false, reason: "foreign snapshot attempt", digest: rawDigest };
  }
  if (expected.compatibilityHash && snapshot.compatibilityHash !== expected.compatibilityHash) {
    return { valid: false, reason: "incompatible snapshot", digest: rawDigest };
  }
  if (snapshot.mode === "profile") {
    const profileValidation = validateTimingProfile(snapshot.profile, {
      compatibilityHash: snapshot.compatibilityHash,
    });
    if (!profileValidation.valid || !validProducer(snapshot.profileProducer)) {
      return { valid: false, reason: "invalid snapshot profile", digest: rawDigest };
    }
    if (
      snapshot.profileProducer.runId !== snapshot.profile.producer.runId ||
      snapshot.profileProducer.attempt !== snapshot.profile.producer.attempt
    ) {
      return { valid: false, reason: "snapshot profile producer mismatch", digest: rawDigest };
    }
  } else if (snapshot.profile !== null || snapshot.profileProducer !== null) {
    return { valid: false, reason: "fallback snapshot contains a profile", digest: rawDigest };
  }
  return { valid: true, snapshot, digest: snapshot.digest };
}

function median(numbers) {
  const ordered = [...numbers].sort((left, right) => left - right);
  const middle = Math.floor(ordered.length / 2);
  return ordered.length % 2 ? ordered[middle] : (ordered[middle - 1] + ordered[middle]) / 2;
}

export function allocateFiles(files, snapshot, shardCount = 2, nativeShardAssignments) {
  if (!Array.isArray(files) || files.length > MAX_ENTRIES) throw new Error("Invalid current file inventory");
  if (!Number.isInteger(shardCount) || shardCount < 1 || shardCount > 16) {
    throw new Error("Invalid shard count");
  }
  const parsedSnapshot = snapshot && typeof snapshot === "object" && "valid" in snapshot
    ? snapshot
    : parseTimingSnapshot(JSON.stringify(snapshot ?? {}));
  if (!parsedSnapshot.valid || !parsedSnapshot.snapshot) {
    throw new Error(`Cannot allocate files without a valid timing snapshot: ${parsedSnapshot.reason ?? "invalid snapshot"}`);
  }
  const usableSnapshot = parsedSnapshot.snapshot;
  const inputDigest = parsedSnapshot.digest;
  const profileValidation = usableSnapshot.mode === "profile"
    ? validateTimingProfile(usableSnapshot.profile, { compatibilityHash: usableSnapshot.compatibilityHash })
    : { valid: false };
  if (usableSnapshot.mode === "profile" && !profileValidation.valid) {
    throw new Error(`Cannot allocate files from an invalid profile snapshot: ${profileValidation.reason}`);
  }
  const profile = profileValidation.valid ? profileValidation.profile : null;
  const profileEntries = new Map((profile?.entries ?? []).map((entry) => [entry.identity, entry]));
  const perProjectDurations = new Map();
  const currentHashes = new Map(files.map((file) => [file.identity, file.contentHash]));
  for (const entry of profile?.entries ?? []) {
    const parsed = parseIdentity(entry.identity);
    const currentHash = currentHashes.get(entry.identity);
    if (currentHash && currentHash !== entry.contentHash) continue;
    const values = perProjectDurations.get(parsed.project) ?? [];
    values.push(entry.durationMs);
    perProjectDurations.set(parsed.project, values);
  }
  const seen = new Set();
  const candidates = files.map((file) => {
    const parsed = parseIdentity(file?.identity);
    if (!parsed || parsed.project !== file.project || parsed.path !== file.path) {
      throw new Error(`Invalid current file identity: ${String(file?.identity)}`);
    }
    if (seen.has(file.identity)) throw new Error(`Duplicate current file identity: ${file.identity}`);
    seen.add(file.identity);
    if (!SHA256.test(file.contentHash)) throw new Error(`Invalid current file hash: ${file.identity}`);
    const exact = profileEntries.get(file.identity);
    if (profile && exact && exact.contentHash === file.contentHash) {
      return { ...file, estimateMs: exact.durationMs, estimateSource: "profile" };
    }
    const projectDurations = perProjectDurations.get(file.project) ?? [];
    if (profile && projectDurations.length > 0) {
      return { ...file, estimateMs: median(projectDurations), estimateSource: "project-median" };
    }
    return { ...file, estimateMs: 1, estimateSource: "default" };
  });
  const totalsMs = Array(shardCount).fill(0);
  let assignments;
  if (profile) {
    assignments = [];
    const allocationOrder = [...candidates].sort((left, right) =>
      right.estimateMs - left.estimateMs || left.identity.localeCompare(right.identity),
    );
    for (const candidate of allocationOrder) {
      let shard = 1;
      for (let index = 1; index < totalsMs.length; index += 1) {
        if (totalsMs[index] < totalsMs[shard - 1]) shard = index + 1;
      }
      totalsMs[shard - 1] += candidate.estimateMs;
      assignments.push({
        identity: candidate.identity,
        contentHash: candidate.contentHash,
        estimateMs: candidate.estimateMs,
        estimateSource: candidate.estimateSource,
        shard,
      });
    }
  } else {
    if (!Array.isArray(nativeShardAssignments) || nativeShardAssignments.length !== candidates.length) {
      throw new Error("Count fallback requires complete native shard assignments");
    }
    const nativeShards = new Map();
    for (const assignment of nativeShardAssignments) {
      if (
        !assignment ||
        typeof assignment.identity !== "string" ||
        !Number.isInteger(assignment.shard) ||
        assignment.shard < 1 ||
        assignment.shard > shardCount ||
        !seen.has(assignment.identity) ||
        nativeShards.has(assignment.identity)
      ) {
        throw new Error("Count fallback requires complete native shard assignments");
      }
      nativeShards.set(assignment.identity, assignment.shard);
    }
    if (nativeShards.size !== seen.size) {
      throw new Error("Count fallback requires complete native shard assignments");
    }
    assignments = candidates.map((candidate) => {
      const shard = nativeShards.get(candidate.identity);
      if (!shard) throw new Error("Count fallback requires complete native shard assignments");
      totalsMs[shard - 1] += 1;
      return {
        identity: candidate.identity,
        contentHash: candidate.contentHash,
        estimateMs: 1,
        estimateSource: "native",
        shard,
      };
    });
  }
  assignments.sort((left, right) => left.identity.localeCompare(right.identity));
  const mode = profile ? "profile" : "fallback";
  const plan = { schema: "kandev.frontend-timing-allocation/v1", snapshotDigest: inputDigest, mode, shardCount, assignments };
  return {
    mode,
    snapshotDigest: inputDigest,
    planDigest: sha256(canonicalJson(plan)),
    assignments,
    totalsMs,
  };
}

function normalizeStatus(status) {
  return ["passed", "failed", "skipped", "pending", "todo", "disabled"].includes(status)
    ? status
    : null;
}

function safeReportTest(testCase) {
  return Boolean(
    testCase &&
      typeof testCase.fullName === "string" &&
      testCase.fullName.length > 0 &&
      normalizeStatus(testCase.status) &&
      Number.isInteger(testCase.retryCount) &&
      testCase.retryCount >= 0 &&
      typeof testCase.flaky === "boolean",
  );
}

function relativeWebPath(fileName, webRoot) {
  if (typeof fileName !== "string" || fileName.length === 0) return null;
  const normalized = fileName.replaceAll("\\", "/").replace(/^file:\/\//, "").split("?")[0];
  const normalizedRoot = typeof webRoot === "string" ? webRoot.replaceAll("\\", "/").replace(/\/$/, "") : "";
  if (normalizedRoot && normalized.startsWith(`${normalizedRoot}/`)) {
    const relativePath = normalized.slice(normalizedRoot.length + 1);
    return isSafePath(relativePath) ? relativePath : null;
  }
  const marker = "/apps/web/";
  const markerIndex = normalized.lastIndexOf(marker);
  const relativePath = markerIndex >= 0
    ? normalized.slice(markerIndex + marker.length)
    : normalized.startsWith("apps/web/")
      ? normalized.slice("apps/web/".length)
      : !normalized.startsWith("/")
        ? normalized
        : null;
  return isSafePath(relativePath) ? relativePath : null;
}

function sortedOutcomeKeys(tests, identity) {
  if (!Array.isArray(tests)) return null;
  const keys = [];
  for (const testCase of tests) {
    if (!safeReportTest(testCase)) return null;
    keys.push(JSON.stringify([identity, testCase.fullName, testCase.status]));
  }
  return keys.sort();
}

function addFailure(failures, message) {
  if (!failures.includes(message)) failures.push(message);
}

export function reconcileShardReports(reports, expected = {}) {
  const failures = [];
  if (!Array.isArray(reports) || reports.length !== 2) {
    return { valid: false, failures: ["expected exactly two shard reports"] };
  }
  const shards = new Map();
  const planDigests = new Set();
  const modes = new Set();
  const allAssigned = new Map();
  const allCompleted = new Map();
  const allNativeOutcomes = [];
  const allReportedOutcomes = [];
  const timingEntries = [];

  for (const pair of reports) {
    const timing = pair?.timing;
    const native = pair?.native;
    if (!timing || timing.schema !== REPORT_SCHEMA) {
      addFailure(failures, "invalid timing report schema");
      continue;
    }
    if (!Number.isInteger(timing.shard) || timing.shard < 1 || timing.shard > 2) {
      addFailure(failures, "invalid shard number");
      continue;
    }
    if (shards.has(timing.shard)) addFailure(failures, "duplicate shard report");
    shards.set(timing.shard, pair);
    if (timing.shardCount !== 2) addFailure(failures, "shard count mismatch");
    if (timing.sourceSha !== expected.sourceSha) addFailure(failures, "source SHA mismatch");
    if (timing.runId !== String(expected.runId)) addFailure(failures, "workflow run mismatch");
    if (!RUN_ID.test(String(timing.attempt))) addFailure(failures, "invalid job attempt");
    if (timing.reason !== "passed") addFailure(failures, "shard did not finish successfully");
    if (timing.unhandledErrorCount !== 0) addFailure(failures, "unhandled test errors present");
    if (
      timing.snapshot?.digest !== expected.snapshotDigest ||
      timing.snapshot?.runId !== String(expected.snapshotRunId) ||
      timing.snapshot?.attempt !== String(expected.snapshotAttempt)
    ) {
      addFailure(failures, "timing snapshot provenance mismatch");
    }
    const allocation = timing.allocation;
    if (!allocation || !["profile", "fallback"].includes(allocation.mode)) {
      addFailure(failures, "invalid allocation evidence");
      continue;
    }
    if (!SHA256.test(allocation.planDigest)) addFailure(failures, "invalid allocation digest");
    planDigests.add(allocation.planDigest);
    modes.add(allocation.mode);
    if (!Array.isArray(allocation.assigned) || !Array.isArray(timing.completed)) {
      addFailure(failures, "missing assigned or completed module evidence");
      continue;
    }
    const assignedForShard = new Map();
    for (const item of allocation.assigned) {
      const parsed = parseIdentity(item?.identity);
      if (!parsed || !SHA256.test(item.contentHash) || item.shard !== timing.shard) {
        addFailure(failures, "invalid assigned file identity");
        continue;
      }
      if (assignedForShard.has(item.identity) || allAssigned.has(item.identity)) {
        addFailure(failures, "duplicate or overlapping assigned file");
      }
      assignedForShard.set(item.identity, item);
      allAssigned.set(item.identity, item);
    }
    const completedForShard = new Map();
    for (const module of timing.completed) {
      if (
        !parseIdentity(module?.identity) ||
        !SHA256.test(module.contentHash) ||
        !["passed", "skipped"].includes(module.state) ||
        !Number.isFinite(module.durationMs) ||
        module.durationMs < 0 ||
        !Array.isArray(module.tests)
      ) {
        addFailure(failures, "invalid completed module evidence");
        continue;
      }
      if (completedForShard.has(module.identity)) addFailure(failures, "duplicate completed module");
      const assigned = assignedForShard.get(module.identity);
      if (!assigned || assigned.contentHash !== module.contentHash) {
        addFailure(failures, "completed module was not assigned to this shard");
      }
      completedForShard.set(module.identity, module);
      allCompleted.set(module.identity, module);
      timingEntries.push({ identity: module.identity, contentHash: module.contentHash, durationMs: module.durationMs });
      const keys = sortedOutcomeKeys(module.tests, module.identity);
      if (keys === null) addFailure(failures, "invalid completed test outcomes");
      else allReportedOutcomes.push(...keys);
    }
    if (assignedForShard.size !== completedForShard.size) addFailure(failures, "assigned and completed file counts differ");
    for (const identity of assignedForShard.keys()) {
      if (!completedForShard.has(identity)) addFailure(failures, "assigned file did not complete");
    }
    if (!native || !Array.isArray(native.testResults)) {
      addFailure(failures, "missing native Vitest JSON report");
      continue;
    }
    const nativeForShard = new Map();
    for (const result of native.testResults) {
      if (result?.status !== "passed") addFailure(failures, "native Vitest test file did not pass");
      if (!Array.isArray(result?.assertionResults)) addFailure(failures, "native Vitest outcomes are missing");
      const relativePath = relativeWebPath(result?.name, expected.webRoot);
      const matches = [...assignedForShard.keys()].filter((identity) => parseIdentity(identity)?.path === relativePath);
      if (matches.length !== 1) {
        addFailure(failures, "native report contains a missing, duplicate, or foreign file");
        continue;
      }
      const identity = matches[0];
      if (nativeForShard.has(identity)) addFailure(failures, "duplicate native test file");
      nativeForShard.set(identity, result);
      const keys = sortedOutcomeKeys(
        (Array.isArray(result.assertionResults) ? result.assertionResults : []).map((assertion) => ({
          fullName: assertion.fullName,
          status: assertion.status,
          retryCount: 0,
          flaky: false,
        })),
        identity,
      );
      if (keys === null) addFailure(failures, "invalid native test outcomes");
      else allNativeOutcomes.push(...keys);
    }
    if (nativeForShard.size !== assignedForShard.size) addFailure(failures, "native file inventory differs from assignment");
  }

  if (shards.size !== 2 || !shards.has(1) || !shards.has(2)) addFailure(failures, "missing shard report");
  if (planDigests.size !== 1) addFailure(failures, "shards used different allocation plans");
  if (modes.size !== 1) addFailure(failures, "shards used different timing modes");
  allNativeOutcomes.sort();
  allReportedOutcomes.sort();
  if (JSON.stringify(allNativeOutcomes) !== JSON.stringify(allReportedOutcomes)) {
    addFailure(failures, "native and timing test outcomes differ");
  }
  const files = [...allAssigned.keys()].sort();
  if (allCompleted.size !== allAssigned.size) addFailure(failures, "incomplete current file inventory");
  return {
    valid: failures.length === 0,
    failures,
    files,
    tests: allNativeOutcomes.length,
    outcomes: allNativeOutcomes.map((key) => JSON.parse(key)),
    entries: timingEntries.sort((left, right) => left.identity.localeCompare(right.identity)),
    mode: modes.size === 1 ? [...modes][0] : null,
    planDigest: planDigests.size === 1 ? [...planDigests][0] : null,
  };
}

function comparisonBundle(reports, label) {
  const emptyOutcomeCounts = { passed: 0, failed: 0, skipped: 0, pending: 0, todo: 0, disabled: 0 };
  if (!Array.isArray(reports) || reports.length !== 2 || !reports[0]?.timing) {
    return {
      valid: false,
      failures: [`${label} does not contain two complete shard reports`],
      sourceSha: null,
      runId: null,
      attempts: [],
      snapshotDigest: null,
      mode: null,
      files: [],
      entries: [],
      outcomes: [],
      outcomeCounts: emptyOutcomeCounts,
      retries: 0,
      flakyTests: 0,
    };
  }
  const first = reports[0].timing;
  const reconciled = reconcileShardReports(reports, {
    sourceSha: first.sourceSha,
    runId: first.runId,
    snapshotDigest: first.snapshot?.digest,
    snapshotRunId: first.snapshot?.runId,
    snapshotAttempt: first.snapshot?.attempt,
  });
  const failures = [...reconciled.failures];
  if (reports.some((report) => report.native?.success !== true || report.native?.numFailedTests !== 0)) {
    addFailure(failures, `${label} contains an unsuccessful native Vitest report`);
  }
  if (!reconciled.outcomes?.some((outcome) => outcome[2] === "passed")) {
    addFailure(failures, `${label} contains skipped-only test evidence`);
  }
  const outcomeCounts = Object.fromEntries(
    ["passed", "failed", "skipped", "pending", "todo", "disabled"].map((status) => [status, 0]),
  );
  for (const [, , status] of reconciled.outcomes ?? []) outcomeCounts[status] += 1;
  const reportedTests = reports.flatMap((report) => report.timing?.completed ?? [])
    .flatMap((module) => module.tests ?? []);
  return {
    valid: failures.length === 0,
    failures,
    sourceSha: first.sourceSha,
    runId: first.runId,
    attempts: [...new Set(reports.map((report) => report.timing.attempt))].sort(),
    snapshotDigest: first.snapshot?.digest,
    mode: reconciled.mode,
    files: reconciled.files ?? [],
    entries: reconciled.entries ?? [],
    outcomes: reconciled.outcomes ?? [],
    outcomeCounts,
    retries: reportedTests.reduce((total, testCase) => total + (Number.isInteger(testCase.retryCount) ? testCase.retryCount : 0), 0),
    flakyTests: reportedTests.filter((testCase) => testCase.flaky === true).length,
  };
}

export function compareShardReports(baselineReports, candidateReports) {
  const baseline = comparisonBundle(baselineReports, "baseline");
  const candidate = comparisonBundle(candidateReports, "candidate");
  const failures = [...baseline.failures, ...candidate.failures];
  if (baseline.valid && candidate.valid) {
    if (baseline.sourceSha !== candidate.sourceSha) addFailure(failures, "application source SHA differs");
    if (baseline.runId === candidate.runId && JSON.stringify(baseline.attempts) === JSON.stringify(candidate.attempts)) {
      addFailure(failures, "baseline and candidate are the same workflow attempt");
    }
    if (JSON.stringify(baseline.files) !== JSON.stringify(candidate.files)) {
      addFailure(failures, "complete project and file identities differ");
    }
    const baselineHashes = baseline.entries.map(({ identity, contentHash }) => [identity, contentHash]);
    const candidateHashes = candidate.entries.map(({ identity, contentHash }) => [identity, contentHash]);
    baselineHashes.sort(([left], [right]) => left.localeCompare(right));
    candidateHashes.sort(([left], [right]) => left.localeCompare(right));
    if (JSON.stringify(baselineHashes) !== JSON.stringify(candidateHashes)) {
      addFailure(failures, "test file source hashes differ");
    }
    if (JSON.stringify(baseline.outcomes) !== JSON.stringify(candidate.outcomes)) {
      addFailure(failures, "full test titles, multiplicities, or terminal outcomes differ");
    }
  }
  return {
    schema: "kandev.frontend-timing-comparison/v1",
    valid: failures.length === 0,
    failures,
    baseline: {
      sourceSha: baseline.sourceSha,
      runId: baseline.runId,
      attempts: baseline.attempts,
      snapshotDigest: baseline.snapshotDigest,
      mode: baseline.mode,
      files: baseline.files.length,
      tests: baseline.outcomes.length,
      outcomes: baseline.outcomeCounts,
      retries: baseline.retries,
      flakyTests: baseline.flakyTests,
    },
    candidate: {
      sourceSha: candidate.sourceSha,
      runId: candidate.runId,
      attempts: candidate.attempts,
      snapshotDigest: candidate.snapshotDigest,
      mode: candidate.mode,
      files: candidate.files.length,
      tests: candidate.outcomes.length,
      outcomes: candidate.outcomeCounts,
      retries: candidate.retries,
      flakyTests: candidate.flakyTests,
    },
  };
}

function readJson(file) {
  return JSON.parse(readFileSync(file, "utf8"));
}

function findFiles(root, predicate) {
  if (!existsSync(root)) return [];
  const found = [];
  for (const entry of readdirSync(root, { withFileTypes: true })) {
    const file = path.join(root, entry.name);
    if (entry.isDirectory()) found.push(...findFiles(file, predicate));
    else if (entry.isFile() && predicate(file)) found.push(file);
  }
  return found.sort();
}

function readShardPairs(reportsDirectory) {
  const timingFiles = findFiles(reportsDirectory, (file) => /^frontend-timing-[12]\.json$/.test(path.basename(file)));
  const nativeFiles = findFiles(reportsDirectory, (file) => /^test-results-[12]\.json$/.test(path.basename(file)));
  if (timingFiles.length !== 2 || nativeFiles.length !== 2) {
    throw new Error("Expected exactly one native and timing report for each frontend shard");
  }
  return timingFiles.map((timingFile) => {
    const shard = path.basename(timingFile).match(/-([12])\.json$/)?.[1];
    const matches = nativeFiles.filter((file) => path.basename(file) === `test-results-${shard}.json`);
    if (matches.length !== 1) throw new Error(`Missing or duplicate native report for shard ${shard}`);
    return {
      timing: readJson(timingFile),
      native: readJson(matches[0]),
    };
  });
}

function parseArguments(args) {
  const values = new Map();
  for (let index = 0; index < args.length; index += 1) {
    const key = args[index];
    if (!key.startsWith("--")) throw new Error(`Unexpected argument: ${key}`);
    const value = args[index + 1];
    if (value === undefined || value.startsWith("--")) throw new Error(`Missing value for ${key}`);
    values.set(key.slice(2), value);
    index += 1;
  }
  return values;
}

function required(values, name) {
  const value = values.get(name);
  if (!value) throw new Error(`Missing --${name}`);
  return value;
}

function writeGitHubOutputs(values) {
  const output = process.env.GITHUB_OUTPUT;
  if (!output) return;
  const lines = Object.entries(values).map(([key, value]) => `${key}=${value ?? ""}`);
  writeFileSync(output, `${lines.join("\n")}\n`, { flag: "a" });
}

function snapshotCommand(values) {
  const compatibilityHash = required(values, "compatibility-hash");
  const profilePath = values.get("profile");
  let profile = null;
  if (profilePath && existsSync(profilePath)) {
    try {
      profile = readJson(profilePath);
    } catch {
      profile = null;
    }
  }
  const snapshot = createTimingSnapshot({
    profile,
    sourceSha: required(values, "source-sha"),
    runId: required(values, "run-id"),
    attempt: required(values, "attempt"),
    compatibilityHash,
  });
  const outputPath = required(values, "output");
  mkdirSync(path.dirname(outputPath), { recursive: true });
  writeFileSync(outputPath, `${JSON.stringify(snapshot)}\n`);
  const artifactName = `frontend-timing-input-${snapshot.runId}-${snapshot.attempt}`;
  writeGitHubOutputs({
    digest: snapshot.digest,
    run_id: snapshot.runId,
    attempt: snapshot.attempt,
    profile_producer_run_id: snapshot.profileProducer?.runId,
    profile_producer_attempt: snapshot.profileProducer?.attempt,
    compatibility_hash: snapshot.compatibilityHash,
    artifact_name: artifactName,
  });
  process.stdout.write(`${JSON.stringify({ mode: snapshot.mode, digest: snapshot.digest })}\n`);
}

function reconcileCommand(values) {
  const reportsDirectory = required(values, "reports-dir");
  const result = reconcileShardReports(readShardPairs(reportsDirectory), {
    sourceSha: required(values, "source-sha"),
    runId: required(values, "run-id"),
    snapshotDigest: required(values, "snapshot-digest"),
    snapshotRunId: required(values, "snapshot-run-id"),
    snapshotAttempt: required(values, "snapshot-attempt"),
  });
  if (!result.valid) throw new Error(`Frontend shard reconciliation failed: ${result.failures.join("; ")}`);
  const profile = createTimingProfile({
    compatibilityHash: required(values, "compatibility-hash"),
    producer: {
      runId: required(values, "run-id"),
      attempt: required(values, "attempt"),
    },
    entries: result.entries,
  });
  const outputPath = required(values, "profile-output");
  mkdirSync(path.dirname(outputPath), { recursive: true });
  writeFileSync(outputPath, `${JSON.stringify(profile)}\n`);
  process.stdout.write(`${JSON.stringify({ files: result.files.length, tests: result.tests, mode: result.mode })}\n`);
}

function compareCommand(values) {
  const baseline = readShardPairs(required(values, "baseline-dir"));
  const candidate = readShardPairs(required(values, "candidate-dir"));
  const result = compareShardReports(baseline, candidate);
  process.stdout.write(`${JSON.stringify(result)}\n`);
  if (!result.valid) process.exitCode = 1;
}

export function runCli(args = process.argv.slice(2)) {
  const [command, ...remaining] = args;
  const values = parseArguments(remaining);
  if (command === "snapshot") return snapshotCommand(values);
  if (command === "reconcile") return reconcileCommand(values);
  if (command === "compare") return compareCommand(values);
  throw new Error(`Unknown frontend timing command: ${command}`);
}

const thisFile = fileURLToPath(import.meta.url);
if (process.argv[1] && path.resolve(process.argv[1]) === thisFile) {
  try {
    runCli();
  } catch (error) {
    process.stderr.write(`${error instanceof Error ? error.message : String(error)}\n`);
    process.exitCode = 1;
  }
}
