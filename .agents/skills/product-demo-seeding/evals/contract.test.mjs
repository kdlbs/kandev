import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const evalDir = path.dirname(fileURLToPath(import.meta.url));
const skillDir = path.resolve(evalDir, "..");
const bundle = [
  fs.readFileSync(path.join(skillDir, "SKILL.md"), "utf8"),
  ...fs
    .readdirSync(path.join(skillDir, "references"))
    .filter((name) => name.endsWith(".md"))
    .sort()
    .map((name) => fs.readFileSync(path.join(skillDir, "references", name), "utf8")),
].join("\n");

test("requires a freshly fetched clean origin/main capture checkout", () => {
  assert.match(bundle, /fetch(?:ed)?[^\n]{0,80}origin\/main/i);
  assert.match(bundle, /(?:detached|clean)[^\n]{0,80}(?:worktree|checkout)/i);
  assert.match(bundle, /HEAD[^\n]{0,80}(?:equals|matches|same)[^\n]{0,80}origin\/main/i);
});

test("isolates deterministic provider capture resources", () => {
  assert.match(bundle, /mock GitHub[^\n]{0,80}(?:and|\/)[^\n]{0,80}Jira/i);
  for (const resource of ["HOME", "KANDEV_HOME_DIR", "database", "ports", "display", "browser profile"]) {
    assert.match(bundle, new RegExp(resource, "i"));
  }
  assert.match(bundle, /fixed[^\n]{0,100}(?:IDs?|timestamps?)/i);
  assert.match(bundle, /(?:no|never)[^\n]{0,100}(?:credentials?|network)/i);
});

test("rejects stale capture state and protects developer data", () => {
  assert.match(bundle, /reject[^\n]{0,100}stale (?:UI|selector|script|capture)/i);
  assert.match(bundle, /never[^\n]{0,100}developer(?:'s)? (?:instance|database|DB|data)/i);
  assert.match(bundle, /outside production assets/i);
});

test("hands off native routes, proof, provenance, and teardown", () => {
  assert.match(bundle, /separate desktop and (?:native[- ]mobile|mobile)/i);
  assert.match(bundle, /provenance/i);
  assert.match(bundle, /teardown/i);
});

test("resets visible state before every recorded take", () => {
  assert.match(bundle, /fresh[^\n]{0,80}(?:database|DB)[^\n]{0,80}(?:every|each)[^\n]{0,40}(?:recorded )?take/i);
  assert.match(bundle, /(?:duplicate|accumulated)[^\n]{0,100}(?:task|fixture|record)/i);
  assert.match(bundle, /rehearsal[^\n]{0,100}(?:reset|discard|fresh database|fresh DB)/i);
});

test("keeps temporary capture harness code out of clean source status", () => {
  assert.match(bundle, /temporary capture (?:specs?|harness)[^\n]{0,140}(?:CAPTURE_ROOT|outside (?:the )?(?:source )?worktree)/i);
  assert.match(bundle, /(?:locally excluded|explicit exclusion|whitelist)[^\n]{0,120}(?:git status|clean[- ]worktree|status gate)/i);
  assert.match(bundle, /command-local[^\n]{0,100}core\.excludesFile/i);
  assert.match(bundle, /never[^\n]{0,100}(?:shared )?\.git\/info\/exclude/i);
});
