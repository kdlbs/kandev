import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import fs from "node:fs/promises";
import test, { afterEach } from "node:test";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import {
  buildEntry,
  buildIndex,
  main,
  parseManifestFields,
  parsePluginsYaml,
  pullRequestValidationFailed,
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
const textResponse = (text) => ({
  ok: text !== "",
  status: text ? 200 : 404,
  text: async () => text,
});
const binaryResponse = (body) => ({
  ok: true,
  status: 200,
  body: new ReadableStream({
    start(controller) {
      controller.enqueue(body);
      controller.close();
    },
  }),
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

test("parsePluginsYaml reads ordered canvas previews", () => {
  const specs = parsePluginsYaml(
    [
      "plugins:",
      "  - id: board",
      "    repo: acme/board",
      "    kind: canvas",
      "    previews:",
      "      - url: https://cdn.example/cover.webp",
      "        alt: Board cover",
      "      - url: https://cdn.example/detail.webp",
      "        alt: Board detail",
      "    featured: true",
    ].join("\n"),
  );
  assert.deepEqual(specs[0].previews, [
    { url: "https://cdn.example/cover.webp", alt: "Board cover" },
    { url: "https://cdn.example/detail.webp", alt: "Board detail" },
  ]);
  assert.equal(specs[0].featured, true);
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
      "display_name: Foo\ndescription: A foo\nauthor: Example contributors\nicon: icon.svg\ncategories: [x]",
    repoMeta: {
      id: 123,
      full_name: "acme/foo",
      stargazers_count: 42,
      pushed_at: "2026-02-02T00:00:00Z",
      owner: { id: 456, login: "acme" },
    },
  });

  const { record, error } = await buildEntry({ id: "foo", repo: "acme/foo" });
  assert.equal(error, undefined);
  assert.equal(record.name, "Foo");
  assert.equal(record.version, "1.2.0");
  assert.equal(record.author, "Example contributors");
  assert.equal(record.package_url, "https://dl/foo-1.2.0.tar.gz");
  assert.equal(
    record.icon_url,
    "https://raw.githubusercontent.com/acme/foo/v1.2.0/icon.svg",
  );
  assert.equal(record.stars, 42);
  assert.equal(record.updated_at, "2026-02-02T00:00:00Z");
  assert.deepEqual(record.categories, ["x"]);
});

test("buildEntry rejects a reserved Kandev author from an unrelated repository", async () => {
  stubGitHub({
    release: {
      tag_name: "1.0.0",
      assets: [
        {
          name: "foo-1.0.0.tar.gz",
          browser_download_url: "https://dl/foo-1.0.0.tar.gz",
        },
      ],
    },
    manifestText: "display_name: Foo\nauthor: K A N D E V",
    repoMeta: {
      id: 123,
      full_name: "acme/foo",
      stargazers_count: 1,
      owner: { id: 456, login: "acme" },
    },
  });

  const result = await buildEntry({ id: "foo", repo: "acme/foo" });
  assert.match(result.error, /reserved Kandev author/);
});

test("buildEntry binds publisher evidence and digest to the inspected release archive", async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), "kandev-plugin-registry-native-test-"),
  );
  const inspector = path.join(directory, "inspector.mjs");
  await fs.writeFile(
    inspector,
    `#!/usr/bin/env node\nprocess.stdout.write(JSON.stringify({id:"foo",version:"1.0.0",kind:"plugin",display_name:"Foo",description:"A foo",author:"Example contributors",categories:["tools"],icon:"icon.svg",repo_url:"https://github.com/acme/foo"}));`,
    { mode: 0o755 },
  );
  const previousInspector = process.env.KANDEV_PLUGIN_PACKAGE_INSPECTOR;
  process.env.KANDEV_PLUGIN_PACKAGE_INSPECTOR = inspector;
  const packageBytes = new Uint8Array([9, 8, 7, 6]);
  globalThis.fetch = async (url) => {
    const value = String(url);
    if (value.includes("/releases/latest")) {
      return jsonResponse({
        tag_name: "1.0.0",
        assets: [
          {
            name: "foo-1.0.0.tar.gz",
            browser_download_url: "https://dl.example/foo.tar.gz",
          },
        ],
      });
    }
    if (value.includes("/repos/"))
      return jsonResponse({
        id: 123,
        full_name: "acme/foo",
        stargazers_count: 3,
        owner: { id: 456, login: "acme" },
      });
    if (value === "https://dl.example/foo.tar.gz")
      return binaryResponse(packageBytes);
    throw new Error(`unexpected fetch: ${value}`);
  };
  try {
    const result = await buildEntry({ id: "foo", repo: "acme/foo" });
    assert.equal(result.error, undefined);
    assert.deepEqual(result.record.publisher, {
      schema_version: 1,
      repository_id: "123",
      owner_id: "456",
      login: "acme",
      repository: "acme/foo",
      official: false,
    });
    assert.equal(
      result.record.package_sha256,
      createHash("sha256").update(packageBytes).digest("hex"),
    );
    assert.equal(result.record.author, "Example contributors");
  } finally {
    if (previousInspector === undefined)
      delete process.env.KANDEV_PLUGIN_PACKAGE_INSPECTOR;
    else process.env.KANDEV_PLUGIN_PACKAGE_INSPECTOR = previousInspector;
    await fs.rm(directory, { recursive: true, force: true });
  }
});

