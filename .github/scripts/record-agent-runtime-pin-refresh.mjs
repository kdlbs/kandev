import fs from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

import { readCatalogue } from "../../scripts/update-agent-runtime-pins.mjs";

const SCRIPT_DIR = path.dirname(fileURLToPath(import.meta.url));

export const DEFAULT_OUTPUT_DIR = path.resolve(
  SCRIPT_DIR,
  "../../docs/plans/managed-runtime-pin-automation",
);
export const PLAN_FILENAME = "plan.md";
export const WORK_ORDER_FILENAME = "task-01-refresh-pins.md";
export const REQUIREMENT_ID = "REQ-AGENTS-RUNTIME-UPDATES-001";
const SYSTEM_DESIGN_PATH = "../../specs/agents/system-design/runtime-updates-01.md";
const ACCEPTANCE_CRITERIA = Object.freeze([
  "AC-AGENTS-RUNTIME-UPDATES-001.5",
  "AC-AGENTS-RUNTIME-UPDATES-001.7",
  "AC-AGENTS-RUNTIME-UPDATES-001.9",
  "AC-AGENTS-RUNTIME-UPDATES-001.10",
]);
const VALIDATION_COMMANDS = Object.freeze([
  "node --test scripts/update-agent-runtime-pins.test.mjs",
  "(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/agents ./internal/agent/managedruntime ./internal/agent/runtime/lifecycle ./internal/agent/hostutility ./internal/agent/settings/controller ./internal/agent/settings/handlers)",
]);

function assertSamePackageSet(before, after) {
  const beforePackages = Object.keys(before).sort();
  const afterPackages = Object.keys(after).sort();
  if (beforePackages.join("\n") !== afterPackages.join("\n")) {
    throw new Error("before and after catalogues must contain the same trusted packages");
  }
}

export function diffCatalogues(before, after) {
  assertSamePackageSet(before, after);
  return Object.keys(after)
    .sort()
    .filter((packageName) => before[packageName] !== after[packageName])
    .map((packageName) => ({
      packageName,
      previous: before[packageName],
      proposed: after[packageName],
    }));
}

function changedVersionsTable(changes) {
  const lines = [
    "| Package | Previous | Proposed |",
    "| --- | --- | --- |",
    ...changes.map(
      ({ packageName, previous, proposed }) =>
        `| \`${packageName}\` | \`${previous}\` | \`${proposed}\` |`,
    ),
  ];
  return lines.join("\n");
}

function validationList() {
  return VALIDATION_COMMANDS.map((command) => `- \`${command}\` passed.`).join("\n");
}

export function renderPlan(changes, created) {
  return `---
created: ${created}
status: completed
requirements:
  - ${REQUIREMENT_ID}
system_design:
  - ${SYSTEM_DESIGN_PATH}
legacy_specs: []
---

# Managed runtime pin refresh

## Overview

The scheduled managed runtime pin updater generated this delivery record after
the trusted catalogue and managed-runtime command validation passed. It records
one grouped review change and does not activate a runtime.

## Changed versions

${changedVersionsTable(changes)}

## Validation

${validationList()}

## Work orders

- [x] [Task 01: Refresh managed runtime pins](${WORK_ORDER_FILENAME})
`;
}

export function renderWorkOrder(changes, created) {
  return `---
id: "01-refresh-pins"
title: "Refresh managed runtime pins"
status: completed
wave: 1
depends_on: []
created: ${created}
plan: "${PLAN_FILENAME}"
requirements:
  - ${REQUIREMENT_ID}
acceptance_criteria:
${ACCEPTANCE_CRITERIA.map((criterion) => `  - ${criterion}`).join("\n")}
system_design:
  - ${SYSTEM_DESIGN_PATH}
---

# Task 01: Refresh managed runtime pins

## Summary

Refresh the reviewed managed npm runtime defaults from stable \`latest\`
metadata. The scheduled updater validates the catalogue and managed runtime
commands before writing this completed record.

## Changed packages

${changedVersionsTable(changes)}

## Validation

${validationList()}

## Results

The managed runtime pin updater and the six-package Go validation completed
before this record was generated. The workflow does not install, activate, or
merge a runtime. Package, argument, Copilot plugin, and documentation coverage
contracts remain in their owning tests.
`;
}

function utcDate(now) {
  return now.toISOString().slice(0, 10);
}

async function writeRecord(filePath, content) {
  await fs.mkdir(path.dirname(filePath), { recursive: true });
  await fs.writeFile(filePath, content, "utf8");
}

export async function generateDeliveryRecord({
  beforePath,
  afterPath,
  outputDir = DEFAULT_OUTPUT_DIR,
  validationPassed = false,
  now = new Date(),
} = {}) {
  if (!beforePath || !afterPath) {
    throw new Error("before and after catalogue paths are required");
  }
  if (!validationPassed) {
    throw new Error("managed runtime validation must pass before records are written");
  }
  const before = await readCatalogue(beforePath);
  const after = await readCatalogue(afterPath);
  const changes = diffCatalogues(before, after);
  if (changes.length === 0) {
    return { changed: false, changes };
  }

  const created = utcDate(now);
  const planPath = path.join(outputDir, PLAN_FILENAME);
  const workOrderPath = path.join(outputDir, WORK_ORDER_FILENAME);
  await writeRecord(planPath, renderPlan(changes, created));
  await writeRecord(workOrderPath, renderWorkOrder(changes, created));
  return { changed: true, changes, planPath, workOrderPath };
}

export function parseArguments(argv) {
  let beforePath = "";
  let afterPath = "";
  let outputDir = DEFAULT_OUTPUT_DIR;
  let validationPassed = false;

  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    if (argument === "--validation-passed") {
      validationPassed = true;
      continue;
    }
    if (argument === "--before" || argument === "--after" || argument === "--output-dir") {
      const value = argv[index + 1] ?? "";
      if (!value || value.startsWith("--")) {
        throw new Error(`${argument} requires a path`);
      }
      index += 1;
      if (argument === "--before") beforePath = value;
      if (argument === "--after") afterPath = value;
      if (argument === "--output-dir") outputDir = value;
      continue;
    }
    throw new Error(`unknown option: ${argument}`);
  }

  if (!beforePath || !afterPath) {
    throw new Error("--before and --after are required");
  }
  return { beforePath, afterPath, outputDir, validationPassed };
}

async function main(argv = process.argv.slice(2)) {
  const result = await generateDeliveryRecord(parseArguments(argv));
  process.stdout.write(
    result.changed
      ? `Managed runtime delivery record updated for ${result.changes.length} package(s).\n`
      : "Managed runtime pins are unchanged; no delivery record written.\n",
  );
  if (!result.changed) {
    process.stdout.write("changed=false\n");
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch((error) => {
    console.error(`Managed runtime delivery record failed: ${error.message}`);
    process.exitCode = 1;
  });
}
