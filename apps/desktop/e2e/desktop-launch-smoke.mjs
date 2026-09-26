#!/usr/bin/env node
import { createServer } from "node:http";
import assert from "node:assert/strict";
import {
  chmod,
  copyFile,
  mkdir,
  mkdtemp,
  readFile,
  readdir,
  rm,
  writeFile,
} from "node:fs/promises";
import { existsSync } from "node:fs";
import { homedir, tmpdir } from "node:os";
import { delimiter, dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { execFileSync, spawn, spawnSync } from "node:child_process";

const __dirname = dirname(fileURLToPath(import.meta.url));
const desktopRoot = resolve(__dirname, "..");
const repoRoot = resolve(desktopRoot, "../..");
const backendRoot = join(repoRoot, "apps/backend");
const remoteHelperBuilder = join(repoRoot, "scripts/release/remote-helper-assets.mjs");
const helperNames = [
  "agentctl-linux-amd64",
  "agentctl-linux-arm64",
  "agentctl-darwin-amd64",
  "agentctl-darwin-arm64",
];
export const RELEASE_DESKTOP_STARTUP_TIMEOUT_MS = 120_000;
export const DESKTOP_SMOKE_VERSION = "0.0.0-e2e";

// The Rust side (apps/desktop/src-tauri/src/backend.rs) does a two-stage wait before it
// navigates the webview: wait_for_backend polls GET /health every 250ms against a bounded
// HEALTH_TIMEOUT (60s), then wait_for_ready polls GET /ready every 250ms with NO timeout —
// it only gives up on child exit or shutdown. HEALTH_REQUESTED_TIMEOUT_MS must stay above
// HEALTH_TIMEOUT so the fake runtime doesn't get killed here before the Rust side even
// finishes the first stage; desktop-launch-smoke.test.mjs asserts that relationship so the
// two stay in sync. READY_REQUESTED_TIMEOUT_MS only bounds this test — the fake runtime
// answers /ready immediately once it's listening, so the real wait_for_ready being unbounded
// doesn't matter here.
export const HEALTH_REQUESTED_TIMEOUT_MS = 90_000;
export const READY_REQUESTED_TIMEOUT_MS = 60_000;
export const ROOT_REQUESTED_TIMEOUT_MS = 60_000;

// Only run the CLI behavior when this file is executed directly (`node desktop-launch-smoke.mjs`
// or the fake-runtime re-exec below) — not when desktop-launch-smoke.test.mjs imports it.
const isEntryPoint = resolve(process.argv[1] ?? "") === fileURLToPath(import.meta.url);
if (isEntryPoint) {
  if (process.argv[2] === "--fake-runtime") {
    await runFakeRuntime(process.argv[3], process.argv.slice(4));
  } else {
    await runSmoke();
  }
}

async function runSmoke() {
  if (
    process.platform === "linux" &&
    !process.env.DISPLAY &&
    process.env.KANDEV_DESKTOP_SMOKE_XVFB !== "1"
  ) {
    if (!commandExists("xvfb-run")) {
      throw new Error("The desktop smoke requires DISPLAY or xvfb-run on Linux");
    }
    const result = spawnSync("xvfb-run", ["-a", process.execPath, fileURLToPath(import.meta.url)], {
      cwd: repoRoot,
      env: { ...process.env, KANDEV_DESKTOP_SMOKE_XVFB: "1" },
      stdio: "inherit",
    });
    if (result.error) throw result.error;
    if (result.status !== 0) throw new Error(`Desktop smoke exited with status ${result.status}`);
    return;
  }

  await runHappyPathSmoke();
  if (process.platform === "linux") {
    await runConflictRecoverySmoke();
  } else {
    console.log(
      "Desktop multi-window interaction smoke requires Linux X11 input support; skipped here.",
    );
  }
}

async function runHappyPathSmoke() {
  const appBinary = resolve(desktopRoot, "src-tauri/target/release/kandev-desktop");
  if (!existsSync(appBinary)) {
    throw new Error(`Missing desktop binary at ${appBinary}; run pnpm build first`);
  }

  const tmp = await mkdtemp(join(tmpdir(), "kandev-desktop-e2e-"));
  const runtimeDir = join(tmp, "runtime");
  const stateDir = join(tmp, "state");
  await mkdir(join(runtimeDir, "bin"), { recursive: true });
  await mkdir(stateDir, { recursive: true });

  await writeFakeRuntime(runtimeDir, stateDir, "happy");

  const child = spawn(appBinary, [], {
    cwd: repoRoot,
    detached: process.platform !== "win32",
    env: desktopSmokeEnvironment(runtimeDir),
    stdio: ["ignore", "pipe", "pipe"],
  });

  let stdout = "";
  let stderr = "";
  child.stdout?.on("data", (chunk) => {
    stdout += chunk;
  });
  child.stderr?.on("data", (chunk) => {
    stderr += chunk;
  });

  const failIfExited = () => {
    if (child.exitCode !== null) {
      throw new Error(`desktop app exited early with code ${child.exitCode}\n${stdout}\n${stderr}`);
    }
  };
  const describeChild = () => `[stdout]\n${stdout}\n[stderr]\n${stderr}`;

  try {
    await waitForFile(
      join(stateDir, "health-requested"),
      HEALTH_REQUESTED_TIMEOUT_MS,
      failIfExited,
      describeChild,
    );
    await waitForFile(
      join(stateDir, "ready-requested"),
      READY_REQUESTED_TIMEOUT_MS,
      failIfExited,
      describeChild,
    );
    await waitForFile(
      join(stateDir, "root-requested"),
      ROOT_REQUESTED_TIMEOUT_MS,
      failIfExited,
      describeChild,
    );
  } finally {
    await stopProcess(child);
    await rm(tmp, { recursive: true, force: true });
  }

  console.log(
    "Desktop smoke passed: WebView requested / after backend health and readiness succeeded.",
  );

  await runReleaseShapedSmoke(appBinary);
}

async function runReleaseShapedSmoke(appBinary) {
  if (process.platform !== "linux") {
    console.log("Release-shaped Desktop launcher smoke runs on Linux only; skipping.");
    return;
  }

  const commit = execFileSync("git", ["rev-parse", "HEAD"], {
    cwd: repoRoot,
    encoding: "utf8",
  }).trim();
  execFileSync(
    "make",
    [
      "build-kandev",
      "build-agentctl",
      "build-agentctl-remote",
      `VERSION=${DESKTOP_SMOKE_VERSION}`,
      `COMMIT=${commit}`,
    ],
    { cwd: backendRoot, stdio: "inherit" },
  );

  const tmp = await mkdtemp(join(tmpdir(), "kandev-desktop-release-e2e-"));
  const runtimeDir = join(tmp, "runtime");
  const homeDir = join(tmp, "home");
  const port = await findAvailablePort();
  const runtime = await writeReleaseShapedRuntime({
    sourceBinDir: join(backendRoot, "bin"),
    runtimeDir,
    homeDir,
    version: DESKTOP_SMOKE_VERSION,
    commit,
  });

  await runPackagedLauncherSmoke(runtimeDir, homeDir);

  const launchedViaXvfb = !process.env.DISPLAY && commandExists("xvfb-run");
  const command = launchedViaXvfb ? "xvfb-run" : appBinary;
  const args = launchedViaXvfb ? ["-a", appBinary] : [];
  const child = spawn(command, args, {
    cwd: repoRoot,
    detached: true,
    env: {
      ...process.env,
      HOME: join(tmp, "user-home"),
      KANDEV_DESKTOP_RUNTIME_DIR: runtimeDir,
      KANDEV_DESKTOP_PORT: String(port),
      KANDEV_HOME_DIR: homeDir,
      KANDEV_E2E_MOCK: "true",
      KANDEV_INTERNAL_CONFIG_FILE: "",
      KANDEV_AGENTCTL_LINUX_AMD64_BINARY: "",
      KANDEV_AGENTCTL_LINUX_BINARY: "",
      WEBKIT_DISABLE_COMPOSITING_MODE: "1",
      NO_AT_BRIDGE: "1",
    },
    stdio: ["ignore", "pipe", "pipe"],
  });

  let stdout = "";
  let stderr = "";
  child.stdout?.on("data", (chunk) => {
    stdout += chunk;
  });
  child.stderr?.on("data", (chunk) => {
    stderr += chunk;
  });
  const failIfExited = () => {
    if (child.exitCode !== null) {
      throw new Error(
        `release-shaped desktop app exited early with code ${child.exitCode}\n${stdout}\n${stderr}`,
      );
    }
  };
  const describeChild = () => `[stdout]\n${stdout}\n[stderr]\n${stderr}`;

  try {
    await waitForHttp(
      `http://127.0.0.1:${port}/ready`,
      RELEASE_DESKTOP_STARTUP_TIMEOUT_MS,
      failIfExited,
      undefined,
      describeChild,
    );
    await waitForHttp(
      `http://127.0.0.1:${port}/`,
      RELEASE_DESKTOP_STARTUP_TIMEOUT_MS,
      failIfExited,
      undefined,
      describeChild,
    );
    execFileSync(
      "go",
      [
        "test",
        "-count=1",
        "-run",
        "^TestAgentctlResolverPackagedDesktopBundleUsesPreseededCache$",
        "./internal/agent/runtime/lifecycle",
      ],
      {
        cwd: backendRoot,
        stdio: "inherit",
        env: {
          ...process.env,
          KANDEV_BUNDLE_DIR: runtimeDir,
          KANDEV_HOME_DIR: homeDir,
          KANDEV_AGENTCTL_LINUX_AMD64_BINARY: "",
          KANDEV_AGENTCTL_LINUX_BINARY: "",
          KANDEV_DESKTOP_SMOKE: "1",
          KANDEV_DESKTOP_SMOKE_BUNDLE_DIR: runtimeDir,
          KANDEV_DESKTOP_SMOKE_HOME_DIR: homeDir,
          KANDEV_DESKTOP_SMOKE_VERSION: DESKTOP_SMOKE_VERSION,
          KANDEV_DESKTOP_SMOKE_COMMIT: commit,
          KANDEV_INTERNAL_CONFIG_FILE: "",
        },
      },
    );
  } finally {
    await stopProcess(child);
    await rm(tmp, { recursive: true, force: true });
  }

  console.log(
    `Release-shaped Desktop smoke passed: the actual launcher served / from a standard bundle and the resolver selected the verified cached helper at ${runtime.cachePath}.`,
  );
}

async function runPackagedLauncherSmoke(runtimeDir, homeDir) {
  const launcherBinary = join(runtimeDir, "bin", "kandev");
  const launcherPort = await findAvailablePort();
  const child = spawn(launcherBinary, ["--headless", "--port", String(launcherPort)], {
    cwd: repoRoot,
    detached: true,
    env: {
      ...process.env,
      KANDEV_BUNDLE_DIR: runtimeDir,
      KANDEV_HOME_DIR: homeDir,
      KANDEV_E2E_MOCK: "true",
      KANDEV_INTERNAL_CONFIG_FILE: "",
      KANDEV_AGENTCTL_LINUX_AMD64_BINARY: "",
      KANDEV_AGENTCTL_LINUX_BINARY: "",
    },
    stdio: ["ignore", "pipe", "pipe"],
  });
  let stdout = "";
  let stderr = "";
  child.stdout?.on("data", (chunk) => {
    stdout += chunk;
  });
  child.stderr?.on("data", (chunk) => {
    stderr += chunk;
  });
  const failIfExited = () => {
    if (child.exitCode !== null) {
      throw new Error(
        `release bundle launcher exited early with code ${child.exitCode}\n${stdout}\n${stderr}`,
      );
    }
  };
  const describeChild = () => `[stdout]\n${stdout}\n[stderr]\n${stderr}`;

  try {
    await waitForHttp(
      `http://127.0.0.1:${launcherPort}/ready`,
      RELEASE_DESKTOP_STARTUP_TIMEOUT_MS,
      failIfExited,
      undefined,
      describeChild,
    );
    await waitForHttp(
      `http://127.0.0.1:${launcherPort}/`,
      RELEASE_DESKTOP_STARTUP_TIMEOUT_MS,
      failIfExited,
      undefined,
      describeChild,
    );
  } finally {
    await stopProcess(child);
  }
}

async function runConflictRecoverySmoke() {
  const appBinary = resolve(desktopRoot, "src-tauri/target/release/kandev-desktop");
  const tmp = await mkdtemp(join(tmpdir(), "kandev-desktop-conflict-e2e-"));
  const inputHelper = await buildX11InputHelper(join(tmp, "x11-window-input"));
  const runtimeDir = join(tmp, "runtime");
  const stateDir = join(runtimeDir, "e2e-state");
  const instancesDir = join(stateDir, "instances");
  await mkdir(join(runtimeDir, "bin"), { recursive: true });
  await mkdir(instancesDir, { recursive: true });
  await writeFakeRuntime(runtimeDir, stateDir, "conflict");

  const launcher = spawn(appBinary, [], {
    cwd: repoRoot,
    detached: true,
    env: desktopSmokeEnvironment(runtimeDir),
    stdio: ["ignore", "pipe", "pipe"],
  });
  let stdout = "";
  let stderr = "";
  let completed = false;
  launcher.stdout?.on("data", (chunk) => (stdout += chunk));
  launcher.stderr?.on("data", (chunk) => (stderr += chunk));
  const failIfLauncherExited = () => {
    if (launcher.exitCode !== null) {
      throw new Error(
        `conflict launcher exited early with code ${launcher.exitCode}\n${stdout}\n${stderr}`,
      );
    }
  };

  try {
    await waitForFile(
      join(stateDir, "conflict-requested"),
      HEALTH_REQUESTED_TIMEOUT_MS,
      failIfLauncherExited,
    );
    await waitForX11Window(inputHelper, launcher.pid, failIfLauncherExited);
    captureWindowScreenshot(inputHelper, launcher.pid, join(tmp, "conflict-startup.png"));

    await activateLauncherForInstance(
      inputHelper,
      launcher.pid,
      instancesDir,
      1,
      failIfLauncherExited,
    );
    const first = (await waitForReadyInstances(instancesDir, 1, failIfLauncherExited))[0];
    await activateLauncherForInstance(
      inputHelper,
      launcher.pid,
      instancesDir,
      2,
      failIfLauncherExited,
    );
    const instances = await waitForReadyInstances(instancesDir, 2, failIfLauncherExited);
    const second = instances.find((instance) => instance.pid !== first.pid);
    if (!second)
      throw new Error("the second temporary window did not start an independent backend");

    assertDistinctTemporaryInstances(first, second);
    if (!first.home.startsWith(tmpdir()) || !second.home.startsWith(tmpdir())) {
      throw new Error(
        "temporary windows must keep their homes below the operating-system temp directory",
      );
    }
    failIfLauncherExited();

    await sendX11(inputHelper, "quit", first.parentPid);
    await waitForX11WindowGone(inputHelper, first.parentPid, 15_000);
    await waitForPathRemoval(first.home, 15_000);
    failIfLauncherExited();
    const healthyResponse = await fetch(`${second.origin}/health`);
    if (
      !healthyResponse.ok ||
      healthyResponse.headers.get("x-kandev-desktop-health-token") !== second.token
    ) {
      throw new Error("closing one temporary window disturbed the other window's backend");
    }

    await sendX11(inputHelper, "quit", second.parentPid);
    await waitForX11WindowGone(inputHelper, second.parentPid, 15_000);
    await waitForPathRemoval(second.home, 15_000);
    failIfLauncherExited();

    completed = true;
    console.log(
      "Desktop recovery smoke passed: two isolated windows reached readiness; closing either removed only its own home while the sibling backend and conflict launcher stayed active.",
    );
  } finally {
    if (!completed && processIsRunning(launcher.pid)) {
      try {
        captureWindowScreenshot(inputHelper, launcher.pid, join(tmp, "conflict-final.png"));
      } catch {
        // Preserve the primary smoke failure when the window is already gone.
      }
    }
    await stopProcess(launcher);
    if (completed) {
      await rm(tmp, { recursive: true, force: true });
    } else {
      console.error(`Desktop conflict smoke artifacts preserved at ${tmp}`);
    }
  }
}

function captureWindowScreenshot(inputHelper, pid, path) {
  if (!commandExists("import")) return;
  const windowId = execFileSync(inputHelper, ["find", String(pid)], { encoding: "utf8" }).trim();
  execFileSync("import", ["-window", windowId, path]);
}

function desktopSmokeEnvironment(runtimeDir) {
  const environment = {
    ...process.env,
    KANDEV_DESKTOP_RUNTIME_DIR: runtimeDir,
    WEBKIT_DISABLE_COMPOSITING_MODE: "1",
    NO_AT_BRIDGE: "1",
  };
  delete environment.KANDEV_HOME_DIR;
  delete environment.KANDEV_DATABASE_PATH;
  delete environment.KANDEV_DATABASE_DRIVER;
  delete environment.KANDEV_INTERNAL_CONFIG_FILE;
  return environment;
}

async function buildX11InputHelper(outputPath) {
  const sourcePath = join(__dirname, "x11-window-input.c");
  let flags;
  try {
    flags = execFileSync("pkg-config", ["--cflags", "--libs", "x11", "xtst"], {
      encoding: "utf8",
    })
      .trim()
      .split(/\s+/)
      .filter(Boolean);
  } catch (error) {
    throw new Error(`The Linux recovery smoke requires X11 and Xtst development files: ${error}`);
  }
  execFileSync("cc", [sourcePath, "-o", outputPath, ...flags]);
  return outputPath;
}

async function waitForX11Window(inputHelper, pid, tick) {
  await waitForCondition(
    () => {
      tick?.();
      try {
        execFileSync(inputHelper, ["find", String(pid)], { stdio: "ignore" });
        return true;
      } catch {
        return false;
      }
    },
    15_000,
    `Kandev window for process ${pid}`,
  );
}

async function waitForX11WindowGone(inputHelper, pid, timeoutMs) {
  await waitForCondition(
    () => {
      try {
        execFileSync(inputHelper, ["find", String(pid)], { stdio: "ignore" });
        return false;
      } catch {
        return true;
      }
    },
    timeoutMs,
    `Kandev window for process ${pid} to close`,
  );
}

async function activateLauncherForInstance(inputHelper, pid, instancesDir, count, tick) {
  tick?.();
  // The fake runtime writes its conflict marker before the launcher drains stderr and updates the WebView.
  await new Promise((resolveWait) => setTimeout(resolveWait, 750));
  execFileSync(inputHelper, ["activate", String(pid)]);
  process.stdout.write("Desktop recovery smoke: activated the isolated-test action.\n");
  await waitForCondition(
    async () => (await readInstances(instancesDir)).length >= count,
    90_000,
    `${count} temporary backend instance(s)`,
  );
}

async function waitForReadyInstances(instancesDir, count, tick) {
  await waitForCondition(
    async () => {
      tick?.();
      const instances = await readInstances(instancesDir);
      return instances.filter((instance) => instance.rootRequested).length >= count;
    },
    ROOT_REQUESTED_TIMEOUT_MS,
    `${count} ready temporary instance(s)`,
  );
  return (await readInstances(instancesDir))
    .filter((instance) => instance.rootRequested)
    .sort((left, right) => left.pid - right.pid)
    .slice(0, count);
}

async function readInstances(instancesDir) {
  const entries = await readdir(instancesDir, { withFileTypes: true });
  const instances = [];
  for (const entry of entries) {
    if (!entry.isDirectory()) continue;
    try {
      instances.push(
        JSON.parse(await readFile(join(instancesDir, entry.name, "instance.json"), "utf8")),
      );
    } catch (error) {
      if (error.code !== "ENOENT") throw error;
    }
  }
  return instances;
}

function assertDistinctTemporaryInstances(first, second) {
  assert.ok(first.home && second.home, "temporary backend must receive a temporary home");
  assert.notEqual(first.home, second.home, "temporary windows must own different homes");
  assert.equal(first.databasePath, join(first.home, "data", "kandev.db"));
  assert.equal(second.databasePath, join(second.home, "data", "kandev.db"));
  assert.notEqual(
    first.databasePath,
    second.databasePath,
    "temporary windows must use different databases",
  );
  assert.notEqual(first.port, second.port, "temporary windows must use different backend ports");
  assert.notEqual(
    first.origin,
    second.origin,
    "temporary windows must use different owned origins",
  );
  assert.notEqual(first.token, second.token, "temporary windows must use different health tokens");
  assert.ok(first.healthRequested && first.readyRequested && first.rootRequested);
  assert.ok(second.healthRequested && second.readyRequested && second.rootRequested);
}

async function sendX11(inputHelper, command, pid) {
  await waitForX11Window(inputHelper, pid);
  execFileSync(inputHelper, [command, String(pid)]);
}

async function waitForProcessExit(pid, timeoutMs) {
  await waitForCondition(() => !processIsRunning(pid), timeoutMs, `process ${pid} to exit`);
}

async function waitForPathRemoval(path, timeoutMs) {
  await waitForCondition(() => !existsSync(path), timeoutMs, `${path} to be removed`);
}

function processIsRunning(pid) {
  try {
    process.kill(pid, 0);
    return true;
  } catch (error) {
    if (error.code === "ESRCH") return false;
    throw error;
  }
}

async function waitForCondition(predicate, timeoutMs, label) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (await predicate()) return;
    await new Promise((resolveWait) => setTimeout(resolveWait, 200));
  }
  throw new Error(`Timed out waiting for ${label}`);
}

