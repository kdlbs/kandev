import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { digest } from "./sdk-artifact.mjs";
import { packageName } from "./sdk-registry.mjs";

const execute = promisify(execFile);
const repositoryName = "kdlbs/kandev";
const workflowPath = ".github/workflows/release.yml";
const publishStepName = "Publish exact SDK archive";
const intentFileName = "intent.json";
const resultFileName = "result.json";
const maximumRecordBytes = 16 * 1024;
const maximumArchiveBytes = 1024 * 1024;
const maximumArtifacts = 100;
const maximumWorkflowRuns = 1000;
export const UPLOAD_EVIDENCE_WINDOW_MS = 30 * 24 * 60 * 60 * 1000;
export const UPLOAD_ATTEMPT_OUTCOMES = [
  "accepted",
  "uncertain",
  "rejected",
  "never_started",
  "already_public",
];

function attemptError(version, message) {
  const error = new Error(
    `SDK upload history for ${version} is unresolved: ${message}`,
  );
  error.code = "SDK_UPLOAD_HISTORY_UNRESOLVED";
  error.category = "upload_history";
  return error;
}

function publishJobName(hostTag) {
  return `Publish and verify public plugin SDK for ${hostTag}`;
}

function artifactNames(version) {
  if (!/^\d+\.\d+\.\d+$/.test(version ?? ""))
    throw new Error("invalid SDK version for upload attempt artifact");
  return {
    intent: `sdk-npm-upload-intent-v${version}`,
    result: `sdk-npm-upload-result-v${version}`,
  };
}

function validateIdentity(record, expected, kind) {
  const keys = [
    "schema_version",
    "package",
    "sdk_version",
    "sdk_tag",
    "source_commit",
    "record_digest",
    "archive_integrity",
    "workflow_sha",
    "run_id",
    "run_attempt",
    ...(kind === "result" ? ["intent_artifact_id", "outcome"] : []),
  ].sort();
  if (
    !record ||
    typeof record !== "object" ||
    Array.isArray(record) ||
    Object.keys(record).sort().join("\n") !== keys.join("\n")
  )
    throw new Error("attempt record has an invalid shape");
  const identity = {
    schema_version: 1,
    package: packageName,
    sdk_version: expected.sdk_version,
    sdk_tag: expected.sdk_tag,
    source_commit: expected.source_commit,
    record_digest: expected.record_digest,
    archive_integrity: expected.archive_integrity,
  };
  for (const [key, value] of Object.entries(identity))
    if (record[key] !== value)
      throw new Error(`attempt record ${key} conflicts with the signed SDK`);
  if (
    !/^[0-9a-f]{40}$/.test(record.workflow_sha ?? "") ||
    !/^[1-9]\d*$/.test(record.run_id ?? "") ||
    !/^[1-9]\d*$/.test(record.run_attempt ?? "")
  )
    throw new Error("attempt record has an invalid workflow identity");
  if (
    kind === "result" &&
    (!/^[1-9]\d*$/.test(record.intent_artifact_id ?? "") ||
      !UPLOAD_ATTEMPT_OUTCOMES.includes(record.outcome))
  )
    throw new Error("attempt result has an invalid outcome or intent ID");
  return record;
}

function expectedIdentity(source, candidate) {
  return {
    sdk_version: source.record.sdk_version,
    sdk_tag: source.sdkTag,
    source_commit: source.commit,
    record_digest: source.recordDigest,
    archive_integrity: candidate.integrity,
  };
}

function workflowIdentity(workflow) {
  const identity = {
    workflow_sha: workflow.workflow_sha,
    run_id: String(workflow.run_id ?? ""),
    run_attempt: String(workflow.run_attempt ?? ""),
  };
  if (
    !/^[0-9a-f]{40}$/.test(identity.workflow_sha ?? "") ||
    !/^[1-9]\d*$/.test(identity.run_id) ||
    !/^[1-9]\d*$/.test(identity.run_attempt)
  )
    throw new Error("invalid current SDK release workflow identity");
  return identity;
}

