#!/usr/bin/env node
/**
 * The i18n guard allowlist may only grow.
 *
 * `i18nGuardFiles` in eslint.i18n.options.mjs is what makes migrated code stay
 * migrated: once a path is listed, `i18next/no-literal-string` is an error there
 * forever. That protection has one obvious failure mode — someone hits the error,
 * deletes the entry, and the build goes green with the migration silently undone.
 * docs/i18n.md says never to do that; this makes it mechanical.
 *
 * Removing an entry is legitimate exactly once: when a file is deleted or renamed.
 * Both are detectable, so the check only complains when the path still exists.
 *
 * Usage: node scripts/check-guard-allowlist.mjs [--base <ref-or-sha>]
 */
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const WEB_DIR = path.resolve(import.meta.dirname, "..");
const REPO_ROOT = path.resolve(WEB_DIR, "..", "..");
const OPTIONS_PATH = "apps/web/eslint.i18n.options.mjs";

function git(args) {
  return execFileSync("git", args, { cwd: REPO_ROOT, encoding: "utf8" });
}

function resolveBase() {
  const flag = process.argv.indexOf("--base");
  const requested = flag !== -1 && process.argv[flag + 1] ? process.argv[flag + 1] : "origin/main";
  try {
    return git(["merge-base", requested, "HEAD"]).trim();
  } catch {
    return null;
  }
}

const base = resolveBase();
if (!base) {
  console.log("⚠ guard allowlist check skipped — no base ref (pass --base <sha>)");
  process.exit(0);
}

let baseSource;
try {
  // stderr silenced: "exists on disk, but not in <rev>" is the expected result
  // for any branch forked before the options module landed, and printing a
  // `fatal:` line there reads as a failure in CI logs when it is not one.
  baseSource = execFileSync("git", ["show", `${base}:${OPTIONS_PATH}`], {
    cwd: REPO_ROOT,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "ignore"],
  });
} catch {
  // The options module does not exist at the base commit, so nothing can have
  // been removed from it. True for any branch forked before i18n landed.
  console.log("✓ guard allowlist — no baseline to compare (options module is new)");
  process.exit(0);
}

/**
 * The options module is a dependency-free set of exports, so it imports cleanly
 * from a temp path. Parsing the array out with a regex would break the moment
 * someone reformats it.
 */
async function guardFilesFrom(source) {
  const file = path.join(
    fs.mkdtempSync(path.join(os.tmpdir(), "kandev-i18n-guard-")),
    "options.mjs",
  );
  fs.writeFileSync(file, source);
  try {
    const module = await import(`file://${file}`);
    return module.i18nGuardFiles ?? [];
  } finally {
    fs.rmSync(path.dirname(file), { recursive: true, force: true });
  }
}

const before = await guardFilesFrom(baseSource);
const after = await guardFilesFrom(fs.readFileSync(path.join(REPO_ROOT, OPTIONS_PATH), "utf8"));

const afterSet = new Set(after);
// A glob whose file is gone was removed for a legitimate reason.
const removed = before.filter((entry) => {
  if (afterSet.has(entry)) return false;
  const isGlob = /[*?[\]{}]/.test(entry);
  if (isGlob) return true;
  return fs.existsSync(path.join(WEB_DIR, entry));
});

if (removed.length > 0) {
  console.error(
    `\n✖ ${removed.length} entr(ies) removed from i18nGuardFiles:\n\n` +
      removed.map((entry) => `  ${entry}`).join("\n") +
      `\n\n  Those paths are still present, so removing them un-protects copy that` +
      `\n  was already migrated. If a literal there is failing lint, externalize it` +
      `\n  — do not drop the entry. See docs/i18n.md ("The guard is scoped").\n`,
  );
  process.exit(1);
}

const added = after.filter((entry) => !new Set(before).has(entry));
console.log(
  `✓ guard allowlist intact — ${after.length} entr(ies)` +
    (added.length ? `, ${added.length} added` : ""),
);
