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
 * The list also has to stay honest about what it claims, so a second check
 * rejects a path listed twice. A duplicate costs nothing at runtime, so before
 * this check existed it slipped through every gate the repo had while quietly
 * presenting an earlier migration's work as the current PR's.
 *
 * Usage: node scripts/check-guard-allowlist.mjs [--base <ref-or-sha>]
 */
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { git, REPO_ROOT, resolveBase, WEB_DIR } from "./lib/git-base.mjs";
import { duplicateEntries } from "./lib/guard-allowlist.mjs";

const OPTIONS_PATH = "apps/web/eslint.i18n.options.mjs";

const resolved = resolveBase();
if (resolved.skip) {
  console.log(`⚠ guard allowlist check skipped — ${resolved.skip}`);
  process.exit(0);
}
const { base } = resolved;

let baseSource;
try {
  // stderr silenced: "exists on disk, but not in <rev>" is the expected result
  // for any branch forked before the options module landed, and printing a
  // `fatal:` line there reads as a failure in CI logs when it is not one.
  baseSource = git(["show", `${base}:${OPTIONS_PATH}`], { quiet: true });
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
/**
 * An entry may be removed once its path is genuinely gone — a deleted or renamed
 * file, or a migrated directory that was later deleted wholesale. Only a removal
 * whose target still EXISTS un-protects live code, so that is all we flag.
 *
 * Globs are resolved rather than assumed live: flagging every removed glob would
 * block the legitimate flow of migrating `components/foo/**`, listing it, then
 * deleting `foo/` entirely.
 */
const removed = before.filter((entry) => {
  if (afterSet.has(entry)) return false;
  if (/[*?[\]{}]/.test(entry)) {
    return fs.globSync(entry, { cwd: WEB_DIR }).length > 0;
  }
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

/**
 * Nothing above catches a path listed twice: a removal check only ever looks at
 * what left. A duplicate changes no behaviour either, so `i18next/no-literal-string`,
 * `i18n:check` and the new-code half of the ratchet are all indifferent to one —
 * which is exactly why it needs its own check here. See `duplicateEntries` for
 * why this is deliberately exact-match and not a subsumption check.
 */
const duplicates = duplicateEntries(after);

if (duplicates.length > 0) {
  console.error(
    `\n✖ ${duplicates.length} duplicate entr(ies) in i18nGuardFiles:\n\n` +
      duplicates.map((entry) => `  ${entry}`).join("\n") +
      `\n\n  Each of those is already listed elsewhere in the array, so the second` +
      `\n  copy protects nothing new — it only reads as coverage this change added.` +
      `\n  Delete the copy you just wrote; the existing entry already covers it.\n`,
  );
  process.exit(1);
}

const added = after.filter((entry) => !new Set(before).has(entry));
console.log(
  `✓ guard allowlist intact — ${after.length} entr(ies)` +
    (added.length ? `, ${added.length} added` : ""),
);