function artifactDigest(value) {
  const match = /^(?:sha256:)?([a-f0-9]{64})$/.exec(value ?? "");
  if (!match) throw new Error("invalid SDK upload artifact digest");
  return `sha256:${match[1]}`;
}

function safeArtifactId(value) {
  const text = String(value ?? "");
  if (!/^[1-9]\d*$/.test(text) || !Number.isSafeInteger(Number(text)))
    throw new Error("invalid SDK upload artifact ID");
  return text;
}

function parseJson(bytes, version) {
  if (bytes.length === 0 || bytes.length > maximumRecordBytes)
    throw attemptError(
      version,
      "attempt record is empty or exceeds its size limit",
    );
  try {
    return JSON.parse(bytes.toString("utf8"));
  } catch {
    throw attemptError(version, "attempt record is not valid JSON");
  }
}

export class SDKUploadAttempts {
  constructor({
    root,
    directory,
    repository,
    env = process.env,
    run = execute,
    now = () => Date.now(),
  }) {
    Object.assign(this, { root, directory, repository, env, run, now });
    this.runCache = new Map();
    this.workflowRunListCache = new Map();
    this.revisionCache = new Map();
    this.jobCache = new Map();
  }

  async gh(args, binary = false) {
    try {
      return await this.run("gh", args, {
        cwd: this.root,
        encoding: binary ? "buffer" : "utf8",
        maxBuffer: maximumArchiveBytes + 1024 * 1024,
        timeout: 120_000,
      });
    } catch {
      throw new Error("GitHub Actions upload evidence could not be read");
    }
  }

  async json(endpoint, version) {
    try {
      return JSON.parse((await this.gh(["api", endpoint])).stdout);
    } catch {
      throw attemptError(version, "GitHub Actions metadata could not be read");
    }
  }

  async listArtifacts(name, version) {
    const endpoint = `repos/${repositoryName}/actions/artifacts?name=${encodeURIComponent(name)}&per_page=100`;
    const { stdout } = await this.gh([
      "api",
      "--paginate",
      "--jq",
      ".artifacts[] | @base64",
      endpoint,
    ]);
    const lines = String(stdout).split(/\r?\n/).filter(Boolean);
    if (lines.length > maximumArtifacts)
      throw attemptError(version, "artifact history exceeds the lookup limit");
    return lines.map((line) => {
      try {
        return JSON.parse(Buffer.from(line, "base64").toString("utf8"));
      } catch {
        throw attemptError(version, "artifact listing is malformed");
      }
    });
  }

  async workflowRun(runId, version) {
    if (!this.runCache.has(runId)) {
      this.runCache.set(
        runId,
        this.json(`repos/${repositoryName}/actions/runs/${runId}`, version),
      );
    }
    return this.runCache.get(runId);
  }

  async workflowRunsSince(startDate, version) {
    if (!this.workflowRunListCache.has(startDate)) {
      const endpoint = `repos/${repositoryName}/actions/workflows/release.yml/runs?event=workflow_dispatch&branch=main&created=>=${encodeURIComponent(startDate)}&per_page=100`;
      this.workflowRunListCache.set(
        startDate,
        (async () => {
          const { stdout } = await this.gh([
            "api",
            "--paginate",
            "--jq",
            ".workflow_runs[] | {id,run_attempt,path,event,head_branch,head_sha,created_at,repository:{full_name:.repository.full_name}} | @base64",
            endpoint,
          ]);
          const lines = String(stdout).split(/\r?\n/).filter(Boolean);
          if (lines.length > maximumWorkflowRuns)
            throw attemptError(
              version,
              "workflow run history exceeds the lookup limit",
            );
          return lines.map((line) => {
            try {
              return JSON.parse(Buffer.from(line, "base64").toString("utf8"));
            } catch {
              throw attemptError(version, "workflow run listing is malformed");
            }
          });
        })(),
      );
    }
    return this.workflowRunListCache.get(startDate);
  }

