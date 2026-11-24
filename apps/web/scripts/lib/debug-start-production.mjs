/**
 * Integration check for the debug source-start path.
 *
 * `make start-debug` must serve production React even when the invoking shell
 * inherits NODE_ENV=development, while Kandev runtime diagnostics stay active.
 * The check also owns the bounded memory observation used by
 * docs/plans/debug-start-production-react.
 *
 * Baseline mode (`KANDEV_DEBUG_START_REPO` plus
 * `KANDEV_DEBUG_START_EXPECT_REACT=development`) runs the same assertions
 * against a checkout that predates the build-environment fix.
 */
import assert from "node:assert/strict";
import { spawn, execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import fs from "node:fs";
import fsp from "node:fs/promises";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import { chromium } from "@playwright/test";
import { delay, repo } from "./debug-start-production-process.mjs";
export {
  POSIX,
  collectOwnedBackendPids,
  isOwnedBackendProcess,
  processGroupAlive,
  repo,
  stopLauncher,
} from "./debug-start-production-process.mjs";

export const expectedReact = process.env.KANDEV_DEBUG_START_EXPECT_REACT || "production";
if (expectedReact !== "production" && expectedReact !== "development") {
  throw new Error(
    `KANDEV_DEBUG_START_EXPECT_REACT must be production or development, got ${expectedReact}`,
  );
}

export const soakMinutes = readNonNegativeInteger(process.env.KANDEV_DEBUG_START_SOAK_MINUTES);
export const startTimeoutMs = 12 * 60_000;
export const testTimeoutMs = Math.max(startTimeoutMs, (soakMinutes + 15) * 60_000);

export const TASK_TITLE = "5050 memory probe";
export const REACT_PACKAGE = "react-dom";
export const DEVELOPMENT_SENTINELS = [
  "Cannot update a component",
  "Each child in a list",
  "Invalid hook call",
];

/**
 * Read a non-negative integer environment input.
 *
 * @param {string | undefined} value
 * @returns {number}
 */
export function readNonNegativeInteger(value) {
  if (value === undefined || value === "") return 0;
  const parsed = Number(value);
  if (!Number.isInteger(parsed) || parsed < 0) {
    throw new Error(`expected a non-negative integer, got ${value}`);
  }
  return parsed;
}

/**
 * Reserve a free loopback port for the launcher.
 *
 * @returns {Promise<number>}
 */
export async function findFreePort() {
  return await new Promise((resolve, reject) => {
    const server = net.createServer();
    server.unref();
    server.on("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      server.close(() => {
        if (address && typeof address === "object") resolve(address.port);
        else reject(new Error("failed to allocate a loopback port"));
      });
    });
  });
}

/**
 * Poll an HTTP endpoint until it answers successfully or the child exits.
 *
 * @param {string} url
 * @param {number} timeoutMs
 * @param {import("node:child_process").ChildProcess} child
 * @returns {Promise<Response>}
 */
export async function waitForHttp(url, timeoutMs, child) {
  const deadline = Date.now() + timeoutMs;
  let lastError = "";
  while (Date.now() < deadline) {
    if (child.exitCode !== null || child.signalCode !== null) {
      throw new Error(`launcher exited before ${url} became ready (${lastError})`);
    }
    try {
      const response = await fetch(url);
      if (response.ok) return response;
      lastError = `HTTP ${response.status}`;
    } catch (error) {
      lastError = error instanceof Error ? error.message : String(error);
    }
    await delay(500);
  }
  throw new Error(`timed out waiting for ${url} (${lastError})`);
}

/**
 * Parse the server-rendered boot payload from the SPA shell.
 *
 * @param {string} html
 * @returns {Record<string, any>}
 */
export function parseBootPayload(html) {
  const match = html.match(/window\.__KANDEV_BOOT_PAYLOAD__=(\{.*\});<\/script>/s);
  assert.ok(match, "boot payload is present in the served shell");
  return JSON.parse(match[1]);
}

/**
 * Create one disposable task so the workload has a real task route to open.
 *
 * @param {string} baseUrl
 * @param {Record<string, any>} boot
 * @returns {Promise<string>}
 */
