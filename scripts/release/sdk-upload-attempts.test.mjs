import assert from "node:assert/strict";
import test from "node:test";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { digest } from "./sdk-artifact.mjs";
import {
  isDefiniteUploadRejection,
  SDKUploadAttempts,
} from "./sdk-upload-attempts.mjs";

const now = Date.parse("2026-11-26T12:00:00Z");
const workflowSha = "a".repeat(40);
const source = {
  commit: "b".repeat(40),
  hostTag: "v1.2.3",
  currentHostTag: "v1.2.3",
  sdkTag: "pluginsdk/v0.1.0",
  recordDigest: "c".repeat(64),
  record: { sdk_version: "0.1.0" },
};
const candidate = {
  integrity: `sha512-${"A".repeat(86)}==`,
  receipt: {
    run_id: "700",
    run_attempt: "1",
    workflow_sha: workflowSha,
  },
};
const currentWorkflow = {
  workflow_sha: workflowSha,
  run_id: "900",
  run_attempt: "1",
  backfill_tag: "",
};
const publishJobName = (hostTag = source.currentHostTag) =>
  `Publish and verify public plugin SDK for ${hostTag}`;

function record(
  kind,
  {
    runId = "700",
    runAttempt = "1",
    workflow = workflowSha,
    intentArtifactId = "101",
    outcome = "accepted",
  } = {},
) {
  const identity = {
    schema_version: 1,
    package: "@kdlbs/plugin-sdk",
    sdk_version: source.record.sdk_version,
    sdk_tag: source.sdkTag,
    source_commit: source.commit,
    record_digest: source.recordDigest,
    archive_integrity: candidate.integrity,
    workflow_sha: workflow,
    run_id: runId,
    run_attempt: runAttempt,
  };
  return kind === "intent"
    ? identity
    : { ...identity, intent_artifact_id: intentArtifactId, outcome };
}