export async function writeReleaseShapedRuntime({
  sourceBinDir,
  runtimeDir,
  homeDir,
  version,
  commit,
}) {
  const baseDir = dirname(runtimeDir);
  const bundleBinDir = join(runtimeDir, "bin");
  const helperInputDir = join(baseDir, "remote-helper-inputs");
  const helperArtifactDir = join(baseDir, "remote-helper-artifact");
  await mkdir(bundleBinDir, { recursive: true });
  await mkdir(helperInputDir, { recursive: true });

  for (const name of ["kandev", "agentctl"]) {
    const target = join(bundleBinDir, name);
    await copyFile(join(sourceBinDir, name), target);
    await chmod(target, 0o755);
  }
  for (const name of helperNames) {
    const target = join(helperInputDir, name);
    await copyFile(join(sourceBinDir, name), target);
    await chmod(target, 0o755);
  }

  execFileSync(
    process.execPath,
    [
      remoteHelperBuilder,
      "build",
      "--bin-dir",
      helperInputDir,
      "--output-dir",
      helperArtifactDir,
      "--version",
      version,
      "--commit",
      commit,
      "--stable",
      "true",
    ],
    { cwd: repoRoot, stdio: "inherit" },
  );

  const manifestPath = join(helperArtifactDir, "manifests", "standard", "remote-helpers.json");
  const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
  await copyFile(manifestPath, join(runtimeDir, "remote-helpers.json"));
  const helper = manifest.helpers.find((record) => record.platform === "linux/amd64");
  if (!helper) {
    throw new Error("Generated Desktop manifest is missing the linux/amd64 helper");
  }
  const cachePath = join(
    homeDir,
    "cache",
    "remote-helpers",
    version,
    "linux-amd64",
    helper.sha256,
    "agentctl",
  );
  await mkdir(dirname(cachePath), { recursive: true });
  await copyFile(join(helperInputDir, "agentctl-linux-amd64"), cachePath);
  await chmod(cachePath, 0o755);

  return { manifest, cachePath, runtimeDir, homeDir };
}

