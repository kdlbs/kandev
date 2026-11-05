/**
 * Source-start build-mode and cleanup checks.
 *
 * The shared observation helpers live in lib/debug-start-production.mjs.
 */
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import fs from "node:fs";
import fsp from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import {
  POSIX,
  assertOwnedDatabase,
  buildObservationReport,
  collectObservation,
  collectOwnedBackendPids,
  createHostileInputs,
  createLaunchEnvironment,
  createObservedBrowser,
  createRunWorkspace,
  expectedReact,
  isOwnedBackendProcess,
  openWorkloadPage,
  processGroupAlive,
  readFileTail,
  readInstalledReactVersion,
  repo,
  spawnDebugSource,
  startTimeoutMs,
  stopLauncher,
  testTimeoutMs,
  waitForHttp,
} from "./lib/debug-start-production.mjs";

test("launch environment pins a private sqlite database over hostile inputs", () => {
  const homeDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-5050-env-home-"));
  const runDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-5050-env-run-"));
  try {
    const { sentinelDb, sentinelContents, hostileEnv } = createHostileInputs(runDir);
    const { env, configPath, dbPath } = createLaunchEnvironment(homeDir, 12345, runDir, hostileEnv);

    assert.equal(env.KANDEV_DATABASE_DRIVER, "sqlite");
    assert.equal(env.KANDEV_DATABASE_PATH, path.join(homeDir, "data/kandev.db"));
    assert.equal(env.KANDEV_INTERNAL_CONFIG_FILE, configPath);
    assert.equal(env.KANDEV_DATABASE_HOST, undefined);
    assert.equal(env.KANDEV_DATABASE_DBNAME, undefined);
    assert.equal(env.PGHOST, undefined);
    assert.equal(env.PGDATABASE, undefined);
    const config = fs.readFileSync(configPath, "utf8");
    assert.match(config, /driver: "sqlite"/);
    assert.ok(
      config.includes(JSON.stringify(dbPath)),
      "the private config names the owned database",
    );
    assert.equal(fs.readFileSync(sentinelDb, "utf8"), sentinelContents);
  } finally {
    fs.rmSync(homeDir, { recursive: true, force: true });
    fs.rmSync(runDir, { recursive: true, force: true });
  }
});

test("backend ownership requires this checkout, entrypoint, and group leader", () => {
  const owned = {
    pid: 1234,
    ppid: 1,
    pgid: 1234,
    command: `${repo}/apps/backend/bin/kandev __backend`,
  };
  assert.equal(isOwnedBackendProcess(owned), true);
  assert.equal(
    isOwnedBackendProcess({ ...owned, command: "/other/checkout/bin/kandev __backend" }),
    false,
  );
  assert.equal(isOwnedBackendProcess({ ...owned, command: `${repo}/bin/other` }), false);
  assert.equal(isOwnedBackendProcess({ ...owned, pgid: 99 }), false);
});

test("owned backend discovery follows descendants and managed group identity", () => {
  const table = [
    { pid: 100, ppid: 1, pgid: 100, command: "make start-debug" },
    { pid: 101, ppid: 100, pgid: 100, command: "kandev-launcher" },
    {
      pid: 102,
      ppid: 101,
      pgid: 102,
      command: `${repo}/apps/backend/bin/kandev __backend`,
    },
    {
      pid: 103,
      ppid: 101,
      pgid: 101,
      command: `${repo}/apps/backend/bin/kandev __backend`,
    },
    { pid: 104, ppid: 101, pgid: 104, command: "/other/checkout/bin/kandev __backend" },
  ];
  assert.deepEqual(collectOwnedBackendPids([100], table), [102]);
});

test(
  "cleanup drains an owned group after an early startup failure",
  { skip: POSIX ? false : "POSIX process groups are required" },
  async () => {
    const stopDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-5050-stop-"));
    const startLog = path.join(stopDir, "start.log");
    const child = spawn("bash", ["-c", "sleep 300 & exit 1"], {
      detached: true,
      stdio: "ignore",
    });
    try {
      await new Promise((resolve) => child.once("exit", resolve));
      await stopLauncher(child, startLog);
      assert.equal(processGroupAlive(child.pid), false, "the descendant group is drained");
    } finally {
      await stopLauncher(child, startLog).catch(() => {});
      fs.rmSync(stopDir, { recursive: true, force: true });
    }
  },
);