function makeFixture() {
  const directory = mkdtemp(join(tmpdir(), "sdk-attempt-test-"));
  const artifacts = [];
  const archiveBytes = new Map();
  const recordBytes = new Map();
  const kinds = new Map();
  const runAttempts = new Map();
  const jobResults = new Map();
  const workflowRuns = [];
  const createdAt = new Date(now - 2 * 24 * 60 * 60 * 1000).toISOString();
  const repository = {
    file: async () =>
      [
        'SDK_UPLOAD_ATTEMPT_PROTOCOL: "1"',
        "name: Publish and verify public plugin SDK for ${{ needs.prepare.outputs.tag }}",
        "id: retain_sdk_upload_intent",
        "--mode authorize",
        "--mode upload",
        "--mode complete",
      ].join("\n"),
  };
  const trustedRun = (runId, runAttempt = "1", sha = workflowSha) => ({
    id: Number(runId),
    run_id: String(runId),
    run_attempt: Number(runAttempt),
    repository: { full_name: "kdlbs/kandev", id: 11 },
    path: ".github/workflows/release.yml",
    event: "workflow_dispatch",
    head_branch: "main",
    head_sha: sha,
    created_at: createdAt,
  });
  const addArtifact = (
    id,
    kind,
    value,
    {
      runId = "700",
      runAttempt = "1",
      sha = workflowSha,
      headBranch = "main",
      path = ".github/workflows/release.yml",
      event = "workflow_dispatch",
      expired = false,
      expiresAt = new Date(now + 90 * 24 * 60 * 60 * 1000).toISOString(),
    } = {},
  ) => {
    const bytes = Buffer.from(`zip fixture ${id}`);
    archiveBytes.set(String(id), bytes);
    recordBytes.set(String(id), Buffer.from(JSON.stringify(value)));
    kinds.set(String(id), kind);
    const run = trustedRun(runId, runAttempt, sha);
    run.path = path;
    run.event = event;
    run.head_branch = headBranch;
    runAttempts.set(`${runId}/${runAttempt}`, run);
    if (kind === "intent") jobsFor(runId, runAttempt, "success", "success");
    runAttempts.set(`${runId}/${runAttempt}`, run);
    const runIndex = workflowRuns.findIndex(
      (entry) => String(entry.id) === String(runId),
    );
    if (runIndex < 0) workflowRuns.push(run);
    else workflowRuns[runIndex] = run;
    artifacts.push({
      id: Number(id),
      name: `sdk-npm-upload-${kind}-v${source.record.sdk_version}`,
      size_in_bytes: bytes.length,
      digest: `sha256:${digest(bytes)}`,
      expired,
      created_at: new Date(now - 24 * 60 * 60 * 1000).toISOString(),
      expires_at: expiresAt,
      workflow_run: {
        id: Number(runId),
        repository_id: 11,
        head_repository_id: 11,
        head_branch: headBranch,
        head_sha: sha,
      },
    });
  };
  const jobsFor = (
    runId,
    runAttempt,
    publishConclusion = "skipped",
    retainConclusion = "success",
    { conclusion = "success", hostTag = source.currentHostTag, steps } = {},
  ) => {
    const run = trustedRun(runId, runAttempt);
    runAttempts.set(`${runId}/${runAttempt}`, run);
    const existingRun = workflowRuns.findIndex(
      (entry) => String(entry.id) === String(runId),
    );
    if (existingRun < 0) workflowRuns.push(run);
    else workflowRuns[existingRun] = run;
    jobResults.set(`${runId}/${runAttempt}`, [
      {
        name: publishJobName(hostTag),
        status: "completed",
        conclusion,
        steps: steps ?? [
          {
            name: "Retain SDK upload intent",
            conclusion: retainConclusion,
          },
          { name: "Publish exact SDK archive", conclusion: publishConclusion },
        ],
      },
    ]);
  };
  jobsFor("700", "1", "skipped", "skipped");
  const run = async (command, args, options = {}) => {
    if (command === "unzip") {
      const archivePath = args.find(
        (argument) => typeof argument === "string" && argument.endsWith(".zip"),
      );
      const id = /sdk-upload-(\d+)\.zip$/.exec(archivePath)?.[1];
      if (args[0] === "-Z1") return { stdout: `${kinds.get(id)}.json\n` };
      return { stdout: recordBytes.get(id) };
    }
    assert.equal(command, "gh");
    const endpoint = args.find(
      (argument) =>
        typeof argument === "string" && argument.includes("/actions/"),
    );
    if (endpoint.includes("/actions/workflows/release.yml/runs?"))
      return {
        stdout: workflowRuns
          .map((run) =>
            Buffer.from(
              JSON.stringify({
                id: run.id,
                run_attempt: run.run_attempt,
                path: run.path,
                event: run.event,
                head_branch: run.head_branch,
                head_sha: run.head_sha,
                created_at: run.created_at,
                repository: run.repository,
              }),
            ).toString("base64"),
          )
          .join("\n"),
      };
    if (
      endpoint.includes("/actions/artifacts?") &&
      endpoint.includes("name=")
    ) {
      const requestedName = new URLSearchParams(endpoint.split("?")[1]).get(
        "name",
      );
      const lines = artifacts
        .filter((artifact) => artifact.name === requestedName)
        .map((artifact) =>
          Buffer.from(JSON.stringify(artifact)).toString("base64"),
        );
      return { stdout: lines.join("\n") };
    }
    const archiveMatch = /\/actions\/artifacts\/(\d+)\/zip$/.exec(endpoint);
    if (archiveMatch) return { stdout: archiveBytes.get(archiveMatch[1]) };
    const jobsMatch = /\/actions\/runs\/(\d+)\/attempts\/(\d+)\/jobs/.exec(
      endpoint,
    );
    if (jobsMatch) {
      const jobs = jobResults.get(`${jobsMatch[1]}/${jobsMatch[2]}`) ?? [];
      return {
        stdout: jobs
          .map((job) => Buffer.from(JSON.stringify(job)).toString("base64"))
          .join("\n"),
      };
    }
    const attemptMatch = /\/actions\/runs\/(\d+)\/attempts\/(\d+)$/.exec(
      endpoint,
    );
    if (attemptMatch)
      return {
        stdout: JSON.stringify(
          runAttempts.get(`${attemptMatch[1]}/${attemptMatch[2]}`) ??
            trustedRun(attemptMatch[1], attemptMatch[2]),
        ),
      };
    const runMatch = /\/actions\/runs\/(\d+)$/.exec(endpoint);
    if (runMatch) return { stdout: JSON.stringify(trustedRun(runMatch[1])) };
    throw new Error(`unexpected gh api endpoint: ${endpoint}`);
  };
  const makeAdapter = (path, currentNow = now) =>
    new SDKUploadAttempts({
      root: "/repo",
      directory: path,
      repository,
      run,
      now: () => currentNow,
    });
  return {
    directory,
    artifacts,
    addArtifact,
    jobsFor,
    jobResults,
    workflowRuns,
    makeAdapter,
    runAttempts,
  };
}

