import assert from "node:assert/strict";
import test from "node:test";
import {
  preparePublication,
  publishPublication,
  REGISTRY_OBSERVATION_WINDOW_MS,
} from "./sdk-publication-state.mjs";
import { SdkDiagnostics } from "./sdk-diagnostics.mjs";

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
test("permanent absence exhausts the five-minute window with bounded waits and one upload", async () => {
  const f = fixture({ tagged: true, versionVisibleAt: Infinity });
  await assert.rejects(
    publishPublication(f.io, f.context),
    /accepted by npm.*5 minutes/,
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
  assert.equal(f.clock.reads.length, f.clock.waits.length + 1);
});
test("no registry probe starts after the five-minute deadline", async () => {
  const f = fixture({ tagged: true, versionVisibleAt: Infinity });
  await assert.rejects(publishPublication(f.io, f.context), /5 minutes/);
  assert.ok(
    Math.max(...f.clock.reads) < REGISTRY_OBSERVATION_WINDOW_MS,
    f.clock.reads,
  );
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
    throw Object.assign(new Error("dist-tag permission denied"), {
      code: "E403",
      stderr: "npm error code E403",
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
  assert.match(failure.message, /5 minutes/);
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
  assert.match(failure.message, /5 minutes/);
  diagnostics.fail(failure);
  const summary = diagnostics.summary({ status: "failed", failure });
  assert.equal(summary.publication, "confirmed");
  assert.equal(summary.channel_write_outcome, "uncertain");
  assert.equal(summary.failure_category, "uncertain_channel_update");
  assert.match(summary.recovery_action, /latest dist-tag/);
});
