import assert from "node:assert/strict";
import test from "node:test";
import { mkdtemp, rm, readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
  preparePublication,
  publishPublication,
  boundedRegistryValue,
  REGISTRY_OBSERVATION_WINDOW_MS,
} from "./sdk-publication-state.mjs";
import { SdkDiagnostics } from "./sdk-diagnostics.mjs";
import { runRelease } from "./publish-plugin-sdk.mjs";

const fingerprint = "A".repeat(40);
const source = {
  commit: "b".repeat(40),
  hostTag: "v0.100.0",
  sdkTag: "pluginsdk/v0.1.0",
  recordDigest: "c".repeat(64),
  record: {
    schema_version: 1,
    sdk_version: "0.1.0",
    host_release: "0.100.0",
    min_kandev_version: "0.100.0",
    api_versions: [1, 2],
    change_kind: "initial",
  },
};
const tag = () => ({
  commit: source.commit,
  fingerprint,
  recordDigest: source.recordDigest,
});
const candidate = () => ({
  fingerprint,
  integrity: "sha512-example",
  retention: {
    urls: ["https://github.com/kdlbs/kandev/releases/download/sdk.tgz"],
  },
  receipt: {
    schema_version: 1,
    sdk_version: "0.1.0",
    sdk_tag: source.sdkTag,
    source_commit: source.commit,
    host_tag: source.hostTag,
    record_digest: source.recordDigest,
    archive_integrity: "sha512-example",
    bootstrap_allowed: true,
  },
});
const npmVersion = () => ({
  version: source.record.sdk_version,
  integrity: "sha512-example",
  recordDigest: source.recordDigest,
  provenance: true,
});
function fixture({
  tagged = false,
  published = false,
  packageExists = true,
  latest = null,
  versionVisibleAt = 0,
  latestVisibleAt = 0,
  publishError = null,
  latestError = null,
} = {}) {
  const state = {
    tag: tagged ? tag() : null,
    registry: {
      packageExists,
      version: published ? npmVersion() : null,
      latest,
    },
    candidate: candidate(),
  };
  const clock = { now: 0, waits: [], reads: [] };
  let pendingVersionAt = null;
  let pendingLatestAt = null;
  let pendingLatestVersion = null;
  const events = [];
  const io = {
    readTag: async () => structuredClone(state.tag),
    readRegistry: async () => {
      clock.reads.push(clock.now);
      if (pendingVersionAt !== null && clock.now >= pendingVersionAt) {
        state.registry.version = npmVersion();
        pendingVersionAt = null;
      }
      if (pendingLatestAt !== null && clock.now >= pendingLatestAt) {
        state.registry.latest = pendingLatestVersion;
        pendingLatestAt = null;
      }
      return structuredClone(state.registry);
    },
    now: () => clock.now,
    registryWait: async (milliseconds) => {
      clock.waits.push(milliseconds);
      clock.now += milliseconds;
    },
    obtainCandidate: async (_, options) => {
      events.push(["obtain", options]);
      return state.candidate;
    },
    retainCandidate: async () => events.push(["retain"]),
    finalizeCandidate: async () => events.push(["finalize"]),
    verifyFinalizedCandidate: async () => events.push(["verify-finalized"]),
    createTag: async () => {
      events.push(["tag"]);
      state.tag = tag();
    },
    publish: async () => {
      events.push(["publish"]);
      state.registry.packageExists = true;
      pendingVersionAt = versionVisibleAt;
      if (publishError) throw publishError;
    },
    moveLatest: async (version) => {
      events.push(["latest", version]);
      pendingLatestVersion = version;
      pendingLatestAt = latestVisibleAt;
      if (latestError) throw latestError;
    },
    verifyNewerLatest: async () => events.push(["verify-newer"]),
    verifyConsumers: async () => events.push(["consumers"]),
    wait: async () => {},
  };
  return {
    io,
    state,
    events,
    clock,
    context: { source, fingerprint, firstPublishOIDCReady: true },
  };
}

