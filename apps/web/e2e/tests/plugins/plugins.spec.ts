/**
 * E2E: the real kandev gRPC plugin system, end to end.
 *
 * Supersedes the old HTTP+HMAC "native JS plugin" spec (docs/plans/plugins/
 * GRPC-CONTRACT.md froze the new transport: plugin backends are Go binaries
 * spawned by kandev via hashicorp/go-plugin, talking gRPC over a unix
 * socket — no HTTP server, no webhook_secret, no in-process Node fixture).
 *
 * The "plugin process" here is the real `plugin-fixture` Go binary
 * (apps/backend/cmd/plugin-fixture), packaged by `make -C apps/backend
 * e2e-plugin-package` into apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz
 * (id `kandev-plugin-e2e`). `e2e/global-setup.ts` checks that file exists
 * before the suite runs (same pattern as the kandev/mock-agent binary
 * checks) — see that file and the Makefile's `build-e2e-plugin-package` /
 * `e2e-plugin-package` targets for how to (re)build it.
 *
 * Flow:
 *   1. Install the package through the real Settings > Plugins upload UI
 *      (POST /api/plugins/install, multipart) — the backend extracts it to
 *      the worker's isolated `<KANDEV_HOME_DIR>/plugins/kandev-plugin-e2e/`
 *      and spawns it synchronously, so it comes back `active` in the same
 *      response. No signature was attached, so the unsigned badge shows.
 *   2. Reload the SPA — the boot payload now carries the active plugin — and
 *      confirm its nav item, top-level route, `task-sidebar` slot, and
 *      `main-top-bar` slot render via the real `/api/plugins/:id/bundle`
 *      static-file proxy.
 *   3. Create a task while the plugin's own page stays mounted (no
 *      navigation in between) and prove BOTH real gRPC paths at once:
 *        - task.created -> Deliverer -> plugin subprocess OnEvent RPC,
 *          which appends a deliveries.jsonl line under the plugin's real
 *          KANDEV_PLUGIN_DATA_DIR (polled directly off disk — the strongest
 *          evidence that a delivery crossed the real transport, not a mock).
 *        - task.created -> WS -> registry.registerWsHandler -> the page's
 *          own live counter.
 *   4. Disable/enable from the UI: nav item disappears/reappears live (no
 *      reload — registry unregister/re-load).
 *   5. Uninstall (with confirmation): the row disappears and the plugin's
 *      directory tree is removed from disk (process stopped, package
 *      extraction cleaned up).
 *   6. Separately, uploading a corrupted (non-gzip) package surfaces
 *      `install-plugin-error` instead of silently failing.
 */
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import type { Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";

const PLUGIN_ID = "kandev-plugin-e2e";
const NAV_ITEM_ID = "e2e-hello";
const PLUGIN_ROUTE = "/plugins/e2e-hello";

const PACKAGE_PATH = path.resolve(
  __dirname,
  "../../../../../apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz",
);

/** Every deliveries.jsonl `event_type` recorded so far, read straight off
 * disk from the plugin's real KANDEV_PLUGIN_DATA_DIR (no in-process mock —
 * this is the fixture Go binary's own gRPC OnEvent handler writing to its
 * data dir, per apps/backend/cmd/plugin-fixture/plugin.go). */
function deliveredEventTypes(pluginsDir: string): string[] {
  const deliveriesPath = path.join(pluginsDir, PLUGIN_ID, "data", "deliveries.jsonl");
  if (!fs.existsSync(deliveriesPath)) return [];
  return fs
    .readFileSync(deliveriesPath, "utf8")
    .split("\n")
    .filter((line) => line.trim().length > 0)
    .map((line) => (JSON.parse(line) as { event_type: string }).event_type);
}

async function openInstallDialog(page: Page) {
  await page.goto("/settings/plugins");
  await page.getByTestId("install-plugin-trigger").click();
  await expect(page.getByTestId("install-plugin-dialog")).toBeVisible();
}

async function uploadPackage(page: Page, filePath: string) {
  await page.getByTestId("install-plugin-tab-upload").click();
  await page.getByTestId("install-plugin-file-input").setInputFiles(filePath);
  await page.getByTestId("install-plugin-upload-submit").click();
}

async function uninstallViaApi(apiClient: ApiClient) {
  await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
}

/**
 * Stages a filesystem sideload: extracts the same fixture package the
 * upload tests use directly into `<pluginsDir>/<id>/<version>/`, with no
 * `{id}.yml` record — the on-disk shape `Service.Sync`'s directory-sideload
 * step looks for (docs/specs/plugins/spec.md "Filesystem sideloading &
 * sync"). `checksums.txt` lands in the directory alongside the rest of the
 * package; the sideload path ignores it (only the tarball-install path
 * verifies checksums).
 */
function stageDirSideload(pluginsDir: string): string {
  const versionDir = path.join(pluginsDir, PLUGIN_ID, "1.0.0");
  fs.mkdirSync(versionDir, { recursive: true });
  execFileSync("tar", ["-xzf", PACKAGE_PATH, "-C", versionDir]);
  return versionDir;
}

test.describe("Plugins — gRPC plugin install/load/live-update/uninstall", () => {
  // Repeat-each safety: the plugin id is fixed, and Install rejects a
  // duplicate <id>/<version> with pkgtar.ErrVersionExists (409). Whether the
  // test's own UI-driven uninstall ran or not, always clean up via the API
  // so the next iteration starts from a clean slate.
  test.afterEach(async ({ apiClient }) => {
    await uninstallViaApi(apiClient);
  });

  test("installs via upload, loads the UI, live-updates via WS+gRPC, and uninstalls", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    const pluginsDir = path.join(backend.tmpDir, ".kandev", "plugins");

    // --- 1. Install via the real upload UI ---
    await openInstallDialog(testPage);
    await uploadPackage(testPage, PACKAGE_PATH);

    const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
    await expect(pluginRow).toBeVisible({ timeout: 15_000 });
    await expect(pluginRow.getByText("Active", { exact: true })).toBeVisible();
    await expect(pluginRow.getByTestId("plugin-unsigned-badge")).toBeVisible();
    // Successful install closes the dialog (use-plugin-actions.ts afterInstall).
    await expect(testPage.getByTestId("install-plugin-dialog")).toBeHidden();

    // --- 2. Navigate off the Settings takeover (its sidebar mode hides the
    // main nav — see app-sidebar.tsx's `settingsMode` branch) and reload:
    // boot payload now carries the active plugin. ---
    await testPage.goto("/");
    await testPage.reload();
    const navItem = testPage.getByTestId(`plugin-nav-item-${NAV_ITEM_ID}`);
    await expect(navItem).toBeVisible({ timeout: 15_000 });
    await expect(navItem).toHaveText("Hello E2E");
    await expect(testPage.locator("#hello-status-left")).toHaveText("Hello status bar no-task");
    await expect(testPage.locator("#hello-status-right")).toHaveText("Hello status bar no-task");

    // --- 2b. main-top-bar slot renders on the default app top bar (Home) ---
    await expect(testPage.locator("#hello-main-top-bar")).toBeVisible();
    await expect(testPage.locator("#hello-main-top-bar")).toHaveText("Hello kanban");

    await navItem.click();
    await expect(testPage).toHaveURL(new RegExp(`${PLUGIN_ROUTE}$`));
    const pluginPage = testPage.locator("#hello-plugin-page");
    await expect(pluginPage).toBeVisible();
    await expect(pluginPage).toHaveText("Hello E2E");

    // --- 3. task-sidebar slot renders on a real task detail page ---
    const seedTask = await apiClient.createTask(seedData.workspaceId, "Plugin sidebar seed task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto(`/t/${seedTask.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.sidebar).toBeVisible({ timeout: 10_000 });
    await expect(testPage.locator("#hello-sidebar")).toBeVisible();
    await expect(testPage.locator("#hello-status-left")).toContainText(seedTask.id);

    // --- 4. Back on the plugin's own page (mounted, no navigation from here
    // on): create a task and prove BOTH the live WS path (counter) and the
    // real gRPC delivery path (deliveries.jsonl) together. Count deliveries
    // rather than asserting absence — the seed task in step 3 already
    // triggered one (the plugin was active for it too). ---
    await testPage.goto(PLUGIN_ROUTE);
    const counter = testPage.locator("#hello-task-counter");
    await expect(counter).toBeVisible();
    const counterBefore = Number((await counter.textContent()) ?? "0");
    const deliveriesBefore = deliveredEventTypes(pluginsDir).filter(
      (t) => t === "task.created",
    ).length;

    await apiClient.createTask(seedData.workspaceId, "Plugin live task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    await expect
      .poll(() => deliveredEventTypes(pluginsDir).filter((t) => t === "task.created").length, {
        timeout: 15_000,
        intervals: [250, 500, 1000],
      })
      .toBe(deliveriesBefore + 1);
    await expect(counter).toHaveText(String(counterBefore + 1), { timeout: 15_000 });

    // --- 5. Disable from Settings > Plugins: registry unregisters live, so
    // the nav item is gone as soon as we're back on a page that renders the
    // main sidebar (Settings itself replaces that sidebar with its own
    // takeover — see the `settingsMode` branch in app-sidebar.tsx). ---
    await testPage.goto("/settings/plugins");
    await expect(pluginRow.getByText("Active", { exact: true })).toBeVisible();
    await pluginRow.getByRole("button", { name: "Disable" }).click();
    await expect(pluginRow.getByText("Disabled", { exact: true })).toBeVisible({ timeout: 10_000 });
    await testPage.goto("/");
    await expect(testPage.getByTestId(`plugin-nav-item-${NAV_ITEM_ID}`)).toHaveCount(0);
    await expect(testPage.locator("#hello-status-left")).toHaveCount(0);
    await expect(testPage.locator("#hello-status-right")).toHaveCount(0);

    // --- 6. Re-enable: nav item reappears live (no reload needed) ---
    await testPage.goto("/settings/plugins");
    await pluginRow.getByRole("button", { name: "Enable" }).click();
    await expect(pluginRow.getByText("Active", { exact: true })).toBeVisible({ timeout: 10_000 });
    await testPage.goto("/");
    await expect(testPage.getByTestId(`plugin-nav-item-${NAV_ITEM_ID}`)).toBeVisible();
    await expect(testPage.locator("#hello-status-left")).toHaveText("Hello status bar no-task");
    await expect(testPage.locator("#hello-status-right")).toHaveText("Hello status bar no-task");

    await testPage.goto("/settings/plugins");

    // --- 7. Uninstall via UI (with confirmation): row disappears, package
    // directory tree is removed from disk. ---
    await pluginRow.getByRole("button", { name: "Uninstall" }).click();
    await testPage.getByRole("button", { name: "Confirm uninstall" }).click();
    await expect(pluginRow).toHaveCount(0, { timeout: 10_000 });

    const pluginDir = path.join(pluginsDir, PLUGIN_ID);
    await expect.poll(() => fs.existsSync(pluginDir), { timeout: 10_000 }).toBe(false);
  });

  test("settings page: schema-driven form, secret masking, and Host GetConfig delivery", async ({
    testPage,
    apiClient,
    backend,
  }) => {
    test.setTimeout(90_000);
    const pluginsDir = path.join(backend.tmpDir, ".kandev", "plugins");
    const secretToken = "ghp_e2e_secret_token";

    // --- Install via the upload UI, then click through to the detail page ---
    await openInstallDialog(testPage);
    await uploadPackage(testPage, PACKAGE_PATH);
    const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
    await expect(pluginRow).toBeVisible({ timeout: 15_000 });

    await testPage.getByTestId(`plugin-row-link-${PLUGIN_ID}`).click();
    await expect(testPage).toHaveURL(new RegExp(`/settings/plugins/${PLUGIN_ID}$`));
    await expect(testPage.getByTestId(`plugin-detail-${PLUGIN_ID}`)).toBeVisible();
    await expect(testPage.getByTestId("plugin-manifest-card")).toBeVisible();

    // --- Fill the config_schema-driven form: secret token + plain string ---
    const tokenInput = testPage.getByTestId("plugin-config-field-api_token").locator("input");
    const greetingInput = testPage.getByTestId("plugin-config-field-greeting").locator("input");
    await expect(tokenInput).toHaveAttribute("type", "password");
    await tokenInput.fill(secretToken);
    await greetingInput.fill("hello from e2e");
    await expect(tokenInput).toHaveAttribute("data-settings-dirty", "true");
    await expect(greetingInput).toHaveAttribute("data-settings-dirty", "true");
    await expect(testPage.getByTestId("plugin-settings-card")).toHaveAttribute(
      "data-settings-dirty",
      "true",
    );
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .click();

    // --- After save the form re-fetches the MASKED config: the token shows
    // as the placeholder, never the cleartext; the greeting round-trips. ---
    await expect(tokenInput).toHaveValue("********", { timeout: 15_000 });
    await expect(greetingInput).toHaveValue("hello from e2e");

    // --- The config file never persists the cleartext: the secret field is
    // a reference into kandev's encrypted vault. ---
    const configPath = path.join(pluginsDir, `${PLUGIN_ID}.config.yml`);
    await expect.poll(() => fs.existsSync(configPath), { timeout: 10_000 }).toBe(true);
    const configFile = fs.readFileSync(configPath, "utf8");
    expect(configFile).not.toContain(secretToken);
    expect(configFile).toContain(`vault:plugin:${PLUGIN_ID}:config:api_token`);

    // --- The operator API never returns the cleartext either. ---
    const configRes = await apiClient.rawRequest("GET", `/api/plugins/${PLUGIN_ID}/config`);
    const configBody = (await configRes.json()) as { config: Record<string, unknown> };
    expect(configBody.config.api_token).toBe("********");
    expect(configBody.config.greeting).toBe("hello from e2e");

    // --- Saving restarted the plugin; it must be Active again. ---
    await testPage.goto("/settings/plugins");
    await expect(pluginRow.getByText("Active", { exact: true })).toBeVisible({ timeout: 15_000 });

    // --- Prove the Host GetConfig gRPC path: the webhook makes the fixture
    // binary call Host.GetConfig and snapshot the result to config.json in
    // its data dir — cleartext secret included. ---
    const webhookRes = await apiClient.rawRequest(
      "POST",
      `/api/plugins/${PLUGIN_ID}/webhooks/test-hook`,
      {},
    );
    expect(webhookRes.status).toBe(200);

    const snapshotPath = path.join(pluginsDir, PLUGIN_ID, "data", "config.json");
    await expect
      .poll(
        () => {
          if (!fs.existsSync(snapshotPath)) return null;
          return (JSON.parse(fs.readFileSync(snapshotPath, "utf8")) as Record<string, unknown>)
            .api_token;
        },
        { timeout: 15_000, intervals: [250, 500, 1000] },
      )
      .toBe(secretToken);

    // --- And the plugin-scoped secret primitives: the same webhook makes
    // the fixture SetSecret("probe") then GetSecret it back through the
    // vault, writing the round-tripped value to secret-probe.json. ---
    const probePath = path.join(pluginsDir, PLUGIN_ID, "data", "secret-probe.json");
    await expect
      .poll(
        () => {
          if (!fs.existsSync(probePath)) return null;
          return (JSON.parse(fs.readFileSync(probePath, "utf8")) as Record<string, unknown>).probe;
        },
        { timeout: 15_000, intervals: [250, 500, 1000] },
      )
      .toBe("s3cret-roundtrip");
  });

  test("uploading a corrupted package surfaces install-plugin-error", async ({ testPage }) => {
    const junkPath = path.join(os.tmpdir(), `kandev-e2e-corrupt-plugin-${Date.now()}.tar.gz`);
    fs.writeFileSync(junkPath, "not a real gzip archive, just junk bytes\n".repeat(8));

    try {
      await openInstallDialog(testPage);
      await uploadPackage(testPage, junkPath);

      await expect(testPage.getByTestId("install-plugin-error")).toBeVisible({ timeout: 10_000 });
      // The failed upload never installed anything — no row should appear.
      await expect(testPage.getByTestId(`plugin-row-${PLUGIN_ID}`)).toHaveCount(0);
    } finally {
      fs.rmSync(junkPath, { force: true });
    }
  });

  test("filesystem sideload: Sync discovers a directory drop as disabled, enabling activates it", async ({
    testPage,
    backend,
  }) => {
    test.setTimeout(60_000);
    const pluginsDir = path.join(backend.tmpDir, ".kandev", "plugins");
    stageDirSideload(pluginsDir);

    // --- Before syncing, the sideload sits on disk with no record. ---
    await testPage.goto("/settings/plugins");
    await expect(testPage.getByTestId(`plugin-row-${PLUGIN_ID}`)).toHaveCount(0);

    // --- Sync discovers it and registers it disabled — never auto-spawned. ---
    await testPage.getByTestId("plugins-sync-button").click();
    const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
    await expect(pluginRow).toBeVisible({ timeout: 15_000 });
    await expect(pluginRow.getByText("Disabled", { exact: true })).toBeVisible();

    // --- Enable it via the UI: the record transitions to active and the
    // real subprocess is spawned/handshaken. ---
    await pluginRow.getByRole("button", { name: "Enable" }).click();
    await expect(pluginRow.getByText("Active", { exact: true })).toBeVisible({ timeout: 15_000 });

    // --- The nav item appears once the boot payload/store reflect the now-
    // active plugin (reload, matching the upload flow's own assertion). ---
    await testPage.goto("/");
    await testPage.reload();
    await expect(testPage.getByTestId(`plugin-nav-item-${NAV_ITEM_ID}`)).toBeVisible({
      timeout: 15_000,
    });
  });
});
