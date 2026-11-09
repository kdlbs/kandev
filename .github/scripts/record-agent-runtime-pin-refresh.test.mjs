import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

import {
  PLAN_FILENAME,
  WORK_ORDER_FILENAME,
  diffCatalogues,
  generateDeliveryRecord,
  parseArguments,
  renderPlan,
  renderWorkOrder,
} from "./record-agent-runtime-pin-refresh.mjs";

const SCRIPT_DIR = path.dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = path.resolve(SCRIPT_DIR, "../..");
const CATALOGUE_PATH = "apps/backend/internal/agent/agents/managed_npm_runtime_versions.json";
const PLAN_PATH = `docs/plans/managed-runtime-pin-automation/${PLAN_FILENAME}`;
const WORK_ORDER_PATH = `docs/plans/managed-runtime-pin-automation/${WORK_ORDER_FILENAME}`;
const DESIGN_PATH = "docs/specs/agents/system-design/runtime-updates-01.md";
const REQUIREMENT_PATH = "docs/specs/agents/requirements/runtime-updates.md";
const CREATED = new Date("2026-11-05T00:00:00Z");

const beforeCatalogue = {
  "@example/agent": "1.0.0",
  "opencode-ai": "1.18.32",
  "@opencode/cli": "2.0.18",
  "unchanged-agent": "3.2.1",
};
const afterCatalogue = {
  ...beforeCatalogue,
  "@example/agent": "1.1.0",
  "opencode-ai": "1.19.5",
  "@opencode/cli": "2.2.5",
};

async function temporaryDirectory(t) {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), "kandev-pin-record-"));
  t.after(() => fs.rm(directory, { recursive: true, force: true }));
  return directory;
}

async function writeCatalogue(directory, name, value) {
  const cataloguePath = path.join(directory, name);
  await fs.writeFile(cataloguePath, `${JSON.stringify(value, null, 2)}\n`, "utf8");
  return cataloguePath;
}

async function generate(t, { before = beforeCatalogue, after = afterCatalogue } = {}) {
  const directory = await temporaryDirectory(t);
  const beforePath = await writeCatalogue(directory, "before.json", before);
  const afterPath = await writeCatalogue(directory, "after.json", after);
  const outputDir = path.join(directory, "records");
  const result = await generateDeliveryRecord({
    beforePath,
    afterPath,
    outputDir,
    validationPassed: true,
    now: CREATED,
  });
  return { directory, outputDir, result };
}

async function recordContents(result) {
  return {
    [PLAN_PATH]: await fs.readFile(result.planPath, "utf8"),
    [WORK_ORDER_PATH]: await fs.readFile(result.workOrderPath, "utf8"),
    [DESIGN_PATH]: await fs.readFile(path.join(REPO_ROOT, DESIGN_PATH), "utf8"),
    [REQUIREMENT_PATH]: await fs.readFile(path.join(REPO_ROOT, REQUIREMENT_PATH), "utf8"),
  };
}

function coverageDiff() {
  return [
    { filename: CATALOGUE_PATH, status: "modified" },
    { filename: PLAN_PATH, status: "added" },
    { filename: WORK_ORDER_PATH, status: "added" },
  ];
}

test("records only changed packages and omits unchanged pins", async (t) => {
  const { result } = await generate(t);
  assert.equal(result.changed, true);
  assert.deepEqual(
    result.changes.map(({ packageName, previous, proposed }) => ({
      packageName,
      previous,
      proposed,
    })),
    [
      { packageName: "@example/agent", previous: "1.0.0", proposed: "1.1.0" },
      { packageName: "@opencode/cli", previous: "2.0.18", proposed: "2.2.5" },
      { packageName: "opencode-ai", previous: "1.18.32", proposed: "1.19.5" },
    ],
  );

  const contents = await recordContents(result);
  for (const record of [contents[PLAN_PATH], contents[WORK_ORDER_PATH]]) {
    assert.match(record, /@example\/agent/);
    assert.match(record, /1\.1\.0/);
    assert.doesNotMatch(record, /unchanged-agent/);
  }
  assert.match(contents[PLAN_PATH], /\[Task 01: Refresh managed runtime pins\]\(task-01-refresh-pins\.md\)/);
  assert.match(contents[WORK_ORDER_PATH], /status: completed/);
  assert.match(contents[WORK_ORDER_PATH], /AC-AGENTS-RUNTIME-UPDATES-001\.10/);
});

test("an unchanged catalogue writes no completed record", async (t) => {
  const directory = await temporaryDirectory(t);
  const beforePath = await writeCatalogue(directory, "before.json", beforeCatalogue);
  const afterPath = await writeCatalogue(directory, "after.json", beforeCatalogue);
  const outputDir = path.join(directory, "records");
  const result = await generateDeliveryRecord({
    beforePath,
    afterPath,
    outputDir,
    validationPassed: true,
    now: CREATED,
  });

  assert.equal(result.changed, false);
  assert.deepEqual(result.changes, []);
  await assert.rejects(fs.access(path.join(outputDir, PLAN_FILENAME)), { code: "ENOENT" });
  await assert.rejects(fs.access(path.join(outputDir, WORK_ORDER_FILENAME)), { code: "ENOENT" });
});