async function makeReleaseRunner(f, { uploadAttempts = {} } = {}) {
  const directory = await mkdtemp(join(tmpdir(), "sdk-release-runner-"));
  const outputFile = join(directory, "output.txt");
  const summaries = [];
  const releaseSource = {
    ...source,
    currentCommit: source.commit,
    currentHostTag: source.hostTag,
    packConfig: { node: "24.19.0", npm: "11.21.0" },
  };
  const repository = {
    loadSource: async () => releaseSource,
    text: async () => releaseSource.currentCommit,
  };
  f.io.uploadAttempts = {
    authorize: async () => ({ action: "upload", attempts: [] }),
    writeIntent: async () => {},
    verifyRetainedIntent: async () => ({ artifact_id: "501" }),
    writeResult: async () => {},
    history: async () => [],
    ...uploadAttempts,
  };
  const env = {
    GITHUB_ACTIONS: "true",
    GITHUB_REPOSITORY: "kdlbs/kandev",
    GITHUB_EVENT_NAME: "workflow_dispatch",
    GITHUB_REF: "refs/heads/main",
    GITHUB_WORKFLOW_REF:
      "kdlbs/kandev/.github/workflows/release.yml@refs/heads/main",
    RUNNER_ENVIRONMENT: "github-hosted",
    SDK_SCOPE_OWNERSHIP_CONFIRMED: "true",
    SDK_FIRST_PUBLISH_OIDC_READY: "false",
    RELEASE_GPG_FINGERPRINT: fingerprint,
    SDK_WORKFLOW_SHA: "d".repeat(40),
    SDK_BACKFILL_TAG: "",
    SDK_UPLOAD_STATE_DIR: directory,
    SDK_UPLOAD_INTENT_ARTIFACT_ID: "501",
    SDK_UPLOAD_INTENT_ARTIFACT_DIGEST: "e".repeat(64),
    GITHUB_RUN_ID: "901",
    GITHUB_RUN_ATTEMPT: "1",
    GITHUB_OUTPUT: outputFile,
  };
  const run = (mode) =>
    runRelease({
      mode,
      hostTag: source.hostTag,
      root: directory,
      env,
      makeRepository: () => repository,
      makeDirectory: async () => join(directory, `work-${mode}`),
      removeDirectory: async () => {},
      summaryWriter: async (summary) => summaries.push(summary),
      verifyToolsImpl: () => {},
      execFileImpl: async (command) => ({
        stdout: command === "npm" ? "11.21.0" : "",
      }),
      makeReleaseIO: ({ diagnostics }) => {
        f.io.diagnostics = diagnostics;
        return f.io;
      },
      diagnosticsLog: null,
      diagnosticsError: () => {},
    });
  return { directory, outputFile, summaries, env, run };
}
// @covers AC-PLUGINS-SDK-DISTRIBUTION-003.2
test("SDK evidence is finalized after signed tagging and before npm publication", async () => {
  const f = fixture();
  await preparePublication(f.io, f.context);
  await publishPublication(f.io, f.context);
  const position = (name) => {
    const index = f.events.findIndex(([event]) => event === name);
    assert.notEqual(index, -1, `${name} event is missing`);
    return index;
  };
  assert.ok(position("retain") < position("tag"));
  assert.ok(position("tag") < position("finalize"));
  assert.ok(position("finalize") < position("verify-finalized"));
  assert.ok(position("verify-finalized") < position("publish"));
});
test("unfinalized evidence blocks npm publication and bootstrap", async () => {
  for (const packageExists of [false, true]) {
    const f = fixture({ tagged: true, packageExists });
    f.context.firstPublishOIDCReady = false;
    f.io.verifyFinalizedCandidate = async () => {
      throw new Error("SDK evidence is not finalized");
    };
    await assert.rejects(publishPublication(f.io, f.context), /not finalized/);
    assert.equal(
      f.events.some(([event]) => event === "publish" || event === "consumers"),
      false,
    );
  }
});
for (const tagged of [false, true])
  for (const published of [false, true]) {
    test(`recovers immutable state: tag=${tagged}, npm=${published}`, async () => {
      const f = fixture({ tagged, published });
      await preparePublication(f.io, f.context);
      assert.equal(
        f.events.filter(([event]) => event === "tag").length,
        tagged ? 0 : 1,
      );
      assert.equal(f.events[0][1].allowCreate, !tagged && !published);
      assert.ok(
        f.events.findIndex(([event]) => event === "retain") <
          (tagged
            ? Infinity
            : f.events.findIndex(([event]) => event === "tag")),
      );
      const result = await publishPublication(f.io, f.context);
      assert.equal(result.status, "verified");
      assert.equal(
        f.events.filter(([event]) => event === "publish").length,
        published ? 0 : 1,
      );
      assert.equal(f.events.at(-1)[0], "consumers");
    });
  }
