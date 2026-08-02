#!/usr/bin/env node
/**
 * Runs both ratchet checks, forwarding CLI args to each.
 *
 * `pnpm run i18n:ratchet --base <sha>` used to append the flag to the last token
 * of a `&&` chain, so only the second script saw it and the two disagreed about
 * which base they were judging. A tiny runner keeps them in step.
 */
import { spawnSync } from "node:child_process";
import path from "node:path";

const args = process.argv.slice(2);
for (const script of ["check-new-code-i18n.mjs", "check-guard-allowlist.mjs"]) {
  const result = spawnSync(process.execPath, [path.join(import.meta.dirname, script), ...args], {
    stdio: "inherit",
  });
  if (result.status !== 0) process.exit(result.status ?? 1);
}