test("retains a bounded intent and verifies its exact artifact ID, digest, and retention", async () => {
  const fixture = makeFixture();
  const directory = await fixture.directory;
  try {
    const adapter = fixture.makeAdapter(directory);
    const intent = await adapter.writeIntent(
      source,
      candidate,
      currentWorkflow,
    );
    assert.deepEqual(
      JSON.parse(await readFile(join(directory, "intent.json"), "utf8")),
      intent,
    );
    fixture.addArtifact(101, "intent", intent, {
      runId: currentWorkflow.run_id,
      runAttempt: currentWorkflow.run_attempt,
    });
    const metadata = fixture.artifacts[0];
    const verified = await adapter.verifyRetainedIntent(
      source,
      candidate,
      currentWorkflow,
      "101",
      metadata.digest.slice("sha256:".length),
    );
    assert.equal(verified.artifact_id, "101");
    await assert.rejects(
      adapter.verifyRetainedIntent(
        source,
        candidate,
        currentWorkflow,
        "999",
        metadata.digest.slice("sha256:".length),
      ),
      /artifact ID was not found/,
    );
    await assert.rejects(
      adapter.verifyRetainedIntent(
        source,
        candidate,
        currentWorkflow,
        "101",
        "f".repeat(64),
      ),
      /digest differs/,
    );
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("accepted and uncertain upload records authorize observation only", async () => {
  for (const outcome of ["accepted", "uncertain"]) {
    const fixture = makeFixture();
    const directory = await fixture.directory;
    try {
      fixture.addArtifact(101, "intent", record("intent"));
      fixture.addArtifact(
        102,
        "result",
        record("result", { outcome, intentArtifactId: "101" }),
      );
      const adapter = fixture.makeAdapter(directory);
      const decision = await adapter.authorize(
        source,
        candidate,
        currentWorkflow,
      );
      assert.equal(decision.action, "observe");
      assert.equal(decision.attempt.outcome, outcome);
      assert.equal(decision.attempt.observing_prior_write, true);
    } finally {
      await rm(directory, { recursive: true, force: true });
    }
  }
});

test("incomplete trusted artifact metadata blocks fresh upload authorization", async () => {
  const fixture = makeFixture();
  const directory = await fixture.directory;
  try {
    fixture.addArtifact(101, "intent", record("intent"));
    delete fixture.artifacts[0].workflow_run;
    await assert.rejects(
      fixture
        .makeAdapter(directory)
        .authorize(source, candidate, currentWorkflow),
      /upload history .* could not be trusted|upload history .* unresolved/i,
    );
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("a successful retained-intent step blocks a new upload when both artifacts are missing", async () => {
  const fixture = makeFixture();
  const directory = await fixture.directory;
  try {
    fixture.jobsFor("700", "1", "success", "success");
    await assert.rejects(
      fixture
        .makeAdapter(directory)
        .authorize(source, candidate, currentWorkflow),
      /retained intent record is missing/,
    );
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("a completed origin with skipped retention and upload steps permits a fresh upload", async () => {
  const fixture = makeFixture();
  const directory = await fixture.directory;
  try {
    fixture.jobsFor("700", "1", "skipped", "skipped");
    const decision = await fixture
      .makeAdapter(directory)
      .authorize(source, candidate, currentWorkflow);
    assert.equal(decision.action, "upload");
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("a completed skipped publisher job with no steps proves the origin never uploaded", async () => {
  const fixture = makeFixture();
  const directory = await fixture.directory;
  try {
    fixture.jobsFor("700", "1", "skipped", "skipped", {
      conclusion: "skipped",
      steps: [],
    });
    const decision = await fixture
      .makeAdapter(directory)
      .authorize(source, candidate, {
        ...currentWorkflow,
        backfill_tag: source.currentHostTag,
      });
    assert.equal(decision.action, "upload");
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("a missing artifact from a separate backfill blocks a third upload", async () => {
  const fixture = makeFixture();
  const directory = await fixture.directory;
  try {
    fixture.jobsFor("700", "1", "skipped", "skipped");
    fixture.jobsFor("800", "1", "success", "success");
    await assert.rejects(
      fixture.makeAdapter(directory).authorize(source, candidate, {
        ...currentWorkflow,
        backfill_tag: source.currentHostTag,
      }),
      /retained intent record is missing/,
    );
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("a surviving intent from a separate backfill still authorizes observation", async () => {
  const fixture = makeFixture();
  const directory = await fixture.directory;
  try {
    fixture.jobsFor("700", "1", "skipped", "skipped");
    fixture.jobsFor("800", "1", "success", "success");
    fixture.addArtifact(201, "intent", record("intent", { runId: "800" }), {
      runId: "800",
    });
    const decision = await fixture
      .makeAdapter(directory)
      .authorize(source, candidate, {
        ...currentWorkflow,
        backfill_tag: source.currentHostTag,
      });
    assert.equal(decision.action, "observe");
    assert.equal(decision.attempt.outcome, "uncertain");
    assert.equal(decision.attempt.run_id, "800");
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("a different host release does not enter this SDK attempt history", async () => {
  const fixture = makeFixture();
  const directory = await fixture.directory;
  try {
    fixture.jobsFor("700", "1", "skipped", "skipped");
    fixture.jobsFor("650", "1", "success", "success", {
      hostTag: "v9.9.9",
    });
    const decision = await fixture
      .makeAdapter(directory)
      .authorize(source, candidate, {
        ...currentWorkflow,
        backfill_tag: source.currentHostTag,
      });
    assert.equal(decision.action, "upload");
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("authorization ignores the current in-flight producer attempt before intent retention", async () => {
  const fixture = makeFixture();
  const directory = await fixture.directory;
  try {
    fixture.jobResults.set("700/1", [
      {
        name: publishJobName(),
        status: "in_progress",
        steps: [
          {
            name: "Retain SDK upload intent",
            status: "pending",
            conclusion: null,
          },
          {
            name: "Publish exact SDK archive",
            status: "pending",
            conclusion: null,
          },
        ],
      },
    ]);
    const decision = await fixture
      .makeAdapter(directory)
      .authorize(source, candidate, {
        ...currentWorkflow,
        run_id: candidate.receipt.run_id,
      });
    assert.equal(decision.action, "upload");
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("completion accepts skipped upload steps while the current job is still running", async () => {
  const fixture = makeFixture();
  const directory = await fixture.directory;
  try {
    fixture.jobsFor("900", "1", "skipped", "skipped");
    fixture.jobResults.get("900/1")[0].status = "in_progress";
    const history = await fixture
      .makeAdapter(directory)
      .history(source, candidate, currentWorkflow, {
        includeCurrentAttempt: true,
      });
    assert.deepEqual(history, []);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("a successful retained-intent step from an earlier run attempt blocks a new upload", async () => {
  const fixture = makeFixture();
  const directory = await fixture.directory;
  try {
    fixture.jobsFor("900", "1", "success", "success");
    await assert.rejects(
      fixture.makeAdapter(directory).authorize(source, candidate, {
        ...currentWorkflow,
        run_attempt: "2",
      }),
      /retained intent record is missing/,
    );
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("incomplete producer job metadata blocks fresh upload authorization", async () => {
  const fixture = makeFixture();
  const directory = await fixture.directory;
  try {
    fixture.jobResults.set("700/1", [
      {
        name: publishJobName(),
        status: "completed",
        steps: [{ name: "Retain SDK upload intent", conclusion: "success" }],
      },
    ]);
    await assert.rejects(
      fixture
        .makeAdapter(directory)
        .authorize(source, candidate, currentWorkflow),
      /producer job evidence is incomplete/,
    );
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("artifacts with metadata from another branch are ignored", async () => {
  const fixture = makeFixture();
  const directory = await fixture.directory;
  try {
    fixture.addArtifact(101, "intent", record("intent"), {
      runId: "701",
      headBranch: "feature/change",
    });
    const decision = await fixture
      .makeAdapter(directory)
      .authorize(source, candidate, currentWorkflow);
    assert.equal(decision.action, "upload");
    assert.deepEqual(decision.attempts, []);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("expired trusted artifacts block fresh upload authorization", async () => {
  const fixture = makeFixture();
  const directory = await fixture.directory;
  try {
    fixture.addArtifact(101, "intent", record("intent"), {
      expiresAt: new Date(now - 1).toISOString(),
    });
    await assert.rejects(
      fixture
        .makeAdapter(directory)
        .authorize(source, candidate, currentWorkflow),
      /trusted attempt artifact has expired/,
    );
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("only explicit npm authentication rejection proves an upload was rejected", () => {
  assert.equal(
    isDefiniteUploadRejection({ category: "authentication" }),
    false,
  );
  assert.equal(
    isDefiniteUploadRejection({ npmCode: "E403", category: "authentication" }),
    false,
  );
  assert.equal(isDefiniteUploadRejection({ npmCode: "ENEEDAUTH" }), true);
  assert.equal(isDefiniteUploadRejection({ npmCode: "E401" }), true);
  assert.equal(
    isDefiniteUploadRejection(
      new Error("proxy authentication connection lost"),
    ),
    false,
  );
  assert.equal(
    isDefiniteUploadRejection(new Error("version already exists in registry")),
    false,
  );
});

test("an already-public result authorizes observation instead of another upload", async () => {
  const fixture = makeFixture();
  const directory = await fixture.directory;
  try {
    fixture.addArtifact(101, "intent", record("intent"));
    fixture.addArtifact(
      102,
      "result",
      record("result", { outcome: "already_public", intentArtifactId: "101" }),
    );
    const decision = await fixture
      .makeAdapter(directory)
      .authorize(source, candidate, currentWorkflow);
    assert.equal(decision.action, "observe");
    assert.equal(decision.attempt.outcome, "already_public");
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("a proven rejection permits a fresh upload only in a new matching backfill", async () => {
  const fixture = makeFixture();
  const directory = await fixture.directory;
  try {
    fixture.addArtifact(101, "intent", record("intent"));
    fixture.addArtifact(
      102,
      "result",
      record("result", { outcome: "rejected", intentArtifactId: "101" }),
    );
    const adapter = fixture.makeAdapter(directory);
    const decision = await adapter.authorize(source, candidate, {
      ...currentWorkflow,
      backfill_tag: source.currentHostTag,
    });
    assert.equal(decision.action, "upload");
    await assert.rejects(
      adapter.authorize(source, candidate, {
        ...currentWorkflow,
        run_id: "700",
        run_attempt: "2",
        backfill_tag: source.currentHostTag,
      }),
      /new Stable backfill run/,
    );
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("a skipped publish step proves a missing result never started", async () => {
  const fixture = makeFixture();
  const directory = await fixture.directory;
  try {
    fixture.addArtifact(101, "intent", record("intent"));
    fixture.jobsFor("700", "1", "skipped");
    const adapter = fixture.makeAdapter(directory);
    const decision = await adapter.authorize(source, candidate, {
      ...currentWorkflow,
      backfill_tag: source.currentHostTag,
    });
    assert.equal(decision.action, "upload");
    assert.deepEqual(
      (await adapter.history(source, candidate)).map(
        (attempt) => attempt.outcome,
      ),
      ["never_started"],
    );
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("an intent without a result stays uncertain unless the upload step was skipped", async () => {
  const fixture = makeFixture();
  const directory = await fixture.directory;
  try {
    fixture.addArtifact(101, "intent", record("intent"));
    const adapter = fixture.makeAdapter(directory);
    const decision = await adapter.authorize(
      source,
      candidate,
      currentWorkflow,
    );
    assert.equal(decision.action, "observe");
    assert.equal(decision.attempt.outcome, "uncertain");
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("missing and conflicting history blocks authorization", async () => {
  const fixture = makeFixture();
  const directory = await fixture.directory;
  try {
    const adapter = fixture.makeAdapter(
      directory,
      now + 31 * 24 * 60 * 60 * 1000,
    );
    await assert.rejects(
      adapter.authorize(source, candidate, currentWorkflow),
      /outside the 30-day evidence window/,
    );
    const conflicting = record("intent");
    conflicting.archive_integrity = `sha512-${"B".repeat(86)}==`;
    fixture.addArtifact(101, "intent", conflicting);
    await assert.rejects(
      fixture
        .makeAdapter(directory)
        .authorize(source, candidate, currentWorkflow),
      /conflicts with the signed SDK/,
    );
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});