test("conflicting tag or registry identity stops SDK mutations", async () => {
  for (const mutation of [
    (f) => {
      f.state.tag.commit = "d".repeat(40);
    },
    (f) => {
      f.state.tag.fingerprint = "D".repeat(40);
    },
    (f) => {
      f.state.registry.version.integrity = "different";
    },
    (f) => {
      f.state.registry.version.recordDigest = "different";
    },
  ]) {
    const f = fixture({ tagged: true, published: true });
    mutation(f);
    await assert.rejects(
      preparePublication(f.io, f.context),
      /conflict|signature/,
    );
    assert.equal(
      f.events.some(([event]) => ["tag", "publish", "retain"].includes(event)),
      false,
    );
  }
});
test("confirmed absent package reports bootstrap only after candidate retention", async () => {
  const f = fixture({ packageExists: false });
  f.context.firstPublishOIDCReady = false;
  await preparePublication(f.io, f.context);
  assert.equal(
    (await publishPublication(f.io, f.context)).status,
    "bootstrap_required",
  );
  assert.ok(f.events.some(([event]) => event === "retain"));
  assert.equal(
    f.events.some(([event]) => event === "publish" || event === "consumers"),
    false,
  );
  f.state.registry.packageExists = true;
  f.state.registry.version = { ...npmVersion(), provenance: false };
  assert.equal(
    (await publishPublication(f.io, f.context)).origin,
    "manual_bootstrap",
  );
});
test("an accepted first-package upload is observed even if bootstrap readiness changes", async () => {
  const f = fixture({ tagged: true, packageExists: false });
  f.context.firstPublishOIDCReady = false;
  let reads = 0;
  const readRegistry = f.io.readRegistry;
  f.io.readRegistry = async () => {
    reads += 1;
    if (reads === 2) {
      f.state.registry.packageExists = true;
      f.state.registry.version = npmVersion();
    }
    return readRegistry();
  };
  const result = await publishPublication(f.io, {
    ...f.context,
    uploadAttempt: {
      outcome: "accepted",
      run_id: "901",
      run_attempt: "1",
      observing_prior_write: true,
    },
  });
  assert.equal(result.status, "verified");
  assert.equal(
    f.events.some(([event]) => event === "publish"),
    false,
  );
  assert.equal(
    f.events.some(([event]) => event === "consumers"),
    true,
  );
});
test("read failures and existing-package setup errors are not bootstrap states", async () => {
  const unreadable = fixture();
  unreadable.io.readRegistry = async () => {
    throw new Error("registry 403");
  };
  await assert.rejects(
    preparePublication(unreadable.io, unreadable.context),
    /403/,
  );
  assert.deepEqual(unreadable.events, []);
  const f = fixture({ tagged: true });
  f.context.firstPublishOIDCReady = false;
  f.io.publish = async () => {
    throw new Error("OIDC publisher permission denied");
  };
  await assert.rejects(
    publishPublication(f.io, f.context),
    /permission|publication/,
  );
});
test("uncertain successful writes are reread instead of repeated", async () => {
  const f = fixture();
  f.io.createTag = async () => {
    f.events.push(["tag"]);
    f.state.tag = tag();
    throw new Error("connection lost");
  };
  await preparePublication(f.io, f.context);
  f.io.publish = async () => {
    f.events.push(["publish"]);
    f.state.registry.version = npmVersion();
    throw new Error("connection lost");
  };
  assert.equal((await publishPublication(f.io, f.context)).status, "verified");
  assert.equal(f.events.filter(([event]) => event === "tag").length, 1);
  assert.equal(f.events.filter(([event]) => event === "publish").length, 1);
});
test("latest never downgrades and dist-tag denial does not report success", async () => {
  const newer = fixture({ tagged: true, published: true, latest: "0.2.0" });
  assert.equal(
    (await publishPublication(newer.io, newer.context)).status,
    "verified",
  );
  assert.ok(newer.events.some(([event]) => event === "verify-newer"));
  assert.equal(
    newer.events.some(([event]) => event === "latest"),
    false,
  );
  const denied = fixture({ tagged: true, published: true });
  denied.io.moveLatest = async () => {
    throw new Error("dist-tag permission denied");
  };
  await assert.rejects(
    publishPublication(denied.io, denied.context),
    /dist-tag/,
  );
  assert.equal(
    denied.events.some(([event]) => event === "consumers"),
    false,
  );
});
test("missing authenticated evidence after publication cannot be recreated", async () => {
  const f = fixture({ tagged: true });
  f.io.obtainCandidate = async (_, options) => {
    assert.equal(options.allowCreate, false);
    throw new Error("missing authenticated receipt");
  };
  await assert.rejects(preparePublication(f.io, f.context), /receipt/);
  assert.deepEqual(f.events, []);
});
test("unapproved manual upload and failed public consumers leave publication incomplete", async () => {
  const manual = fixture({ tagged: true, published: true });
  manual.state.registry.version.provenance = false;
  manual.state.candidate.receipt.bootstrap_allowed = false;
  await assert.rejects(
    publishPublication(manual.io, manual.context),
    /provenance/,
  );
  const broken = fixture({ tagged: true, published: true, latest: "0.1.0" });
  broken.io.verifyConsumers = async () => {
    throw new Error("public Go install failed");
  };
  await assert.rejects(
    publishPublication(broken.io, broken.context),
    /public Go/,
  );
});
test("npm version visible after 90 seconds completes without a second upload", async () => {
  const f = fixture({ tagged: true, versionVisibleAt: 90_000 });
  const result = await publishPublication(f.io, f.context);
  assert.equal(result.status, "verified");
  assert.equal(f.events.filter(([event]) => event === "publish").length, 1);
  assert.equal(f.clock.now, 90_000);
  assert.deepEqual(f.clock.waits, [10_000, 20_000, 30_000, 30_000]);
  assert.equal(f.events.at(-1)[0], "consumers");
});
test("SDK version visible after 309 seconds completes without a second upload", async () => {
  const f = fixture({ tagged: true, versionVisibleAt: 309_000 });
  const result = await publishPublication(f.io, f.context);
  assert.equal(result.status, "verified");
  assert.equal(f.events.filter(([event]) => event === "publish").length, 1);
  assert.equal(f.clock.now, 330_000);
});
test("delayed latest dist-tag visibility uses the same observation window", async () => {
  const f = fixture({
    tagged: true,
    published: true,
    latestVisibleAt: 90_000,
  });
  const result = await publishPublication(f.io, f.context);
  assert.equal(result.status, "verified");
  assert.equal(f.events.filter(([event]) => event === "latest").length, 1);
  assert.equal(f.clock.now, 90_000);
  assert.deepEqual(f.clock.waits, [10_000, 20_000, 30_000, 30_000]);
  assert.equal(f.events.at(-1)[0], "consumers");
});
test("latest dist-tag visible after 309 seconds completes without a second mutation", async () => {
  const f = fixture({
    tagged: true,
    published: true,
    latestVisibleAt: 309_000,
  });
  const result = await publishPublication(f.io, f.context);
  assert.equal(result.status, "verified");
  assert.equal(f.events.filter(([event]) => event === "latest").length, 1);
  assert.equal(f.clock.now, 330_000);
});
test("SDK version visible at the deadline completes on the final probe", async () => {
  const f = fixture({
    tagged: true,
    versionVisibleAt: REGISTRY_OBSERVATION_WINDOW_MS,
  });
  const result = await publishPublication(f.io, f.context);
  assert.equal(result.status, "verified");
  assert.equal(f.clock.reads.at(-1), REGISTRY_OBSERVATION_WINDOW_MS);
  assert.equal(f.events.filter(([event]) => event === "publish").length, 1);
});
test("latest dist-tag visible at the deadline completes on the final probe", async () => {
  const f = fixture({
    tagged: true,
    published: true,
    latestVisibleAt: REGISTRY_OBSERVATION_WINDOW_MS,
  });
  const result = await publishPublication(f.io, f.context);
  assert.equal(result.status, "verified");
  assert.equal(f.clock.reads.at(-1), REGISTRY_OBSERVATION_WINDOW_MS);
  assert.equal(f.events.filter(([event]) => event === "latest").length, 1);
});
test("permanent absence exhausts the ten-minute window with bounded waits and one upload", async () => {
  const f = fixture({ tagged: true, versionVisibleAt: Infinity });
  await assert.rejects(
    publishPublication(f.io, f.context),
    /accepted by npm.*10 minutes/,
  );
  assert.equal(f.events.filter(([event]) => event === "publish").length, 1);
  assert.ok(
    f.clock.waits.every((wait) => wait > 0 && wait <= 30_000),
    f.clock.waits,
  );
  assert.equal(
    f.clock.waits.reduce((total, wait) => total + wait, 0),
    REGISTRY_OBSERVATION_WINDOW_MS,
  );
  assert.equal(f.clock.reads.length, f.clock.waits.length + 2);
});
test("registry observation performs one final probe at the deadline", async () => {
  const f = fixture({ tagged: true, versionVisibleAt: Infinity });
  await assert.rejects(publishPublication(f.io, f.context), /10 minutes/);
  assert.ok(
    Math.max(...f.clock.reads) <= REGISTRY_OBSERVATION_WINDOW_MS,
    f.clock.reads,
  );
  assert.equal(f.clock.reads.at(-1), REGISTRY_OBSERVATION_WINDOW_MS);
  assert.equal(f.clock.now, REGISTRY_OBSERVATION_WINDOW_MS);
});
test("registry read errors during observation fail distinctly from confirmed absence", async () => {
  const f = fixture({ tagged: true });
  const readRegistry = f.io.readRegistry;
  let reads = 0;
  f.io.readRegistry = async () => {
    reads += 1;
    if (reads > 1) throw new Error("registry read failed with HTTP 500");
    return readRegistry();
  };
  await assert.rejects(
    publishPublication(f.io, f.context),
    /registry read failed with HTTP 500/,
  );
  assert.equal(f.events.filter(([event]) => event === "publish").length, 1);
  assert.deepEqual(f.clock.waits, []);
});
test("transient registry read errors retry and appear in observations", async () => {
  const f = fixture({ tagged: true });
  const readRegistry = f.io.readRegistry;
  let observationReads = 0;
  f.io.readRegistry = async (...args) => {
    if (f.events.some(([event]) => event === "publish")) {
      observationReads += 1;
      if (observationReads === 1)
        throw Object.assign(new Error("registry read failed with HTTP 503"), {
          code: "REGISTRY_READ_FAILURE",
          retryable: true,
        });
    }
    return readRegistry(...args);
  };
  const diagnostics = new SdkDiagnostics({
    hostTag: "v0.100.0",
    sdkVersion: "0.1.0",
    log: null,
  });
  f.io.diagnostics = diagnostics;
  const result = await publishPublication(f.io, f.context);
  assert.equal(result.status, "verified");
  assert.equal(f.events.filter(([event]) => event === "publish").length, 1);
  assert.deepEqual(f.clock.waits, [10_000]);
  assert.equal(
    diagnostics.observations.some(
      (event) => event.phase === "version_visibility" && event.read_error,
    ),
    true,
  );
});
test("exhausted transient registry reads return the final read error", async () => {
  const f = fixture({ tagged: true });
  let observationReads = 0;
  f.io.readRegistry = async () => {
    if (f.events.some(([event]) => event === "publish")) {
      observationReads += 1;
      throw Object.assign(new Error("registry read failed with HTTP 503"), {
        code: "REGISTRY_READ_FAILURE",
        retryable: true,
      });
    }
    return {
      packageExists: true,
      version: null,
      latest: null,
    };
  };
  await assert.rejects(publishPublication(f.io, f.context), (error) => {
    assert.equal(error.code, "REGISTRY_READ_FAILURE");
    assert.equal(error.retryable, true);
    return true;
  });
  assert.equal(observationReads, 22);
  assert.equal(f.clock.now, REGISTRY_OBSERVATION_WINDOW_MS);
  assert.equal(f.events.filter(([event]) => event === "publish").length, 1);
});
test("definite authentication rejection stops after one reconciliation read", async () => {
  const f = fixture({
    tagged: true,
    versionVisibleAt: Infinity,
    publishError: Object.assign(new Error("npm error code ENEEDAUTH"), {
      code: "ENEEDAUTH",
      stderr: "npm error code ENEEDAUTH",
    }),
  });
  await assert.rejects(publishPublication(f.io, f.context), /ENEEDAUTH/);
  assert.equal(f.events.filter(([event]) => event === "publish").length, 1);
  assert.equal(f.clock.reads.length, 2);
  assert.deepEqual(f.clock.waits, []);
});