export async function createProbeTask(baseUrl, boot) {
  const workspace = boot.initialState?.workspaces?.items?.[0];
  const workflow = boot.initialState?.workflows?.items?.[0];
  assert.ok(workspace?.id, "the disposable home has a workspace");
  assert.ok(workflow?.id, "the disposable home has a workflow");
  const response = await fetch(`${baseUrl}/api/v1/tasks`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({
      workspace_id: workspace.id,
      workflow_id: workflow.id,
      title: TASK_TITLE,
    }),
  });
  assert.equal(response.status, 200, "the probe task is created");
  const task = await response.json();
  assert.ok(task.id, "the probe task has an id");
  return task.id;
}

/**
 * Extract local asset paths referenced by the built shell.
 *
 * @param {string} html
 * @returns {string[]}
 */
export function assetPathsFromShell(html) {
  const paths = new Set();
  for (const match of html.matchAll(/(?:src|href)="(\/assets\/[^"]+)"/g)) {
    paths.add(match[1]);
  }
  return [...paths].sort();
}

/**
 * @param {Buffer} content
 * @returns {string}
 */
export function sha256(content) {
  return createHash("sha256").update(content).digest("hex");
}

/**
 * Verify the served assets are the fresh build and inspect React sentinels.
 *
 * @param {string} baseUrl
 * @param {string} repoRoot
 * @returns {Promise<{assetCount: number, sentinelAssets: string[]}>}
 */
export async function verifyServedAssets(baseUrl, repoRoot) {
  const distDir = path.join(repoRoot, "apps/web/dist");
  const distIndexPath = path.join(distDir, "index.html");
  const distIndex = await fsp.readFile(distIndexPath, "utf8");
  const servedIndex = await (await fetch(`${baseUrl}/`)).text();

  assert.deepEqual(
    assetPathsFromShell(servedIndex),
    assetPathsFromShell(distIndex),
    "the served shell references the current build's assets",
  );

  const assetPaths = assetPathsFromShell(distIndex);
  assert.ok(assetPaths.length > 0, "the build produced JavaScript assets");
  const sentinelAssets = [];
  for (const assetPath of assetPaths) {
    const local = await fsp.readFile(path.join(distDir, assetPath));
    const response = await fetch(`${baseUrl}${assetPath}`);
    assert.equal(response.status, 200, `${assetPath} is served`);
    const served = Buffer.from(await response.arrayBuffer());
    assert.equal(sha256(served), sha256(local), `${assetPath} matches the fresh build`);
    const text = served.toString("utf8");
    if (DEVELOPMENT_SENTINELS.some((sentinel) => text.includes(sentinel))) {
      sentinelAssets.push(assetPath);
    }
  }

  if (expectedReact === "production") {
    assert.deepEqual(
      sentinelAssets,
      [],
      "production assets omit React development warning sentinels",
    );
  } else {
    assert.ok(
      sentinelAssets.length > 0,
      "development assets retain React development warning sentinels",
    );
  }
  return { assetCount: assetPaths.length, sentinelAssets };
}

/**
 * Install the React DevTools hook before application scripts execute.
 */
export function installReactObservationHook() {
  const renderers = [];
  const hook = {
    supportsFiber: true,
    renderers: new Map(),
    inject(renderer) {
      const id = this.renderers.size + 1;
      this.renderers.set(id, renderer);
      renderers.push({
        id,
        version: renderer.version,
        bundleType: renderer.bundleType,
        rendererPackageName: renderer.rendererPackageName,
      });
      return id;
    },
    onCommitFiberRoot() {},
    onCommitFiberUnmount() {},
    onPostCommitFiberRoot() {},
    checkDCE() {},
  };
  Object.defineProperty(window, "__KANDEV_START_RENDERERS__", {
    value: renderers,
    configurable: true,
  });
  Object.defineProperty(window, "__REACT_DEVTOOLS_GLOBAL_HOOK__", {
    value: hook,
    configurable: true,
    writable: true,
  });
}

/**
 * @param {import("@playwright/test").Page} page
 * @returns {Promise<Array<{version: string, bundleType: number, rendererPackageName: string}>>}
 */
export async function readReactRenderers(page) {
  await page.waitForFunction(
    () =>
      Array.isArray(window.__KANDEV_START_RENDERERS__) &&
      window.__KANDEV_START_RENDERERS__.length > 0,
    undefined,
    { timeout: 30_000 },
  );
  return await page.evaluate(() => window.__KANDEV_START_RENDERERS__);
}

/**
 * Click the Home navigation link and assert the SPA route.
 *
 * @param {import("@playwright/test").Page} page
 */
