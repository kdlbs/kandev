import assert from "node:assert/strict";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test, { afterEach } from "node:test";
import {
  buildEntry,
  buildIndex,
  parseManifestFields,
  parsePluginsYaml,
  readPriorDocument,
  readResponseBytes,
} from "./build-index.mjs";

const realFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = realFetch;
});

/** Stub global fetch, routing by URL to a release / manifest / repo response. */
function stubGitHub({ release, manifestText, repoMeta }) {
  globalThis.fetch = async (url) => {
    const u = String(url);
    if (u.includes("/releases/latest")) return jsonResponse(release);
    if (u.includes("/manifest.yaml")) return textResponse(manifestText ?? "");
    if (u.includes("/repos/")) return jsonResponse(repoMeta);
    throw new Error(`unexpected fetch: ${u}`);
  };
}

const verifiedPackage = (overrides = {}) => ({
  id: "foo",
  version: "1.2.0",
  sha256: "a".repeat(64),
  signed: false,
  ...overrides,
});

const jsonResponse = (body, ok = body !== null) => ({
  ok,
  status: ok ? 200 : 500,
  json: async () => body ?? {},
  text: async () => JSON.stringify(body ?? {}),
});
const textResponse = (text) => ({
  ok: text !== "",
  status: text ? 200 : 404,
  text: async () => text,
});

test("parsePluginsYaml reads the constrained pointer list", () => {
  const specs = parsePluginsYaml(
    [
      "# a comment",
      "plugins:",
      "  - id: hello",
      "    repo: kdlbs/kandev-plugin-hello",
      "    featured: true",
      "  - id: agent-stats",
      "    repo: kdlbs/kandev-plugin-agent-stats",
      "    categories: [analytics, ops]",
    ].join("\n"),
  );
  assert.equal(specs.length, 2);
  assert.deepEqual(specs[0], {
    id: "hello",
    repo: "kdlbs/kandev-plugin-hello",
    featured: true,
  });
  assert.deepEqual(specs[1].categories, ["analytics", "ops"]);
});

test("parseManifestFields extracts presentation keys and ignores the rest", () => {
  const fields = parseManifestFields(
    [
      "id: hello",
      "api_version: 1",
      'display_name: "Hello"',
      "description: A starter plugin",
      "icon: icon.svg",
      "categories: [getting-started]",
      "min_kandev_version: 0.72.0",
      "capabilities:",
      "  state: true",
    ].join("\n"),
  );
  assert.equal(fields.display_name, "Hello");
  assert.equal(fields.description, "A starter plugin");
  assert.equal(fields.icon, "icon.svg");
  assert.equal(fields.min_kandev_version, "0.72.0");
  assert.deepEqual(fields.categories, ["getting-started"]);
  assert.equal("id" in fields, false);
});

test("parseManifestFields reads block-sequence categories", () => {
  const fields = parseManifestFields(
    [
      "display_name: Multi",
      "categories:",
      "  - integrations",
      "  - analytics",
      "author: kandev",
    ].join("\n"),
  );
  assert.deepEqual(fields.categories, ["integrations", "analytics"]);
  assert.equal(fields.author, "kandev");
});

test("readResponseBytes stops a chunked package at the download limit", async () => {
  const response = new Response(
    new ReadableStream({
      start(controller) {
        controller.enqueue(new Uint8Array([1, 2, 3]));
        controller.enqueue(new Uint8Array([4, 5, 6]));
        controller.close();
      },
    }),
  );

  await assert.rejects(
    readResponseBytes(response, 5),
    /download exceeds 5 bytes/,
  );
});

test("buildEntry rejects an unsafe curated ID before any network request", async () => {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () => {
    throw new Error("network must not be reached");
  };
  try {
    const result = await buildEntry({ id: "../escape", repo: "acme/plugin" });
    assert.match(result.error, /unsafe curated plugin ID/);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("readPriorDocument identifies the unreadable retention source", async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), "registry-prior-test-"),
  );
  const priorPath = path.join(directory, "prior.json");
  await fs.writeFile(priorPath, "not json");
  try {
    await assert.rejects(readPriorDocument(priorPath), new RegExp(priorPath));
  } finally {
    await fs.rm(directory, { recursive: true, force: true });
  }
});

