/**
 * The ratchet is actually WIRED UP.
 *
 * Every other test here proves the matcher is right and the diff attribution is
 * right. None of them would notice if the CI step, the pre-commit hook or the
 * script's own call to `findNewSleeps` were deleted — the rule would still be
 * correct, its tests would still pass, and the gate would silently never run
 * again. That is the worst failure mode available to a check: it reports a clean
 * tree forever, in the one place nobody thinks to look.
 *
 * So these assert the call sites, not the logic. They are the mutants
 * "delete the CI step", "delete the hook", and "delete the call while keeping
 * the function".
 *
 * Matching is on the script FILENAME rather than on surrounding YAML structure,
 * so reformatting, renaming a step or reordering the workflow does not break
 * them — only actually removing the invocation does.
 */
import fs from "node:fs";
import path from "node:path";

import { describe, expect, it } from "vitest";

const WEB_DIR = path.resolve(import.meta.dirname, "../..");
const REPO_ROOT = path.resolve(WEB_DIR, "../..");

const RATCHET_SCRIPT = "check-new-e2e-sleeps.mjs";

const read = (rel: string) => fs.readFileSync(path.join(REPO_ROOT, rel), "utf8");

describe("sleep ratchet wiring", () => {
  /**
   * Both invocations, not just the filename.
   *
   * `toContain(RATCHET_SCRIPT)` alone was too weak: the step has a
   * `pull_request` branch and a `push` branch, so deleting either one left the
   * other's mention of the filename and the assertion still passed. A mutation
   * run caught it. Count the call sites instead.
   */
  it("runs in CI, in the frontend workflow, on both event paths", () => {
    const workflow = read(".github/workflows/frontend-tests.yml");
    expect(workflow).toMatch(/^\s+- name: Run E2E sleep ratchet$/m);
    const calls = workflow.match(new RegExp(`node scripts/${RATCHET_SCRIPT}`, "g")) ?? [];
    expect(calls).toHaveLength(2);
  });

  /**
   * `pull_request` must NOT pass `--base`. The checkout is the merge ref and
   * `pull_request.base.sha` is an ancestor of it, so handing it in makes every
   * commit main landed since the PR opened read as this PR's new code (#2160
   * did exactly this to the i18n ratchet).
   */
  it("does not pass --base on pull_request in CI", () => {
    const workflow = read(".github/workflows/frontend-tests.yml");
    const step = workflow.slice(workflow.indexOf("Run E2E sleep ratchet"));
    // Match the pull_request branch structurally rather than slicing at the
    // first `BASE_SHA=` literal: a future YAML comment mentioning that name
    // would move the slice boundary and quietly shrink what is asserted
    // (review finding). This is strictly tighter than the slice was — it pins
    // the invocation, its bare form, and the early exit, in order.
    //
    // How the event name is read is deliberately NOT pinned. It was
    // `"${{ github.event_name }}"` inline until the merge-queue work moved it
    // to an `EVENT_NAME` env var, which changed nothing about the guarantee
    // here and broke this assertion anyway. What matters is that the
    // pull_request branch runs the ratchet bare and returns: the trailing
    // newline after the script name is what rejects a smuggled `--base`.
    expect(step).toMatch(
      new RegExp(
        `== "pull_request" \\]\\];? then\\n` +
          `\\s+node scripts/${RATCHET_SCRIPT}\\n` +
          `\\s+exit 0\\n`,
      ),
    );
  });

  /**
   * The hook id is matched to end-of-line. `toContain("id: e2e-sleep-ratchet")`
   * was satisfied by `id: e2e-sleep-ratchet-disabled`, so renaming the hook to
   * switch it off passed the test — a mutation run caught it.
   */
  it("runs as a pre-commit hook on e2e changes", () => {
    const config = read(".pre-commit-config.yaml");
    expect(config).toMatch(/^\s+- id: e2e-sleep-ratchet$/m);
    expect(config).toContain(RATCHET_SCRIPT);
    // The hook must fire for the tree it guards.
    expect(config).toContain("^apps/web/e2e/.*\\.tsx?$");
  });

  it("is reachable as a package script", () => {
    const pkg = JSON.parse(read("apps/web/package.json"));
    expect(pkg.scripts["e2e:sleep-ratchet"]).toContain(RATCHET_SCRIPT);
    expect(pkg.scripts["e2e:waits"]).toContain("report-e2e-waits.mjs");
    expect(pkg.scripts["lint:e2e-sleeps"]).toContain("eslint.e2e-sleeps.config.mjs");
  });

  /**
   * The CLI must actually invoke the decision function. A shell that imports it
   * and then exits 0 unconditionally is the "delete the call, keep the function"
   * mutant, and it looks completely healthy from the outside.
   */
  it("the CLI calls findNewSleeps and exits non-zero on violations", () => {
    const cli = read(`apps/web/scripts/${RATCHET_SCRIPT}`);
    expect(cli).toContain("await findNewSleeps(");
    expect(cli).toContain("process.exit(1)");
    // The failure exit must be reachable from the violations branch, not
    // orphaned below an unconditional success exit.
    expect(cli.indexOf("violations.length === 0")).toBeLessThan(cli.indexOf("process.exit(1)"));
  });

  /**
   * The eslint rule is registered in the main config too, on the directories
   * already converted. Losing this registration removes editor feedback and the
   * graduation path, without any test elsewhere noticing.
   */
  it("registers the rule in the main eslint config", () => {
    const config = read("apps/web/eslint.config.mjs");
    expect(config).toContain("e2eSleepGuardFiles");
    expect(config).toContain('"e2e-sleeps/no-unsanctioned-sleep": "error"');
  });

  /**
   * The guard list may only grow. A directory listed here was measured at zero,
   * so removing an entry means somebody added a sleep to a clean directory and
   * deleted the guard rather than the sleep.
   */
  it("keeps every seeded guard directory", () => {
    const config = read("apps/web/eslint-rules/no-unsanctioned-sleep.mjs");
    for (const dir of [
      "e2e/scripts/**/*.{ts,tsx}",
      "e2e/tests/cli-mode/**/*.{ts,tsx}",
      "e2e/tests/docker/**/*.{ts,tsx}",
      "e2e/tests/github/**/*.{ts,tsx}",
      "e2e/tests/i18n/**/*.{ts,tsx}",
      "e2e/tests/kanban/**/*.{ts,tsx}",
      "e2e/tests/preview/**/*.{ts,tsx}",
    ]) {
      expect(config).toContain(`"${dir}"`);
    }
  });
});