export async function goHome(page) {
  const homeLink = page.locator('a[href^="/?home=overview"]').first();
  await homeLink.waitFor({ state: "attached", timeout: 30_000 });
  await homeLink.click({ timeout: 5_000 });
  await page.waitForURL(
    (url) => url.pathname === "/" && url.searchParams.get("home") === "overview",
    { timeout: 5_000 },
  );
}

/**
 * Perform required SPA navigation that exercises React updates without reloads.
 *
 * @param {import("@playwright/test").Page} page
 * @param {string} taskId
 * @returns {Promise<{taskId: string, themeBefore: string | null, themeAfter: string | null}>}
 */
export async function runWorkload(page, taskId) {
  await goHome(page);

  const taskCard = page.getByRole("button", { name: new RegExp(TASK_TITLE) }).first();
  await taskCard.waitFor({ state: "visible", timeout: 30_000 });
  await taskCard.click({ timeout: 5_000 });
  await page.waitForURL((url) => url.pathname === `/t/${taskId}`, { timeout: 5_000 });
  await goHome(page);

  const settings = page.getByTestId("sidebar-settings-gear");
  await settings.waitFor({ state: "visible", timeout: 30_000 });
  await settings.click({ timeout: 5_000 });
  await page.waitForURL((url) => url.pathname.startsWith("/settings"), { timeout: 5_000 });

  const themeToggle = page.locator('button[aria-label^="Switch to "]').first();
  await themeToggle.waitFor({ state: "visible", timeout: 30_000 });
  const themeBefore = await themeToggle.getAttribute("aria-label");
  await themeToggle.click({ timeout: 5_000 });
  await page.waitForFunction(
    (previous) => {
      const control = document.querySelector('button[aria-label^="Switch to "]');
      return control !== null && control.getAttribute("aria-label") !== previous;
    },
    themeBefore,
    { timeout: 5_000 },
  );
  const themeAfter = await themeToggle.getAttribute("aria-label");
  assert.notEqual(themeAfter, themeBefore, "the theme toggle changes state");

  await goHome(page);
  return { taskId, themeBefore, themeAfter };
}

/**
 * @param {import("@playwright/test").Page} page
 * @param {import("@playwright/test").CDPSession} cdp
 * @param {import("@playwright/test").CDPSession} browserCdp
 * @param {string} label
 * @param {number} startedAt
 */
export async function takeSample(page, cdp, browserCdp, label, startedAt) {
  const heap = await cdp.send("Runtime.getHeapUsage");
  const measures = await page.evaluate(() => {
    const entries = performance.getEntriesByType("measure");
    const counts = new Map();
    for (const entry of entries) {
      counts.set(entry.name, (counts.get(entry.name) || 0) + 1);
    }
    const top = [...counts.entries()]
      .sort((left, right) => right[1] - left[1])
      .slice(0, 5)
      .map(([name, count]) => ({ name, count }));
    return { count: entries.length, top };
  });
  const renderer = await readRendererProcessMemory(browserCdp);
  return {
    label,
    elapsedMs: Date.now() - startedAt,
    jsHeapUsedBytes: heap.usedSize,
    jsHeapTotalBytes: heap.totalSize,
    measureCount: measures.count,
    dominantMeasures: measures.top,
    rendererProcessBytes: renderer.bytes,
    rendererProcessUnavailableReason: renderer.reason,
  };
}

/**
 * Look up renderer process memory through Chromium's process list and `ps`.
 *
 * @param {import("@playwright/test").CDPSession} browserCdp
 * @returns {Promise<{bytes: number | null, reason: string | null}>}
 */
export async function readRendererProcessMemory(browserCdp) {
  if (process.platform === "win32") {
    return { bytes: null, reason: "renderer process memory is not collected on Windows" };
  }
  try {
    const { processInfo } = await browserCdp.send("SystemInfo.getProcessInfo");
    const rendererIds = processInfo
      .filter((entry) => entry.type === "renderer")
      .map((entry) => entry.id);
    if (rendererIds.length === 0) {
      return { bytes: null, reason: "no renderer process was reported" };
    }
    const output = execFileSync("ps", ["-o", "rss=", "-p", rendererIds.join(",")], {
      encoding: "utf8",
    });
    const kilobytes = output
      .trim()
      .split(/\s+/)
      .filter(Boolean)
      .reduce((sum, value) => sum + Number(value), 0);
    if (!Number.isFinite(kilobytes) || kilobytes <= 0) {
      return { bytes: null, reason: "ps returned no renderer RSS values" };
    }
    return { bytes: kilobytes * 1024, reason: null };
  } catch (error) {
    return {
      bytes: null,
      reason: error instanceof Error ? error.message : String(error),
    };
  }
}