  trustedRun(run, version) {
    const repository = run?.repository;
    return Boolean(
      run &&
      repository?.full_name === repositoryName &&
      run.path === workflowPath &&
      run.event === "workflow_dispatch" &&
      run.head_branch === "main" &&
      /^[0-9a-f]{40}$/.test(run.head_sha ?? ""),
    );
  }

  async workflowUsesUploadGate(workflowSha, version) {
    if (!this.revisionCache.has(workflowSha)) {
      const revision = (async () => {
        if (typeof this.repository?.file !== "function")
          throw attemptError(version, "trusted workflow source is unavailable");
        let workflow;
        try {
          workflow = await this.repository.file(workflowSha, workflowPath);
        } catch {
          throw attemptError(
            version,
            "trusted workflow revision is unavailable",
          );
        }
        return (
          workflow.includes('SDK_UPLOAD_ATTEMPT_PROTOCOL: "1"') &&
          workflow.includes(
            "name: Publish and verify public plugin SDK for ${{ needs.prepare.outputs.tag }}",
          ) &&
          workflow.includes("id: retain_sdk_upload_intent") &&
          workflow.includes("--mode authorize") &&
          workflow.includes("--mode upload") &&
          workflow.includes("--mode complete")
        );
      })();
      this.revisionCache.set(workflowSha, revision);
    }
    return this.revisionCache.get(workflowSha);
  }

  async workflowHasUploadGate(workflowSha, version) {
    if (!(await this.workflowUsesUploadGate(workflowSha, version)))
      throw attemptError(
        version,
        "trusted workflow revision lacks the upload gate",
      );
  }

  async validateRunAttempt(
    runId,
    runAttempt,
    workflowSha,
    version,
    { requireUploadGate = true } = {},
  ) {
    const run = await this.json(
      `repos/${repositoryName}/actions/runs/${runId}/attempts/${runAttempt}`,
      version,
    );
    if (
      !this.trustedRun(run, version) ||
      String(run.run_id ?? run.id) !== String(runId) ||
      Number(run.run_attempt) !== Number(runAttempt) ||
      run.head_sha !== workflowSha
    )
      throw attemptError(version, "producer run or attempt identity conflicts");
    if (requireUploadGate)
      await this.workflowHasUploadGate(workflowSha, version);
    return run;
  }

  async trustedArtifactRun(artifact, version) {
    const metadata = artifact?.workflow_run;
    if (!metadata)
      throw attemptError(version, "artifact producer metadata is missing");
    if (
      !Number.isSafeInteger(metadata.id) ||
      metadata.id <= 0 ||
      !Number.isSafeInteger(metadata.repository_id) ||
      metadata.repository_id <= 0 ||
      !Number.isSafeInteger(metadata.head_repository_id) ||
      metadata.head_repository_id <= 0 ||
      !/^[0-9a-f]{40}$/.test(metadata.head_sha ?? "") ||
      typeof metadata.head_branch !== "string"
    )
      throw attemptError(version, "artifact producer metadata is incomplete");
    if (metadata.head_branch !== "main") return null;
    const runId = String(metadata.id);
    const run = await this.workflowRun(runId, version);
    if (
      !run?.repository?.full_name ||
      typeof run.path !== "string" ||
      typeof run.event !== "string" ||
      typeof run.head_branch !== "string" ||
      typeof run.head_sha !== "string"
    )
      throw attemptError(version, "workflow run metadata is incomplete");
    if (!this.trustedRun(run, version)) return null;
    const repositoryId = run.repository?.id;
    if (
      run.head_sha !== metadata.head_sha ||
      (Number.isSafeInteger(repositoryId) &&
        metadata.repository_id !== repositoryId) ||
      (Number.isSafeInteger(repositoryId) &&
        metadata.head_repository_id !== repositoryId)
    )
      throw attemptError(
        version,
        "artifact producer metadata conflicts with its run",
      );
    return { run, runId };
  }

