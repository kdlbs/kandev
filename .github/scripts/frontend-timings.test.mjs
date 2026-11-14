import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

import {
  allocateFiles,
  createTimingProfile,
  createTimingSnapshot,
  compareShardReports,
  parseTimingSnapshot,
  reconcileShardReports,
  validateTimingProfile,
} from "./frontend-timings.mjs";

const COMPATIBILITY_HASH = "a".repeat(64);
const SOURCE_SHA = "b".repeat(40);
const RUN_ID = "12345";
const ATTEMPT = "2";

function profile(entries = []) {
  return createTimingProfile({
    compatibilityHash: COMPATIBILITY_HASH,
    producer: { runId: "12300", attempt: "1" },
    entries,
  });
}

function files() {
  return [
    { identity: "node:a.test.ts", project: "node", path: "a.test.ts", contentHash: "1".repeat(64) },
    { identity: "browser:b.test.ts", project: "browser", path: "b.test.ts", contentHash: "2".repeat(64) },
    { identity: "browser-locales:c.test.ts", project: "browser-locales", path: "c.test.ts", contentHash: "3".repeat(64) },
    { identity: "node:d.test.ts", project: "node", path: "d.test.ts", contentHash: "4".repeat(64) },
  ];
}

function fallbackAssignments(inventory) {
  return inventory.map((entry, index) => ({ identity: entry.identity, shard: (index % 2) + 1 }));
}

function comparisonReports(runId, mode = "fallback", status = "passed", sourceSha = SOURCE_SHA) {
  const inventory = files().slice(0, 2);
  const producer = { runId: "12300", attempt: "1" };
  const profileData = mode === "profile"
    ? profile(inventory.map(({ identity, contentHash }) => ({ identity, contentHash, durationMs: 10 })))
    : null;
  const snapshot = createTimingSnapshot({
    profile: profileData,
    sourceSha,
    runId,
    attempt: "1",
    compatibilityHash: COMPATIBILITY_HASH,
  });
  const nativeAssignments = mode === "profile"
    ? undefined
    : inventory.map((entry, index) => ({ identity: entry.identity, shard: index + 1 }));
  const plan = allocateFiles(inventory, snapshot, 2, nativeAssignments);
  return [1, 2].map((shard) => {
    const assigned = plan.assignments.filter((entry) => entry.shard === shard);
    const completed = assigned.map((entry) => ({
      identity: entry.identity,
      contentHash: entry.contentHash,
      state: status === "skipped" ? "skipped" : "passed",
      durationMs: 10,
      tests: [{ fullName: "suite test", status, retryCount: 0, flaky: false }],
    }));
    const testResults = assigned.map((entry) => ({
      name: `/workspace/apps/web/${entry.identity.split(":")[1]}`,
      status: "passed",
      assertionResults: [{ fullName: "suite test", status }],
    }));
    return {
      timing: {
        schema: "kandev.frontend-timing-report/v1",
        sourceSha,
        runId,
        attempt: "1",
        shard,
        shardCount: 2,
        reason: "passed",
        snapshot: { digest: snapshot.digest, runId, attempt: "1" },
        allocation: { mode: plan.mode, planDigest: plan.planDigest, assigned },
        completed,
        unhandledErrorCount: 0,
      },
      native: { success: true, numFailedTests: 0, testResults },
    };
  });
}

test("timing profiles enforce bounded identities, hashes, durations, and compatibility", () => {
  const valid = profile([
    { identity: "node:a.test.ts", contentHash: "1".repeat(64), durationMs: 10 },
  ]);
  assert.equal(validateTimingProfile(valid, { compatibilityHash: COMPATIBILITY_HASH }).valid, true);
  assert.equal(validateTimingProfile(valid, { compatibilityHash: "c".repeat(64) }).valid, false);
  assert.equal(validateTimingProfile({ ...valid, entries: [...valid.entries, ...valid.entries] }).valid, false);
  assert.equal(
    validateTimingProfile({ ...valid, entries: [{ ...valid.entries[0], identity: "node:../outside.test.ts" }] }).valid,
    false,
  );
  assert.equal(
    validateTimingProfile({ ...valid, entries: [{ ...valid.entries[0], contentHash: "bad" }] }).valid,
    false,
  );
  assert.equal(
    validateTimingProfile({ ...valid, entries: [{ ...valid.entries[0], durationMs: Number.NaN }] }).valid,
    false,
  );
  assert.equal(
    validateTimingProfile({ ...valid, entries: [{ ...valid.entries[0], durationMs: -1 }] }).valid,
    false,
  );
  assert.equal(
    validateTimingProfile({ ...valid, entries: Array.from({ length: 20_001 }, (_, i) => ({
      identity: `node:${i}.test.ts`, contentHash: "1".repeat(64), durationMs: 1,
    })) }).valid,
    false,
  );
});