test("ambiguous E403 authentication diagnostics remain uncertain until the signed version appears", async () => {
  const f = fixture({
    tagged: true,
    versionVisibleAt: 90_000,
    publishError: Object.assign(new Error("npm error code E403"), {
      npmCode: "E403",
      category: "authentication",
    }),
  });
  const result = await publishPublication(f.io, f.context);
  assert.equal(result.status, "verified");
  assert.equal(f.events.filter(([event]) => event === "publish").length, 1);
  assert.deepEqual(f.clock.waits, [10_000, 20_000, 30_000, 30_000]);
});
test("uncertain delayed write is reconciled by registry reads without another upload", async () => {
  const f = fixture({
    tagged: true,
    versionVisibleAt: 90_000,
    publishError: new Error("connection lost"),
  });
  const result = await publishPublication(f.io, f.context);
  assert.equal(result.status, "verified");
  assert.equal(f.events.filter(([event]) => event === "publish").length, 1);
  assert.deepEqual(f.clock.waits, [10_000, 20_000, 30_000, 30_000]);
});
test("accepted upload recovery observes the version without a second upload", async () => {
  const f = fixture({ tagged: true });
  const readRegistry = f.io.readRegistry;
  f.io.readRegistry = async (...args) => {
    if (f.clock.now >= 90_000) f.state.registry.version = npmVersion();
    return readRegistry(...args);
  };
  const diagnostics = new SdkDiagnostics({
    hostTag: "v0.100.0",
    sdkVersion: "0.1.0",
    log: null,
  });
  f.io.diagnostics = diagnostics;
  const result = await publishPublication(f.io, {
    ...f.context,
    noFreshUpload: true,
    uploadAttempt: {
      outcome: "accepted",
      run_id: "42",
      run_attempt: "1",
      observing_prior_write: true,
    },
  });
  assert.equal(result.status, "verified");
  assert.equal(f.events.filter(([event]) => event === "publish").length, 0);
  assert.equal(diagnostics.writeOutcome, "accepted");
  assert.equal(diagnostics.uploadAttempt.observing_prior_write, true);
  assert.equal(
    diagnostics.summary({ status: result.status, result }).upload_attempt
      .run_id,
    "42",
  );
  assert.equal(f.events.at(-1)[0], "consumers");
});
test("rejected upload recovery never retries the rejected write in the same run", async () => {
  const f = fixture({ tagged: true });
  let failure;
  await assert.rejects(
    publishPublication(f.io, {
      ...f.context,
      noFreshUpload: true,
      uploadAttempt: {
        outcome: "rejected",
        run_id: "42",
        run_attempt: "1",
      },
    }),
    (error) => {
      failure = error;
      assert.match(error.message, /rejected/);
      return true;
    },
  );
  assert.equal(f.events.filter(([event]) => event === "publish").length, 0);
  assert.equal(failure.category, "npm_authentication");
});
test("completion without a resolved upload attempt cannot authorize a write", async () => {
  const f = fixture({ tagged: true });
  let failure;
  await assert.rejects(
    publishPublication(f.io, { ...f.context, noFreshUpload: true }),
    (error) => {
      failure = error;
      assert.match(error.message, /upload history/);
      return true;
    },
  );
  assert.equal(f.events.filter(([event]) => event === "publish").length, 0);
  assert.equal(failure.category, "upload_history");
});