  async artifactRecord(
    artifact,
    kind,
    source,
    candidate,
    { minimumRetention = false } = {},
  ) {
    const version = source.record.sdk_version;
    const trusted = await this.trustedArtifactRun(artifact, version);
    if (!trusted) return null;
    const { run, runId } = trusted;
    await this.workflowHasUploadGate(run.head_sha, version);
    if (typeof artifact.expired !== "boolean")
      throw attemptError(version, "artifact expiry metadata is invalid");
    const expiresAt = Date.parse(artifact.expires_at ?? "");
    if (
      artifact.expired ||
      !Number.isFinite(expiresAt) ||
      expiresAt <= this.now()
    )
      throw attemptError(version, "a trusted attempt artifact has expired");
    const id = safeArtifactId(artifact.id);
    if (
      typeof artifact.name !== "string" ||
      !Number.isSafeInteger(artifact.size_in_bytes) ||
      artifact.size_in_bytes < 1 ||
      artifact.size_in_bytes > maximumArchiveBytes ||
      !/^sha256:[a-f0-9]{64}$/.test(artifact.digest ?? "")
    )
      throw attemptError(version, "artifact ID, size, or digest is invalid");
    if (minimumRetention && expiresAt - this.now() < UPLOAD_EVIDENCE_WINDOW_MS)
      throw attemptError(version, "intent retention is shorter than 30 days");
    let bytes;
    try {
      bytes = Buffer.from(
        (
          await this.gh(
            [
              "api",
              `repos/${repositoryName}/actions/artifacts/${id}/zip`,
              "-H",
              "Accept: application/octet-stream",
            ],
            true,
          )
        ).stdout,
      );
    } catch {
      throw attemptError(version, "artifact archive could not be downloaded");
    }
    if (
      bytes.length !== artifact.size_in_bytes ||
      `sha256:${digest(bytes)}` !== artifact.digest
    )
      throw attemptError(
        version,
        "artifact archive digest conflicts with GitHub metadata",
      );
    const fileName = kind === "intent" ? intentFileName : resultFileName;
    const archive = join(this.directory, `sdk-upload-${id}.zip`);
    await mkdir(this.directory, { recursive: true });
    await writeFile(archive, bytes);
    let entries;
    let recordBytes;
    try {
      entries = (
        await this.run("unzip", ["-Z1", archive], {
          cwd: this.root,
          encoding: "utf8",
          maxBuffer: maximumRecordBytes,
          timeout: 30_000,
        })
      ).stdout
        .trim()
        .split(/\r?\n/);
      if (entries.length !== 1 || entries[0] !== fileName)
        throw new Error("unexpected archive entries");
      recordBytes = Buffer.from(
        (
          await this.run("unzip", ["-p", archive, fileName], {
            cwd: this.root,
            encoding: "buffer",
            maxBuffer: maximumRecordBytes,
            timeout: 30_000,
          })
        ).stdout,
      );
    } catch {
      throw attemptError(
        version,
        "artifact archive does not contain its fixed record",
      );
    }
    const record = parseJson(recordBytes, version);
    const expected = expectedIdentity(source, candidate);
    try {
      validateIdentity(record, expected, kind);
    } catch (error) {
      throw attemptError(version, error.message);
    }
    if (record.workflow_sha !== run.head_sha || record.run_id !== runId)
      throw attemptError(
        version,
        "attempt record does not match its producer run",
      );
    const attempt = await this.validateRunAttempt(
      record.run_id,
      record.run_attempt,
      record.workflow_sha,
      version,
    );
    const createdAt = Date.parse(artifact.created_at ?? "");
    const runCreatedAt = Date.parse(attempt.created_at ?? "");
    if (
      !Number.isFinite(createdAt) ||
      !Number.isFinite(runCreatedAt) ||
      createdAt < runCreatedAt ||
      createdAt > this.now()
    )
      throw attemptError(
        version,
        "artifact timestamp conflicts with its producer",
      );
    return { artifact, record, run: attempt };
  }