test("buildEntry resolves release, manifest, icon_url and stars", async () => {
  stubGitHub({
    release: {
      tag_name: "v1.2.0",
      name: "Release notes",
      published_at: "2026-01-01T00:00:00Z",
      assets: [
        {
          name: "foo-1.2.0.tar.gz",
          browser_download_url: "https://dl/foo-1.2.0.tar.gz",
        },
      ],
    },
    manifestText:
      "display_name: Foo\ndescription: A foo\nauthor: kandev\nicon: icon.svg\ncategories: [x]",
    repoMeta: {
      stargazers_count: 42,
      pushed_at: "2026-02-02T00:00:00Z",
      owner: { login: "acme" },
    },
  });

  const { record, error } = await buildEntry(
    { id: "foo", repo: "acme/foo" },
    { verifyPackage: async () => verifiedPackage() },
  );
  assert.equal(error, undefined);
  assert.equal(record.name, "Foo");
  assert.equal(record.version, "1.2.0");
  assert.equal(record.author, "kandev");
  assert.equal(record.package_url, "https://dl/foo-1.2.0.tar.gz");
  assert.equal(record.package_sha256, "a".repeat(64));
  assert.equal(
    record.icon_url,
    "https://raw.githubusercontent.com/acme/foo/v1.2.0/icon.svg",
  );
  assert.equal(record.stars, 42);
  assert.equal(record.updated_at, "2026-02-02T00:00:00Z");
  assert.deepEqual(record.categories, ["x"]);
});

test("buildEntry errors (not throws) when there is no installable release", async () => {
  stubGitHub({ release: null });
  const { record, error } = await buildEntry({ id: "foo", repo: "acme/foo" });
  assert.equal(record, undefined);
  assert.match(error, /no latest release/);
});

test("buildEntry keeps stars null (never 0) when repo metadata lookup fails", async () => {
  stubGitHub({
    release: {
      tag_name: "1.0.0",
      assets: [
        {
          name: "foo-1.0.0.tar.gz",
          browser_download_url: "https://dl/foo.tar.gz",
        },
      ],
    },
    manifestText: "",
    repoMeta: null, // -> !ok -> throw -> caught
  });
  const { record } = await buildEntry(
    { id: "foo", repo: "acme/foo" },
    { verifyPackage: async () => verifiedPackage({ version: "1.0.0" }) },
  );
  assert.equal(record.stars, null);
  assert.equal(record.author, "acme"); // legacy fallback when the manifest has no author
  assert.equal(record.icon_url, null); // no manifest icon
});

test("empty plugins.yaml parses to no specs and builds a valid empty index", async () => {
  assert.deepEqual(parsePluginsYaml("plugins: []"), []);
  const { document, errors } = await buildIndex([]);
  assert.equal(document.plugins.length, 0);
  assert.equal(errors.length, 0);
  assert.equal(document.schema_version, 1);
  assert.equal(document.source.name, "Kandev Official");
});

test("buildIndex retains a bad entry while still building good peers", async () => {
  // First entry has a release, second does not.
  let call = 0;
  globalThis.fetch = async (url) => {
    const u = String(url);
    if (u.includes("/releases/latest")) {
      call += 1;
      return call === 1
        ? jsonResponse({
            tag_name: "1.0.0",
            assets: [
              { name: "a-1.0.0.tar.gz", browser_download_url: "https://dl/a" },
            ],
          })
        : jsonResponse(null);
    }
    if (u.includes("/manifest.yaml")) return textResponse("");
    return jsonResponse({ stargazers_count: 1, owner: { login: "o" } });
  };

  const { document, errors } = await buildIndex(
    [
      { id: "a", repo: "o/a" },
      { id: "b", repo: "o/b" },
    ],
    {
      priorDocument: {
        schema_version: 1,
        plugins: [priorRecord("b", "o/b", "0.9.0")],
      },
      verifyPackage: async ({ pluginId, version }) =>
        verifiedPackage({ id: pluginId, version }),
    },
  );
  assert.equal(document.plugins.length, 2);
  assert.equal(document.plugins[0].id, "a");
  assert.equal(document.plugins[1].id, "b");
  assert.equal(errors.length, 1);
  assert.equal(document.schema_version, 1);
});

test("buildEntry refuses a differently named tarball instead of falling back", async () => {
  stubGitHub({
    release: {
      tag_name: "v1.2.0",
      assets: [
        {
          name: "some-other-plugin-1.2.0.tar.gz",
          browser_download_url: "https://dl/wrong",
        },
      ],
    },
  });
  let verifierCalled = false;

  const { record, error } = await buildEntry(
    { id: "foo", repo: "acme/foo" },
    {
      verifyPackage: async () => {
        verifierCalled = true;
        return verifiedPackage();
      },
    },
  );

  assert.equal(record, undefined);
  assert.match(error, /exact asset foo-1\.2\.0\.tar\.gz/);
  assert.equal(verifierCalled, false);
});