test("release completion retries transient metadata and archive reads", async () => {
  const f = fixture({
    tagged: true,
    published: true,
    latest: source.record.sdk_version,
  });
  const readRegistry = f.io.readRegistry;
  let reads = 0;
  f.io.readRegistry = async (...args) => {
    reads += 1;
    if (reads === 1)
      throw Object.assign(new Error("registry metadata HTTP 429"), {
        code: "REGISTRY_READ_FAILURE",
        retryable: true,
      });
    if (reads === 2)
      throw Object.assign(
        new Error("registry metadata was visible; tarball HTTP 503"),
        { code: "REGISTRY_READ_FAILURE", retryable: true },
      );
    return readRegistry(...args);
  };
  const runner = await makeReleaseRunner(f);
  try {
    const summary = await runner.run("complete");
    assert.equal(summary.release_status, "verified");
    assert.equal(reads, 4);
    assert.deepEqual(f.clock.waits, [10_000, 20_000]);
    assert.equal(f.events.filter(([event]) => event === "publish").length, 0);
  } finally {
    await rm(runner.directory, { recursive: true, force: true });
  }
});

test("complete observes an already-public result without upload and promotes only after archive identity matches", async () => {
  const f = fixture({ tagged: true });
  const readRegistry = f.io.readRegistry;
  let reads = 0;
  f.io.readRegistry = async (...args) => {
    reads += 1;
    if (reads === 3) f.state.registry.version = npmVersion();
    const value = await readRegistry(...args);
    f.events.push(["verified-registry-read", Boolean(value.version)]);
    return value;
  };
  const runner = await makeReleaseRunner(f, {
    uploadAttempts: {
      history: async () => [
        {
          outcome: "already_public",
          run_id: "900",
          run_attempt: "1",
          intent_artifact_id: "501",
        },
      ],
    },
  });
  try {
    const summary = await runner.run("complete");
    assert.equal(summary.release_status, "verified");
    assert.equal(summary.upload_attempt.outcome, "already_public");
    assert.equal(f.events.filter(([event]) => event === "publish").length, 0);
    const matchingRead = f.events.findIndex(
      ([event, visible]) => event === "verified-registry-read" && visible,
    );
    const promotion = f.events.findIndex(([event]) => event === "latest");
    assert.ok(matchingRead >= 0 && matchingRead < promotion);
    assert.equal(f.events.filter(([event]) => event === "latest").length, 1);
  } finally {
    await rm(runner.directory, { recursive: true, force: true });
  }
});

