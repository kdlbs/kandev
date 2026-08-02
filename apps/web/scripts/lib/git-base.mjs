/**
 * Shared git plumbing for the i18n ratchet checks.
 *
 * Both `check-new-code-i18n.mjs` and `check-guard-allowlist.mjs` need the same
 * fork point and the same fail-closed policy about it, so it lives here rather
 * than being duplicated and drifting.
 */
import { execFileSync } from "node:child_process";
import path from "node:path";

export const WEB_DIR = path.resolve(import.meta.dirname, "..", "..");
export const REPO_ROOT = path.resolve(WEB_DIR, "..", "..");

export function git(args, { quiet = false } = {}) {
  return execFileSync("git", args, {
    cwd: REPO_ROOT,
    encoding: "utf8",
    maxBuffer: 64 * 1024 * 1024,
    stdio: ["ignore", "pipe", quiet ? "ignore" : "inherit"],
  });
}

/**
 * Git reports paths with `/` on every platform; `path.relative` returns `\` on
 * Windows. Comparing the two silently matches nothing, which would make the
 * ratchet discard every finding and report "clean" — the worst failure mode for
 * a guard, because it looks like success.
 */
export function toPosixPath(value) {
  return value.split(path.sep).join("/");
}

/**
 * The fork point to judge against, or `null` when the caller explicitly allows
 * running without one.
 *
 * Fail-closed: if the caller passed `--base`, or we are in CI, an unresolvable
 * base means a broken checkout (usually a shallow clone that never fetched the
 * base), and skipping would turn that into a silent pass. Only a bare local run
 * — a developer with no `origin/main`, e.g. offline or in a fresh init repo —
 * is allowed to skip, because failing there would block unrelated commits for
 * an environment reason.
 *
 * @returns {{ base: string } | { skip: string }}
 */
export function resolveBase(argv = process.argv) {
  const flag = argv.indexOf("--base");
  const explicit = flag !== -1 && argv[flag + 1] ? argv[flag + 1] : null;
  const requested = explicit ?? "origin/main";

  try {
    return { base: git(["merge-base", requested, "HEAD"], { quiet: true }).trim() };
  } catch {
    if (explicit || process.env.CI) {
      console.error(
        `\n✖ cannot resolve a base to compare against (tried "${requested}").\n` +
          `  In CI this means the checkout is incomplete — a shallow clone that\n` +
          `  never fetched the base branch. Refusing to pass silently.\n` +
          `  Fetch the base ref, or pass --base <sha>.\n`,
      );
      process.exit(1);
    }
    return { skip: `no base ref (tried "${requested}"); pass --base <sha> to check explicitly` };
  }
}
