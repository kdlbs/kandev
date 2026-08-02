/**
 * Base resolution against real git topologies.
 *
 * The bug these pin down (#2160): CI passed `--base
 * ${{ github.event.pull_request.base.sha }}` while checking out the MERGE REF.
 * That sha is an ancestor of the merge ref, so `merge-base` returned it verbatim
 * and every commit main landed since the PR opened was judged as the PR's own
 * new code — failing PRs on files not in their diff. A three-dot diff does not
 * help: `base...HEAD` collapses to `base..HEAD` exactly when base is an ancestor.
 *
 * Each scenario is built as a real repository, and every "fixed" assertion is
 * paired with one showing the raw stale base still reproduces the failure — a
 * test that only exercised the happy path would pass against the broken code.
 */
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { afterAll, describe, expect, it } from "vitest";

import { changedFiles } from "./changed-files.mjs";
import { resolveBase } from "./git-base.mjs";

/** A file `main` lands while the PR is open. Not in the PR's diff. */
const FOREIGN = "apps/web/components/settings/agent-generated-task-title-settings.tsx";
/** The PR's own new file. */
const OWNED = "apps/web/components/gitlab/watch-table.tsx";
/** Clean contents for OWNED: no user-facing literal for the ratchet to find. */
const CLEAN_SOURCE = "export const Watch = () => null;\n";

const tmpRoots: string[] = [];

afterAll(() => {
  for (const dir of tmpRoots) fs.rmSync(dir, { recursive: true, force: true });
});

function git(cwd: string, args: string[], { quiet = false }: { quiet?: boolean } = {}) {
  return execFileSync(
    "git",
    [
      "-c",
      "user.email=ratchet@example.test",
      "-c",
      "user.name=Ratchet Test",
      "-c",
      "commit.gpgsign=false",
      // A developer machine may set core.hooksPath globally; an inherited
      // pre-commit hook would fail these fixture commits for unrelated reasons.
      "-c",
      "core.hooksPath=/nonexistent",
      ...args,
    ],
    { cwd, encoding: "utf8", stdio: ["ignore", "pipe", quiet ? "ignore" : "inherit"] },
  );
}

function write(dir: string, rel: string, body: string) {
  const file = path.join(dir, rel);
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, body);
}

function commit(dir: string, message: string) {
  git(dir, ["add", "-A"]);
  git(dir, ["commit", "-q", "-m", message]);
  return git(dir, ["rev-parse", "HEAD"]).trim();
}

function initRepo() {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-ratchet-"));
  tmpRoots.push(dir);
  git(dir, ["init", "-q", "-b", "main"]);
  write(dir, "apps/web/components/base/base.tsx", "export const Base = () => null;\n");
  return { dir, root: commit(dir, "base") };
}

/** Stands in for `origin/main` without needing an actual remote. */
function setOriginMain(dir: string, sha: string) {
  git(dir, ["update-ref", "refs/remotes/origin/main", sha]);
}

function landOnMain(dir: string) {
  write(
    dir,
    FOREIGN,
    [
      'export const Card = () => <CardTitle className="text-base">Agent-generated',
      "titles</CardTitle>;\n",
    ].join(" "),
  );
  return commit(dir, "feat: agent-generated task titles");
}

/**
 * CI's `pull_request` checkout shape:
 *
 *   root ── main ─────────────── mainTip ──┐
 *      └── pr (clean file) ── prHead ──────┴── mergeRef   (first parent = mainTip)
 */
function prMergeRef() {
  const { dir, root } = initRepo();

  git(dir, ["checkout", "-q", "-b", "pr"]);
  write(dir, OWNED, CLEAN_SOURCE);
  const prHead = commit(dir, "feat(i18n): localize GitLab");

  git(dir, ["checkout", "-q", "main"]);
  const mainTip = landOnMain(dir);
  setOriginMain(dir, mainTip);

  // Detach at the base tip and merge the PR in, so the first parent is the base
  // side — exactly how GitHub builds refs/pull/N/merge.
  git(dir, ["checkout", "-q", mainTip]);
  git(dir, ["merge", "-q", "--no-ff", "-m", `Merge ${prHead} into ${mainTip}`, prHead]);

  return { dir, root, prHead, mainTip, mergeRef: git(dir, ["rev-parse", "HEAD"]).trim() };
}

function baseOf(dir: string, argv: string[]) {
  const resolved = resolveBase(argv, dir);
  if ("skip" in resolved) throw new Error(`unexpected skip: ${resolved.skip}`);
  return resolved.base;
}

