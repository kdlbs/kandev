import assert from "node:assert/strict";
import { test } from "node:test";
import { cacheIdentity } from "./go-cache-identity.mjs";

const identity = {
  family: "test-1",
  day: "2026-10-14",
  runner: "Linux-X64",
  dependencies: "dependency-hash",
  recipe: "image-recipe-hash",
  go: {
    GOVERSION: "go1.26.0",
    GOOS: "linux",
    GOARCH: "amd64",
    CGO_ENABLED: "1",
    GOFLAGS: "-trimpath",
    GOAMD64: "v1",
    GOEXPERIMENT: "",
    GOMODCACHE: "/root/go/pkg/mod",
    GOCACHE: "/cache with spaces/build",
  },
  compiler: "gcc 13.3.0",
};
// Cache compatibility is local evidence; complete CI timing is measured separately.

test("daily rollover preserves compatible restore prefix", () => {
  const today = cacheIdentity(identity);
  const tomorrow = cacheIdentity({ ...identity, day: "2026-10-15" });
  assert.notEqual(today.buildKey, tomorrow.buildKey);
  assert.equal(today.buildPrefix, tomorrow.buildPrefix);
  assert.ok(today.buildKey.startsWith(tomorrow.buildPrefix));
  assert.equal(today.moduleKey, tomorrow.moduleKey);
});

test("all migrated Linux callers have finite isolated cache families", () => {
  const families = [
    "test-1",
    "test-2",
    "test-3",
    "test-4",
    "repository-1",
    "repository-2",
    "repository-3",
    "postgres-packages",
    "postgres-1",
    "postgres-2",
    "postgres-3",
    "e2e-helpers",
  ];
  const identities = families.map((family) =>
    cacheIdentity({ ...identity, family }),
  );
  assert.equal(new Set(identities.map((value) => value.buildPrefix)).size, families.length);
});

test("save modes bound cache publication while preserving the default", () => {
  const lint = {
    version: "golangci-lint 2.9.0",
    config: "lint-config",
    path: "/root/.cache/golangci-lint",
  };
  const staticIdentity = { ...identity, family: "static", lint };
  const expected = {
    all: { moduleSave: true, buildSave: true, lintSave: true },
    build: { moduleSave: false, buildSave: true, lintSave: false },
    none: { moduleSave: false, buildSave: false, lintSave: false },
  };
  assert.deepEqual(
    cacheIdentity(staticIdentity),
    cacheIdentity({ ...staticIdentity, saveMode: "all" }),
  );
  for (const [saveMode, flags] of Object.entries(expected)) {
    const value = cacheIdentity({ ...staticIdentity, saveMode });
    assert.deepEqual(
      {
        moduleSave: value.moduleSave,
        buildSave: value.buildSave,
        lintSave: value.lintSave,
      },
      flags,
      saveMode,
    );
  }
  assert.deepEqual(
    {
      moduleSave: cacheIdentity({ ...identity, saveMode: "all" }).moduleSave,
      buildSave: cacheIdentity({ ...identity, saveMode: "all" }).buildSave,
      lintSave: cacheIdentity({ ...identity, saveMode: "all" }).lintSave,
    },
    { moduleSave: true, buildSave: true, lintSave: false },
  );
  assert.throws(() => cacheIdentity({ ...identity, saveMode: "publish" }), /save mode/i);
});

test("same day reuses the generation", () => {
  assert.deepEqual(
    cacheIdentity(identity),
    cacheIdentity({ ...identity, commit: "source-only-change" }),
  );
  assert.equal(cacheIdentity(identity).buildPath, "/cache with spaces/build");
});

test("compatibility changes cannot match", () => {
  const original = cacheIdentity(identity);
  for (const change of [
    { family: "test-2" },
    { runner: "Linux-ARM64" },
    { dependencies: "new-deps" },
    { recipe: "new-image" },
    { compiler: "gcc 14" },
    ...[
      "GOVERSION",
      "GOOS",
      "GOARCH",
      "CGO_ENABLED",
      "GOFLAGS",
      "GOAMD64",
      "GOEXPERIMENT",
    ].map((key) => ({ go: { ...identity.go, [key]: "different" } })),
  ]) {
    const changed = cacheIdentity({ ...identity, ...change });
    assert.ok(
      !original.buildKey.startsWith(changed.buildPrefix),
      JSON.stringify(change),
    );
  }
  assert.equal(
    original.moduleKey,
    cacheIdentity({ ...identity, family: "static" }).moduleKey,
  );
});

test("invalid identity disables optional caching", () => {
  for (const change of [
    { dependencies: "" },
    { recipe: "" },
    { family: "../invalid" },
    { day: "2026-02-30" },
    { go: { ...identity.go, GOCACHE: "off" } },
    { go: { ...identity.go, GOMODCACHE: "/tmp/bad\npath" } },
    { go: { ...identity.go, GOCACHE: "/tmp/*" } },
    { compiler: "" },
  ])
    assert.throws(() => cacheIdentity({ ...identity, ...change }), /invalid/i);
});

