import assert from "node:assert/strict";
import test from "node:test";
import { readPackage } from "./sdk-registry.mjs";

function response(status, body, type = "application/json") {
  const result = new Response(
    typeof body === "string" ? body : JSON.stringify(body),
    { status, headers: { "content-type": type } },
  );
  Object.defineProperty(result, "url", {
    value: "https://registry.npmjs.org/@kdlbs%2fplugin-sdk",
  });
  return result;
}
test("distinguishes confirmed package absence from absent version and unreadable state", async () => {
  const absent = await readPackage("0.1.0", async (url) => {
    assert.equal(url, "https://registry.npmjs.org/@kdlbs%2fplugin-sdk");
    return response(404, { error: "Not found" });
  });
  assert.equal(absent.packageExists, false);
  const existing = await readPackage("0.1.0", async () =>
    response(200, {
      name: "@kdlbs/plugin-sdk",
      versions: {},
      "dist-tags": {},
    }),
  );
  assert.equal(existing.packageExists, true);
  assert.equal(existing.version, null);
  for (const reply of [
    response(403, { error: "Forbidden" }),
    response(500, {}),
    response(404, "<html>not here</html>", "text/html"),
    response(200, {}),
    response(200, "{"),
  ])
    await assert.rejects(
      readPackage("0.1.0", async () => reply),
      /registry/,
    );
  await assert.rejects(
    readPackage("0.1.0", async () => {
      throw new Error("network timeout");
    }),
    /timeout/,
  );
});
test("validates selected registry identity and archive integrity metadata", async () => {
  const version = {
    name: "@kdlbs/plugin-sdk",
    version: "0.1.0",
    dist: {
      integrity: "sha512-" + "A".repeat(86) + "==",
      tarball:
        "https://registry.npmjs.org/@kdlbs/plugin-sdk/-/plugin-sdk-0.1.0.tgz",
    },
  };
  const document = {
    name: "@kdlbs/plugin-sdk",
    versions: { "0.1.0": version },
    "dist-tags": { latest: "0.1.0" },
  };
  assert.equal(
    (await readPackage("0.1.0", async () => response(200, document))).version
      .version,
    "0.1.0",
  );
  for (const dist of [
    { ...version.dist, tarball: "file:///tmp/sdk.tgz" },
    { ...version.dist, integrity: "sha512-invalid" },
  ])
    await assert.rejects(
      readPackage("0.1.0", async () =>
        response(200, {
          ...document,
          versions: { "0.1.0": { ...version, dist } },
        }),
      ),
      /registry/,
    );
});

test("downloads only matching gzip bytes and leaves the destination untouched on failure", async () => {
  const { downloadArchive } = await import("./sdk-registry.mjs");
  const { mkdtemp, writeFile, readFile, rm } = await import("node:fs/promises");
  const { tmpdir } = await import("node:os");
  const { join } = await import("node:path");
  const { createHash } = await import("node:crypto");
  const { gzipSync } = await import("node:zlib");
  const root = await mkdtemp(join(tmpdir(), "sdk-download-"));
  try {
    const path = join(root, "candidate.tgz");
    const bytes = gzipSync("exact candidate bytes");
    const integrity =
      "sha512-" + createHash("sha512").update(bytes).digest("base64");
    const fetchArchive = async () => {
      const r = new Response(bytes, {
        headers: { "content-type": "application/octet-stream" },
      });
      Object.defineProperty(r, "url", {
        value: "https://registry.npmjs.org/sdk.tgz",
      });
      return r;
    };
    await writeFile(path, "unchanged");
    await downloadArchive(
      "https://registry.npmjs.org/sdk.tgz",
      path,
      integrity,
      fetchArchive,
    );
    assert.deepEqual(await readFile(path), bytes);
    await assert.rejects(
      downloadArchive(
        "https://registry.npmjs.org/sdk.tgz",
        path,
        "sha512-wrong",
        fetchArchive,
      ),
      /integrity/,
    );
    assert.deepEqual(await readFile(path), bytes);
    await assert.rejects(
      downloadArchive(
        "https://private.invalid/sdk.tgz",
        path,
        integrity,
        fetchArchive,
      ),
      /registry/,
    );
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
