#!/usr/bin/env node
/**
 * Ratchet: NEW unconditional sleeps in `apps/web/e2e` must go through `dwell()`
 * or `injectLatency()` from `e2e/helpers/causal-waits.ts`.
 *
 * `page.waitForTimeout(500)` and `await new Promise((r) => setTimeout(r, 500))`
 * are the suite's most reliable source of flake: a hand-picked number that
 * happened to be long enough on the machine it was written on. The sanctioned
 * forms take a category and a reason, so a wall-clock wait that survives review
 * has to say why no event exists to wait on instead.
 *
 * ~126 raw sleeps predate this check, and `pnpm lint` is
 * `eslint --max-warnings 0`, so registering the rule repo-wide at any severity
 * would break every unrelated PR until the conversion finishes — the mistake the
 * first i18n migration attempt made. Hence judging the CHANGE, not the FILE:
 *
 *   - a file the change ADDED must be clean outright. New code carries no legacy
 *     debt, so requiring zero sleeps costs nothing.
 *   - a file the change MODIFIED is judged only on the lines it touched. Sleeps
 *     elsewhere in the file belong to the conversion effort, not to this PR.
 *
 * Same contract, same plumbing and same fork-point handling as
 * `check-new-code-i18n.mjs`, which mirrors `golangci-lint --new-from-rev`.
 *
 * The decision logic lives in `lib/e2e-sleep-ratchet.mjs` so it can be driven
 * against real git topologies in a test; this file is the shell that supplies
 * the real repository and turns the result into output and an exit code.
 *
 * Usage:
 *   node scripts/check-new-e2e-sleeps.mjs [--base <ref-or-sha>]
 *
 * Prefer passing no --base; see lib/git-base.mjs for why an explicit one is
 * floored at the fork point.
 */
import path from "node:path";

import { findNewSleeps } from "./lib/e2e-sleep-ratchet.mjs";
import { REPO_ROOT, WEB_DIR } from "./lib/git-base.mjs";

const result = await findNewSleeps({
  repoRoot: REPO_ROOT,
  webDir: WEB_DIR,
  configFile: path.join(WEB_DIR, "eslint.e2e-sleeps.config.mjs"),
  argv: process.argv,
});

if (result.skip) {
  console.log(`⚠ e2e sleep ratchet skipped — ${result.skip}`);
  process.exit(0);
}

const { added, modified, violations } = result;

if (added.length === 0 && modified.length === 0) {
  console.log("✓ e2e sleep ratchet — no e2e source added or modified");
  process.exit(0);
}

if (violations.length === 0) {
  console.log(
    `✓ e2e sleep ratchet — ${added.length} added + ${modified.length} modified e2e file(s) clean`,
  );
  process.exit(0);
}

console.error(
  `\n✖ ${violations.length} unconditional sleep(s) in new e2e code.\n` +
    `  A wall-clock wait must name its category and its reason, so use dwell() or\n` +
    `  injectLatency() from e2e/helpers/causal-waits.ts. Better still, wait on the\n` +
    `  cause: waitForHttp() / watchWs(). See apps/web/e2e/README.md.\n`,
);
for (const violation of violations) {
  const scope = violation.whole ? "new file" : "changed line";
  console.error(`  ${violation.repoPath}:${violation.line}  [${scope}]  ${violation.text}`);
}
console.error("");
process.exit(1);