test("snapshot digest binds the frozen source and producer and rejects corruption", () => {
  const input = createTimingSnapshot({
    profile: profile(),
    sourceSha: SOURCE_SHA,
    runId: RUN_ID,
    attempt: ATTEMPT,
    compatibilityHash: COMPATIBILITY_HASH,
  });
  const text = JSON.stringify(input);
  assert.equal(parseTimingSnapshot(text, {
    sourceSha: SOURCE_SHA,
    runId: RUN_ID,
    attempt: ATTEMPT,
    compatibilityHash: COMPATIBILITY_HASH,
  }).valid, true);
  assert.equal(parseTimingSnapshot(text, { sourceSha: "d".repeat(40) }).valid, false);
  assert.equal(parseTimingSnapshot(text, { runId: "54321" }).valid, false);
  assert.equal(parseTimingSnapshot(text, { attempt: "1" }).valid, false);
  assert.equal(parseTimingSnapshot(JSON.stringify({ ...input, digest: "0".repeat(64) })).valid, false);
  assert.equal(parseTimingSnapshot(" ".repeat(10 * 1024 * 1024 + 1)).valid, false);
});

test("allocation is deterministic, complete, disjoint, and discounts stale file estimates", () => {
  const inventory = files();
  const timings = profile([
    { identity: "node:a.test.ts", contentHash: "1".repeat(64), durationMs: 100 },
    { identity: "browser:b.test.ts", contentHash: "2".repeat(64), durationMs: 90 },
    { identity: "browser-locales:c.test.ts", contentHash: "3".repeat(64), durationMs: 80 },
    { identity: "node:d.test.ts", contentHash: "f".repeat(64), durationMs: 1000 },
  ]);
  const snapshot = createTimingSnapshot({
    profile: timings,
    sourceSha: SOURCE_SHA,
    runId: RUN_ID,
    attempt: ATTEMPT,
    compatibilityHash: COMPATIBILITY_HASH,
  });
  const first = allocateFiles(inventory, snapshot, 2);
  const second = allocateFiles([...inventory].reverse(), snapshot, 2);
  assert.deepEqual(first, second);
  assert.equal(first.mode, "profile");
  assert.equal(first.assignments.length, inventory.length);
  assert.equal(new Set(first.assignments.map((entry) => entry.identity)).size, inventory.length);
  assert.deepEqual(
    first.assignments.map((entry) => entry.identity).sort(),
    inventory.map((entry) => entry.identity).sort(),
  );
  assert.equal(first.assignments.find((entry) => entry.identity === "node:d.test.ts").estimateSource, "project-median");
  assert.equal(first.assignments.find((entry) => entry.identity === "node:d.test.ts").estimateMs, 100);
  assert.ok(Math.abs(first.totalsMs[0] - first.totalsMs[1]) < 90);
});

test("cold timing input preserves the supplied complete native shard plan", () => {
  const cold = createTimingSnapshot({
    profile: null,
    sourceSha: SOURCE_SHA,
    runId: RUN_ID,
    attempt: ATTEMPT,
    compatibilityHash: COMPATIBILITY_HASH,
  });
  const inventory = files();
  const nativeAssignments = fallbackAssignments(inventory);
  const result = allocateFiles(inventory, cold, 2, nativeAssignments);
  assert.equal(result.mode, "fallback");
  assert.ok(result.assignments.every((entry) => entry.estimateSource === "native"));
  assert.deepEqual(
    result.assignments.map(({ identity, shard }) => ({ identity, shard })),
    nativeAssignments.sort((left, right) => left.identity.localeCompare(right.identity)),
  );
  assert.equal(new Set(result.assignments.map((entry) => entry.shard)).size, 2);
  assert.deepEqual(
    result,
    allocateFiles([...inventory].reverse(), cold, 2, nativeAssignments),
  );
  assert.throws(() => allocateFiles(inventory, cold, 2), /native shard assignments/);
  assert.throws(() => allocateFiles(inventory, cold, 2, nativeAssignments.slice(1)), /native shard assignments/);
});

test("empty and one-file inventories remain deterministic and complete", () => {
  const cold = createTimingSnapshot({
    profile: null,
    sourceSha: SOURCE_SHA,
    runId: RUN_ID,
    attempt: ATTEMPT,
    compatibilityHash: COMPATIBILITY_HASH,
  });
  assert.deepEqual(allocateFiles([], cold, 2, []).assignments, []);
  const only = files().slice(0, 1);
  const result = allocateFiles(only, cold, 2, [{ identity: only[0].identity, shard: 1 }]);
  assert.deepEqual(result.assignments.map(({ identity, shard }) => ({ identity, shard })), [
    { identity: only[0].identity, shard: 1 },
  ]);
});

