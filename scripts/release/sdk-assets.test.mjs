import test from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, writeFile, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createHash } from "node:crypto";
import { ReleaseAssets } from "./sdk-assets.mjs";
import { assetNames } from "./sdk-artifact.mjs";

const source = {
  hostTag: "v0.99.0",
  sdkTag: "pluginsdk/v0.1.0",
  commit: "a".repeat(40),
  record: { sdk_version: "0.1.0" },
};
const digest = (bytes) =>
  `sha256:${createHash("sha256").update(bytes).digest("hex")}`;
async function fixture(t) {
  const root = await mkdtemp(join(tmpdir(), "sdk-assets-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const candidate = {};
  for (const key of ["archive", "receiptPath", "signaturePath"]) {
    candidate[key] = join(root, key);
    await writeFile(candidate[key], key);
  }
  const releases = new Map([
    [
      1,
      {
        id: 1,
        tag_name: source.hostTag,
        draft: false,
        immutable: true,
        prerelease: false,
        target_commitish: source.commit,
        assets: [],
      },
    ],
  ]);
  const payloads = new Map();
  const events = [];
  let id = 10;
  let lost = false;
  let pageError = null;
  let changeRead = (value) => value;
  let created = async () => {};
  let temporaryURLs = false;
  async function put(release, name, bytes) {
    const asset = {
      id: id++,
      name,
      size: bytes.length,
      digest: digest(bytes),
      browser_download_url: `https://github.com/kdlbs/kandev/releases/download/${temporaryURLs && release.draft ? "untagged-draft" : encodeURIComponent(release.tag_name)}/${name}`,
    };
    release.assets.push(asset);
    payloads.set(asset.id, Buffer.from(bytes));
    return asset;
  }
  async function api(args) {
    const endpoint = args[1];
    const method = args.includes("--method")
      ? args[args.indexOf("--method") + 1]
      : "GET";
    if (args[0] === "release") {
      events.push("host-upload");
      throw new Error(
        "HTTP 422: Cannot upload assets to an immutable release.",
      );
    }
    if (endpoint.includes("/releases?")) {
      if (pageError) throw pageError;
      const page = Number(
        new URL(`https://api.github.com/${endpoint}`).searchParams.get(
          "page",
        ) ?? 1,
      );
      events.push(`list-${page}`);
      return {
        stdout: JSON.stringify(
          [...releases.values()].slice((page - 1) * 100, page * 100),
        ),
      };
    }
    if (endpoint.includes("/releases/tags/")) {
      const tag = decodeURIComponent(endpoint.split("/releases/tags/")[1]);
      const value = [...releases.values()].find(
        (release) => release.tag_name === tag,
      );
      if (!value) throw new Error("HTTP 404");
      return { stdout: JSON.stringify(value) };
    }
    if (endpoint.includes("/releases/assets/"))
      return { stdout: payloads.get(Number(endpoint.split("/").at(-1))) };
    if (
      method === "POST" &&
      endpoint.startsWith("https://uploads.github.com/")
    ) {
      const url = new URL(endpoint);
      const release = releases.get(Number(url.pathname.split("/").at(-2)));
      if (!release.draft || release.immutable)
        throw new Error("HTTP 422: immutable release");
      events.push("upload");
      await put(
        release,
        url.searchParams.get("name"),
        await readFile(args[args.indexOf("--input") + 1]),
      );
      if (lost) throw new Error("upload response lost");
      return { stdout: "{}" };
    }
    if (method === "POST" && endpoint.endsWith("/releases")) {
      const body = JSON.parse(
        await readFile(args[args.indexOf("--input") + 1]),
      );
      assert.equal(body.draft, true);
      assert.equal(body.make_latest, "false");
      const value = { ...body, id: id++, immutable: false, assets: [] };
      releases.set(value.id, value);
      events.push("create");
      await created(value);
      if (lost) throw new Error("create response lost");
      return { stdout: JSON.stringify(value) };
    }
    const release = releases.get(Number(endpoint.split("/").at(-1)));
    if (!release) throw new Error(`unexpected command: ${args.join(" ")}`);
    if (method === "PATCH") {
      const body = JSON.parse(
        await readFile(args[args.indexOf("--input") + 1]),
      );
      assert.equal(body.make_latest, "false");
      assert.equal(body.draft, false);
      assert.equal(release.assets.length, 3);
      events.push("finalize");
      release.draft = false;
      release.immutable = true;
      for (const asset of release.assets)
        asset.browser_download_url = `https://github.com/kdlbs/kandev/releases/download/${encodeURIComponent(release.tag_name)}/${asset.name}`;
      if (lost) throw new Error("publish response lost");
    }
    return { stdout: JSON.stringify(changeRead(release)) };
  }
  const assets = new ReleaseAssets({
    root,
    directory: join(root, "retained"),
    run: async (cmd, args) => api(args),
    wait: async () => {},
  });
  return {
    root,
    candidate,
    assets,
    releases,
    payloads,
    events,
    put,
    sdk: () =>
      [...releases.values()].find(
        (release) => release.tag_name === source.sdkTag,
      ),
    loseResponses: () => {
      lost = true;
    },
    unreadable: (error) => {
      pageError = error;
    },
    changeRead: (change) => {
      changeRead = change;
    },
    onCreate: (effect) => {
      created = effect;
    },
    temporaryURLs: () => {
      temporaryURLs = true;
    },
    addSDK: (overrides = {}) => {
      const value = {
        id: id++,
        tag_name: source.sdkTag,
        target_commitish: source.commit,
        draft: true,
        immutable: false,
        prerelease: false,
        assets: [],
        ...overrides,
      };
      releases.set(value.id, value);
      return value;
    },
  };
}
// @covers AC-PLUGINS-SDK-DISTRIBUTION-003.5
test("retains on an SDK draft when the host rejects uploads with HTTP 422", async (t) => {
  const f = await fixture(t);
  await f.assets.retain(f.candidate, source);
  assert.equal(f.sdk().draft, true);
  assert.equal(f.sdk().assets.length, 3);
  assert.equal(f.releases.get(1).assets.length, 0);
  assert.equal(f.events.includes("host-upload"), false);
});
test("retains once and resolves lost create, upload, and finalize responses by reads", async (t) => {
  const f = await fixture(t);
  f.loseResponses();
  await f.assets.retain(f.candidate, source);
  await f.assets.retain(f.candidate, source);
  await f.assets.finalize(f.candidate, source);
  await f.assets.finalize(f.candidate, source);
  assert.equal(f.events.filter((event) => event === "create").length, 1);
  assert.equal(f.events.filter((event) => event === "upload").length, 3);
  assert.equal(f.events.filter((event) => event === "finalize").length, 1);
  assert.equal(f.sdk().draft, false);
  assert.equal(f.candidate.retention.urls.length, 3);
  assert.ok(
    f.candidate.retention.urls.every((url) =>
      url.includes("pluginsdk%2Fv0.1.0"),
    ),
  );
});
test("read-only publication rejects drafts and accepts finalized verified SDK evidence", async (t) => {
  const f = await fixture(t);
  await f.assets.retain(f.candidate, source);
  await assert.rejects(
    f.assets.verifyFinalized(f.candidate, source),
    /draft|finaliz/,
  );
  await f.assets.finalize(f.candidate, source);
  const before = [...f.events];
  await f.assets.verifyFinalized(f.candidate, source);
  assert.deepEqual(
    f.events.filter((event) =>
      ["upload", "create", "finalize"].includes(event),
    ),
    before.filter((event) => ["upload", "create", "finalize"].includes(event)),
  );
});
test("conflicting retained bytes and published incomplete evidence fail before writes", async (t) => {
  for (const published of [false, true]) {
    const f = await fixture(t);
    const sdk = f.addSDK({ draft: !published, immutable: published });
    await f.put(sdk, "plugin-sdk-0.1.0.tgz", Buffer.from("conflicting bytes"));
    await assert.rejects(
      f.assets.retain(f.candidate, source),
      /conflict|incomplete/,
    );
    assert.equal(
      f.events.some((event) =>
        ["upload", "create", "finalize"].includes(event),
      ),
      false,
    );
  }
});
test("draft discovery paginates and rejects wrong identities, duplicates, and access errors", async (t) => {
  const f = await fixture(t);
  for (let n = 0; n < 101; n++)
    f.releases.set(1000 + n, {
      id: 1000 + n,
      tag_name: `v9.0.${n}`,
      assets: [],
    });
  f.addSDK({ target_commitish: "b".repeat(40) });
  await assert.rejects(f.assets.retain(f.candidate, source), /identity|target/);
  assert.ok(f.events.includes("list-2"));
  const duplicate = await fixture(t);
  duplicate.addSDK();
  duplicate.addSDK();
  await assert.rejects(
    duplicate.assets.retain(duplicate.candidate, source),
    /duplicate|ambiguous/,
  );
  const denied = await fixture(t);
  denied.unreadable(new Error("HTTP 403"));
  await assert.rejects(denied.assets.retain(denied.candidate, source), /403/);
  assert.equal(denied.events.includes("create"), false);
});
test("complete legacy evidence is reused without copying or rewriting the host", async (t) => {
  const f = await fixture(t);
  for (const [key, name] of Object.entries(assetNames("0.1.0")))
    await f.put(
      f.releases.get(1),
      name,
      await readFile(
        f.candidate[
          { receipt: "receiptPath", signature: "signaturePath" }[key] ?? key
        ],
      ),
    );
  await f.assets.retain(f.candidate, source);
  await f.assets.finalize(f.candidate, source);
  await f.assets.verifyFinalized(f.candidate, source);
  assert.equal(f.sdk(), undefined);
  assert.equal(
    f.events.some((event) => ["create", "upload", "finalize"].includes(event)),
    false,
  );
  assert.ok(
    f.candidate.retention.urls.every((url) => url.includes(source.hostTag)),
  );
});
test("conflicting storage locations and unrelated partial evidence fail closed", async (t) => {
  const f = await fixture(t);
  await f.put(f.releases.get(1), "plugin-sdk-0.1.0.tgz", Buffer.from("host"));
  await f.put(f.addSDK(), "plugin-sdk-0.1.0.tgz", Buffer.from("other"));
  await assert.rejects(
    f.assets.download(source, assetNames("0.1.0")),
    /conflict/,
  );
  const partial = await fixture(t);
  await partial.put(
    partial.releases.get(1),
    "plugin-sdk-0.1.0.tgz",
    Buffer.from("archive"),
  );
  await partial.put(
    partial.addSDK(),
    "plugin-sdk-0.1.0.receipt.json",
    Buffer.from("receipt"),
  );
  await assert.rejects(
    partial.assets.download(source, assetNames("0.1.0")),
    /partial|ambiguous/,
  );
});
test("fresh reads stay bound to the discovered release ID", async (t) => {
  const f = await fixture(t);
  f.addSDK();
  f.changeRead((release) => ({ ...release, id: release.id + 100 }));
  await assert.rejects(f.assets.retain(f.candidate, source), /identity/);
  assert.equal(
    f.events.some((event) => ["create", "upload", "finalize"].includes(event)),
    false,
  );
});
test("conflicting assets discovered after draft creation stop further writes", async (t) => {
  const f = await fixture(t);
  f.onCreate((release) =>
    f.put(release, "plugin-sdk-0.1.0.tgz", Buffer.from("conflicting archive")),
  );
  await assert.rejects(f.assets.retain(f.candidate, source), /conflict/);
  assert.equal(f.events.includes("upload"), false);
});
test("finalization and read-only verification reject a replacement release with matching bytes", async (t) => {
  for (const method of ["finalize", "verifyFinalized"]) {
    const f = await fixture(t);
    await f.assets.retain(f.candidate, source);
    if (method === "verifyFinalized")
      await f.assets.finalize(f.candidate, source);
    const previous = f.sdk();
    const retained = structuredClone(f.candidate.retention);
    f.releases.delete(previous.id);
    const replacement = f.addSDK({
      draft: previous.draft,
      immutable: previous.immutable,
    });
    for (const asset of previous.assets)
      await f.put(replacement, asset.name, f.payloads.get(asset.id));
    const writes = f.events.filter((event) =>
      ["create", "upload", "finalize"].includes(event),
    );
    await assert.rejects(
      f.assets[method](f.candidate, source),
      /identity|release ID/,
    );
    assert.deepEqual(f.candidate.retention, retained);
    assert.deepEqual(
      f.events.filter((event) =>
        ["create", "upload", "finalize"].includes(event),
      ),
      writes,
    );
  }
});
test("temporary draft URLs are not reported as finalized download links", async (t) => {
  const f = await fixture(t);
  f.temporaryURLs();
  await f.assets.retain(f.candidate, source);
  assert.deepEqual(f.candidate.retention.urls, []);
  const downloaded = await f.assets.download(source, assetNames("0.1.0"));
  assert.equal(downloaded.retention.releaseId, f.sdk().id);
  await f.assets.finalize(f.candidate, source);
  assert.equal(f.candidate.retention.urls.length, 3);
  assert.ok(
    f.candidate.retention.urls.every((url) =>
      url.includes("pluginsdk%2Fv0.1.0"),
    ),
  );
});
test("retained asset metadata must match the downloaded payload before writes", async (t) => {
  for (const field of ["size", "digest"]) {
    const f = await fixture(t);
    const asset = await f.put(
      f.addSDK(),
      "plugin-sdk-0.1.0.tgz",
      Buffer.from("archive"),
    );
    if (field === "size") asset.size++;
    else asset.digest = digest(Buffer.from("other"));
    await assert.rejects(
      f.assets.retain(f.candidate, source),
      /integrity conflict/,
    );
    assert.equal(
      f.events.some((event) =>
        ["create", "upload", "finalize"].includes(event),
      ),
      false,
    );
  }
});