export async function writeFakeRuntime(runtimeDir, stateDir, scenario) {
  const fakeRuntime = join(
    runtimeDir,
    "bin",
    process.platform === "win32" ? "kandev.cmd" : "kandev",
  );
  const agentctl = join(
    runtimeDir,
    "bin",
    process.platform === "win32" ? "agentctl.cmd" : "agentctl",
  );
  await writeFile(join(stateDir, "scenario"), scenario);

  if (process.platform === "win32") {
    await writeFile(
      fakeRuntime,
      `@echo off\r\nnode "${fileURLToPath(import.meta.url)}" --fake-runtime "${stateDir}" %*\r\n`,
    );
    await writeFile(agentctl, "@echo off\r\necho fake agentctl\r\n");
  } else {
    await writeFile(
      fakeRuntime,
      `#!/usr/bin/env bash\nexec node "${fileURLToPath(import.meta.url)}" --fake-runtime "${stateDir}" "$@"\n`,
    );
    await writeFile(agentctl, "#!/usr/bin/env bash\necho fake agentctl\n");
    await chmod(fakeRuntime, 0o755);
    await chmod(agentctl, 0o755);
  }

  const helpers = [
    ["linux/amd64", "agentctl-linux-amd64.gz"],
    ["linux/arm64", "agentctl-linux-arm64.gz"],
    ["darwin/amd64", "agentctl-darwin-amd64.gz"],
    ["darwin/arm64", "agentctl-darwin-arm64.gz"],
  ].map(([platform, asset]) => ({ platform, asset, sha256: "0".repeat(64), size_bytes: 1 }));
  await writeFile(
    join(runtimeDir, "remote-helpers.json"),
    `${JSON.stringify(
      {
        schema_version: 1,
        version: "0.0.0",
        commit: "0".repeat(40),
        variant: "standard",
        helpers,
      },
      null,
      2,
    )}\n`,
  );
}

