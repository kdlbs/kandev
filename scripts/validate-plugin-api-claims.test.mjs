import assert from "node:assert/strict";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test, { after } from "node:test";
import {
  extractLiveNames,
  findAbsenceClaims,
  validatePluginApiClaims,
} from "./validate-plugin-api-claims.mjs";

const tempDirs = [];

/**
 * Create an isolated repo-shaped fixture with a `types.ts` and harness files.
 *
 * @param {Record<string, string>} files Fixture files keyed by relative path.
 * @returns {Promise<string>} Temporary fixture directory (acts as repoRoot).
 */
async function createFixture(files) {
  const dir = await fs.mkdtemp(
    path.join(os.tmpdir(), "kandev-plugin-api-claims-"),
  );
  tempDirs.push(dir);
  await Promise.all(
    Object.entries(files).map(async ([file, content]) => {
      const target = path.join(dir, file);
      await fs.mkdir(path.dirname(target), { recursive: true });
      await fs.writeFile(target, content);
    }),
  );
  return dir;
}

after(async () => {
  await Promise.all(
    tempDirs.map((dir) => fs.rm(dir, { recursive: true, force: true })),
  );
});

const FIXTURE_TYPES_TS = `
export interface PluginRegistry {
  registerTaskPanel(registration: TaskPanelRegistration): void;
  registerTaskMenuAction(registration: TaskMenuActionRegistration): void;
}

export interface PluginHostApi {
  /**
   * Curated subset of \`@kandev/ui\` components (Button, Card) plus
   * first-party app UI (RichTextEditor, RichTextReadOnly).
   */
  ui: Record<string, unknown>;
  /** Authenticated, per-user key/value storage. */
  storage: PluginStorageApi;
}

/**
 * Named slot the host renders via \`<PluginSlot name .../>\`. Initial slots:
 * "task-card-indicators" (small icon/badge rendered beside the PR status
 * icon on every kanban card).
 */
export type PluginSlotName = string;
`;

test("extractLiveNames finds register* methods, host properties, ui components, and slot names", () => {
  const names = extractLiveNames(FIXTURE_TYPES_TS);
  assert.ok(names.has("registerTaskPanel"));
  assert.ok(names.has("registerTaskMenuAction"));
  assert.ok(names.has("storage"));
  assert.ok(names.has("RichTextEditor"));
  assert.ok(names.has("RichTextReadOnly"));
  assert.ok(names.has("task-card-indicators"));
});

test("extractLiveNames does not invent names absent from the fixture", () => {
  const names = extractLiveNames(FIXTURE_TYPES_TS);
  assert.ok(!names.has("registerNotAHook"));
  assert.ok(!names.has("nonexistentSlot"));
});

test("findAbsenceClaims matches the deleted stale paragraph as one claim", () => {
  const content = `# Some skill

The current frontend branch does not expose \`registerTaskPanel\`,
\`registerTaskMenuAction\`, \`host.storage\`, \`RichTextEditor\`,
\`RichTextReadOnly\`, or a Kanban-card injection hook. Use the supported slots,
routes, Host state, and shared store documented in the guide; do not invent
future signatures.
`;
  const claims = findAbsenceClaims(content);
  assert.equal(claims.length, 1);
  assert.equal(claims[0].line, 3);
});

test("findAbsenceClaims ignores ordinary prose that just mentions an API name", () => {
  const content = `# Some skill

Use \`registerTaskPanel\` to add a task panel. See the authoring guide for the
full recipe.
`;
  const claims = findAbsenceClaims(content);
  assert.equal(claims.length, 0);
});

test("validatePluginApiClaims reports a finding for a restored stale paragraph", async () => {
  const dir = await createFixture({
    "types.ts": FIXTURE_TYPES_TS,
    "AGENTS.md": `# Frontend

The current frontend branch does not expose \`registerTaskPanel\`,
\`registerTaskMenuAction\`, \`host.storage\`, \`RichTextEditor\`,
\`RichTextReadOnly\`, or a Kanban-card injection hook via
\`task-card-indicators\`. Use the supported slots, routes, Host state, and
shared store documented in the guide; do not invent future signatures.
`,
  });

  const { findings } = await validatePluginApiClaims({
    repoRoot: dir,
    typesPath: path.join(dir, "types.ts"),
    scanFiles: [path.join(dir, "AGENTS.md")],
  });

  assert.ok(findings.length > 0);
  const apis = findings.map((f) => f.api).sort();
  assert.ok(apis.includes("registerTaskPanel"));
  assert.ok(apis.includes("registerTaskMenuAction"));
  assert.ok(apis.includes("storage"));
  assert.ok(apis.includes("RichTextEditor"));
  assert.ok(apis.includes("RichTextReadOnly"));
  assert.ok(apis.includes("task-card-indicators"));
  for (const finding of findings) {
    assert.equal(finding.file, "AGENTS.md");
    assert.equal(typeof finding.line, "number");
  }
});

test("validatePluginApiClaims does not flag a genuinely absent API", async () => {
  const dir = await createFixture({
    "types.ts": FIXTURE_TYPES_TS,
    "AGENTS.md": `# Frontend

This branch does not expose \`registerNotAHook\` yet. Use the supported hooks
documented in the guide instead.
`,
  });

  const { findings } = await validatePluginApiClaims({
    repoRoot: dir,
    typesPath: path.join(dir, "types.ts"),
    scanFiles: [path.join(dir, "AGENTS.md")],
  });

  assert.deepEqual(findings, []);
});

test("validatePluginApiClaims does not false-positive on ordinary documentation", async () => {
  const dir = await createFixture({
    "types.ts": FIXTURE_TYPES_TS,
    "AGENTS.md": `# Frontend

Call \`registerTaskPanel\` from \`initialize\` to add a task panel, and use
\`host.storage\` for small per-user values. See the authoring guide's matrix
for the full contract.
`,
  });

  const { findings } = await validatePluginApiClaims({
    repoRoot: dir,
    typesPath: path.join(dir, "types.ts"),
    scanFiles: [path.join(dir, "AGENTS.md")],
  });

  assert.deepEqual(findings, []);
});

test("validatePluginApiClaims exits clean against the real kandev tree", async () => {
  const repoRoot = path.resolve(import.meta.dirname, "..");
  const { findings } = await validatePluginApiClaims({ repoRoot });
  assert.deepEqual(findings, []);
});