export const DATABASE_ENV_NAMES = [
  "KANDEV_DATABASE_DRIVER",
  "KANDEV_DATABASE_PATH",
  "KANDEV_DATABASE_HOST",
  "KANDEV_DATABASE_PORT",
  "KANDEV_DATABASE_USER",
  "KANDEV_DATABASE_PASSWORD",
  "KANDEV_DATABASE_DBNAME",
  "KANDEV_DATABASE_SSLMODE",
  "KANDEV_DATABASE_MAXCONNS",
  "KANDEV_DATABASE_MINCONNS",
  "KANDEV_SQLITE_READER_CONNS",
  "PGHOST",
  "PGPORT",
  "PGUSER",
  "PGPASSWORD",
  "PGDATABASE",
  "PGSSLMODE",
  "DATABASE_URL",
];

/**
 * Build the child environment for a disposable debug source start.
 *
 * The private configuration file prevents config.yaml discovery from selecting
 * an operator database, and the explicit sqlite values override every
 * inherited database input. The returned paths are the owned database and the
 * generated configuration used by the launcher.
 *
 * @param {string} homeDir
 * @param {number} port
 * @param {string} runDir
 * @param {NodeJS.ProcessEnv} baseEnv
 * @returns {{env: NodeJS.ProcessEnv, configPath: string, dbPath: string}}
 */
export function createLaunchEnvironment(homeDir, port, runDir, baseEnv = process.env) {
  const dbPath = path.join(homeDir, "data", "kandev.db");
  const configPath = path.join(runDir, "isolated-config.yaml");
  fs.writeFileSync(
    configPath,
    ["database:", '  driver: "sqlite"', `  path: ${JSON.stringify(dbPath)}`, ""].join("\n"),
    { mode: 0o600 },
  );

  const env = {
    ...baseEnv,
    NODE_ENV: "development",
    KANDEV_HOME_DIR: homeDir,
    KANDEV_BACKEND_PORT: String(port),
    KANDEV_SERVER_HOST: "127.0.0.1",
    KANDEV_NO_BROWSER: "1",
  };
  for (const name of [
    "KANDEV_SERVER_PORT",
    "KANDEV_PORT",
    "KANDEV_WEB_PORT",
    "KANDEV_WEB_DIST_DIR",
    "KANDEV_LOG_LEVEL",
    "KANDEV_CONFIG_FILE",
    "KANDEV_INTERNAL_CONFIG_FILE",
    "KANDEV_INTERNAL_CONFIG_HOME_FILE",
    "KANDEV_DEBUG_DEV_MODE",
    "KANDEV_DEBUG_AGENT_MESSAGES",
    "KANDEV_DEBUG_PPROF_ENABLED",
    "KANDEV_DEBUG_LOG_DIR",
    "KANDEV_FEATURES_AUTH",
    "KANDEV_MOCK_AGENT",
    "KANDEV_E2E_MOCK",
    ...DATABASE_ENV_NAMES,
  ]) {
    delete env[name];
  }
  env.KANDEV_DATABASE_DRIVER = "sqlite";
  env.KANDEV_DATABASE_PATH = dbPath;
  env.KANDEV_INTERNAL_CONFIG_FILE = configPath;
  return { env, configPath, dbPath };
}

/**
 * Spawn the real debug source-start target and stream its output to a file.
 *
 * @param {NodeJS.ProcessEnv} env
 * @param {string} startLog
 * @returns {{child: import("node:child_process").ChildProcess, logStream: fs.WriteStream}}
 */
export function spawnDebugSource(env, startLog) {
  const logStream = fs.createWriteStream(startLog);
  const child = spawn("make", ["start-debug"], {
    cwd: repo,
    env,
    detached: true,
    stdio: ["ignore", "pipe", "pipe"],
  });
  child.stdout.pipe(logStream, { end: false });
  child.stderr.pipe(logStream, { end: false });
  logStream.on("error", () => {});
  child.once("close", () => {
    if (!logStream.writableEnded) logStream.end();
  });
  return { child, logStream };
}

