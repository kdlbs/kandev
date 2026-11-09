/**
 * Contract tests for the native web toolchain routing.
 *
 * They execute the pre-commit command with the real configuration, including a
 * filename containing spaces, a missing binary and a lint error. They also
 * assert the package commands and workflow call sites so a green local run
 * cannot hide a route that silently reverted to ESLint or `tsc`.
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";

const require = createRequire(new URL("../apps/web/package.json", import.meta.url));
const { parse } = require("yaml");

const REPO_ROOT = path.resolve(fileURLToPath(new URL("..", import.meta.url)));
const WEB_DIR = path.join(REPO_ROOT, "apps", "web");
const packageJson = JSON.parse(fs.readFileSync(path.join(WEB_DIR, "package.json"), "utf8"));
const workflow = (name) =>
  parse(fs.readFileSync(path.join(REPO_ROOT, ".github", "workflows", name), "utf8"));
const preCommit = parse(fs.readFileSync(path.join(REPO_ROOT, ".pre-commit-config.yaml"), "utf8"));

function webLintHook() {
  for (const repo of preCommit.repos ?? []) {
    for (const hook of repo.hooks ?? []) {
      if (hook.id === "web-lint") return hook;
    }
  }
  return undefined;
}

function copyWebProject() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-web-route-"));
  const web = path.join(root, "apps", "web");
  fs.mkdirSync(web, { recursive: true });
  fs.cpSync(path.join(WEB_DIR, "eslint-rules"), path.join(web, "eslint-rules"), {
    recursive: true,
  });
  for (const file of [
    "oxlint.config.mjs",
    "eslint.i18n.options.mjs",
    "package.json",
  ]) {
    fs.copyFileSync(path.join(WEB_DIR, file), path.join(web, file));
  }
  fs.symlinkSync(path.join(WEB_DIR, "node_modules"), path.join(web, "node_modules"), "dir");
  return { root, web };
}

function runHook(files, env = {}) {
  const hook = webLintHook();
  assert.ok(hook, "web-lint hook is configured");
  const { root } = copyWebProject();
  try {
    for (const file of files) {
      const target = path.join(root, file);
      fs.mkdirSync(path.dirname(target), { recursive: true });
      fs.writeFileSync(target, "export function Probe() {\n  debugger;\n  return null;\n}\n");
    }
    const command = `${hook.entry} ${files.map((file) => JSON.stringify(file)).join(" ")}`;
    return spawnSync("/bin/bash", ["-c", command], {
      cwd: root,
      encoding: "utf8",
      env: { ...process.env, ...env },
    });
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
}

test("required package commands keep generation and use the native tools", () => {
  assert.equal(packageJson.scripts.lint, "oxlint -c oxlint.config.mjs --deny-warnings .");
  assert.equal(
    packageJson.scripts["lint:i18n"],
    "oxlint -c oxlint.i18n.config.mjs --deny-warnings",
  );
  assert.equal(
    packageJson.scripts["lint:e2e-sleeps"],
    "oxlint -c oxlint.e2e-sleeps.config.mjs --deny-warnings",
  );
  assert.equal(
    packageJson.scripts.typecheck,
    "pnpm run rich-output:check && tsrs --noEmit --fast",
  );
  assert.equal(
    packageJson.scripts.pretypecheck,
    "node scripts/generate-release-notes.mjs && node scripts/generate-changelog.mjs",
  );
  assert.ok(!packageJson.scripts.typecheck.includes("tsc --noEmit"));
  assert.equal(packageJson.devDependencies.oxlint, "1.91.0");
  assert.equal(packageJson.devDependencies.tsrs, "0.20.1-ts7.1.0-beta");
  assert.ok(packageJson.devDependencies.typescript, "classic TypeScript API remains installed");
  assert.ok(packageJson.devDependencies.eslint, "ESLint adapter host remains installed");
});

test("the changed-file hook forwards real filenames, including spaces", () => {
  const result = runHook(["apps/web/components/space probe.tsx"]);
  assert.notEqual(result.status, 0, "a debugger in a selected file fails the hook");
  assert.match(result.stdout, /eslint\(no-debugger\)/);
});

test("the changed-file hook fails when its binary is missing", () => {
  const limitedPath = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-limited-path-"));
  try {
    // Keep the shell available while removing pnpm and every installed binary.
    fs.symlinkSync("/bin/bash", path.join(limitedPath, "bash"));
    const result = runHook(["apps/web/components/probe.tsx"], { PATH: limitedPath });
    assert.notEqual(result.status, 0, "a missing pnpm/oxlint cannot report success");
    assert.match(`${result.stdout}${result.stderr}`, /pnpm|oxlint/);
  } finally {
    fs.rmSync(limitedPath, { recursive: true, force: true });
  }
});

test("the changed-file hook accepts a clean selected file", () => {
  const hook = webLintHook();
  assert.ok(hook);
  const { root } = copyWebProject();
  try {
    const target = path.join(root, "apps/web/components/clean.tsx");
    fs.mkdirSync(path.dirname(target), { recursive: true });
    fs.writeFileSync(target, "export function Probe() {\n  return null;\n}\n");
    const command = `${hook.entry} ${JSON.stringify("apps/web/components/clean.tsx")}`;
    const result = spawnSync("/bin/bash", ["-c", command], {
      cwd: root,
      encoding: "utf8",
    });
    assert.equal(result.status, 0, `${result.stdout}${result.stderr}`);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("upgrade regression routes its targeted lint through Oxlint", () => {
  const config = workflow("upgrade-regression.yml");
  const steps = Object.values(config.jobs).flatMap((job) => job.steps ?? []);
  const harness = steps.find((step) => step.name === "Check harness contracts");
  assert.ok(harness, "upgrade harness step exists");
  assert.match(
    harness.run,
    /pnpm exec oxlint -c oxlint\.config\.mjs --deny-warnings e2e\/fixtures\/upgrade-backend\.ts/,
  );
  assert.doesNotMatch(harness.run, /pnpm exec eslint/);
  assert.match(harness.run, /e2e\/playwright\.config\.test\.ts/);
});

test("the evidence helper tests join the existing action-pinning job", () => {
  const config = workflow("lint-action-pinning.yml");
  const steps = Object.values(config.jobs).flatMap((job) => job.steps ?? []);
  const run = steps.map((step) => step.run ?? "").join("\n");
  assert.match(run, /python3 \.github\/scripts\/web-toolchain-evidence_test\.py/);
  assert.equal(config.permissions.contents, "read");
});

test("the routing contract runs from the repository root", () => {
  const config = workflow("frontend-tests.yml");
  const job = config.jobs.frontend;
  assert.ok(job, "frontend static job exists");
  const defaultCwd = job.defaults?.run?.["working-directory"] ?? ".";
  const step = (job.steps ?? []).find(
    (candidate) => candidate.name === "Test native web toolchain routing",
  );
  assert.ok(step, "routing contract step exists");
  assert.equal(step.run, "node --test scripts/web-native-toolchain.test.mjs");
  const effective = step["working-directory"] ?? defaultCwd;
  const resolved = effective.includes("github.workspace")
    ? REPO_ROOT
    : path.resolve(REPO_ROOT, effective);
  assert.equal(
    resolved,
    REPO_ROOT,
    `routing step would run from ${resolved}, not the repository root`,
  );
  assert.ok(fs.existsSync(path.join(resolved, "scripts/web-native-toolchain.test.mjs")));
});

test("frontend verification keeps its package commands and job identity", () => {
  const config = workflow("frontend-tests.yml");
  const steps = Object.values(config.jobs).flatMap((job) => job.steps ?? []);
  assert.ok(
    steps.some((step) => step.name === "Run lint" && step.run === "pnpm --filter @kandev/web lint"),
  );
  assert.ok(
    steps.some(
      (step) => step.name === "Run typecheck" && step.run === "pnpm --filter @kandev/web typecheck",
    ),
  );
  const run = steps.map((step) => step.run ?? "").join("\n");
  assert.doesNotMatch(run, /pnpm exec eslint/);
  assert.doesNotMatch(run, /\btsc --noEmit\b/);
});

test("the new tool configurations count as tool settings for size labeling", () => {
  const config = workflow("pr-size-label.yml");
  const script = Object.values(config.jobs)
    .flatMap((job) => job.steps ?? [])
    .find((step) => step.with?.script)?.with?.script;
  const source = script ?? "";
  for (const file of [
    "oxlint.config.mjs",
    "oxlint.i18n.config.mjs",
    "oxlint.e2e-sleeps.config.mjs",
  ]) {
    assert.match(source, new RegExp(file.replace(/\./g, "\\.")));
  }
});
