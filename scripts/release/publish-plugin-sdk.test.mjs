import assert from "node:assert/strict";
import test from "node:test";
import {
  preparePublication,
  publishPublication,
} from "./sdk-publication-state.mjs";

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
  integrity: "sha512-example",
  recordDigest: source.recordDigest,
  provenance: true,
});
function fixture({
  tagged = false,
  published = false,
  packageExists = true,
  latest = null,
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
  const events = [];
  const io = {
    readTag: async () => structuredClone(state.tag),
    readRegistry: async () => structuredClone(state.registry),
    obtainCandidate: async (_, options) => {
      events.push(["obtain", options]);
      return state.candidate;
    },
    retainCandidate: async () => events.push(["retain"]),
    createTag: async () => {
      events.push(["tag"]);
      state.tag = tag();
    },
    publish: async () => {
      events.push(["publish"]);
      state.registry.packageExists = true;
      state.registry.version = npmVersion();
    },
    moveLatest: async (version) => {
      events.push(["latest", version]);
      state.registry.latest = version;
    },
    verifyNewerLatest: async () => events.push(["verify-newer"]),
    verifyConsumers: async () => events.push(["consumers"]),
    wait: async () => {},
  };
  return {
    io,
    state,
    events,
    context: { source, fingerprint, firstPublishOIDCReady: true },
  };
}
// @covers AC-PLUGINS-SDK-DISTRIBUTION-003.2
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