/**
 * @returns {string}
 */
export function readInstalledReactVersion() {
  return JSON.parse(
    fs.readFileSync(path.join(repo, "apps/web/node_modules/react-dom/package.json"), "utf8"),
  ).version;
}

/**
 * Open the SPA with the React observation hook installed before app scripts.
 *
 * @param {import("@playwright/test").Browser} browser
 * @param {string} baseUrl
 */
export async function openObservedPage(browser, baseUrl) {
  const context = await browser.newContext();
  const page = await context.newPage();
  const debugMessages = [];
  const pageErrors = [];
  page.on("console", (message) => {
    if (message.type() === "debug") debugMessages.push(message.text());
  });
  page.on("pageerror", (error) => pageErrors.push(error.message));
  await page.addInitScript(installReactObservationHook);
  await page.goto(`${baseUrl}/`, { waitUntil: "domcontentloaded" });
  await page.waitForFunction(() => window.__KANDEV_DEBUG === true);
  return { context, page, debugMessages, pageErrors };
}

/**
 * @param {import("@playwright/test").Page} page
 * @param {string} reactVersion
 */
export async function assertReactRuntime(page, reactVersion) {
  assert.equal(await page.title(), "Debug Kandev", "the debug launcher keeps its runtime title");
  const renderers = await readReactRenderers(page);
  const reactRenderer = renderers.find(
    (renderer) => renderer.rendererPackageName === REACT_PACKAGE,
  );
  assert.ok(reactRenderer, `the page registered the ${REACT_PACKAGE} renderer`);
  assert.equal(reactRenderer.version, reactVersion, "the renderer version is the installed React");
  assert.equal(
    reactRenderer.bundleType,
    expectedReact === "production" ? 0 : 1,
    `the served ${REACT_PACKAGE} renderer is ${expectedReact}`,
  );
}

/**
 * Create the probe task and prepare the SPA for the observation workload.
 *
 * @param {import("@playwright/test").Page} page
 * @param {string} baseUrl
 * @param {string} shell
 * @returns {Promise<string>}
 */
export async function openWorkloadPage(page, baseUrl, shell) {
  const taskId = await createProbeTask(baseUrl, parseBootPayload(shell));
  await page.reload({ waitUntil: "domcontentloaded" });
  await page.waitForFunction(() => window.__KANDEV_DEBUG === true);
  const skip = page.getByRole("button", { name: "Skip" });
  if (await skip.count()) {
    await skip.first().click();
    await skip
      .first()
      .waitFor({ state: "detached", timeout: 5_000 })
      .catch(() => {});
  }
  return taskId;
}

/**
 * Verify the runtime diagnostics that must survive the production React build.
 *
 * @param {string} baseUrl
 * @param {string} homeDir
 */
export async function assertKandevDiagnostics(baseUrl, homeDir) {
  const pprofResponse = await fetch(`${baseUrl}/debug/pprof/`);
  assert.equal(pprofResponse.status, 200, "the pprof index is accessible");
  assert.match(await pprofResponse.text(), /profile/, "the pprof index lists profiles");

  const backendLogPath = path.join(homeDir, "logs/backend-logs.log");
  let backendLog = "";
  const logDeadline = Date.now() + 10_000;
  while (Date.now() < logDeadline) {
    try {
      backendLog = await fsp.readFile(backendLogPath, "utf8");
    } catch {
      backendLog = "";
    }
    if (/"level":"debug"|DEBUG/.test(backendLog)) break;
    await delay(250);
  }
  assert.match(backendLog, /"level":"debug"|DEBUG/, "the backend writes debug file events");

  const manifest = JSON.parse(
    await fsp.readFile(path.join(homeDir, "supervisor/launch.json"), "utf8"),
  );
  assert.equal(manifest.env.KANDEV_DEBUG_AGENT_MESSAGES, "true");
  assert.equal(manifest.env.KANDEV_DEBUG_PPROF_ENABLED, "true");
  assert.equal(manifest.home_dir, homeDir);
}

/**
 * @param {import("@playwright/test").Page} page
 * @param {import("@playwright/test").CDPSession} cdp
 * @param {import("@playwright/test").CDPSession} browserCdp
 * @param {{samples: Array<Record<string, unknown>>, timing: {startedAt: number, observationStartedAt: number}, taskId: string, rounds: Array<Record<string, unknown>>}} context
 */
