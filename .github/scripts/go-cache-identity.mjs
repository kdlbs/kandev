import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { appendFileSync, readFileSync } from "node:fs";
import { isAbsolute, resolve } from "node:path";
import { pathToFileURL } from "node:url";

const hash = (value) =>
  createHash("sha256").update(JSON.stringify(value)).digest("hex");
const required = (value) => {
  if (typeof value !== "string" || !value.trim() || /[\r\n]/.test(value))
    throw new Error("invalid cache identity");
  return value;
};
const cachePath = (value) => {
  required(value);
  if (!isAbsolute(value) || /[*?!\[\]{}]/.test(value))
    throw new Error("invalid cache path");
  return value;
};

export function cacheIdentity({
  family,
  day,
  runner,
  dependencies,
  recipe,
  go,
  compiler,
  lint,
  saveMode = "all",
}) {
  const families = [
    "static",
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
    "ambient",
    "e2e-helpers",
    "e2e-build",
  ];
  if (!families.includes(family))
    throw new Error("invalid cache family");
  if (!["all", "build", "none"].includes(saveMode))
    throw new Error("invalid cache save mode");
  if (
    !/^\d{4}-\d{2}-\d{2}$/.test(day) ||
    new Date(day).toISOString().slice(0, 10) !== day
  )
    throw new Error("invalid cache day");
  const platform = [
    required(runner),
    ...["GOVERSION", "GOOS", "GOARCH"].map((key) => required(go[key])),
  ];
  const moduleKey = `go-mod-v2-${hash([...platform, required(dependencies)])}`;
  const buildEnv = ["CGO_ENABLED", "GOFLAGS", "GOAMD64", "GOEXPERIMENT"].map(
    (key) => {
      if (typeof go[key] !== "string")
        throw new Error("invalid Go build environment");
      return go[key];
    },
  );
  const buildPrefix = `go-build-v2-${family}-${hash([moduleKey, required(recipe), buildEnv, go.CGO_ENABLED === "1" ? required(compiler) : "no-cgo"])}-`;
  const result = {
    moduleKey,
    modulePath: cachePath(go.GOMODCACHE),
    buildPrefix,
    buildKey: `${buildPrefix}${day}`,
    buildPath: cachePath(go.GOCACHE),
    moduleSave: saveMode === "all",
    buildSave: saveMode === "all" || saveMode === "build",
    lintSave: saveMode === "all" && family === "static" && Boolean(lint),
  };
  if (lint) {
    result.lintPrefix = `go-lint-v2-${hash([buildPrefix, required(lint.version), required(lint.config)])}-`;
    result.lintKey = `${result.lintPrefix}${day}`;
    result.lintPath = cachePath(lint.path);
  }
  return result;
}

function main() {
  // Missing identity disables this optional optimization; it never broadens a key.
  const output = process.env.GITHUB_OUTPUT;
  try {
    const root = resolve(process.env.GITHUB_WORKSPACE || ".");
    const cwd = resolve(root, "apps/backend");
    const run = (command, args) =>
      execFileSync(command, args, {
        cwd,
        encoding: "utf8",
        timeout: 30_000,
      }).trim();
    const go = JSON.parse(
      run("go", [
        "env",
        "-json",
        "GOVERSION",
        "GOOS",
        "GOARCH",
        "CGO_ENABLED",
        "GOFLAGS",
        "GOAMD64",
        "GOEXPERIMENT",
        "GOMODCACHE",
        "GOCACHE",
        "CC",
      ]),
    );
    const filesHash = (paths) =>
      hash(
        paths.map((path) => [path, readFileSync(resolve(root, path), "utf8")]),
      );
    const lint =
      process.env.CACHE_FAMILY === "static"
        ? {
            version: run("golangci-lint", ["--version"]),
            config: filesHash(["apps/backend/.golangci.yml"]),
            path: process.env.GOLANGCI_LINT_CACHE,
          }
        : undefined;
    const values = cacheIdentity({
      family: process.env.CACHE_FAMILY,
      saveMode: process.env.CACHE_SAVE_MODE ?? "all",
      day: new Date().toISOString().slice(0, 10),
      runner: `${required(process.env.RUNNER_OS)}-${required(process.env.RUNNER_ARCH)}`,
      dependencies: filesHash([
        "apps/backend/go.mod",
        "apps/backend/go.sum",
        "pluginsdk/go.mod",
        "pluginsdk/go.sum",
      ]),
      recipe: filesHash([
        ".github/docker/ci-base/Dockerfile",
        ".github/actions/cache-go/action.yml",
        ".github/scripts/go-cache-identity.mjs",
      ]),
      go,
      compiler:
        go.CGO_ENABLED === "1" ? run(go.CC, ["--version"]).split("\n")[0] : "",
      lint,
    });
    appendFileSync(
      output,
      Object.entries({ ...values, enabled: "true" })
        .map(([key, value]) => `${key}=${value}\n`)
        .join(""),
    );
    console.log(JSON.stringify(values, null, 2));
  } catch (error) {
    console.warn(
      `::warning::Go caching disabled: ${String(error.message).replace(/[\r\n]/g, " ")}`,
    );
    if (output) appendFileSync(output, "enabled=false\n");
  }
}
if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(resolve(process.argv[1])).href
)
  main();
