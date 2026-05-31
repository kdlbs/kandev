import { spawnSync } from "node:child_process";

/**
 * launchd's `bootout` is asynchronous: the command returns as soon as launchd
 * accepts the request, but the job (and, for a live service, its process) may
 * still be tearing down. Issuing `bootstrap` for the same label before that
 * teardown finishes races launchd and surfaces as:
 *
 *   Bootstrap failed: 5: Input/output error
 *
 * This is invisible on a fresh install (nothing to tear down) and in unit tests
 * (no live launchctl), but it reliably breaks self-update — there the service is
 * actively running when `kandev service install` reloads it. The helpers below
 * make the bootout→bootstrap dance robust: wait for the old job to actually
 * disappear, then retry the bootstrap through the brief residual EIO window.
 */

export type LaunchctlResult = { status: number | null };
export type LaunchctlStdio = "inherit" | "ignore";
export type LaunchctlRunner = (args: string[], stdio: LaunchctlStdio) => LaunchctlResult;
export type SleepFn = (ms: number) => void;

export type LaunchctlDeps = {
  run?: LaunchctlRunner;
  sleep?: SleepFn;
};

const BOOTOUT_POLL_INTERVAL_MS = 100;
const BOOTOUT_POLL_ATTEMPTS = 50; // ~5s ceiling waiting for teardown
const BOOTSTRAP_MAX_ATTEMPTS = 5;
const BOOTSTRAP_RETRY_BASE_MS = 300;

export function spawnLaunchctl(args: string[], stdio: LaunchctlStdio): LaunchctlResult {
  const res = spawnSync("launchctl", args, { stdio });
  return { status: res.status };
}

// Block the current thread for `ms`. The launchctl orchestration runs in the
// synchronous `service install`/`start`/`restart` paths, so we can't await a
// timer — Atomics.wait gives a dependency-free blocking sleep.
export function sleepSync(ms: number): void {
  if (ms <= 0) return;
  Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, ms);
}

function isLoaded(target: string, run: LaunchctlRunner): boolean {
  // `launchctl print <target>` exits non-zero once the label is gone.
  return run(["print", target], "ignore").status === 0;
}

/**
 * Boot out a launchd job and wait until launchd has fully removed it. A no-op
 * (returns promptly) when the label isn't loaded, so fresh installs aren't
 * slowed down.
 */
export function bootoutAndWait(target: string, deps: LaunchctlDeps = {}): void {
  const run = deps.run ?? spawnLaunchctl;
  const sleep = deps.sleep ?? sleepSync;
  run(["bootout", target], "ignore");
  for (let attempt = 0; attempt < BOOTOUT_POLL_ATTEMPTS; attempt += 1) {
    if (!isLoaded(target, run)) {
      return;
    }
    sleep(BOOTOUT_POLL_INTERVAL_MS);
  }
}

/**
 * Bootstrap a launchd job, retrying through the transient EIO that launchd
 * returns while a previous instance of the same label finishes tearing down.
 * Throws with the last exit code if every attempt fails.
 */
export function bootstrapWithRetry(
  domain: string,
  plistPath: string,
  deps: LaunchctlDeps = {},
): void {
  const run = deps.run ?? spawnLaunchctl;
  const sleep = deps.sleep ?? sleepSync;
  let lastStatus: number | null = null;
  for (let attempt = 1; attempt <= BOOTSTRAP_MAX_ATTEMPTS; attempt += 1) {
    lastStatus = run(["bootstrap", domain, plistPath], "inherit").status;
    if (lastStatus === 0) {
      return;
    }
    if (attempt < BOOTSTRAP_MAX_ATTEMPTS) {
      sleep(BOOTSTRAP_RETRY_BASE_MS * attempt);
    }
  }
  throw new Error(`launchctl bootstrap ${domain} ${plistPath} failed with code ${lastStatus}`);
}

/**
 * Reload a launchd job: fully unload (waiting for teardown) then bootstrap with
 * retry. This is the safe sequence whenever the target might currently be
 * running — installs that refresh a live service, and `start`.
 */
export function reloadService(
  target: string,
  domain: string,
  plistPath: string,
  deps: LaunchctlDeps = {},
): void {
  bootoutAndWait(target, deps);
  bootstrapWithRetry(domain, plistPath, deps);
}