test("gate reconciliation binds both complete shard reports to the frozen input and native outcomes", () => {
  const identities = ["node:a.test.ts", "browser:b.test.ts"];
  const hashes = ["1".repeat(64), "2".repeat(64)];
  const snapshot = createTimingSnapshot({
    profile: null,
    sourceSha: SOURCE_SHA,
    runId: RUN_ID,
    attempt: ATTEMPT,
    compatibilityHash: COMPATIBILITY_HASH,
  });
  const reports = identities.map((identity, index) => {
    const shard = index + 1;
    const assertion = {
      fullName: `suite > test ${shard}`,
      status: "passed",
      retryCount: 0,
      flaky: false,
    };
    const timing = {
      schema: "kandev.frontend-timing-report/v1",
      sourceSha: SOURCE_SHA,
      runId: RUN_ID,
      attempt: ATTEMPT,
      shard,
      shardCount: 2,
      reason: "passed",
      snapshot: { digest: snapshot.digest, runId: RUN_ID, attempt: ATTEMPT },
      allocation: {
        mode: "fallback",
        planDigest: "c".repeat(64),
        assigned: [{ identity, contentHash: hashes[index], shard }],
      },
      completed: [{
        identity,
        contentHash: hashes[index],
        state: "passed",
        durationMs: 10,
        tests: [{ ...assertion }],
      }],
      unhandledErrorCount: 0,
    };
    const native = {
      testResults: [{
        status: "passed",
        name: `/workspace/apps/web/${identity.split(":")[1]}`,
        assertionResults: [{ ...assertion }],
      }],
    };
    return { timing, native };
  });
  const expected = {
    sourceSha: SOURCE_SHA,
    runId: RUN_ID,
    snapshotDigest: snapshot.digest,
    snapshotRunId: RUN_ID,
    snapshotAttempt: ATTEMPT,
    webRoot: "/workspace/apps/web",
  };
  const reconciled = reconcileShardReports(reports, expected);
  assert.equal(reconciled.files.length, 2);
  assert.equal(reconciled.tests, 2);
  assert.equal(reconcileShardReports(reports, { ...expected, snapshotDigest: "d".repeat(64) }).valid, false);

  const duplicate = structuredClone(reports);
  duplicate[1].timing.allocation.assigned.push(duplicate[0].timing.allocation.assigned[0]);
  assert.equal(reconcileShardReports(duplicate, expected).valid, false);

  const mismatch = structuredClone(reports);
  mismatch[1].native.testResults[0].assertionResults[0].status = "skipped";
  assert.equal(reconcileShardReports(mismatch, expected).valid, false);

  const missing = reports.slice(0, 1);
  assert.equal(reconcileShardReports(missing, expected).valid, false);
});