export async function runSampleLoop(page, cdp, browserCdp, context) {
  for (let minute = 1; minute <= soakMinutes; minute += 1) {
    const target = context.timing.observationStartedAt + minute * 60_000;
    while (Date.now() < target) {
      context.rounds.push(await runWorkload(page, context.taskId));
      await delay(Math.min(5_000, Math.max(0, target - Date.now())));
    }
    context.samples.push(
      await takeSample(page, cdp, browserCdp, `minute-${minute}`, context.timing.startedAt),
    );
  }
  context.rounds.push(await runWorkload(page, context.taskId));
  context.samples.push(await takeSample(page, cdp, browserCdp, "end", context.timing.startedAt));
}

/**
 * Read the launcher's resolved database path from its startup line.
 *
 * @param {string} startLog
 * @returns {string | undefined}
 */
export function readDatabasePath(startLog) {
  try {
    const match = fs.readFileSync(startLog, "utf8").match(/^\[kandev\] db: (.+)$/m);
    return match ? match[1].trim() : undefined;
  } catch {
    return undefined;
  }
}

/**
 * Prove the launcher opened the database under this check's home before any
 * API write occurs.
 *
 * @param {string} startLog
 * @param {string} homeDir
 * @param {string} dbPath
 */
export async function assertOwnedDatabase(startLog, homeDir, dbPath) {
  const expected = path.join(homeDir, "data", "kandev.db");
  assert.equal(dbPath, expected, "the pinned database path is under the owned home");
  let resolved;
  const deadline = Date.now() + 10_000;
  while (Date.now() < deadline) {
    resolved = readDatabasePath(startLog);
    if (resolved) break;
    await delay(100);
  }
  assert.equal(resolved, expected, "the launcher resolves the owned SQLite database");
  const stat = await fsp.stat(expected);
  assert.ok(stat.isFile(), "the owned SQLite database exists before API writes");
}

/**
 * @param {string} file
 * @param {number} lines
 * @returns {Promise<string>}
 */
export async function readFileTail(file, lines) {
  try {
    const content = await fsp.readFile(file, "utf8");
    return content.split("\n").slice(-lines).join("\n");
  } catch {
    return "(no launcher output captured)";
  }
}

export const HOSTILE_DB_DRIVER = "postgres";
export const HOSTILE_DB_HOST = "203.0.113.10";
export const OUTSIDE_DB_SENTINEL = "outside database sentinel\n";

/**
 * Build hostile inherited database/configuration inputs for the isolation test.
 *
 * @param {string} runDir
 * @returns {{sentinelDb: string, sentinelContents: string, hostileConfig: string, hostileEnv: NodeJS.ProcessEnv}}
 */
export function createHostileInputs(runDir) {
  const sentinelDb = path.join(runDir, "outside.db");
  fs.writeFileSync(sentinelDb, OUTSIDE_DB_SENTINEL);
  const hostileConfig = path.join(runDir, "hostile-config.yaml");
  fs.writeFileSync(
    hostileConfig,
    [
      "database:",
      `  driver: "${HOSTILE_DB_DRIVER}"`,
      `  path: ${JSON.stringify(sentinelDb)}`,
      `  host: "${HOSTILE_DB_HOST}"`,
      '  dbName: "outside"',
      "",
    ].join("\n"),
  );
  const hostileEnv = {
    ...process.env,
    KANDEV_DATABASE_DRIVER: HOSTILE_DB_DRIVER,
    KANDEV_DATABASE_PATH: sentinelDb,
    KANDEV_DATABASE_HOST: HOSTILE_DB_HOST,
    KANDEV_DATABASE_PORT: "5432",
    KANDEV_DATABASE_USER: "outside",
    KANDEV_DATABASE_PASSWORD: "outside",
    KANDEV_DATABASE_DBNAME: "outside",
    KANDEV_INTERNAL_CONFIG_FILE: hostileConfig,
    PGHOST: HOSTILE_DB_HOST,
    PGDATABASE: "outside",
  };
  return { sentinelDb, sentinelContents: OUTSIDE_DB_SENTINEL, hostileConfig, hostileEnv };
}

/**
 * Launch Chromium, install the observation hook, and assert the React runtime.
 *
 * @param {string} baseUrl
 * @param {string} reactVersion
 */