test("authorize rejects a fresh write for an absent unapproved first package", async () => {
  const f = fixture({ tagged: true, packageExists: false });
  f.state.candidate.receipt.bootstrap_allowed = false;
  let intents = 0;
  const runner = await makeReleaseRunner(f, {
    uploadAttempts: { writeIntent: async () => (intents += 1) },
  });
  try {
    await assert.rejects(runner.run("authorize"), /absent package/);
    assert.equal(runner.summaries.at(-1).status, "failed");
    assert.equal(intents, 0);
    assert.doesNotMatch(
      await readFile(runner.outputFile, "utf8").catch(() => ""),
      /upload=true/,
    );
  } finally {
    await rm(runner.directory, { recursive: true, force: true });
  }
});

test("authorize bootstrap result includes the candidate used in its summary", async () => {
  const f = fixture({ tagged: true, packageExists: false });
  const runner = await makeReleaseRunner(f);
  const exitCode = process.exitCode;
  try {
    const summary = await runner.run("authorize");
    assert.equal(summary.release_status, "bootstrap_required");
    assert.equal(summary.integrity, f.state.candidate.integrity);
    assert.deepEqual(summary.assets, f.state.candidate.retention.urls);
    assert.match(summary.instructions, /exact archive/);
  } finally {
    process.exitCode = exitCode;
    await rm(runner.directory, { recursive: true, force: true });
  }
});

test("authorize retries a transient registry read before resuming a prior upload", async () => {
  const f = fixture({ tagged: true });
  const readRegistry = f.io.readRegistry;
  let reads = 0;
  f.io.readRegistry = async (...args) => {
    reads += 1;
    if (reads === 1)
      throw Object.assign(new Error("registry metadata HTTP 503"), {
        code: "REGISTRY_READ_FAILURE",
        retryable: true,
      });
    return readRegistry(...args);
  };
  let intents = 0;
  const runner = await makeReleaseRunner(f, {
    uploadAttempts: {
      authorize: async () => ({
        action: "observe",
        attempt: { outcome: "accepted", run_id: "800", run_attempt: "1" },
      }),
      writeIntent: async () => (intents += 1),
    },
  });
  try {
    const summary = await runner.run("authorize");
    assert.equal(summary.release_status, "observing_prior_upload");
    assert.equal(summary.upload_attempt.outcome, "accepted");
    assert.equal(reads, 2);
    assert.deepEqual(f.clock.waits, [10_000]);
    assert.equal(intents, 0);
  } finally {
    await rm(runner.directory, { recursive: true, force: true });
  }
});

test("upload rechecks first-package readiness after the package disappears", async () => {
  const f = fixture({ tagged: true, packageExists: true });
  f.state.candidate.receipt.bootstrap_allowed = false;
  const runner = await makeReleaseRunner(f);
  try {
    assert.equal(
      (await runner.run("authorize")).release_status,
      "upload_authorized",
    );
    f.state.registry.packageExists = false;
    await assert.rejects(runner.run("upload"), /absent package/);
    assert.equal(runner.summaries.at(-1).status, "failed");
    assert.equal(f.events.filter(([event]) => event === "publish").length, 0);
  } finally {
    await rm(runner.directory, { recursive: true, force: true });
  }
});

test("upload records an E403 with authentication diagnostics as uncertain", async () => {
  const f = fixture({
    tagged: true,
    publishError: Object.assign(new Error("npm error code E403"), {
      npmCode: "E403",
      category: "authentication",
    }),
  });
  let recordedOutcome = null;
  const runner = await makeReleaseRunner(f, {
    uploadAttempts: {
      writeResult: async (
        _source,
        _candidate,
        _workflow,
        _intentId,
        outcome,
      ) => {
        recordedOutcome = outcome;
      },
    },
  });
  try {
    const summary = await runner.run("upload");
    assert.equal(summary.status, "upload_attempt_recorded");
    assert.equal(summary.upload_attempt.outcome, "uncertain");
    assert.equal(recordedOutcome, "uncertain");
  } finally {
    await rm(runner.directory, { recursive: true, force: true });
  }
});

test("upload records a failure before npm starts as never_started", async () => {
  const f = fixture({ tagged: true });
  f.io.readRegistry = async () => {
    throw Object.assign(new Error("registry authentication HTTP 403"), {
      code: "REGISTRY_READ_FAILURE",
      retryable: false,
    });
  };
  let verifiedIntent = false;
  let recordedOutcome;
  const runner = await makeReleaseRunner(f, {
    uploadAttempts: {
      verifyRetainedIntent: async () => {
        verifiedIntent = true;
        return { artifact_id: "501" };
      },
      writeResult: async (_source, _candidate, _workflow, _id, outcome) => {
        recordedOutcome = outcome;
      },
    },
  });
  try {
    await assert.rejects(runner.run("upload"), /HTTP 403/);
    assert.equal(verifiedIntent, true);
    assert.equal(recordedOutcome, "never_started");
    assert.equal(f.events.filter(([event]) => event === "publish").length, 0);
  } finally {
    await rm(runner.directory, { recursive: true, force: true });
  }
});

test("complete preserves a visible accepted attempt before checking OIDC provenance", async () => {
  const f = fixture({ tagged: true, published: true });
  f.state.registry.version.provenance = false;
  let historyReads = 0;
  const runner = await makeReleaseRunner(f, {
    uploadAttempts: {
      history: async () => {
        historyReads += 1;
        return [{ outcome: "accepted", run_id: "800", run_attempt: "1" }];
      },
    },
  });
  try {
    await assert.rejects(runner.run("complete"), /workflow provenance/);
    assert.equal(historyReads, 1);
    assert.equal(f.events.filter(([event]) => event === "publish").length, 0);
    assert.equal(f.events.filter(([event]) => event === "consumers").length, 0);
  } finally {
    await rm(runner.directory, { recursive: true, force: true });
  }
});