test("an unchanged catalogue leaves an existing stale record untouched", async (t) => {
  const directory = await temporaryDirectory(t);
  const beforePath = await writeCatalogue(directory, "before.json", beforeCatalogue);
  const afterPath = await writeCatalogue(directory, "after.json", beforeCatalogue);
  const outputDir = path.join(directory, "records");
  await fs.mkdir(outputDir, { recursive: true });
  const stale = "stale record\n";
  await fs.writeFile(path.join(outputDir, PLAN_FILENAME), stale, "utf8");
  await fs.writeFile(path.join(outputDir, WORK_ORDER_FILENAME), stale, "utf8");

  const result = await generateDeliveryRecord({
    beforePath,
    afterPath,
    outputDir,
    validationPassed: true,
    now: CREATED,
  });

  assert.equal(result.changed, false);
  assert.equal(await fs.readFile(path.join(outputDir, PLAN_FILENAME), "utf8"), stale);
  assert.equal(await fs.readFile(path.join(outputDir, WORK_ORDER_FILENAME), "utf8"), stale);
});

test("validation must pass before a completed record is written", async (t) => {
  const directory = await temporaryDirectory(t);
  const beforePath = await writeCatalogue(directory, "before.json", beforeCatalogue);
  const afterPath = await writeCatalogue(directory, "after.json", afterCatalogue);
  const outputDir = path.join(directory, "records");

  await assert.rejects(
    generateDeliveryRecord({ beforePath, afterPath, outputDir, validationPassed: false }),
    /validation must pass/,
  );
  await assert.rejects(fs.access(outputDir), { code: "ENOENT" });
});

test("rejects invalid pins and package-set changes", async (t) => {
  const directory = await temporaryDirectory(t);
  const beforePath = await writeCatalogue(directory, "before.json", beforeCatalogue);
  const prereleasePath = await writeCatalogue(directory, "prerelease.json", {
    ...beforeCatalogue,
    "@example/agent": "1.1.0-beta.1",
  });
  const missingPath = await writeCatalogue(directory, "missing.json", {
    "opencode-ai": "1.18.32",
    "@opencode/cli": "2.0.18",
  });

  await assert.rejects(
    generateDeliveryRecord({
      beforePath,
      afterPath: prereleasePath,
      outputDir: path.join(directory, "records"),
      validationPassed: true,
    }),
    /stable SemVer/,
  );
  await assert.rejects(
    generateDeliveryRecord({
      beforePath,
      afterPath: missingPath,
      outputDir: path.join(directory, "records"),
      validationPassed: true,
    }),
    /same trusted packages/,
  );
});

test("generated records satisfy documentation coverage", async (t) => {
  const { validateCoverage } = await import("./pr-docs.cjs");
  const { result } = await generate(t);
  const validation = validateCoverage({
    changedFiles: coverageDiff(),
    fileContents: await recordContents(result),
  });

  assert.equal(validation.ok, true, validation.errors.join("; "));
  assert.equal(validation.status, "covered");
  assert.deepEqual(validation.workOrders, [WORK_ORDER_PATH]);
});

test("catalogue-only and broken-reference fixtures fail coverage", async (t) => {
  const { validateCoverage } = await import("./pr-docs.cjs");
  const { result } = await generate(t);
  const contents = await recordContents(result);

  const catalogueOnly = validateCoverage({
    changedFiles: [{ filename: CATALOGUE_PATH, status: "modified" }],
    fileContents: contents,
  });
  assert.equal(catalogueOnly.ok, false);
  assert.equal(catalogueOnly.status, "missing");

  const broken = validateCoverage({
    changedFiles: coverageDiff(),
    fileContents: {
      ...contents,
      [WORK_ORDER_PATH]: contents[WORK_ORDER_PATH].replace(
        "../../specs/agents/system-design/runtime-updates-01.md",
        "../../specs/agents/system-design/missing-design.md",
      ),
    },
  });
  assert.equal(broken.ok, false);
  assert.equal(broken.status, "invalid");
  assert.equal(
    broken.errors.some((error) => error.includes("missing-design.md")),
    true,
    broken.errors.join("; "),
  );
});

test("renders deterministic records for a supplied date", () => {
  const changes = diffCatalogues(beforeCatalogue, afterCatalogue);
  assert.equal(renderPlan(changes, "2026-11-05"), renderPlan(changes, "2026-11-05"));
  assert.match(renderPlan(changes, "2026-11-05"), /created: 2026-11-05/);
  assert.match(renderWorkOrder(changes, "2026-11-05"), /created: 2026-11-05/);
  for (const target of [
    renderPlan(changes, "2026-11-05"),
    renderWorkOrder(changes, "2026-11-05"),
  ]) {
    for (const { packageName, previous, proposed } of changes) {
      assert.ok(target.includes(`\`${packageName}\``), `${packageName} missing from rendered record`);
      assert.ok(target.includes(`\`${previous}\``), `${previous} missing from rendered record`);
      assert.ok(target.includes(`\`${proposed}\``), `${proposed} missing from rendered record`);
    }
  }
  assert.match(renderPlan(changes, "2026-11-05"), /-tags fts5/);
});

test("parses required paths and the validation gate", () => {
  const parsed = parseArguments(["--before", "a.json", "--after", "b.json"]);
  assert.equal(parsed.beforePath, "a.json");
  assert.equal(parsed.afterPath, "b.json");
  assert.equal(parsed.validationPassed, false);
  assert.throws(() => parseArguments(["--before", "a.json"]), /--before and --after/);
  assert.throws(
    () => parseArguments(["--before", "a.json", "--after", "b.json", "--unexpected"]),
    /unknown option/,
  );
});