  async writeIntent(source, candidate, workflow) {
    const version = source.record.sdk_version;
    const record = validateIdentity(
      {
        schema_version: 1,
        package: packageName,
        ...expectedIdentity(source, candidate),
        ...workflowIdentity(workflow),
      },
      expectedIdentity(source, candidate),
      "intent",
    );
    await mkdir(this.directory, { recursive: true });
    await writeFile(
      join(this.directory, intentFileName),
      `${JSON.stringify(record, null, 2)}\n`,
      { flag: "wx" },
    );
    return record;
  }

  async verifyRetainedIntent(
    source,
    candidate,
    workflow,
    artifactId,
    digestValue,
  ) {
    const version = source.record.sdk_version;
    const identity = workflowIdentity(workflow);
    const id = safeArtifactId(artifactId);
    const expectedDigest = artifactDigest(digestValue);
    let local;
    try {
      local = parseJson(
        await readFile(join(this.directory, intentFileName)),
        version,
      );
      validateIdentity(local, expectedIdentity(source, candidate), "intent");
    } catch {
      throw attemptError(
        version,
        "current upload intent is missing or malformed",
      );
    }
    if (
      local.workflow_sha !== identity.workflow_sha ||
      local.run_id !== identity.run_id ||
      local.run_attempt !== identity.run_attempt
    )
      throw attemptError(
        version,
        "current upload intent belongs to another run attempt",
      );
    const name = artifactNames(version).intent;
    const artifacts = await this.listArtifacts(name, version);
    const matches = artifacts.filter(
      (artifact) => String(artifact?.id) === id && artifact?.name === name,
    );
    if (matches.length !== 1)
      throw attemptError(version, "retained intent artifact ID was not found");
    const actualDigest = artifactDigest(matches[0].digest);
    if (actualDigest !== expectedDigest)
      throw attemptError(
        version,
        "retained intent digest differs from the upload action",
      );
    const verified = await this.artifactRecord(
      matches[0],
      "intent",
      source,
      candidate,
      {
        minimumRetention: true,
      },
    );
    if (
      !verified ||
      verified.record.workflow_sha !== identity.workflow_sha ||
      verified.record.run_id !== identity.run_id ||
      verified.record.run_attempt !== identity.run_attempt
    )
      throw attemptError(
        version,
        "retained intent is not owned by this run attempt",
      );
    return { artifact_id: id, record: verified.record };
  }

  async writeResult(source, candidate, workflow, intentArtifactId, outcome) {
    const version = source.record.sdk_version;
    if (!UPLOAD_ATTEMPT_OUTCOMES.includes(outcome))
      throw new Error("invalid SDK upload outcome");
    const result = validateIdentity(
      {
        schema_version: 1,
        package: packageName,
        ...expectedIdentity(source, candidate),
        ...workflowIdentity(workflow),
        intent_artifact_id: safeArtifactId(intentArtifactId),
        outcome,
      },
      expectedIdentity(source, candidate),
      "result",
    );
    await mkdir(this.directory, { recursive: true });
    await writeFile(
      join(this.directory, resultFileName),
      `${JSON.stringify(result, null, 2)}\n`,
      { flag: "wx" },
    );
    return result;
  }

