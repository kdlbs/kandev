import { chmod, mkdir, mkdtemp, readFile, readdir, rm, stat, writeFile } from "node:fs/promises";
import { createHash } from "node:crypto";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { test } from "node:test";
import { spawn } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";

import {
  HEALTH_REQUESTED_TIMEOUT_MS,
  ROOT_REQUESTED_TIMEOUT_MS,
  waitForHttp,
  waitForFile,
  writeFakeRuntime,
  writeReleaseShapedRuntime,
} from "./desktop-launch-smoke.mjs";

const __dirname = dirname(fileURLToPath(import.meta.url));
const desktopRoot = resolve(__dirname, "..");
const repoRoot = resolve(desktopRoot, "../..");
const backendRsPath = resolve(__dirname, "../src-tauri/src/backend.rs");
const smokeScriptPath = resolve(__dirname, "desktop-launch-smoke.mjs");
const mainRsPath = resolve(__dirname, "../src-tauri/src/main.rs");
const shellRsPath = resolve(__dirname, "../src-tauri/src/shell.rs");
const startupHtmlPath = resolve(__dirname, "../index.html");
const startupMainPath = resolve(__dirname, "../src/main.ts");
const startupStylesPath = resolve(__dirname, "../src/styles.css");
const startupLocalesDir = resolve(__dirname, "../src/locales");
const tauriConfigPath = resolve(desktopRoot, "src-tauri/tauri.conf.json");
const startupCapabilityPath = resolve(desktopRoot, "src-tauri/capabilities/startup.json");
const appRequire = createRequire(resolve(repoRoot, "apps/web/package.json"));

const STARTUP_LOCALES = ["en", "pt-pt", "zh-cn", "zh-hk", "zh-tw", "ja"];
const STARTUP_COPY_KEYS = [
  "loadingTitle",
  "loadingDetail",
  "conflictTitle",
  "singleDatabaseRule",
  "singleDataFolderRule",
  "conflictHomeDetail",
  "conflictDatabaseDetail",
  "temporaryHeading",
  "temporaryWarning",
  "temporaryAction",
  "temporaryStarting",
  "temporaryHomeLabel",
  "temporaryCleanup",
  "technicalDetails",
  "technicalDetailsHide",
  "spawnError",
  "startupFailureTitle",
];