test("buildEntry uses the native package inspector for a real package fixture", async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), "kandev-plugin-registry-real-inspector-"),
  );
  const backendDirectory = path.resolve(
    path.dirname(fileURLToPath(import.meta.url)),
    "../apps/backend",
  );
  const inspector = path.join(
    directory,
    process.platform === "win32" ? "plugin-package.exe" : "plugin-package",
  );
  const packageDirectory = path.join(directory, "package");
  const archivePath = path.join(directory, "foo-1.0.0.tar.gz");
  await fs.mkdir(path.join(packageDirectory, "server"), { recursive: true });
  const [goos, goarch] = execFileSync("go", ["env", "GOOS", "GOARCH"], {
    cwd: backendDirectory,
    encoding: "utf8",
  })
    .trim()
    .split(/\s+/);
  await fs.writeFile(
    path.join(packageDirectory, "manifest.yaml"),
    `id: foo\napi_version: 1\nversion: 1.0.0\ndisplay_name: Foo\ndescription: A foo\nauthor: Example contributors\ncategories: [tools]\nrepo_url: https://github.com/acme/foo\nruntime:\n  type: binary\n  executables:\n    ${goos}-${goarch}: server/plugin-${goos}-${goarch}\n`,
  );
  await fs.writeFile(
    path.join(packageDirectory, "server", `plugin-${goos}-${goarch}`),
    "binary",
  );
  execFileSync("go", ["build", "-o", inspector, "./cmd/plugin-package"], {
    cwd: backendDirectory,
  });
  execFileSync(
    "go",
    ["run", "./cmd/plugin-pack", "-dir", packageDirectory, "-out", archivePath],
    { cwd: backendDirectory },
  );
  const packageBytes = await fs.readFile(archivePath);

  const previousInspector = process.env.KANDEV_PLUGIN_PACKAGE_INSPECTOR;
  process.env.KANDEV_PLUGIN_PACKAGE_INSPECTOR = inspector;
  globalThis.fetch = async (url) => {
    const value = String(url);
    if (value.includes("/releases/latest"))
      return jsonResponse({
        tag_name: "1.0.0",
        assets: [
          {
            name: "foo-1.0.0.tar.gz",
            browser_download_url: "https://dl.example/foo.tar.gz",
          },
        ],
      });
    if (value.includes("/repos/"))
      return jsonResponse({
        id: 123,
        full_name: "acme/foo",
        stargazers_count: 3,
        owner: { id: 456, login: "acme" },
      });
    if (value === "https://dl.example/foo.tar.gz")
      return binaryResponse(packageBytes);
    throw new Error(`unexpected fetch: ${value}`);
  };
  try {
    const result = await buildEntry({ id: "foo", repo: "acme/foo" });
    assert.equal(result.error, undefined);
    assert.equal(result.record.publisher.login, "acme");
    assert.equal(
      result.record.package_sha256,
      createHash("sha256").update(packageBytes).digest("hex"),
    );
  } finally {
    if (previousInspector === undefined)
      delete process.env.KANDEV_PLUGIN_PACKAGE_INSPECTOR;
    else process.env.KANDEV_PLUGIN_PACKAGE_INSPECTOR = previousInspector;
    await fs.rm(directory, { recursive: true, force: true });
  }
});