test("complete does not treat unreadable visible upload history as manual bootstrap", async () => {
  const f = fixture({ tagged: true, published: true });
  f.state.registry.version.provenance = false;
  const runner = await makeReleaseRunner(f, {
    uploadAttempts: {
      history: async () => {
        throw new Error("authenticated upload history is unavailable");
      },
    },
  });
  try {
    await assert.rejects(
      runner.run("complete"),
      /authenticated upload history is unavailable/,
    );
    assert.equal(f.events.filter(([event]) => event === "latest").length, 0);
    assert.equal(f.events.filter(([event]) => event === "consumers").length, 0);
  } finally {
    await rm(runner.directory, { recursive: true, force: true });
  }
});

test("complete preserves a genuine manual bootstrap with complete empty history", async () => {
  const f = fixture({ tagged: true, published: true });
  f.state.registry.version.provenance = false;
  const runner = await makeReleaseRunner(f, {
    uploadAttempts: { history: async () => [] },
  });
  try {
    const summary = await runner.run("complete");
    assert.equal(summary.origin, "manual_bootstrap");
    assert.equal(f.events.filter(([event]) => event === "publish").length, 0);
  } finally {
    await rm(runner.directory, { recursive: true, force: true });
  }
});
test("release runner retries an accepted upload by observation without a second write", async () => {
  const f = fixture({
    tagged: true,
    packageExists: false,
    versionVisibleAt: 90_000,
  });
  const directory = await mkdtemp(join(tmpdir(), "sdk-publish-runner-"));
  const outputFile = join(directory, "output.txt");
  const summaries = [];
  const uploadAttempts = {
    outcome: null,
    intentCount: 0,
    resultCount: 0,
    async authorize() {
      if (this.outcome)
        return {
          action: "observe",
          attempt: {
            outcome: this.outcome,
            run_id: "901",
            run_attempt: "1",
            observing_prior_write: true,
          },
        };
      return { action: "upload" };
    },
    async writeIntent() {
      this.intentCount += 1;
    },
    async verifyRetainedIntent() {
      return { artifact_id: "501" };
    },
    async writeResult(_source, _candidate, _workflow, _id, outcome) {
      this.outcome = outcome;
      this.resultCount += 1;
    },
    async history() {
      return this.outcome
        ? [
            {
              outcome: this.outcome,
              run_id: "901",
              run_attempt: "1",
            },
          ]
        : [];
    },
  };
  f.io.uploadAttempts = uploadAttempts;
  const releaseSource = {
    ...source,
    currentCommit: source.commit,
    currentHostTag: source.hostTag,
    packConfig: { node: "24.19.0", npm: "11.21.0" },
  };
  const repository = {
    loadSource: async () => releaseSource,
    text: async () => releaseSource.currentCommit,
  };
  const env = {
    GITHUB_ACTIONS: "true",
    GITHUB_REPOSITORY: "kdlbs/kandev",
    GITHUB_EVENT_NAME: "workflow_dispatch",
    GITHUB_REF: "refs/heads/main",
    GITHUB_WORKFLOW_REF:
      "kdlbs/kandev/.github/workflows/release.yml@refs/heads/main",
    RUNNER_ENVIRONMENT: "github-hosted",
    SDK_SCOPE_OWNERSHIP_CONFIRMED: "true",
    SDK_FIRST_PUBLISH_OIDC_READY: "true",
    RELEASE_GPG_FINGERPRINT: fingerprint,
    SDK_WORKFLOW_SHA: "d".repeat(40),
    SDK_BACKFILL_TAG: "",
    SDK_UPLOAD_STATE_DIR: directory,
    SDK_UPLOAD_INTENT_ARTIFACT_ID: "501",
    SDK_UPLOAD_INTENT_ARTIFACT_DIGEST: "e".repeat(64),
    GITHUB_RUN_ID: "901",
    GITHUB_RUN_ATTEMPT: "1",
    GITHUB_OUTPUT: outputFile,
  };
  const run = (mode) =>
    runRelease({
      mode,
      hostTag: source.hostTag,
      root: directory,
      env,
      makeRepository: () => repository,
      makeDirectory: async () => join(directory, `work-${mode}`),
      removeDirectory: async () => {},
      summaryWriter: async (summary) => summaries.push(summary),
      verifyToolsImpl: () => {},
      execFileImpl: async (command) => ({
        stdout: command === "npm" ? "11.21.0" : "",
      }),
      makeReleaseIO: ({ diagnostics }) => {
        f.io.diagnostics = diagnostics;
        return f.io;
      },
      diagnosticsLog: null,
      diagnosticsError: () => {},
    });
  try {
    await run("authorize");
    assert.match(await readFile(outputFile, "utf8"), /upload=true/);
    await run("upload");
    assert.equal(uploadAttempts.outcome, "accepted");
    env.SDK_FIRST_PUBLISH_OIDC_READY = "false";
    f.state.registry.packageExists = false;
    env.GITHUB_RUN_ATTEMPT = "2";
    env.SDK_UPLOAD_INTENT_ARTIFACT_ID = "";
    env.SDK_UPLOAD_INTENT_ARTIFACT_DIGEST = "";
    await run("authorize");
    assert.match(await readFile(outputFile, "utf8"), /upload=false/);
    const summary = await run("complete");
    assert.equal(summary.release_status, "verified");
    assert.equal(summary.upload_attempt.outcome, "accepted");
    assert.equal(summary.upload_attempt.observing_prior_write, true);
    assert.equal(f.events.filter(([event]) => event === "publish").length, 1);
    assert.equal(uploadAttempts.intentCount, 1);
    assert.equal(uploadAttempts.resultCount, 1);
    assert.equal(f.clock.now, 90_000);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});
test("uncertain latest write is reconciled without repeating the dist-tag mutation", async () => {
  const f = fixture({
    tagged: true,
    published: true,
    latestVisibleAt: 60_000,
    latestError: new Error("connection lost"),
  });
  const result = await publishPublication(f.io, f.context);
  assert.equal(result.status, "verified");
  assert.equal(f.events.filter(([event]) => event === "latest").length, 1);
  assert.equal(f.clock.now, 60_000);
  assert.equal(f.events.at(-1)[0], "consumers");
});
test("verified registry version appears in observation and success summary", async () => {
  const f = fixture({ tagged: true });
  const diagnostics = new SdkDiagnostics({
    hostTag: "v0.100.0",
    sdkVersion: "0.1.0",
    log: null,
  });
  f.io.diagnostics = diagnostics;
  await preparePublication(f.io, f.context);
  const result = await publishPublication(f.io, f.context);
  const summary = diagnostics.summary({ status: result.status, result });
  const visible = summary.observations.filter(
    (event) => event.phase === "version_visibility" && event.visible,
  );
  assert.equal(visible.length, 1);
  assert.equal(visible[0].observed_version, "0.1.0");
  assert.equal(visible[0].write_outcome, "accepted");
  assert.equal(summary.phases.version_visibility, "passed");
  assert.equal(summary.publication, "confirmed");
});
test("rejected latest update keeps immutable publication confirmed", async () => {
  const f = fixture({ tagged: true, published: true });
  const diagnostics = new SdkDiagnostics({
    hostTag: "v0.100.0",
    sdkVersion: "0.1.0",
    log: null,
  });
  f.io.diagnostics = diagnostics;
  f.io.moveLatest = async () => {
    throw Object.assign(new Error("dist-tag update failed"), {
      npmCode: "E403",
    });
  };
  const failure = await publishPublication(f.io, f.context).then(
    () => null,
    (error) => error,
  );
  assert.ok(failure);
  assert.match(failure.message, /dist-tag/);
  diagnostics.fail(failure);
  const summary = diagnostics.summary({ status: "failed", failure });
  assert.equal(summary.publication, "confirmed");
  assert.equal(summary.channel_promotion, "failed");
  assert.equal(summary.channel_write_outcome, "rejected");
  assert.equal(summary.last_confirmed_phase, "version_visibility");
  assert.equal(summary.failure_category, "authentication");
  assert.match(summary.recovery_action, /trusted publisher/);
});
test("accepted latest update that never appears keeps publication confirmed", async () => {
  const f = fixture({
    tagged: true,
    published: true,
    latestVisibleAt: Infinity,
  });
  const diagnostics = new SdkDiagnostics({
    hostTag: "v0.100.0",
    sdkVersion: "0.1.0",
    log: null,
  });
  f.io.diagnostics = diagnostics;
  const failure = await publishPublication(f.io, f.context).then(
    () => null,
    (error) => error,
  );
  assert.match(failure.message, /10 minutes/);
  diagnostics.fail(failure);
  const summary = diagnostics.summary({ status: "failed", failure });
  assert.equal(summary.publication, "confirmed");
  assert.equal(summary.channel_promotion, "failed");
  assert.equal(summary.channel_write_outcome, "accepted");
  assert.equal(
    summary.failure_category,
    "accepted_channel_awaiting_visibility",
  );
  assert.match(summary.recovery_action, /latest dist-tag/);
});
test("uncertain latest update that never appears keeps publication confirmed", async () => {
  const f = fixture({
    tagged: true,
    published: true,
    latestVisibleAt: Infinity,
    latestError: new Error("connection lost"),
  });
  const diagnostics = new SdkDiagnostics({
    hostTag: "v0.100.0",
    sdkVersion: "0.1.0",
    log: null,
  });
  f.io.diagnostics = diagnostics;
  const failure = await publishPublication(f.io, f.context).then(
    () => null,
    (error) => error,
  );
  assert.match(failure.message, /10 minutes/);
  diagnostics.fail(failure);
  const summary = diagnostics.summary({ status: "failed", failure });
  assert.equal(summary.publication, "confirmed");
  assert.equal(summary.channel_write_outcome, "uncertain");
  assert.equal(summary.failure_category, "uncertain_channel_update");
  assert.match(summary.recovery_action, /latest dist-tag/);
});
test("preparation failure marks candidate verification failed", async () => {
  const f = fixture();
  const diagnostics = new SdkDiagnostics({
    hostTag: "v0.100.0",
    sdkVersion: "0.1.0",
    log: null,
  });
  f.io.diagnostics = diagnostics;
  f.io.finalizeCandidate = async () => {
    throw new Error("candidate finalization failed");
  };
  const failure = await preparePublication(f.io, f.context).then(
    () => null,
    (error) => error,
  );
  assert.match(failure.message, /finalization/);
  diagnostics.fail(failure);
  const summary = diagnostics.summary({ status: "failed", failure });
  assert.equal(summary.phases.candidate_verification, "failed");
  assert.equal(summary.last_confirmed_phase, "source_verification");
});
test("registry diagnostic values are bounded", () => {
  const bounded = boundedRegistryValue("a".repeat(200));
  assert.equal(bounded.length, 128);
  assert.ok(bounded.endsWith("..."));
  assert.equal(boundedRegistryValue(null), null);
});