async function withTempDir(run) {
  const dir = await mkdtemp(join(tmpdir(), "wait-for-file-"));
  try {
    return await run(dir);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
}

test("waitForFile resolves once the target file appears", async () => {
  await withTempDir(async (dir) => {
    const target = join(dir, "marker");
    const write = new Promise((r) => setTimeout(r, 50)).then(() => writeFile(target, "1"));
    await Promise.all([waitForFile(target, 2_000), write]);
  });
});

test("waitForFile throws a timeout error including the describeDetail() text", async () => {
  await withTempDir(async (dir) => {
    const target = join(dir, "never-appears");
    await assert.rejects(
      () => waitForFile(target, 100, undefined, () => "custom diagnostic detail"),
      (err) => {
        assert.match(err.message, /Timed out waiting for/);
        assert.match(err.message, /custom diagnostic detail/);
        return true;
      },
    );
  });
});

test("waitForFile omits the detail block when describeDetail is not provided", async () => {
  await withTempDir(async (dir) => {
    const target = join(dir, "never-appears");
    await assert.rejects(
      () => waitForFile(target, 100),
      (err) => {
        assert.equal(err.message, `Timed out waiting for ${target}`);
        return true;
      },
    );
  });
});

test("waitForFile calls tick on every poll and surfaces a tick failure immediately", async () => {
  await withTempDir(async (dir) => {
    const target = join(dir, "never-appears");
    let calls = 0;
    await assert.rejects(
      () =>
        waitForFile(target, 5_000, () => {
          calls += 1;
          if (calls >= 2) {
            throw new Error("boom");
          }
        }),
      /boom/,
    );
    assert.ok(calls >= 2, `expected at least 2 tick() calls, got ${calls}`);
  });
});

test("waitForHttp backs off after an unsuccessful HTTP response", async () => {
  const originalFetch = globalThis.fetch;
  const pauses = [];
  let attempts = 0;
  globalThis.fetch = async () => ({ ok: ++attempts > 1 });
  try {
    await waitForHttp("http://unused.test", 1_000, undefined, async (duration) => {
      pauses.push(duration);
    });
  } finally {
    globalThis.fetch = originalFetch;
  }

  assert.equal(attempts, 2);
  assert.deepEqual(pauses, [100]);
});

test("desktop smoke runtime uses standard resources without bundled remote helpers", async () => {
  await withTempDir(async (dir) => {
    const runtimeDir = resolve(dir, "runtime");
    const stateDir = resolve(dir, "state");
    await mkdir(resolve(runtimeDir, "bin"), { recursive: true });
    await mkdir(stateDir, { recursive: true });
    await writeFakeRuntime(runtimeDir, stateDir, "happy");

    assert.deepEqual(
      (await readdir(resolve(runtimeDir, "bin"))).sort(),
      process.platform === "win32" ? ["agentctl.cmd", "kandev.cmd"] : ["agentctl", "kandev"],
    );
    const manifest = JSON.parse(await readFile(resolve(runtimeDir, "remote-helpers.json"), "utf8"));
    assert.equal(manifest.variant, "standard");
    assert.equal(manifest.schema_version, 1);
  });
});

test("release-shaped Desktop runtime seeds a verified helper outside the standard bundle", async () => {
  await withTempDir(async (dir) => {
    const sourceBinDir = resolve(dir, "source-bin");
    const runtimeDir = resolve(dir, "runtime");
    const homeDir = resolve(dir, "home");
    await mkdir(sourceBinDir, { recursive: true });
    for (const name of [
      "kandev",
      "agentctl",
      "agentctl-linux-amd64",
      "agentctl-linux-arm64",
      "agentctl-darwin-amd64",
      "agentctl-darwin-arm64",
    ]) {
      const path = resolve(sourceBinDir, name);
      await writeFile(path, `#!/bin/sh\n# ${name}\n`);
      await chmod(path, 0o755);
    }

    const runtime = await writeReleaseShapedRuntime({
      sourceBinDir,
      runtimeDir,
      homeDir,
      version: "1.2.3",
      commit: "a".repeat(40),
    });

    assert.deepEqual((await readdir(resolve(runtimeDir, "bin"))).sort(), ["agentctl", "kandev"]);
    assert.equal(runtime.manifest.variant, "standard");
    assert.equal(runtime.manifest.version, "1.2.3");
    assert.equal(runtime.manifest.commit, "a".repeat(40));
    const helper = runtime.manifest.helpers.find(({ platform }) => platform === "linux/amd64");
    assert.ok(helper);
    const cachedHelper = await readFile(runtime.cachePath);
    assert.equal(createHash("sha256").update(cachedHelper).digest("hex"), helper.sha256);
    assert.equal(cachedHelper.length, helper.size_bytes);
    assert.ok((await stat(runtime.cachePath)).mode & 0o111);
  });
});

test("release-shaped Desktop smoke runs the real launcher with a preseeded helper cache", async () => {
  const source = await readFile(smokeScriptPath, "utf8");
  assert.match(source, /spawn\(launcherBinary, \["--headless", "--port", String\(launcherPort\)\]/);
  assert.match(source, /KANDEV_BUNDLE_DIR: runtimeDir/);
  assert.match(source, /KANDEV_AGENTCTL_LINUX_AMD64_BINARY: ""/);
  assert.match(source, /KANDEV_DESKTOP_RUNTIME_DIR: runtimeDir/);
  assert.match(source, /TestAgentctlResolverPackagedDesktopBundleUsesPreseededCache/);
  assert.match(source, /await runReleaseShapedSmoke\(appBinary\)/);
});

test("health-requested timeout stays above the Rust backend's own HEALTH_TIMEOUT", async () => {
  const source = await readFile(backendRsPath, "utf8");
  const match = source.match(/const HEALTH_TIMEOUT: Duration = Duration::from_secs\((\d+)\);/);
  assert.ok(
    match,
    "could not find HEALTH_TIMEOUT in backend.rs — update this test if it moved or was renamed",
  );

  const rustHealthTimeoutMs = Number(match[1]) * 1_000;
  assert.ok(
    HEALTH_REQUESTED_TIMEOUT_MS > rustHealthTimeoutMs,
    `HEALTH_REQUESTED_TIMEOUT_MS (${HEALTH_REQUESTED_TIMEOUT_MS}ms) must exceed backend.rs's HEALTH_TIMEOUT ` +
      `(${rustHealthTimeoutMs}ms) — otherwise this smoke test can kill a launcher the Rust side would still ` +
      "consider healthy-pending, turning a legitimately slow CI run into a spurious failure.",
  );
  assert.ok(
    ROOT_REQUESTED_TIMEOUT_MS > 0 && Number.isInteger(ROOT_REQUESTED_TIMEOUT_MS),
    "ROOT_REQUESTED_TIMEOUT_MS must be a positive integer",
  );
});

test("Close Context owns Cmd/Ctrl+W without a native window-close fallback", async () => {
  const [mainSource, shellSource] = await Promise.all([
    readFile(mainRsPath, "utf8"),
    readFile(shellRsPath, "utf8"),
  ]);

  assert.match(mainSource, /MENU_CLOSE_CONTEXT, "Close Context"[\s\S]*CmdOrCtrl\+KeyW/);
  assert.match(
    shellSource,
    /MENU_CLOSE_CONTEXT\s*=>\s*Some\(MenuAction::Emit\(CLOSE_CONTEXT_EVENT\)\)/,
  );
  assert.doesNotMatch(mainSource, /PredefinedMenuItem::close_window/);
  assert.doesNotMatch(
    mainSource,
    /\.accelerator\("CmdOrCtrl\+KeyW"\)[\s\S]{0,160}shutdown_and_exit/,
  );
});

test("generic app activation never consumes a pending notification route", async () => {
  const mainSource = await readFile(mainRsPath, "utf8");

  assert.doesNotMatch(mainSource, /emit_pending_notification_route/);
  assert.match(mainSource, /tauri_plugin_single_instance::init\([\s\S]*activate_main_window/);
  assert.match(mainSource, /RunEvent::Reopen[\s\S]*activate_main_window/);
});

test("main window registers download handling before its configured WebView is created", async () => {
  const [mainSource, configSource] = await Promise.all([
    readFile(mainRsPath, "utf8"),
    readFile(tauriConfigPath, "utf8"),
  ]);
  const mainWindow = JSON.parse(configSource).app.windows.find(({ label }) => label === "main");

  assert.ok(mainWindow, "the configured main window must exist");
  assert.equal(
    mainWindow.create,
    false,
    "Tauri must leave creation to the configured callback builder",
  );
  assert.match(
    mainSource,
    /WebviewWindowBuilder::from_config[\s\S]*?\.on_download\([\s\S]*?\)\s*\.build\(\)/,
  );
  assert.match(mainSource, /downloads::handle_download_event/);
});

test("fullscreen uses the platform-native desktop accelerators", async () => {
  const mainSource = await readFile(mainRsPath, "utf8");

  assert.match(mainSource, /target_os = "macos"[\s\S]*"Ctrl\+Cmd\+F"/);
  assert.match(mainSource, /not\(target_os = "macos"\)[\s\S]*"F11"/);
});

test("startup conflict UI keeps the database rule prominent and isolates the temporary action", async () => {
  const [html, source] = await Promise.all([
    readFile(startupHtmlPath, "utf8"),
    readFile(startupMainPath, "utf8"),
  ]);

  assert.match(html, /data-testid="startup-conflict"/);
  assert.match(html, /data-testid="conflict-rule"/);
  assert.match(html, /data-testid="temporary-instance-section"/);
  assert.match(html, /data-testid="temporary-instance-button"/);
  assert.match(html, /data-testid="startup-spinner"/);
  assert.match(html, /data-startup-drag-region/);
  assert.match(source, /invoke\("start_temporary_test_instance"\)/);
  assert.match(source, /setAttribute\("data-tauri-drag-region"/);
  assert.match(source, /status\.kind === "conflict"/);
  assert.match(source, /temporaryHome/);
  assert.match(source, /temporaryAction/);
});

test("terminal startup panels announce conflict and failure assertively", async () => {
  const html = await readFile(startupHtmlPath, "utf8");

  assert.match(html, /<section[^>]*id="conflict-panel"[^>]*aria-live="assertive"/s);
  assert.match(html, /<section[^>]*id="failure-panel"[^>]*aria-live="assertive"/s);
});

test("the recovery command is granted only to the bundled startup page and Vite startup origin", async () => {
  const [configSource, capabilitySource, defaultCapabilitySource] = await Promise.all([
    readFile(tauriConfigPath, "utf8"),
    readFile(startupCapabilityPath, "utf8"),
    readFile(resolve(desktopRoot, "src-tauri/capabilities/default.json"), "utf8"),
  ]);
  const config = JSON.parse(configSource);
  const capability = JSON.parse(capabilitySource);
  const defaultCapability = JSON.parse(defaultCapabilitySource);

  assert.ok(config.app.security.capabilities.includes("startup"));
  assert.equal(config.app.windows[0].decorations, true);
  assert.equal(config.app.windows[0].titleBarStyle, "Overlay");
  assert.equal(config.app.windows[0].hiddenTitle, true);
  assert.deepEqual(capability.permissions, ["allow-start-temporary-test-instance"]);
  assert.deepEqual(capability.remote.urls, ["http://127.0.0.1:1420"]);
  assert.deepEqual(capability.windows, ["main"]);
  assert.equal(capability.local, true);
  assert.ok(defaultCapability.permissions.includes("core:window:allow-start-dragging"));
});

test("first-paint and loaded startup styles use Kandev theme colors and the nine-square grid", async () => {
  const [html, styles] = await Promise.all([
    readFile(startupHtmlPath, "utf8"),
    readFile(startupStylesPath, "utf8"),
  ]);
  const inlineStyles = html.match(/<style>([\s\S]*?)<\/style>/)?.[1];

  assert.ok(inlineStyles, "first-paint startup styles must be inline");
  for (const source of [inlineStyles, styles]) {
    assert.match(source, /--startup-bg:\s*#fff(?:fff)?/i);
    assert.match(source, /--startup-dark-bg:\s*#181818/i);
    assert.match(source, /--startup-primary:\s*oklch\(0\.51\s+0\.23\s+277\)/i);
    assert.match(source, /--startup-dark-primary:\s*oklch\(0\.59\s+0\.2\s+277\)/i);
    assert.match(source, /\.spinner-grid-cube:nth-child\(9\)/);
    assert.match(source, /prefers-reduced-motion:\s*reduce/);
    assert.match(source, /\.startup-shell\.is-failed[\s\S]*\.spinner-grid/);
  }
  assert.equal((html.match(/class="spinner-grid-cube"/g) ?? []).length, 9);
});

test("startup status and recovery copy exist in all six desktop locales", async () => {
  const dictionaries = await Promise.all(
    STARTUP_LOCALES.map(async (locale) => {
      const source = await readFile(resolve(startupLocalesDir, `${locale}.json`), "utf8");
      return [locale, JSON.parse(source)];
    }),
  );

  for (const [locale, dictionary] of dictionaries) {
    for (const key of STARTUP_COPY_KEYS) {
      assert.equal(typeof dictionary[key], "string", `${locale}.${key} must be translated`);
      assert.ok(dictionary[key].trim().length > 0, `${locale}.${key} must not be empty`);
    }
  }
  const english = dictionaries.find(([locale]) => locale === "en")?.[1];
  assert.ok(english, "English startup catalog must exist");
  for (const [locale, dictionary] of dictionaries) {
    if (locale === "en") continue;
    for (const key of STARTUP_COPY_KEYS) {
      assert.notEqual(
        dictionary[key],
        english[key],
        `${locale}.${key} must not fall back to English`,
      );
    }
  }
});

test(
  "rendered startup matches both themes, respects reduced motion, and removes the grid on errors",
  { timeout: 60_000 },
  async () => {
    const { chromium } = appRequire("@playwright/test");
    const vite = resolve(desktopRoot, "node_modules/vite/bin/vite.js");
    const preview = spawn(
      process.execPath,
      [vite, "preview", "--host", "127.0.0.1", "--port", "4178", "--strictPort"],
      { cwd: desktopRoot, stdio: ["ignore", "pipe", "pipe"] },
    );
    let serverOutput = "";
    preview.stdout.on("data", (chunk) => (serverOutput += chunk));
    preview.stderr.on("data", (chunk) => (serverOutput += chunk));

    let browser;
    try {
      await waitForPreviewHttp("http://127.0.0.1:4178", 15_000, () => {
        if (preview.exitCode !== null) throw new Error(serverOutput);
      });
      browser = await chromium.launch({ headless: true, args: ["--no-sandbox"] });

      const firstPaintContext = await browser.newContext({
        colorScheme: "dark",
        reducedMotion: "reduce",
      });
      const firstPaintPage = await firstPaintContext.newPage();
      await firstPaintPage.route(/\.css(?:\?|$)/, (route) => route.abort());
      await firstPaintPage.goto("http://127.0.0.1:4178", { waitUntil: "networkidle" });
      const firstPaint = await firstPaintPage.evaluate(() => ({
        background: getComputedStyle(document.body).backgroundColor,
        primary: getComputedStyle(document.querySelector(".spinner-grid")).color,
        cubes: document.querySelectorAll(".spinner-grid-cube").length,
        animation: getComputedStyle(document.querySelector(".spinner-grid-cube")).animationName,
      }));
      assert.equal(firstPaint.background, "rgb(24, 24, 24)");
      assert.equal(firstPaint.cubes, 9);
      assert.equal(firstPaint.animation, "none");
      assert.match(firstPaint.primary, /oklch|rgb|color/i);
      await firstPaintContext.close();

      for (const [language, expectedLocale] of [
        ["zh-Hant-HK", "zh-hk"],
        ["zh-Hant-MO", "zh-hk"],
        ["zh-Hant-TW", "zh-tw"],
        ["zh-Hant", "zh-tw"],
      ]) {
        const localeContext = await browser.newContext();
        await localeContext.addInitScript(
          (languages) => {
            Object.defineProperty(navigator, "languages", {
              configurable: true,
              value: languages,
            });
          },
          [language],
        );
        const localePage = await localeContext.newPage();
        await localePage.goto("http://127.0.0.1:4178", { waitUntil: "networkidle" });
        assert.equal(await localePage.locator("html").getAttribute("lang"), expectedLocale);
        await localeContext.close();
      }

      const lightContext = await browser.newContext({
        colorScheme: "light",
        reducedMotion: "reduce",
      });
      const lightPage = await lightContext.newPage();
      await lightPage.addInitScript(() => {
        Object.defineProperty(navigator, "userAgent", {
          configurable: true,
          value: "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0)",
        });
        window.__TAURI_INTERNALS__ = {
          invoke: async (command) => {
            (window.__startupInvocations ??= []).push(command);
            if (command === "start_temporary_test_instance") {
              throw new Error("temporary launcher unavailable");
            }
          },
        };
      });
      await lightPage.goto("http://127.0.0.1:4178", { waitUntil: "networkidle" });
      await lightPage.waitForFunction(
        () => typeof window.__KANDEV_DESKTOP_SET_STATUS === "function",
      );
      assert.equal(
        await lightPage
          .locator("[data-startup-drag-region]")
          .getAttribute("data-tauri-drag-region"),
        "",
      );

      const lightStyle = await lightPage.evaluate(() => ({
        background: getComputedStyle(document.body).backgroundColor,
        primary: getComputedStyle(document.querySelector(".spinner-grid")).color,
        animation: getComputedStyle(document.querySelector(".spinner-grid-cube")).animationName,
      }));
      assert.equal(lightStyle.background, "rgb(255, 255, 255)");
      assert.equal(lightStyle.animation, "none");
      assert.match(lightStyle.primary, /oklch|rgb|color/i);

      await lightPage.setViewportSize({ width: 390, height: 844 });
      await lightPage.evaluate(() => {
        window.__KANDEV_DESKTOP_SET_STATUS({
          kind: "failure",
          detail: "<img src=x onerror=alert(1)>",
          home: "/tmp/kandev-temporary-home",
        });
      });
      assert.equal(await lightPage.locator("[data-testid=startup-failure]").isVisible(), true);
      assert.equal(await lightPage.locator("[data-testid=startup-spinner]").isVisible(), false);
      assert.equal(await lightPage.locator("#failure-detail img").count(), 0);
      assert.match(await lightPage.locator("#failure-detail").innerText(), /<img/);
      assert.equal(await lightPage.locator("#failure-home").isVisible(), true);
      assert.equal(
        await lightPage.locator("#failure-home-path").innerText(),
        "/tmp/kandev-temporary-home",
      );
      assert.equal(
        await lightPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
        true,
        "the retained data path should fit a narrow startup window without horizontal overflow",
      );

      await lightPage.evaluate(() => {
        window.__KANDEV_DESKTOP_SET_STATUS({
          kind: "conflict",
          conflict: {
            target_kind: "home",
            target_path: "/home/test/.kandev",
            database_path: "/home/test/.kandev/data/kandev.db",
            storage_kind: "sqlite_in_home",
            owner: { pid: 41, executable: "/usr/bin/kandev", started_at: "2026-09-25T12:00:00Z" },
          },
        });
      });
      assert.equal(await lightPage.locator("[data-testid=startup-conflict]").isVisible(), true);
      assert.equal(await lightPage.locator("[data-testid=startup-spinner]").isVisible(), false);
      assert.match(
        await lightPage.locator("[data-testid=conflict-rule]").innerText(),
        /one Kandev instance.*database/i,
      );
      assert.equal(await lightPage.locator("#owner-details").isVisible(), true);

      for (const storageKind of ["sqlite_external", "postgres"]) {
        await lightPage.evaluate((kind) => {
          window.__KANDEV_DESKTOP_SET_STATUS({
            kind: "conflict",
            conflict: {
              target_kind: "home",
              target_path: "/home/test/.kandev",
              storage_kind: kind,
              database_path: kind === "sqlite_external" ? "/shared/kandev.db" : undefined,
            },
          });
        }, storageKind);
        assert.match(
          await lightPage.locator("[data-testid=conflict-rule]").innerText(),
          /one Kandev instance.*data folder/i,
          `home conflict with ${storageKind} should describe the locked data folder`,
        );
      }

      await lightPage.evaluate(() => {
        window.__KANDEV_DESKTOP_SET_STATUS({
          kind: "conflict",
          conflict: {
            target_kind: "database",
            target_path: "/shared/kandev.db",
            storage_kind: "sqlite_external",
            database_path: "/shared/kandev.db",
          },
        });
      });
      assert.match(
        await lightPage.locator("[data-testid=conflict-rule]").innerText(),
        /one Kandev instance.*database/i,
        "external database conflict should describe the locked database",
      );
      await lightPage.locator("[data-testid=temporary-instance-button]").click();
      await lightPage.locator("[data-testid=temporary-instance-button]").click();
      await lightPage.waitForFunction(() => window.__startupInvocations?.length === 2);
      assert.deepEqual(await lightPage.evaluate(() => window.__startupInvocations), [
        "start_temporary_test_instance",
        "start_temporary_test_instance",
      ]);
      assert.equal(
        await lightPage.locator("#temporary-action-feedback").getAttribute("aria-live"),
        "assertive",
      );
      assert.match(
        await lightPage.locator("#temporary-action-feedback").innerText(),
        /temporary launcher unavailable/,
      );
      await lightContext.close();
    } finally {
      await browser?.close();
      preview.kill("SIGTERM");
    }
  },
);

async function waitForPreviewHttp(url, timeoutMs, tick, pause = delay) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    tick?.();
    let ready = false;
    try {
      const response = await fetch(url);
      ready = response.ok;
    } catch {
      // Retry after the common delay below.
    }
    if (ready) return;
    await pause(100);
  }
  throw new Error(`Timed out waiting for ${url}`);
}