test("buildEntry grants Official Kandev only to an explicit kdlbs entry", async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), "kandev-plugin-registry-official-test-"),
  );
  const inspector = path.join(directory, "inspector.mjs");
  await fs.writeFile(
    inspector,
    `#!/usr/bin/env node\nprocess.stdout.write(JSON.stringify({id:"foo",version:"1.0.0",kind:"plugin",author:"kandev"}));`,
    { mode: 0o755 },
  );
  const previousInspector = process.env.KANDEV_PLUGIN_PACKAGE_INSPECTOR;
  process.env.KANDEV_PLUGIN_PACKAGE_INSPECTOR = inspector;
  globalThis.fetch = async (url) => {
    const value = String(url);
    if (value.includes("/releases/latest"))
      return jsonResponse({
        tag_name: "1.0.0",
        assets: [
          {
            name: "foo-1.0.0.tar.gz",
            browser_download_url: "https://dl.example/foo.tar.gz",
          },
        ],
      });
    if (value.includes("/repos/"))
      return jsonResponse({
        id: 123,
        full_name: "kdlbs/foo",
        stargazers_count: 3,
        owner: { id: 456, login: "kdlbs" },
      });
    if (value === "https://dl.example/foo.tar.gz")
      return binaryResponse(new Uint8Array([1, 2, 3]));
    throw new Error(`unexpected fetch: ${value}`);
  };
  try {
    const result = await buildEntry({
      id: "foo",
      repo: "kdlbs/foo",
      official: true,
    });
    assert.equal(result.error, undefined);
    assert.equal(result.record.publisher.official, true);

    const rejected = await buildEntry({ id: "foo", repo: "kdlbs/foo" });
    assert.match(rejected.error, /reserved Kandev author/);
  } finally {
    if (previousInspector === undefined)
      delete process.env.KANDEV_PLUGIN_PACKAGE_INSPECTOR;
    else process.env.KANDEV_PLUGIN_PACKAGE_INSPECTOR = previousInspector;
    await fs.rm(directory, { recursive: true, force: true });
  }
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
    repoMeta: {
      id: 123,
      full_name: "acme/foo",
      owner: { id: 456, login: "acme" },
    },
  });
  const { record } = await buildEntry({ id: "foo", repo: "acme/foo" });
  assert.equal(record.stars, null);
  assert.equal(record.author, "");
  assert.equal(record.icon_url, null); // no manifest icon
});

test("buildIndex retains a previous star count when refresh metadata is unavailable", async () => {
  stubGitHub({
    release: {
      tag_name: "1.0.0",
      assets: [{ name: "foo-1.0.0.tar.gz", browser_download_url: "https://dl/foo.tar.gz" }],
    },
    manifestText: "display_name: Foo",
    repoMeta: {
      id: 123,
      full_name: "acme/foo",
      owner: { id: 456, login: "acme" },
    },
  });
  const { document } = await buildIndex(
    [{ id: "foo", repo: "acme/foo" }],
    { plugins: [{ id: "foo", stars: 42 }] },
  );
  assert.equal(document.plugins[0].stars, 42);
});

test("buildEntry rejects a canvas when its trusted inspector is unavailable", async () => {
  const previousInspector = process.env.KANDEV_CANVAS_PACKAGE_INSPECTOR;
  delete process.env.KANDEV_CANVAS_PACKAGE_INSPECTOR;
  stubGitHub({
    release: {
      tag_name: "1.0.0",
      assets: [{ name: "board-1.0.0.tar.gz", browser_download_url: "https://dl/board.tar.gz" }],
    },
    manifestText: "display_name: Board",
    repoMeta: {
      id: 123,
      full_name: "acme/board",
      stargazers_count: 1,
      owner: { id: 456, login: "acme" },
    },
  });
  try {
    const result = await buildEntry({
      id: "board",
      repo: "acme/board",
      kind: "canvas",
      previews: [{ url: "https://cdn.example/cover.webp", alt: "Board" }],
    });
    assert.equal(result.record, undefined);
    assert.match(result.error, /KANDEV_CANVAS_PACKAGE_INSPECTOR is not configured/);
  } finally {
    if (previousInspector === undefined)
      delete process.env.KANDEV_CANVAS_PACKAGE_INSPECTOR;
    else process.env.KANDEV_CANVAS_PACKAGE_INSPECTOR = previousInspector;
  }
});