  async history(
    source,
    candidate,
    workflow = null,
    { includeCurrentAttempt = false } = {},
  ) {
    const version = source.record.sdk_version;
    const names = artifactNames(version);
    const [intents, results] = await Promise.all([
      this.listArtifacts(names.intent, version),
      this.listArtifacts(names.result, version),
    ]);
    const records = [];
    for (const artifact of intents) {
      if (artifact?.name !== names.intent) continue;
      const verified = await this.artifactRecord(
        artifact,
        "intent",
        source,
        candidate,
      );
      if (verified) records.push({ ...verified, kind: "intent" });
    }
    for (const artifact of results) {
      if (artifact?.name !== names.result) continue;
      const verified = await this.artifactRecord(
        artifact,
        "result",
        source,
        candidate,
      );
      if (verified) records.push({ ...verified, kind: "result" });
    }
    const grouped = new Map();
    for (const entry of records) {
      const key = `${entry.record.run_id}/${entry.record.run_attempt}`;
      const group = grouped.get(key) ?? {
        run_id: entry.record.run_id,
        run_attempt: entry.record.run_attempt,
      };
      if (group[entry.kind])
        throw attemptError(
          version,
          "multiple records exist for one run attempt",
        );
      group[entry.kind] = entry;
      grouped.set(key, group);
    }
    await this.reconcileMissingRetainedIntents(
      source,
      candidate,
      workflow,
      grouped,
      includeCurrentAttempt,
    );
    const attempts = [];
    for (const group of grouped.values()) {
      if (!group.intent) {
        throw attemptError(
          version,
          "result record has no matching upload intent",
        );
      }
      const evidence = await this.jobAttemptEvidence(
        group.intent.record.run_id,
        group.intent.record.run_attempt,
        version,
        source.currentHostTag,
      );
      if (
        !evidence.retainedIntent ||
        evidence.retainedIntent.conclusion !== "success"
      )
        throw attemptError(
          version,
          "retained intent artifact conflicts with producer job evidence",
        );
      if (group.result && evidence.publish.conclusion === "skipped")
        throw attemptError(
          version,
          "upload result conflicts with a skipped publish step",
        );
      let outcome;
      if (group.result) {
        if (
          group.result.record.intent_artifact_id !==
          String(group.intent.artifact.id)
        )
          throw attemptError(
            version,
            "result does not identify its retained intent",
          );
        outcome = group.result.record.outcome;
      } else {
        outcome = (await this.publishStepSkipped(
          group.intent.record.run_id,
          group.intent.record.run_attempt,
          version,
          source.currentHostTag,
        ))
          ? "never_started"
          : "uncertain";
      }
      attempts.push({
        outcome,
        run_id: group.intent.record.run_id,
        run_attempt: group.intent.record.run_attempt,
        workflow_sha: group.intent.record.workflow_sha,
        intent_artifact_id: String(group.intent.artifact.id),
        created_at: group.intent.artifact.created_at,
      });
    }
    attempts.sort(
      (left, right) =>
        Date.parse(left.created_at) - Date.parse(right.created_at),
    );
    return attempts;
  }

  async jobAttemptEvidence(
    runId,
    runAttempt,
    version,
    hostTag,
    { required = true } = {},
  ) {
    const key = `${runId}/${runAttempt}/${hostTag}`;
    if (!this.jobCache.has(key)) {
      this.jobCache.set(
        key,
        (async () => {
          const endpoint = `repos/${repositoryName}/actions/runs/${runId}/attempts/${runAttempt}/jobs?per_page=100`;
          const { stdout } = await this.gh([
            "api",
            "--paginate",
            "--jq",
            ".jobs[] | @base64",
            endpoint,
          ]);
          let jobs;
          try {
            jobs = String(stdout)
              .split(/\r?\n/)
              .filter(Boolean)
              .map((line) =>
                JSON.parse(Buffer.from(line, "base64").toString("utf8")),
              );
          } catch {
            throw attemptError(version, "producer job evidence is malformed");
          }
          const job = jobs.find(
            (entry) => entry.name === publishJobName(hostTag),
          );
          if (!job) {
            if (!required) return null;
            throw attemptError(version, "producer job evidence is incomplete");
          }
          if (
            job.status === "completed" &&
            job.conclusion === "skipped" &&
            (!Array.isArray(job.steps) || job.steps.length === 0)
          )
            return { job, retainedIntent: null, publish: null };
          const retainedIntent = job?.steps?.find(
            (entry) => entry.name === "Retain SDK upload intent",
          );
          const publish = job?.steps?.find(
            (entry) => entry.name === publishStepName,
          );
          if (!Array.isArray(job.steps) || !retainedIntent || !publish)
            throw attemptError(version, "producer job evidence is incomplete");
          return { job, retainedIntent, publish };
        })(),
      );
    }
    const evidence = await this.jobCache.get(key);
    if (!evidence && required)
      throw attemptError(version, "producer job evidence is incomplete");
    return evidence;
  }

