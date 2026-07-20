import assert from "node:assert/strict";
import test, { afterEach } from "node:test";
import {
  buildEntry,
  buildIndex,
  parseManifestFields,
  parsePluginsYaml,
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

const jsonResponse = (body, ok = body !== null) => ({
  ok,
  status: ok ? 200 : 500,
  json: async () => body ?? {},
  text: async () => JSON.stringify(body ?? {}),
});
const textResponse = (text) => ({ ok: text !== "", status: text ? 200 : 404, text: async () => text });

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
  assert.deepEqual(specs[0], { id: "hello", repo: "kdlbs/kandev-plugin-hello", featured: true });
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
    ["display_name: Multi", "categories:", "  - integrations", "  - analytics", "author: kandev"].join(
      "\n",
    ),
  );
  assert.deepEqual(fields.categories, ["integrations", "analytics"]);
  assert.equal(fields.author, "kandev");
});

test("buildEntry resolves release, manifest, icon_url and stars", async () => {
  stubGitHub({
    release: {
      tag_name: "v1.2.0",
      name: "Release notes",
      published_at: "2026-01-01T00:00:00Z",
      assets: [{ name: "foo-1.2.0.tar.gz", browser_download_url: "https://dl/foo-1.2.0.tar.gz" }],
    },
    manifestText: "display_name: Foo\ndescription: A foo\nicon: icon.svg\ncategories: [x]",
    repoMeta: { stargazers_count: 42, pushed_at: "2026-02-02T00:00:00Z", owner: { login: "acme" } },
  });

  const { record, error } = await buildEntry({ id: "foo", repo: "acme/foo" });
  assert.equal(error, undefined);
  assert.equal(record.name, "Foo");
  assert.equal(record.version, "1.2.0");
  assert.equal(record.package_url, "https://dl/foo-1.2.0.tar.gz");
  assert.equal(record.icon_url, "https://raw.githubusercontent.com/acme/foo/v1.2.0/icon.svg");
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
    release: { tag_name: "1.0.0", assets: [{ name: "foo-1.0.0.tar.gz", browser_download_url: "https://dl/foo.tar.gz" }] },
    manifestText: "",
    repoMeta: null, // -> !ok -> throw -> caught
  });
  const { record } = await buildEntry({ id: "foo", repo: "acme/foo" });
  assert.equal(record.stars, null);
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

test("buildIndex skips bad entries but still builds the good ones", async () => {
  // First entry has a release, second does not.
  let call = 0;
  globalThis.fetch = async (url) => {
    const u = String(url);
    if (u.includes("/releases/latest")) {
      call += 1;
      return call === 1
        ? jsonResponse({ tag_name: "1.0.0", assets: [{ name: "a-1.0.0.tar.gz", browser_download_url: "https://dl/a" }] })
        : jsonResponse(null);
    }
    if (u.includes("/manifest.yaml")) return textResponse("");
    return jsonResponse({ stargazers_count: 1, owner: { login: "o" } });
  };

  const { document, errors } = await buildIndex([
    { id: "a", repo: "o/a" },
    { id: "b", repo: "o/b" },
  ]);
  assert.equal(document.plugins.length, 1);
  assert.equal(document.plugins[0].id, "a");
  assert.equal(errors.length, 1);
  assert.equal(document.schema_version, 1);
});