describe("resolveBase on a pull_request merge ref", () => {
  it("reproduces the failure when the stale base is used verbatim", () => {
    const { dir, root } = prMergeRef();

    // Guards the scenario itself: without the floor, main's file reads as added.
    expect(changedFiles(root, "A", dir)).toContain(FOREIGN);
  });

  it("floors a stale --base at the fork point, so main's file is not reported", () => {
    const { dir, root, mainTip } = prMergeRef();

    expect(baseOf(dir, ["--base", root])).toBe(mainTip);

    const added = changedFiles(baseOf(dir, ["--base", root]), "A", dir);
    expect(added).not.toContain(FOREIGN);
    expect(added).toEqual([OWNED]);
  });

  it("resolves the same base with no --base at all", () => {
    const { dir, mainTip } = prMergeRef();

    expect(baseOf(dir, [])).toBe(mainTip);
    expect(changedFiles(baseOf(dir, []), "A", dir)).toEqual([OWNED]);
  });

  it("leaves an already-current --base alone", () => {
    const { dir, mainTip } = prMergeRef();

    expect(baseOf(dir, ["--base", mainTip])).toBe(mainTip);
  });

  // Selection only — whether the literal inside is then flagged is ESLint's job,
  // and check-new-code-i18n.mjs resolves its config from this repo's own root, so
  // it cannot be pointed at a fixture. What matters here is that raising the base
  // does not narrow the file set past the PR's own work: a file the change added
  // must still reach the linter.
  it("still selects the PR's own added file, literal and all", () => {
    const { dir, root, mainTip } = prMergeRef();
    git(dir, ["checkout", "-q", "-B", "pr2", mainTip]);
    write(dir, OWNED, "export const W = () => <p>Hardcoded</p>;\n");
    commit(dir, "feat: add a literal");
    setOriginMain(dir, mainTip);

    expect(changedFiles(baseOf(dir, ["--base", root]), "A", dir)).toEqual([OWNED]);
  });
});

describe("resolveBase on other checkout shapes", () => {
  it("floors a rebased branch, which has no merge commit but the same defect", () => {
    const { dir, root } = initRepo();
    const mainTip = landOnMain(dir);
    setOriginMain(dir, mainTip);

    // Rebased onto the new tip: linear history that still CONTAINS main's commit.
    git(dir, ["checkout", "-q", "-b", "pr"]);
    write(dir, OWNED, CLEAN_SOURCE);
    commit(dir, "feat(i18n): localize GitLab");

    expect(changedFiles(root, "A", dir)).toContain(FOREIGN);
    expect(baseOf(dir, ["--base", root])).toBe(mainTip);
    expect(changedFiles(baseOf(dir, ["--base", root]), "A", dir)).toEqual([OWNED]);
  });

  it("does not advance a push build, where HEAD is the base branch tip", () => {
    const { dir, root } = initRepo();
    const mainTip = landOnMain(dir);
    setOriginMain(dir, mainTip);

    // `push` to main passes github.event.before. Advancing to the fork point
    // would compare HEAD with itself and wave the pushed commit through.
    expect(baseOf(dir, ["--base", root])).toBe(root);
    expect(changedFiles(baseOf(dir, ["--base", root]), "A", dir)).toContain(FOREIGN);
  });

  it("keeps an explicit base when there is no origin/main to floor against", () => {
    const { dir, root } = initRepo();
    git(dir, ["checkout", "-q", "-b", "pr"]);
    write(dir, OWNED, CLEAN_SOURCE);
    commit(dir, "feat: add");

    expect(baseOf(dir, ["--base", root])).toBe(root);
  });
});

describe("the guard-allowlist check inherits the same base", () => {
  /**
   * check-guard-allowlist.mjs reads the baseline with `git show <base>:<path>`.
   * With a stale base that baseline predates main's entries, so a PR deleting one
   * of them is not flagged — the removal-guard's whole purpose, silently lost.
   */
  it("reads a baseline that includes what main landed", () => {
    const { dir, root } = initRepo();
    const allowlist = "apps/web/eslint.i18n.options.mjs";

    git(dir, ["checkout", "-q", "-b", "pr"]);
    write(dir, OWNED, CLEAN_SOURCE);
    commit(dir, "feat: pr work");
    const prHead = git(dir, ["rev-parse", "HEAD"]).trim();

    git(dir, ["checkout", "-q", "main"]);
    write(dir, allowlist, 'export const i18nGuardFiles = ["components/settings/**"];\n');
    const mainTip = commit(dir, "feat: migrate settings");
    setOriginMain(dir, mainTip);
    git(dir, ["checkout", "-q", mainTip]);
    git(dir, ["merge", "-q", "--no-ff", "-m", "merge", prHead]);

    const stale = () => git(dir, ["show", `${root}:${allowlist}`], { quiet: true });
    expect(stale).toThrow(); // the baseline does not exist at the stale base

    expect(git(dir, ["show", `${baseOf(dir, ["--base", root])}:${allowlist}`])).toContain(
      "components/settings/**",
    );
  });
});
