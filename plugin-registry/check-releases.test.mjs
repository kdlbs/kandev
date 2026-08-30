import assert from "node:assert/strict";
import test from "node:test";
import { detectReleaseChanges } from "./check-releases.mjs";

const specs = [
  { id: "alpha", repo: "acme/alpha" },
  { id: "beta", repo: "acme/beta" },
];

function indexWith(records) {
  return {
    schema_version: 1,
    plugins: records.map(({ id, repo, version }) => ({
      id,
      repo_url: `https://github.com/${repo}`,
      version,
    })),
  };
}

function release(id, version) {
  return {
    tag_name: `v${version}`,
    assets: [
      {
        name: `${id}-${version}.tar.gz`,
        browser_download_url: `https://dl/${id}`,
      },
    ],
  };
}

test("detectReleaseChanges signals a newer curated exact package", async () => {
  const current = indexWith([
    { id: "alpha", repo: "acme/alpha", version: "1.0.0" },
    { id: "beta", repo: "acme/beta", version: "1.0.0" },
  ]);
  const result = await detectReleaseChanges(specs, current, {
    fetchLatestRelease: async (spec) =>
      release(spec.id, spec.id === "alpha" ? "1.1.0" : "1.0.0"),
  });

  assert.equal(result.rebuild, true);
  assert.deepEqual(result.candidates, ["alpha@1.1.0"]);
  assert.deepEqual(result.errors, []);
});

test("detectReleaseChanges surfaces candidates already beyond the publication SLO", async () => {
  const current = indexWith([
    { id: "alpha", repo: "acme/alpha", version: "1.0.0" },
  ]);
  const staleRelease = release("alpha", "1.1.0");
  staleRelease.published_at = new Date(
    Date.now() - 11 * 60 * 1000,
  ).toISOString();

  const result = await detectReleaseChanges([specs[0]], current, {
    fetchLatestRelease: async () => staleRelease,
  });

  assert.equal(result.rebuild, true);
  assert.deepEqual(result.slaBreaches, ["alpha@1.1.0"]);
});

test("detectReleaseChanges queries only allowlisted repositories", async () => {
  const requested = [];
  const current = indexWith([
    { id: "alpha", repo: "acme/alpha", version: "1.0.0" },
    { id: "beta", repo: "acme/beta", version: "1.0.0" },
    { id: "unlisted", repo: "evil/unlisted", version: "9.0.0" },
  ]);

  await detectReleaseChanges(specs, current, {
    fetchLatestRelease: async (spec) => {
      requested.push(spec.repo);
      return release(spec.id, "1.0.0");
    },
  });

  assert.deepEqual(requested, ["acme/alpha", "acme/beta"]);
});

test("detectReleaseChanges ignores an unchanged curated release", async () => {
  const current = indexWith([
    { id: "alpha", repo: "acme/alpha", version: "1.0.0" },
    { id: "beta", repo: "acme/beta", version: "1.0.0" },
  ]);
  const result = await detectReleaseChanges(specs, current, {
    fetchLatestRelease: async (spec) => release(spec.id, "1.0.0"),
  });

  assert.equal(result.rebuild, false);
  assert.deepEqual(result.errors, []);
});

test("detectReleaseChanges reports a newer release missing its exact asset", async () => {
  const current = indexWith([
    { id: "alpha", repo: "acme/alpha", version: "1.0.0" },
  ]);
  const result = await detectReleaseChanges([specs[0]], current, {
    fetchLatestRelease: async () => ({
      tag_name: "v1.1.0",
      assets: [
        {
          name: "wrong-1.1.0.tar.gz",
          browser_download_url: "https://dl/wrong",
        },
      ],
    }),
  });

  assert.equal(result.rebuild, false);
  assert.match(result.errors[0], /alpha-1\.1\.0\.tar\.gz/);
});

test("detectReleaseChanges preserves an explicit provider error as visible evidence", async () => {
  const current = indexWith([
    { id: "alpha", repo: "acme/alpha", version: "1.0.0" },
  ]);
  const result = await detectReleaseChanges([specs[0]], current, {
    fetchLatestRelease: async () => {
      throw new Error("provider unavailable");
    },
  });

  assert.equal(result.rebuild, false);
  assert.match(result.errors[0], /provider unavailable/);
});

test("detectReleaseChanges rejects an untrusted current index", async () => {
  await assert.rejects(
    detectReleaseChanges(
      specs,
      { schema_version: 2, plugins: [] },
      {
        fetchLatestRelease: async () => release("alpha", "1.0.0"),
      },
    ),
    /schema_version 1/,
  );
});