async function runFakeRuntime(stateDir, args) {
  const portIndex = args.indexOf("--port");
  const port = portIndex >= 0 ? Number(args[portIndex + 1]) : 0;
  const isHeadless = args.includes("--headless");

  if (!isHeadless || !Number.isInteger(port) || port <= 0) {
    await writeFile(join(stateDir, "invalid-args"), JSON.stringify(args));
    process.exit(2);
  }

  const scenario = (await readFile(join(stateDir, "scenario"), "utf8")).trim();
  if (scenario === "conflict" && !process.env.KANDEV_HOME_DIR) {
    const targetPath = join(process.env.HOME || homedir(), ".kandev");
    const conflict = {
      version: 1,
      target_kind: "home",
      target_path: targetPath,
      storage_kind: "sqlite_in_home",
      database_path: join(targetPath, "data", "kandev.db"),
      owner: {
        pid: process.pid,
        executable: "fake-kandev",
        started_at: new Date().toISOString(),
      },
    };
    await writeFile(join(stateDir, "conflict-requested"), JSON.stringify(conflict));
    process.stderr.write(`KANDEV_DESKTOP_CONFLICT_V1 ${JSON.stringify(conflict)}\n`);
    process.exitCode = 1;
    return;
  }

  const isTemporaryInstance = scenario === "conflict";
  const instanceDir = isTemporaryInstance
    ? join(stateDir, "instances", String(process.pid))
    : stateDir;
  await mkdir(instanceDir, { recursive: true });
  const record = {
    pid: process.pid,
    parentPid: process.ppid,
    home: process.env.KANDEV_HOME_DIR || "",
    databasePath: process.env.KANDEV_DATABASE_PATH || "",
    port,
    token: process.env.KANDEV_DESKTOP_HEALTH_TOKEN || "",
    origin: `http://127.0.0.1:${port}`,
    healthRequested: false,
    readyRequested: false,
    rootRequested: false,
  };
  const saveRecord = () =>
    writeFile(join(instanceDir, "instance.json"), JSON.stringify(record, null, 2));
  await saveRecord();
  await writeFile(join(instanceDir, "launched"), JSON.stringify({ args, port }));

  const server = createServer(async (req, res) => {
    if (req.url === "/health") {
      record.healthRequested = true;
      await saveRecord();
      await writeFile(join(instanceDir, "health-requested"), "1");
      const headers = { "content-type": "application/json" };
      if (process.env.KANDEV_DESKTOP_HEALTH_TOKEN) {
        headers["x-kandev-desktop-health-token"] = process.env.KANDEV_DESKTOP_HEALTH_TOKEN;
      }
      res.writeHead(200, headers);
      res.end('{"status":"ok"}');
      return;
    }

    if (req.url === "/ready") {
      record.readyRequested = true;
      await saveRecord();
      await writeFile(join(instanceDir, "ready-requested"), "1");
      res.writeHead(200, { "content-type": "application/json" });
      res.end('{"status":"ok"}');
      return;
    }

    if (req.url === "/") {
      record.rootRequested = true;
      await saveRecord();
      await writeFile(join(instanceDir, "root-requested"), "1");
      res.writeHead(200, { "content-type": "text/html" });
      res.end("<!doctype html><title>Kandev</title><main>Kandev desktop smoke</main>");
      return;
    }

    res.writeHead(404);
    res.end("not found");
  });

  await new Promise((resolveListen) => server.listen(port, "127.0.0.1", resolveListen));

  let stopping = false;
  const stop = async () => {
    if (stopping) return;
    stopping = true;
    record.terminated = true;
    await saveRecord();
    await writeFile(join(instanceDir, "terminated"), "1");
    server.close(() => process.exit(0));
  };
  process.on("SIGTERM", stop);
  process.on("SIGINT", stop);
}

