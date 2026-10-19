import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

const workflow = readFileSync(new URL("../.github/workflows/backend-tests.yml", import.meta.url), "utf8");

function step(name) {
  const start = workflow.indexOf(`      - name: ${name}\n`);
  assert.notEqual(start, -1);
  const next = workflow.indexOf("\n      - name:", start + 1);
  return workflow.slice(start, next === -1 ? undefined : next);
}

function script(name) {
  const block = step(name);
  const start = block.indexOf("        run: |\n");
  assert.notEqual(start, -1);
  return block.slice(start + "        run: |\n".length).split("\n")
    .filter((line) => line.startsWith("          ") || line === "")
    .map((line) => line.slice(10)).join("\n");
}

function fixture(t) {
  const directory = mkdtempSync(join(tmpdir(), "backend-lint-workflow-"));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  const git = (...args) => execFileSync("git", ["-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", ...args], {
    cwd: directory, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"],
  }).trim();
  git("init", "-q");
  git("config", "user.email", "fixture@example.test");
  git("config", "user.name", "Workflow fixture");
  git("commit", "--allow-empty", "-qm", "initial");
  mkdirSync(join(directory, "apps/backend"), { recursive: true });
  writeFileSync(join(directory, "apps/backend/base.go"), "package fixture\n\nfunc Base() {}\n");
  git("add", ".");
  git("commit", "-qm", "base");
  const base = git("rev-parse", "HEAD");
  git("update-ref", "refs/remotes/origin/main", base);
  writeFileSync(join(directory, "apps/backend/feature.go"), "package fixture\nfunc Feature ( ){ }\n");
  git("add", ".");
  git("commit", "-qm", "feature");
  return { directory, base, head: git("rev-parse", "HEAD") };
}

test("manual backend lint compares with the default branch, preserving other event bases", (t) => {
  const repository = fixture(t);
  for (const event of ["workflow_dispatch", "pull_request", "push", "merge_group"]) {
    const environmentFile = join(repository.directory, `environment-${event}`);
    execFileSync("bash", ["-c", script("Resolve base SHA")], {
      cwd: repository.directory,
      env: {
        ...process.env, EVENT_NAME: event, BASE_REF: "main", GITHUB_ENV: environmentFile,
        PUSH_BEFORE: event === "push" ? repository.base : "",
        MERGE_GROUP_BASE_SHA: repository.base,
      },
      stdio: "pipe",
    });
    assert.match(readFileSync(environmentFile, "utf8"), new RegExp(`^BASE_SHA=${repository.base}$`, "m"), event);
  }
  assert.match(step("Resolve base SHA"), /BASE_REF: \$\{\{ github\.base_ref \|\| github\.event\.repository\.default_branch \}\}/);
});

test("backend gofmt finds changed files from the backend working directory", (t) => {
  const repository = fixture(t);
  const command = script("Check gofmt").replaceAll("${{ github.sha }}", repository.head);
  const run = () => spawnSync("bash", ["-c", command], {
    cwd: join(repository.directory, "apps/backend"),
    env: { ...process.env, BASE_SHA: repository.base }, encoding: "utf8",
  });
  const unformatted = run();
  assert.equal(unformatted.status, 1, unformatted.stdout + unformatted.stderr);
  assert.match(unformatted.stdout, /feature\.go/);
  execFileSync("gofmt", ["-w", "feature.go"], { cwd: join(repository.directory, "apps/backend") });
  const formatted = run();
  assert.equal(formatted.status, 0, formatted.stdout + formatted.stderr);
});
