/**
 * The sleep ratchet's decision logic, separated from its CLI shell.
 *
 * Split out so the ratchet can be driven against real git topologies in a test
 * — a temp repository with the same `apps/web/e2e/…` layout — rather than only
 * through its individual helpers. Testing `parseAddedLineRanges` and the eslint
 * rule proves each part works; it does not prove the parts are wired together,
 * and a gate that is perfectly tested but never actually consulted reports a
 * clean tree forever. `findNewSleeps` is the seam where those two claims meet,
 * so it is the thing the tests drive.
 *
 * Everything path-shaped is a parameter for the same reason. The CLI supplies
 * the real repository; the tests supply a throwaway one.
 */
import path from "node:path";

import { ESLint } from "eslint";

import { SLEEP_RULE_ID } from "../../eslint-rules/no-unsanctioned-sleep.mjs";
import { changedPaths, isE2eCandidate, webDiff } from "./changed-files.mjs";
import { lineInRanges, parseAddedLineRanges } from "./diff-ranges.mjs";
import { resolveBase, toPosixPath } from "./git-base.mjs";

/**
 * Sleeps introduced by the change between the resolved base and the working
 * tree.
 *
 * Added files are judged whole; modified files only on the lines they touched.
 *
 * @returns `{ skip }` when no base could be resolved and skipping is allowed,
 *   otherwise `{ base, added, modified, violations }`.
 */
export async function findNewSleeps({ repoRoot, webDir, configFile, argv = [] }) {
  const resolved = resolveBase(argv, repoRoot);
  if (resolved.skip) return { skip: resolved.skip };
  const { base } = resolved;

  const added = changedPaths(base, "A", repoRoot).filter(isE2eCandidate);
  // Renames are judged like modifications: moving a legacy spec must not
  // suddenly demand that every sleep in it be converted. A pure rename adds no
  // lines and so reports nothing.
  const modified = changedPaths(base, "MRC", repoRoot).filter(isE2eCandidate);
  const targets = [...added, ...modified];
  if (targets.length === 0) return { base, added, modified, violations: [] };

  const addedRanges =
    modified.length === 0 ? new Map() : parseAddedLineRanges(webDiff(base, repoRoot));

  const eslint = new ESLint({
    cwd: webDir,
    overrideConfigFile: configFile,
    // `causal-waits.ts` is in the config's `ignores`, and passing an ignored
    // file explicitly is a warning that would otherwise print on every run that
    // touches it.
    warnIgnored: false,
  });
  const results = await eslint.lintFiles(targets.map((file) => path.join(repoRoot, file)));

  const addedSet = new Set(added);
  const violations = [];
  for (const result of results) {
    const repoPath = toPosixPath(path.relative(repoRoot, result.filePath));
    const whole = addedSet.has(repoPath);
    const ranges = addedRanges.get(repoPath);
    for (const message of result.messages) {
      if (message.ruleId !== SLEEP_RULE_ID) continue;
      if (!whole && !lineInRanges(ranges, message.line)) continue;
      violations.push({
        repoPath,
        whole,
        line: message.line,
        messageId: message.messageId,
        text: message.message,
      });
    }
  }

  return { base, added, modified, violations };
}
