// Cross-platform E2E `git` shim.
//
// The E2E backend invokes `git` via PATH lookup. Tests prepend a per-worker bin
// directory (see backend.ts) whose `git` launcher runs this script instead of
// the real binary. Written in Node so it behaves identically on macOS, Linux,
// and Windows and never depends on the developer's login shell (bash/zsh/fish)
// or on a POSIX `sh` being present. It replaces an earlier `/bin/sh` shim whose
// `case` block inside a `$(...)` substitution tripped bash 5.x's parser and
// failed every checkout routed through it.
//
// Behavior (all opt-in via env vars, absent files = transparent passthrough):
//   - Skip leading Git global options when finding the subcommand.
//   - If KANDEV_E2E_GIT_DELAY_FILE holds a positive ms value and the subcommand
//     is `fetch`/`pull`, sleep that long before running real git (simulates slow
//     network git so prepare-panel streaming stays observable). A JSON object
//     with `startedFile` and `releaseFile` provides a deterministic test gate.
//   - If the subcommand is `push` and KANDEV_E2E_GITLAB_PUSH_FILE matches the
//     repo's origin remote, record the push args and exit 0 without pushing.
//   - Otherwise exec the real git binary with the original args, restoring the
//     unshimmed PATH so it isn't found recursively.

import { spawnSync } from "node:child_process";
import fs from "node:fs";

/** Returns the first non-option token after Git's leading global options. */
function findSubcommand(args) {
  let i = 0;
  while (i < args.length) {
    const arg = args[i];
    if (arg === "-c" || arg === "-C") {
      i += 2;
      continue;
    }
    if (arg.startsWith("-")) {
      i += 1;
      continue;
    }
    return arg;
  }
  return "";
}

/** Blocking sleep in milliseconds, used to simulate slow network git. */
function sleepMs(ms) {
  const shared = new SharedArrayBuffer(4);
  Atomics.wait(new Int32Array(shared), 0, 0, ms);
}

function readFileSafe(file) {
  if (!file) return "";
  try {
    return fs.readFileSync(file, "utf8").trim();
  } catch {
    return "";
  }
}

/** Real git, found on the un-shimmed PATH so the shim isn't invoked recursively. */
function runRealGit(args, extraEnv) {
  const originalPath = process.env.KANDEV_E2E_ORIGINAL_PATH ?? "";
  const env = { ...process.env, PATH: originalPath, ...extraEnv };
  // Windows honors both PATH and Path; overwrite whichever the parent used.
  if ("Path" in env) env.Path = originalPath;
  const result = spawnSync("git", args, { stdio: "inherit", env });
  if (result.error) {
    process.stderr.write(`git-shim: failed to exec git: ${result.error.message}\n`);
    return 1;
  }
  if (typeof result.status === "number") return result.status;
  return 1; // terminated by signal
}

/** Sleeps before fetch/pull when a positive delay file is present. */
function maybeDelay(subcommand) {
  if (subcommand !== "fetch" && subcommand !== "pull") return;
  const raw = readFileSafe(process.env.KANDEV_E2E_GIT_DELAY_FILE);
  if (/^[0-9]+$/.test(raw)) {
    const delayMs = Number(raw);
    if (delayMs > 0) sleepMs(delayMs);
    return;
  }
  try {
    const gate = JSON.parse(raw);
    if (typeof gate.startedFile !== "string" || typeof gate.releaseFile !== "string") return;
    fs.writeFileSync(gate.startedFile, "started");
    while (!fs.existsSync(gate.releaseFile)) sleepMs(50);
  } catch {
    // Invalid or absent delay configuration means transparent passthrough.
  }
}

/**
 * Records a matching `git push` and returns true when it was intercepted, so
 * the caller skips the real push. Only intercepts when the configured remote
 * matches the repo's actual origin, mirroring the prior shell shim.
 */
function maybeInterceptPush(subcommand, args) {
  if (subcommand !== "push") return false;
  const expectedRemote = readFileSafe(process.env.KANDEV_E2E_GITLAB_PUSH_FILE);
  if (!expectedRemote) return false;
  const originalPath = process.env.KANDEV_E2E_ORIGINAL_PATH ?? "";
  const lookup = spawnSync("git", ["config", "--get", "remote.origin.url"], {
    env: { ...process.env, PATH: originalPath, Path: originalPath },
    encoding: "utf8",
  });
  const actualRemote = (lookup.stdout ?? "").trim();
  if (actualRemote !== expectedRemote) return false;
  const recordFile = process.env.KANDEV_E2E_GITLAB_PUSH_RECORD_FILE;
  if (recordFile) {
    try {
      fs.writeFileSync(recordFile, `${args.join(" ")}\n`);
    } catch {
      // Non-fatal: the test polling the record file will simply time out.
    }
  }
  return true;
}

function main() {
  const args = process.argv.slice(2);
  const subcommand = findSubcommand(args);
  maybeDelay(subcommand);
  if (maybeInterceptPush(subcommand, args)) {
    process.exit(0);
  }
  process.exit(runRealGit(args));
}

main();