export async function createObservedBrowser(baseUrl, reactVersion) {
  const browser = await chromium.launch({
    headless: true,
    args: ["--enable-precise-memory-info"],
  });
  try {
    const { context, page, debugMessages, pageErrors } = await openObservedPage(browser, baseUrl);
    await assertReactRuntime(page, reactVersion);
    return { browser, context, page, debugMessages, pageErrors };
  } catch (error) {
    await browser.close().catch(() => {});
    throw error;
  }
}

/**
 * Run the required workload, diagnostics, and memory samples for one observation.
 *
 * @param {{browser: import("@playwright/test").Browser, context: import("@playwright/test").BrowserContext, page: import("@playwright/test").Page, debugMessages: string[], pageErrors: string[], taskId: string, baseUrl: string, homeDir: string, repoRoot: string, startedAt: number, sentinelDb: string, sentinelContents: string}} input
 */
export async function collectObservation(input) {
  let documentLoads = 0;
  input.page.on("load", () => {
    documentLoads += 1;
  });

  const cdp = await input.context.newCDPSession(input.page);
  const browserCdp = await input.browser.newBrowserCDPSession();
  const samples = [await takeSample(input.page, cdp, browserCdp, "start", input.startedAt)];
  const observationStartedAt = Date.now();
  const workloadRounds = [await runWorkload(input.page, input.taskId)];

  const kandevDebugMessage = input.debugMessages.find((message) =>
    /^\[[a-z0-9-]+:[a-z0-9-]+\]/.test(message),
  );
  assert.ok(kandevDebugMessage, "a namespaced Kandev debug message reached the browser console");
  assert.equal(
    fs.readFileSync(input.sentinelDb, "utf8"),
    input.sentinelContents,
    "the hostile outside database remains untouched",
  );

  const assets = await verifyServedAssets(input.baseUrl, input.repoRoot);
  await assertKandevDiagnostics(input.baseUrl, input.homeDir);
  await runSampleLoop(input.page, cdp, browserCdp, {
    samples,
    timing: { startedAt: input.startedAt, observationStartedAt },
    taskId: input.taskId,
    rounds: workloadRounds,
  });
  await browserCdp.detach();
  assert.equal(documentLoads, 0, "the observation workload does not reload the document");
  assert.deepEqual(
    input.pageErrors,
    [],
    `the page raised no uncaught errors: ${input.pageErrors.join(" | ")}`,
  );

  return { assets, kandevDebugMessage, samples, workloadRounds, documentLoads };
}

/**
 * Build the machine-readable observation report.
 *
 * @param {{expectedReact: string, repo: string, reactVersion: string, browser: import("@playwright/test").Browser, configPath: string, dbPath: string, assets: {assetCount: number, sentinelAssets: string[]}, kandevDebugMessage: string | undefined, documentLoads: number, workloadRounds: Array<Record<string, unknown>>, samples: Array<Record<string, unknown>>}} input
 */
export function buildObservationReport(input) {
  return {
    expectedReact: input.expectedReact,
    repo: input.repo,
    soakMinutes,
    reactVersion: input.reactVersion,
    browserVersion: input.browser.version(),
    nodeVersion: process.version,
    platform: `${process.platform} ${os.release()}`,
    configPath: input.configPath,
    dbPath: input.dbPath,
    servedAssetCount: input.assets.assetCount,
    developmentSentinelAssets: input.assets.sentinelAssets,
    kandevDebugMessage: input.kandevDebugMessage,
    documentLoads: input.documentLoads,
    workloadRounds: input.workloadRounds.length,
    firstWorkload: input.workloadRounds[0],
    heapSnapshotPerformanceMeasures: {
      available: false,
      reason: "not collected by this integration check",
    },
    userTimingRetainedBytes: {
      available: false,
      reason: "not collected by this integration check",
    },
    samples: input.samples,
  };
}

/**
 * Create the owned temporary home, run directory, free port, and start time.
 *
 * @returns {Promise<{homeDir: string, runDir: string, startLog: string, port: number, startedAt: number}>}
 */
export async function createRunWorkspace() {
  const homeDir = await fsp.mkdtemp(path.join(os.tmpdir(), "kandev-5050-home-"));
  const runDir = await fsp.mkdtemp(path.join(os.tmpdir(), "kandev-5050-run-"));
  return {
    homeDir,
    runDir,
    startLog: path.join(runDir, "start.log"),
    port: await findFreePort(),
    startedAt: Date.now(),
  };
}