test("linter version and configuration isolate lint snapshots", () => {
  const lint = {
    version: "golangci-lint 2.9.0",
    config: "lint-config",
    path: "/root/.cache/golangci-lint",
  };
  const current = cacheIdentity({ ...identity, family: "static", lint });
  for (const changed of [
    { ...lint, version: "2.10.0" },
    { ...lint, config: "new-config" },
  ]) {
    assert.ok(
      !current.lintKey.startsWith(
        cacheIdentity({ ...identity, family: "static", lint: changed })
          .lintPrefix,
      ),
    );
  }
});

test("CLI identity failure leaves required commands runnable", async () => {
  const { mkdtempSync, readFileSync, rmSync } = await import("node:fs");
  const { tmpdir } = await import("node:os");
  const { spawnSync } = await import("node:child_process");
  const directory = mkdtempSync(`${tmpdir()}/go-cache-`);
  try {
    const result = spawnSync(
      "bash",
      ["-e", "-c", 'node "$IDENTITY_SCRIPT"; echo required-command-ran'],
      {
        encoding: "utf8",
        env: {
          ...process.env,
          IDENTITY_SCRIPT: new URL("./go-cache-identity.mjs", import.meta.url)
            .pathname,
          GITHUB_WORKSPACE: directory,
          GITHUB_OUTPUT: `${directory}/output`,
          CACHE_FAMILY: "test-1",
        },
      },
    );
    assert.equal(result.status, 0, result.stderr);
    assert.match(result.stdout, /required-command-ran/);
    assert.equal(
      readFileSync(`${directory}/output`, "utf8"),
      "enabled=false\n",
    );
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});

test("static CLI uses the configured linter cache without unsupported subcommands", async () => {
  const {
    mkdtempSync,
    mkdirSync,
    writeFileSync,
    readFileSync,
    rmSync,
    copyFileSync,
  } = await import("node:fs");
  const { tmpdir } = await import("node:os");
  const { spawnSync } = await import("node:child_process");
  const directory = mkdtempSync(`${tmpdir()}/go-static-cache-`);
  try {
    for (const subdir of [
      "bin",
      "apps/backend",
      "pluginsdk",
      ".github/docker/ci-base",
      ".github/actions/cache-go",
      ".github/scripts",
    ])
      mkdirSync(`${directory}/${subdir}`, { recursive: true });
    for (const file of [
      "apps/backend/go.mod",
      "apps/backend/go.sum",
      "pluginsdk/go.mod",
      "pluginsdk/go.sum",
      "apps/backend/.golangci.yml",
      ".github/docker/ci-base/Dockerfile",
      ".github/actions/cache-go/action.yml",
    ])
      writeFileSync(`${directory}/${file}`, "fixture");
    copyFileSync(
      new URL("./go-cache-identity.mjs", import.meta.url),
      `${directory}/.github/scripts/go-cache-identity.mjs`,
    );
    writeFileSync(
      `${directory}/bin/go`,
      `#!/bin/sh\ncat <<'JSON'\n${JSON.stringify({ ...identity.go, CGO_ENABLED: "0", CC: "cc" })}\nJSON\n`,
      { mode: 0o755 },
    );
    // The pinned v2.9.0 command surface has cache clean/status, but no cache path.
    writeFileSync(
      `${directory}/bin/golangci-lint`,
      '#!/bin/sh\n[ "$*" = "--version" ] || exit 1\necho "golangci-lint has version 2.9.0"\n',
      { mode: 0o755 },
    );
    const result = spawnSync(
      process.execPath,
      [`${directory}/.github/scripts/go-cache-identity.mjs`],
      {
        encoding: "utf8",
        env: {
          ...process.env,
          PATH: `${directory}/bin:${process.env.PATH}`,
          GITHUB_WORKSPACE: directory,
          GITHUB_OUTPUT: `${directory}/output`,
          CACHE_FAMILY: "static",
          CACHE_SAVE_MODE: "build",
          RUNNER_OS: "Linux",
          RUNNER_ARCH: "X64",
          GOLANGCI_LINT_CACHE: "/tmp/lint cache",
        },
      },
    );
    assert.equal(result.status, 0, result.stderr);
    const output = readFileSync(`${directory}/output`, "utf8");
    assert.match(output, /enabled=true/);
    assert.match(output, /lintPath=\/tmp\/lint cache/);
    assert.match(output, /moduleKey=go-mod-v2-/);
    assert.match(output, /moduleSave=false/);
    assert.match(output, /buildSave=true/);
    assert.match(output, /lintSave=false/);
    for (const file of ["pluginsdk/go.mod", "pluginsdk/go.sum"]) {
      writeFileSync(`${directory}/${file}`, "changed SDK dependency");
      const changed = spawnSync(
        process.execPath,
        [`${directory}/.github/scripts/go-cache-identity.mjs`],
        {
          encoding: "utf8",
          env: {
            ...process.env,
            PATH: `${directory}/bin:${process.env.PATH}`,
            GITHUB_WORKSPACE: directory,
            GITHUB_OUTPUT: `${directory}/changed-output`,
            CACHE_FAMILY: "static",
            RUNNER_OS: "Linux",
            RUNNER_ARCH: "X64",
            GOLANGCI_LINT_CACHE: "/tmp/lint cache",
          },
        },
      );
      assert.equal(changed.status, 0, changed.stderr);
      const next = JSON.parse(changed.stdout);
      assert.notEqual(
        next.moduleKey,
        JSON.parse(result.stdout).moduleKey,
        file,
      );
      writeFileSync(`${directory}/${file}`, "fixture");
    }
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});