  async reconcileMissingRetainedIntents(
    source,
    candidate,
    workflow,
    groups,
    includeCurrentAttempt,
  ) {
    const version = source.record.sdk_version;
    const expected = new Map();
    const add = (runId, runAttempt, workflowSha) => {
      const key = `${runId}/${runAttempt}`;
      const existing = expected.get(key);
      if (existing && existing.workflow_sha !== workflowSha)
        throw attemptError(
          version,
          "known producer workflow identities conflict",
        );
      expected.set(key, {
        run_id: String(runId),
        run_attempt: String(runAttempt),
        workflow_sha: workflowSha,
      });
    };
    const current = workflow ? workflowIdentity(workflow) : null;
    const originIsCurrentInFlightAttempt =
      current &&
      !includeCurrentAttempt &&
      String(candidate.receipt.run_id) === current.run_id &&
      String(candidate.receipt.run_attempt) === current.run_attempt;
    if (!originIsCurrentInFlightAttempt)
      add(
        candidate.receipt.run_id,
        candidate.receipt.run_attempt,
        candidate.receipt.workflow_sha,
      );
    if (workflow) {
      const lastAttempt =
        Number(current.run_attempt) - (includeCurrentAttempt ? 0 : 1);
      for (let attempt = 1; attempt <= lastAttempt; attempt++)
        add(current.run_id, attempt, current.workflow_sha);
    }

    const originRun = await this.workflowRun(candidate.receipt.run_id, version);
    const originCreatedAt = Date.parse(originRun?.created_at ?? "");
    if (!Number.isFinite(originCreatedAt))
      throw attemptError(version, "origin workflow timestamp is unavailable");
    const runs = await this.workflowRunsSince(
      new Date(originCreatedAt).toISOString().slice(0, 10),
      version,
    );
    for (const listedRun of runs) {
      if (
        listedRun?.repository?.full_name !== repositoryName ||
        listedRun.path !== workflowPath ||
        listedRun.event !== "workflow_dispatch" ||
        listedRun.head_branch !== "main"
      )
        continue;
      const runId = safeArtifactId(listedRun.id);
      const runAttemptCount = Number(listedRun.run_attempt);
      const createdAt = Date.parse(listedRun.created_at ?? "");
      if (
        !/^[0-9a-f]{40}$/.test(listedRun.head_sha ?? "") ||
        !Number.isSafeInteger(runAttemptCount) ||
        runAttemptCount < 1 ||
        !Number.isFinite(createdAt)
      )
        throw attemptError(version, "workflow run metadata is incomplete");
      if (createdAt < originCreatedAt) continue;
      const lastAttempt =
        current && runId === current.run_id
          ? Number(current.run_attempt) - (includeCurrentAttempt ? 0 : 1)
          : runAttemptCount;
      for (
        let attempt = 1;
        attempt <= Math.min(runAttemptCount, lastAttempt);
        attempt++
      ) {
        const attemptNumber = String(attempt);
        const run = await this.validateRunAttempt(
          runId,
          attemptNumber,
          listedRun.head_sha,
          version,
          { requireUploadGate: false },
        );
        if (!(await this.workflowUsesUploadGate(run.head_sha, version)))
          continue;
        const evidence = await this.jobAttemptEvidence(
          runId,
          attemptNumber,
          version,
          source.currentHostTag,
          { required: false },
        );
        if (evidence) add(runId, attemptNumber, run.head_sha);
      }
    }

    for (const expectedAttempt of expected.values()) {
      const run = await this.validateRunAttempt(
        expectedAttempt.run_id,
        expectedAttempt.run_attempt,
        expectedAttempt.workflow_sha,
        version,
        { requireUploadGate: false },
      );
      if (!(await this.workflowUsesUploadGate(run.head_sha, version))) continue;
      const evidence = await this.jobAttemptEvidence(
        expectedAttempt.run_id,
        expectedAttempt.run_attempt,
        version,
        source.currentHostTag,
      );
      const group = groups.get(
        `${expectedAttempt.run_id}/${expectedAttempt.run_attempt}`,
      );
      if (
        evidence.job.status === "completed" &&
        evidence.job.conclusion === "skipped" &&
        !evidence.retainedIntent &&
        !evidence.publish
      ) {
        if (group?.intent)
          throw attemptError(
            version,
            "retained intent artifact conflicts with a skipped publisher job",
          );
        continue;
      }
      if (!evidence.retainedIntent || !evidence.publish)
        throw attemptError(version, "producer job evidence is incomplete");
      if (evidence.retainedIntent.conclusion === "success") {
        if (!group?.intent)
          throw attemptError(version, "retained intent record is missing");
        continue;
      }
      if (
        evidence.retainedIntent.conclusion === "skipped" &&
        evidence.publish.conclusion === "skipped"
      ) {
        if (group?.intent)
          throw attemptError(
            version,
            "retained intent artifact conflicts with producer job evidence",
          );
        continue;
      }
      throw attemptError(version, "producer job evidence is incomplete");
    }
  }