test("buildEntry uses inspected canvas presentation metadata and preserves preview order", async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), "kandev-registry-test-"),
  );
  const inspector = path.join(directory, "inspector.mjs");
  await fs.writeFile(
    inspector,
    `#!/usr/bin/env node\nprocess.stdout.write(JSON.stringify({id:"board",version:"1.0.0",kind:"canvas",display_name:"Inspected Board",description:"From archive",author:"archive-author",min_kandev_version:"2.0.0",repo_url:"https://github.com/acme/board",license:"MIT"}));\n`,
    { mode: 0o755 },
  );
  const previousInspector = process.env.KANDEV_CANVAS_PACKAGE_INSPECTOR;
  process.env.KANDEV_CANVAS_PACKAGE_INSPECTOR = inspector;
  const packageBytes = new Uint8Array([1, 2, 3, 4]);
  globalThis.fetch = async (url) => {
    const value = String(url);
    if (value.includes("/releases/latest")) {
      return jsonResponse({
        tag_name: "v1.0.0",
        assets: [
          {
            name: "board-1.0.0.tar.gz",
            browser_download_url: "https://dl.example/board.tar.gz",
          },
        ],
      });
    }
    if (value.includes("/manifest.yaml"))
      return textResponse("display_name: Board\ndescription: A board\n");
    if (value.includes("/repos/"))
      return jsonResponse({
        id: 123,
        full_name: "acme/board",
        stargazers_count: 2,
        owner: { id: 456, login: "acme" },
      });
    if (value === "https://dl.example/board.tar.gz")
      return binaryResponse(packageBytes);
    throw new Error(`unexpected fetch: ${value}`);
  };
  try {
    const result = await buildEntry({
      id: "board",
      repo: "acme/board",
      kind: "canvas",
      previews: [{ url: "https://cdn.example/cover.webp", alt: "Board cover" }],
    });
    assert.equal(result.error, undefined);
    assert.equal(result.record.kind, "canvas");
    assert.equal(result.record.name, "Inspected Board");
    assert.equal(result.record.description, "From archive");
    assert.equal(result.record.author, "archive-author");
    assert.equal(result.record.min_kandev_version, "2.0.0");
    assert.deepEqual(result.record.previews, [
      { url: "https://cdn.example/cover.webp", alt: "Board cover" },
    ]);
    assert.equal(
      result.record.package_sha256,
      createHash("sha256").update(packageBytes).digest("hex"),
    );
  } finally {
    if (previousInspector === undefined)
      delete process.env.KANDEV_CANVAS_PACKAGE_INSPECTOR;
    else process.env.KANDEV_CANVAS_PACKAGE_INSPECTOR = previousInspector;
    await fs.rm(directory, { recursive: true, force: true });
  }
});

test("buildEntry bounds streamed canvas package responses", async () => {
  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), "kandev-registry-stream-test-"),
  );
  const inspector = path.join(directory, "inspector.mjs");
  await fs.writeFile(
    inspector,
    `process.stdout.write(JSON.stringify({id:"board",version:"1.0.0",kind:"canvas"}));`,
    { mode: 0o755 },
  );
  const previousInspector = process.env.KANDEV_CANVAS_PACKAGE_INSPECTOR;
  process.env.KANDEV_CANVAS_PACKAGE_INSPECTOR = inspector;
  globalThis.fetch = async (url) => {
    const value = String(url);
    if (value.includes("/releases/latest"))
      return jsonResponse({
        tag_name: "v1.0.0",
        assets: [
          {
            name: "board-1.0.0.tar.gz",
            browser_download_url: "https://dl.example/board.tar.gz",
          },
        ],
      });
    if (value.includes("/manifest.yaml")) return textResponse("");
    if (value.includes("/repos/"))
      return jsonResponse({
        id: 123,
        full_name: "acme/board",
        stargazers_count: 1,
        owner: { id: 456, login: "acme" },
      });
    if (value === "https://dl.example/board.tar.gz") {
      return {
        ok: true,
        status: 200,
        body: new ReadableStream({
          start(controller) {
            controller.enqueue(new Uint8Array(5 * 1024 * 1024));
            controller.enqueue(new Uint8Array(5 * 1024 * 1024 + 1));
            controller.close();
          },
        }),
        arrayBuffer: async () => new ArrayBuffer(0),
      };
    }
    throw new Error(`unexpected fetch: ${value}`);
  };
  try {
    const result = await buildEntry({
      id: "board",
      repo: "acme/board",
      kind: "canvas",
      previews: [{ url: "https://cdn.example/cover.webp", alt: "Board" }],
    });
    assert.match(result.error, /exceeds the package size limit/);
  } finally {
    if (previousInspector === undefined)
      delete process.env.KANDEV_CANVAS_PACKAGE_INSPECTOR;
    else process.env.KANDEV_CANVAS_PACKAGE_INSPECTOR = previousInspector;
    await fs.rm(directory, { recursive: true, force: true });
  }
});