test(
  "cleanup is idempotent for repeated stop requests",
  { skip: POSIX ? false : "POSIX process groups are required" },
  async () => {
    const stopDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-5050-stop-"));
    const startLog = path.join(stopDir, "start.log");
    const child = spawn("bash", ["-c", "sleep 300"], {
      detached: true,
      stdio: "ignore",
    });
    try {
      await Promise.all([stopLauncher(child, startLog), stopLauncher(child, startLog)]);
      assert.equal(processGroupAlive(child.pid), false, "repeated cleanup drains the group");
    } finally {
      await stopLauncher(child, startLog).catch(() => {});
      fs.rmSync(stopDir, { recursive: true, force: true });
    }
  },
);

test(
  `debug source start serves ${expectedReact} React with diagnostics`,
  {
    skip: process.platform === "win32" ? "source startup is verified on POSIX hosts" : false,
    timeout: testTimeoutMs,
  },
  async (t) => {
    const { homeDir, runDir, startLog, port, startedAt } = await createRunWorkspace();
    const baseUrl = `http://127.0.0.1:${port}`;

    const { sentinelDb, sentinelContents, hostileEnv } = createHostileInputs(runDir);
    const { env, configPath, dbPath } = createLaunchEnvironment(homeDir, port, runDir, hostileEnv);

    await fsp.rm(path.join(repo, "apps/web/dist"), { recursive: true, force: true });
    const { child, logStream } = spawnDebugSource(env, startLog);

    let browser;
    let stopPromise;
    const requestStop = () => (stopPromise ??= stopLauncher(child, startLog));
    const onSignal = (signal) => {
      void requestStop().finally(() => {
        process.removeListener("SIGINT", onInterrupt);
        process.removeListener("SIGTERM", onTerminate);
        process.kill(process.pid, signal);
      });
    };
    const onInterrupt = () => onSignal("SIGINT");
    const onTerminate = () => onSignal("SIGTERM");
    process.once("SIGINT", onInterrupt);
    process.once("SIGTERM", onTerminate);
    const watchdog = setTimeout(
      () => {
        void (async () => {
          if (browser) await browser.close().catch(() => {});
          await requestStop();
        })();
      },
      Math.max(30_000, testTimeoutMs - 60_000),
    );
    watchdog.unref();

    try {
      await waitForHttp(`${baseUrl}/health`, startTimeoutMs, child);
      const shellResponse = await waitForHttp(`${baseUrl}/`, 60_000, child);
      const shell = await shellResponse.text();
      await assertOwnedDatabase(startLog, homeDir, dbPath);

      const buildIndex = await fsp.stat(path.join(repo, "apps/web/dist/index.html"));
      assert.ok(
        buildIndex.mtimeMs >= startedAt,
        "the debug start rebuilt the web assets instead of reusing stale output",
      );

      const reactVersion = readInstalledReactVersion();
      const observed = await createObservedBrowser(baseUrl, reactVersion);
      browser = observed.browser;
      const taskId = await openWorkloadPage(observed.page, baseUrl, shell);

      const observation = await collectObservation({
        browser,
        context: observed.context,
        page: observed.page,
        debugMessages: observed.debugMessages,
        pageErrors: observed.pageErrors,
        taskId,
        baseUrl,
        homeDir,
        repoRoot: repo,
        startedAt,
        sentinelDb,
        sentinelContents,
      });

      const report = buildObservationReport({
        expectedReact,
        repo,
        reactVersion,
        browser,
        configPath,
        dbPath,
        assets: observation.assets,
        kandevDebugMessage: observation.kandevDebugMessage,
        documentLoads: observation.documentLoads,
        workloadRounds: observation.workloadRounds,
        samples: observation.samples,
      });
      console.log(`KANDEV_DEBUG_START_REPORT ${JSON.stringify(report)}`);

      const first = observation.samples[0];
      const last = observation.samples[observation.samples.length - 1];
      t.diagnostic(
        `React ${expectedReact} ${reactVersion}; assets=${observation.assets.assetCount}; ` +
          `workload rounds=${observation.workloadRounds.length}; ` +
          `measures ${first.measureCount}->${last.measureCount}; ` +
          `JS heap ${first.jsHeapUsedBytes}->${last.jsHeapUsedBytes} bytes`,
      );
    } catch (error) {
      const tail = await readFileTail(startLog, 80);
      throw new Error(
        `${error instanceof Error ? error.message : String(error)}\n\n--- make start-debug tail ---\n${tail}`,
      );
    } finally {
      clearTimeout(watchdog);
      process.removeListener("SIGINT", onInterrupt);
      process.removeListener("SIGTERM", onTerminate);
      if (browser) await browser.close().catch(() => {});
      await requestStop();
      if (!logStream.writableEnded) {
        await new Promise((resolve) => logStream.end(resolve));
      }
      await fsp.rm(homeDir, { recursive: true, force: true });
      await fsp.rm(runDir, { recursive: true, force: true });
    }
  },
);