test("native JSON reconciliation preserves repeated titles as multiplicities", () => {
  const root = mkdtempSync(path.join(os.tmpdir(), "kandev-frontend-timing-"));
  try {
    const files = ["node:a.test.ts", "browser:b.test.ts"];
    const reports = files.map((identity, index) => {
      const shard = index + 1;
      const outcomes = [
        { fullName: "repeated title", status: "passed", retryCount: 0, flaky: false },
        { fullName: "repeated title", status: "passed", retryCount: 0, flaky: false },
      ];
      return {
        timing: {
          schema: "kandev.frontend-timing-report/v1",
          sourceSha: SOURCE_SHA,
          runId: RUN_ID,
          attempt: ATTEMPT,
          shard,
          shardCount: 2,
          reason: "passed",
          snapshot: { digest: "e".repeat(64), runId: RUN_ID, attempt: ATTEMPT },
          allocation: {
            mode: "fallback",
            planDigest: "f".repeat(64),
            assigned: [{ identity, contentHash: String(index + 1).repeat(64), shard }],
          },
          completed: [{
            identity,
            contentHash: String(index + 1).repeat(64),
            state: "passed",
            durationMs: 1,
            tests: outcomes,
          }],
          unhandledErrorCount: 0,
        },
        native: {
          testResults: [{
            status: "passed",
            name: path.join(root, identity.split(":")[1]),
            assertionResults: outcomes.map((outcome) => ({ ...outcome })),
          }],
        },
      };
    });
    mkdirSync(root, { recursive: true });
    writeFileSync(path.join(root, "fixture"), "fixture");
    const result = reconcileShardReports(reports, {
      sourceSha: SOURCE_SHA,
      runId: RUN_ID,
      snapshotDigest: "e".repeat(64),
      snapshotRunId: RUN_ID,
      snapshotAttempt: ATTEMPT,
      webRoot: root,
    });
    assert.equal(result.valid, true);
    const changedMultiplicity = structuredClone(reports);
    changedMultiplicity[0].timing.completed[0].tests.pop();
    assert.equal(reconcileShardReports(changedMultiplicity, {
      sourceSha: SOURCE_SHA,
      runId: RUN_ID,
      snapshotDigest: "e".repeat(64),
      snapshotRunId: RUN_ID,
      snapshotAttempt: ATTEMPT,
      webRoot: root,
    }).valid, false);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("compare mode requires identical complete project, file, test, and outcome evidence", () => {
  const baseline = comparisonReports("12345");
  const candidate = comparisonReports("67890", "profile");
  candidate[0].timing.completed[0].tests[0].retryCount = 2;
  candidate[0].timing.completed[0].tests[0].flaky = true;
  const matched = compareShardReports(baseline, candidate);
  assert.equal(matched.valid, true, matched.failures.join("; "));
  assert.equal(matched.baseline.mode, "fallback");
  assert.equal(matched.candidate.mode, "profile");
  assert.equal(matched.baseline.files, 2);
  assert.equal(matched.candidate.tests, 2);
  assert.equal(matched.baseline.outcomes.passed, 2);
  assert.equal(matched.candidate.retries, 2);
  assert.equal(matched.candidate.flakyTests, 1);

  const changedTitle = structuredClone(candidate);
  changedTitle[0].timing.completed[0].tests[0].fullName = "suite changed";
  changedTitle[0].native.testResults[0].assertionResults[0].fullName = "suite changed";
  assert.equal(compareShardReports(baseline, changedTitle).valid, false);

  const changedMultiplicity = structuredClone(candidate);
  changedMultiplicity[0].timing.completed[0].tests.push({ ...changedMultiplicity[0].timing.completed[0].tests[0] });
  changedMultiplicity[0].native.testResults[0].assertionResults.push({ ...changedMultiplicity[0].native.testResults[0].assertionResults[0] });
  assert.equal(compareShardReports(baseline, changedMultiplicity).valid, false);

  const changedSource = comparisonReports("67890", "profile", "passed", "d".repeat(40));
  assert.equal(compareShardReports(baseline, changedSource).valid, false);
  assert.equal(compareShardReports(baseline, candidate.slice(0, 1)).valid, false);

  const skippedOnly = comparisonReports("67890", "profile", "skipped");
  assert.equal(compareShardReports(baseline, skippedOnly).valid, false);
});

test("compare CLI reads complete artifact sets and exits nonzero on an outcome regression", () => {
  const root = mkdtempSync(path.join(os.tmpdir(), "kandev-frontend-timing-compare-"));
  try {
    const baseline = comparisonReports("12345");
    const candidate = comparisonReports("67890", "profile");
    const directories = ["baseline", "candidate"].map((name) => {
      const directory = path.join(root, name);
      mkdirSync(directory, { recursive: true });
      return directory;
    });
    for (const [index, reports] of [baseline, candidate].entries()) {
      for (const [shardIndex, report] of reports.entries()) {
        const shard = shardIndex + 1;
        const artifactDirectory = path.join(directories[index], `frontend-test-results-${shard}`);
        mkdirSync(artifactDirectory, { recursive: true });
        writeFileSync(path.join(artifactDirectory, `test-results-${shard}.json`), JSON.stringify(report.native));
        writeFileSync(path.join(artifactDirectory, `frontend-timing-${shard}.json`), JSON.stringify(report.timing));
      }
    }
    const helper = fileURLToPath(new URL("./frontend-timings.mjs", import.meta.url));
    const args = [helper, "compare", "--baseline-dir", directories[0], "--candidate-dir", directories[1]];
    const matched = spawnSync(process.execPath, args, { encoding: "utf8" });
    assert.equal(matched.status, 0, matched.stderr);
    assert.equal(JSON.parse(matched.stdout).valid, true);

    const candidateNativePath = path.join(directories[1], "frontend-test-results-1", "test-results-1.json");
    const candidateTimingPath = path.join(directories[1], "frontend-test-results-1", "frontend-timing-1.json");
    const native = JSON.parse(readFileSync(candidateNativePath, "utf8"));
    const timing = JSON.parse(readFileSync(candidateTimingPath, "utf8"));
    native.testResults[0].assertionResults[0].status = "skipped";
    timing.completed[0].tests[0].status = "skipped";
    writeFileSync(candidateNativePath, JSON.stringify(native));
    writeFileSync(candidateTimingPath, JSON.stringify(timing));
    const regressed = spawnSync(process.execPath, args, { encoding: "utf8" });
    assert.equal(regressed.status, 1);
    assert.equal(JSON.parse(regressed.stdout).valid, false);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});