test("buildEntry rejects a canvas without a preview before fetching a release", async () => {
  const result = await buildEntry({
    id: "board",
    repo: "acme/board",
    kind: "canvas",
  });
  assert.match(result.error, /at least one preview/);
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
        ? jsonResponse({
            tag_name: "1.0.0",
            assets: [
              { name: "a-1.0.0.tar.gz", browser_download_url: "https://dl/a" },
            ],
          })
        : jsonResponse(null);
    }
    if (u.includes("/manifest.yaml")) return textResponse("");
    return jsonResponse({
      id: 123,
      full_name: `o/${u.split("/").pop()}`,
      stargazers_count: 1,
      owner: { id: 456, login: "o" },
    });
  };

  const { document, errors, nativeErrors } = await buildIndex([
    { id: "a", repo: "o/a" },
    { id: "b", repo: "o/b" },
  ]);
  assert.equal(document.plugins.length, 1);
  assert.equal(document.plugins[0].id, "a");
  assert.equal(errors.length, 1);
  assert.equal(nativeErrors.length, 0);
  assert.equal(
    (
      await buildIndex([
        { id: "bad-canvas", repo: "o/bad-canvas", kind: "canvas" },
      ])
    ).canvasErrors.length,
    1,
  );
  assert.equal(document.schema_version, 1);
});

test("pull-request validation fails a mixed native result with one invalid entry", async () => {
  globalThis.fetch = async (url) => {
    const value = String(url);
    const good =
      value.includes("/repos/o/good") ||
      value.includes("raw.githubusercontent.com/o/good/");
    const bad =
      value.includes("/repos/o/bad") ||
      value.includes("raw.githubusercontent.com/o/bad/");
    if (value.includes("/releases/latest")) {
      return jsonResponse({
        tag_name: "1.0.0",
        assets: [
          {
            name: `${good ? "good" : "bad"}-1.0.0.tar.gz`,
            browser_download_url: "https://dl.example/package.tar.gz",
          },
        ],
      });
    }
    if (value.includes("/manifest.yaml")) {
      return textResponse(
        bad
          ? "display_name: Bad\nauthor: kandev"
          : "display_name: Good\nauthor: community",
      );
    }
    if (value.includes("/repos/")) {
      return jsonResponse({
        id: good ? 123 : 456,
        full_name: good ? "o/good" : "o/bad",
        stargazers_count: 1,
        owner: { id: good ? 789 : 987, login: "o" },
      });
    }
    throw new Error(`unexpected fetch: ${value}`);
  };

  const directory = await fs.mkdtemp(
    path.join(os.tmpdir(), "kandev-plugin-registry-pr-gate-"),
  );
  const previousExitCode = process.exitCode;
  process.exitCode = undefined;
  try {
    const result = await main({
      specs: [
        { id: "good", repo: "o/good" },
        { id: "bad", repo: "o/bad" },
      ],
      outputPath: path.join(directory, "index.json"),
      eventName: "pull_request",
    });
    assert.deepEqual(
      result.document.plugins.map((entry) => entry.id),
      ["good"],
    );
    assert.equal(result.errors.length, 1);
    assert.equal(result.nativeErrors.length, 1);
    assert.equal(pullRequestValidationFailed(result.nativeErrors, result.canvasErrors), true);
    assert.equal(result.exitCode, 1);
    assert.equal(process.exitCode, 1);
  } finally {
    process.exitCode = previousExitCode;
    await fs.rm(directory, { recursive: true, force: true });
  }
});