export async function waitForFile(path, timeoutMs, tick, describeDetail) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    tick?.();
    if (existsSync(path)) {
      return;
    }
    await new Promise((resolveWait) => setTimeout(resolveWait, 250));
  }
  const detail = describeDetail?.();
  throw new Error(`Timed out waiting for ${path}${detail ? `\n\n${detail}` : ""}`);
}

export async function waitForHttp(
  url,
  timeoutMs,
  tick,
  pause = (ms) => new Promise((resolveWait) => setTimeout(resolveWait, ms)),
  describeDetail,
) {
  const deadline = Date.now() + timeoutMs;
  let lastError = "no response";
  while (Date.now() < deadline) {
    tick?.();
    try {
      const response = await fetch(url, { signal: AbortSignal.timeout(2_000) });
      if (response.ok) return;
      lastError = `HTTP ${response.status}`;
    } catch (error) {
      lastError = error.message;
    }
    await pause(100);
  }
  const detail = describeDetail?.();
  throw new Error(
    `Timed out waiting for ${url} to return success (last result: ${lastError})${detail ? `\n\n${detail}` : ""}`,
  );
}

async function findAvailablePort() {
  const server = createServer();
  await new Promise((resolveListen, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolveListen);
  });
  const address = server.address();
  if (!address || typeof address === "string") {
    throw new Error("Could not determine an available Desktop smoke port");
  }
  await new Promise((resolveClose, reject) => {
    server.close((error) => (error ? reject(error) : resolveClose()));
  });
  return address.port;
}

async function stopProcess(child) {
  if (child.exitCode !== null || child.signalCode !== null) {
    return;
  }
  if (process.platform === "win32") {
    child.kill();
  } else {
    process.kill(-child.pid, "SIGTERM");
  }
  await Promise.race([
    new Promise((resolveExit) => child.once("exit", resolveExit)),
    new Promise((resolveTimeout) => setTimeout(resolveTimeout, 5_000)),
  ]);
  if (child.exitCode === null && child.signalCode === null) {
    if (process.platform === "win32") {
      child.kill("SIGKILL");
    } else {
      process.kill(-child.pid, "SIGKILL");
    }
  }
}

function commandExists(command) {
  const path = process.env.PATH ?? "";
  return path
    .split(delimiter)
    .filter(Boolean)
    .some((entry) => existsSync(join(entry, command)));
}