test("buildEntry rejects verifier identity that differs from the curated release", async () => {
  stubGitHub({
    release: {
      tag_name: "v1.2.0",
      assets: [
        { name: "foo-1.2.0.tar.gz", browser_download_url: "https://dl/foo" },
      ],
    },
    manifestText: "display_name: Foo",
    repoMeta: { owner: { login: "acme" } },
  });

  const { record, error } = await buildEntry(
    { id: "foo", repo: "acme/foo" },
    { verifyPackage: async () => verifiedPackage({ id: "evil" }) },
  );

  assert.equal(record, undefined);
  assert.match(error, /verified package identity/);
});

test("buildEntry rejects a release checksum that differs from the verified package", async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), "registry-verifier-test-"),
  );
  const verifierPath = path.join(directory, "plugin-package-verify");
  await fs.writeFile(
    verifierPath,
    `#!${process.execPath}\nprocess.stdout.write(${JSON.stringify(
      JSON.stringify(verifiedPackage()),
    )});\n`,
    { mode: 0o755 },
  );
  const originalVerifier = process.env.PLUGIN_PACKAGE_VERIFIER;
  process.env.PLUGIN_PACKAGE_VERIFIER = verifierPath;
  globalThis.fetch = async (url) => {
    const requested = String(url);
    if (requested.includes("/releases/latest")) {
      return jsonResponse({
        tag_name: "v1.2.0",
        assets: [
          {
            name: "foo-1.2.0.tar.gz",
            browser_download_url: "https://dl/foo-1.2.0.tar.gz",
          },
          {
            name: "checksums.txt",
            browser_download_url: "https://dl/checksums.txt",
          },
        ],
      });
    }
    if (requested.endsWith("foo-1.2.0.tar.gz")) {
      return new Response("package bytes");
    }
    if (requested.endsWith("checksums.txt")) {
      return new Response(`${"b".repeat(64)}  foo-1.2.0.tar.gz\n`);
    }
    throw new Error(`unexpected fetch: ${requested}`);
  };

  try {
    const { record, error } = await buildEntry({ id: "foo", repo: "acme/foo" });
    assert.equal(record, undefined);
    assert.match(error, /release checksum mismatch/);
  } finally {
    if (originalVerifier === undefined)
      delete process.env.PLUGIN_PACKAGE_VERIFIER;
    else process.env.PLUGIN_PACKAGE_VERIFIER = originalVerifier;
    await fs.rm(directory, { recursive: true, force: true });
  }
});

test("buildIndex retains only still-curated prior records while valid peers advance", async () => {
  const specs = [
    { id: "a", repo: "o/a" },
    { id: "b", repo: "o/b" },
  ];
  const priorDocument = {
    schema_version: 1,
    plugins: [
      priorRecord("a", "o/a", "1.0.0"),
      priorRecord("b", "o/b", "1.0.0"),
      priorRecord("delisted", "o/delisted", "9.0.0"),
    ],
  };

  const result = await buildIndex(specs, {
    priorDocument,
    buildEntryFn: async (spec) =>
      spec.id === "a"
        ? { record: priorRecord("a", "o/a", "2.0.0") }
        : { error: "b: release package failed integrity verification" },
  });

  assert.equal(result.publishable, true);
  assert.deepEqual(
    result.document.plugins.map(({ id, version }) => ({ id, version })),
    [
      { id: "a", version: "2.0.0" },
      { id: "b", version: "1.0.0" },
    ],
  );
  assert.deepEqual(result.retained, ["b"]);
});

test("buildIndex refuses publication when a failed curated entry has no trustworthy prior", async () => {
  const result = await buildIndex([{ id: "a", repo: "o/a" }], {
    priorDocument: { schema_version: 1, plugins: [] },
    buildEntryFn: async () => ({ error: "a: missing exact asset" }),
  });

  assert.equal(result.publishable, false);
  assert.deepEqual(result.document.plugins, []);
  assert.match(result.fatalErrors[0], /no trusted prior record/);
});

test("buildIndex refuses an all-retained rebuild so a provider outage leaves Pages untouched", async () => {
  const result = await buildIndex([{ id: "a", repo: "o/a" }], {
    priorDocument: {
      schema_version: 1,
      plugins: [priorRecord("a", "o/a", "1.0.0")],
    },
    buildEntryFn: async () => ({ error: "a: GitHub provider unavailable" }),
  });

  assert.equal(result.publishable, false);
  assert.deepEqual(result.retained, ["a"]);
  assert.match(result.fatalErrors.at(-1), /no fresh entries/);
});

function priorRecord(id, repo, version) {
  return {
    id,
    name: id,
    description: "",
    author: repo.split("/")[0],
    categories: [],
    icon_url: null,
    repo_url: `https://github.com/${repo}`,
    version,
    min_kandev_version: null,
    package_url: `https://example.test/${id}-${version}.tar.gz`,
    package_sha256: null,
    stars: 1,
    updated_at: "2026-01-01T00:00:00Z",
  };
}
