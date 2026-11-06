import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { cacheIdentity } from "./go-cache-identity.mjs";

const action = readFileSync(
  new URL("../actions/cache-go/action.yml", import.meta.url),
  "utf8",
);

function step(name) {
  const marker = `    - name: ${name}\n`;
  const start = action.indexOf(marker);
  assert.notEqual(start, -1, `missing action step: ${name}`);
  const bodyStart = start + marker.length;
  const next = action.indexOf("\n    - name:", bodyStart);
  return action.slice(bodyStart, next === -1 ? undefined : next);
}

const cacheFixture = ({
  family,
  saveMode,
  misses = [],
  failOperations = [],
  commandStatus = 0,
  cache: existingCache = new Map(),
}) => {
  const identity = cacheIdentity({
    family,
    saveMode,
    day: "2026-10-28",
    runner: "Linux-X64",
    dependencies: "deps",
    recipe: "recipe",
    go: {
      GOVERSION: "go1.26.0",
      GOOS: "linux",
      GOARCH: "amd64",
      CGO_ENABLED: "0",
      GOFLAGS: "-trimpath",
      GOAMD64: "v1",
      GOEXPERIMENT: "",
      GOMODCACHE: "/fixture/go/pkg/mod",
      GOCACHE: "/fixture/go-build",
    },
    compiler: "",
    lint:
      family === "static"
        ? {
            version: "golangci-lint 2.9.0",
            config: "lint-config",
            path: "/fixture/golangci-lint",
          }
        : undefined,
  });
  const operations = [];
  const restored = new Set();
  const cache = existingCache;
  const optional = (name, operation) => {
    try {
      if (failOperations.includes(name)) throw new Error(`${name} failed`);
      operation();
    } catch {
      // The composite action marks every cache service operation optional.
    }
  };
  const restore = (name, key) =>
    optional(`restore-${name}`, () => {
      operations.push(`restore-${name}`);
      const hit = cache.has(key) && !misses.includes(name);
      if (hit) restored.add(name);
    });
  const save = (name, key, enabled) => {
    if (!enabled || commandStatus !== 0 || restored.has(name)) return;
    optional(`save-${name}`, () => {
      operations.push(`save-${name}`);
      cache.set(key, true);
    });
  };

  restore("module", identity.moduleKey);
  restore("build", identity.buildKey);
  if (identity.lintKey) restore("lint", identity.lintKey);
  save("module", identity.moduleKey, identity.moduleSave);
  save("build", identity.buildKey, identity.buildSave);
  if (identity.lintKey) save("lint", identity.lintKey, identity.lintSave);
  operations.push("required-command");

  return { identity, operations, commandStatus, cache };
};

test("composite action keeps restore-only defaults compatible and optional", () => {
  assert.match(action, /save-mode:[\s\S]*?required: false[\s\S]*?default: all/);
  assert.match(step("Resolve installed Go cache identity"), /CACHE_SAVE_MODE: \$\{\{ inputs\.save-mode \}\}/);
  for (const name of [
    "Cache Go module downloads",
    "Restore Go module downloads",
    "Cache Go build generation",
    "Restore Go build generation",
    "Cache static analysis generation",
    "Restore static analysis generation",
  ]) {
    assert.match(step(name), /continue-on-error: true/);
  }
  assert.match(step("Cache Go module downloads"), /uses: actions\/cache@/);
  assert.match(step("Cache Go build generation"), /uses: actions\/cache@/);
  assert.match(step("Cache static analysis generation"), /uses: actions\/cache@/);
  assert.match(step("Restore Go module downloads"), /uses: actions\/cache\/restore@/);
  assert.match(step("Restore Go build generation"), /uses: actions\/cache\/restore@/);
  assert.match(step("Restore static analysis generation"), /uses: actions\/cache\/restore@/);
  for (const [name, output] of [
    ["Cache Go module downloads", "moduleSave"],
    ["Cache Go build generation", "buildSave"],
    ["Cache static analysis generation", "lintSave"],
  ]) {
    assert.match(step(name), new RegExp(`identity\\.outputs\\.${output} == 'true'`));
  }
  for (const [name, output] of [
    ["Restore Go module downloads", "moduleSave"],
    ["Restore Go build generation", "buildSave"],
  ]) {
    const restoreStep = step(name);
    assert.match(restoreStep, new RegExp(`identity\\.outputs\\.${output} != 'true'`));
    assert.doesNotMatch(restoreStep, /uses: actions\/cache@/);
  }
  assert.match(
    step("Restore static analysis generation"),
    /inputs\.family == 'static'[\s\S]*?identity\.outputs\.lintSave != 'true'/,
  );
});

test("main ownership modes publish only the intended snapshots on misses", () => {
  const mainStatic = cacheFixture({ family: "static", saveMode: "all" });
  assert.deepEqual(mainStatic.operations, [
    "restore-module",
    "restore-build",
    "restore-lint",
    "save-module",
    "save-build",
    "save-lint",
    "required-command",
  ]);

  const mainRepository = cacheFixture({ family: "repository-2", saveMode: "build" });
  assert.deepEqual(mainRepository.operations, [
    "restore-module",
    "restore-build",
    "save-build",
    "required-command",
  ]);

  const pullRequest = cacheFixture({ family: "static", saveMode: "none" });
  assert.deepEqual(pullRequest.operations, [
    "restore-module",
    "restore-build",
    "restore-lint",
    "required-command",
  ]);
});

test("exact hits avoid immutable saves and unavailable cache service preserves command status", () => {
  const published = cacheFixture({ family: "static", saveMode: "all" });
  const warm = cacheFixture({
    family: "static",
    saveMode: "all",
    cache: published.cache,
  });
  assert.deepEqual(warm.operations, [
    "restore-module",
    "restore-build",
    "restore-lint",
    "required-command",
  ]);
  assert.notEqual(
    mainRolePrefix("static"),
    mainRolePrefix("repository-2"),
  );

  const unavailable = cacheFixture({
    family: "test-3",
    saveMode: "build",
    failOperations: ["restore-module", "restore-build", "save-build"],
    commandStatus: 17,
  });
  assert.equal(unavailable.commandStatus, 17);
  assert.deepEqual(unavailable.operations, ["required-command"]);
});

function mainRolePrefix(family) {
  return cacheIdentity({
    family,
    day: "2026-10-28",
    runner: "Linux-X64",
    dependencies: "deps",
    recipe: "recipe",
    go: {
      GOVERSION: "go1.26.0",
      GOOS: "linux",
      GOARCH: "amd64",
      CGO_ENABLED: "0",
      GOFLAGS: "-trimpath",
      GOAMD64: "v1",
      GOEXPERIMENT: "",
      GOMODCACHE: "/fixture/go/pkg/mod",
      GOCACHE: "/fixture/go-build",
    },
    compiler: "",
  }).buildPrefix;
}