  async publishStepSkipped(runId, runAttempt, version, hostTag) {
    const { job, publish } = await this.jobAttemptEvidence(
      runId,
      runAttempt,
      version,
      hostTag,
    );
    return (
      job.status === "completed" &&
      (job.conclusion === "skipped" || publish?.conclusion === "skipped")
    );
  }

  async validateCurrentWorkflow(workflow, version) {
    const identity = workflowIdentity(workflow);
    const run = await this.validateRunAttempt(
      identity.run_id,
      identity.run_attempt,
      identity.workflow_sha,
      version,
    );
    return run;
  }

  async validateOrigin(source, candidate, workflow) {
    const version = source.record.sdk_version;
    const receipt = candidate.receipt;
    const run = await this.validateRunAttempt(
      receipt.run_id,
      receipt.run_attempt,
      receipt.workflow_sha,
      version,
    );
    const createdAt = Date.parse(run.created_at ?? "");
    const age = this.now() - createdAt;
    if (
      !Number.isFinite(createdAt) ||
      age < 0 ||
      age > UPLOAD_EVIDENCE_WINDOW_MS
    )
      throw attemptError(
        version,
        "signed receipt is outside the 30-day evidence window",
      );
    if (
      workflow.backfill_tag &&
      workflow.backfill_tag !== source.currentHostTag
    )
      throw attemptError(
        version,
        "backfill tag does not match the signed SDK source",
      );
  }

  async authorize(source, candidate, workflow) {
    const version = source.record.sdk_version;
    await this.validateCurrentWorkflow(workflow, version);
    const attempts = await this.history(source, candidate, workflow);
    const priorWrite = attempts.find((attempt) =>
      ["accepted", "uncertain", "already_public"].includes(attempt.outcome),
    );
    if (priorWrite)
      return {
        action: "observe",
        attempt: { ...priorWrite, observing_prior_write: true },
      };
    if (
      attempts.some(
        (attempt) => !["rejected", "never_started"].includes(attempt.outcome),
      )
    )
      throw attemptError(
        version,
        "attempt outcome does not authorize a new upload",
      );
    if (attempts.some((attempt) => attempt.run_id === String(workflow.run_id)))
      throw attemptError(
        version,
        "a proven rejection requires a new Stable backfill run",
      );
    if (attempts.length && !workflow.backfill_tag)
      throw attemptError(
        version,
        "a proven rejection requires a new Stable backfill run",
      );
    await this.validateOrigin(source, candidate, workflow);
    return { action: "upload", attempts };
  }
}

export function isDefiniteUploadRejection(error) {
  const code = error?.npmCode ?? error?.code;
  if (/^(?:ENEEDAUTH|E401)$/.test(code ?? "")) return true;
  const output = [error?.stderr, error?.stdout, error?.message]
    .filter((value) => typeof value === "string")
    .join("\n");
  return /\bnpm (?:error|ERR!) code (?:ENEEDAUTH|E401)\b/.test(output);
}
