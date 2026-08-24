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
import { holdPluginInstallResponse } from "../../helpers/plugin-install";
import { dwell } from "../../helpers/causal-waits";
import { MAX_INLINE_PLUGIN_FOOTER_ITEMS } from "@/lib/navigation/plugin-footer-budget";
import {
  openInstallDialog,
  PACKAGE_PATH,
  PLUGIN_ID,
  uninstallPluginFixture,
  uploadPackage,
} from "./plugin-test-helpers";

const NAV_ITEM_ID = "e2e-hello";
const PLUGIN_ROUTE = "/plugins/e2e-hello";

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

async function waitForPluginBundleReady(page: Page): Promise<void> {
  const navItem = page.getByTestId(`plugin-nav-item-${NAV_ITEM_ID}`);
  await expect(navItem).toBeVisible({ timeout: 15_000 });
  // The navigation item can be registered before the plugin bundle finishes
  // evaluating. Visiting the plugin page makes the bundle's registration
  // boundary explicit before a manifest keybinding is exercised.
  await navItem.click();
  await expect(page.locator("#hello-plugin-page")).toBeVisible({ timeout: 15_000 });
}

async function uninstallViaApi(apiClient: ApiClient) {
  await uninstallPluginFixture(apiClient);
}

async function installedPluginPath(apiClient: ApiClient): Promise<string> {
  const response = await apiClient.rawRequest("GET", "/api/plugins");
  if (!response.ok) throw new Error(`GET /api/plugins failed: ${response.status}`);
  const body = (await response.json()) as {
    plugins?: Array<{ id: string; install_path: string }>;
  };
  const record = body.plugins?.find((plugin) => plugin.id === PLUGIN_ID);
  if (!record) throw new Error(`plugin ${PLUGIN_ID} was not returned by GET /api/plugins`);
  return record.install_path;
}

function pluginExecutablePath(installPath: string): string {
  const serverDir = path.join(installPath, "server");
  const executable = fs
    .readdirSync(serverDir, { withFileTypes: true })
    .find((entry) => entry.isFile())?.name;
  if (!executable) throw new Error(`no plugin executable found under ${serverDir}`);
  return path.join(serverDir, executable);
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
    await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      app_status_bar_enabled: false,
    });
  });

  test("shows an install spinner while an upload install is pending", async ({ testPage }) => {
    test.setTimeout(90_000);

    await openInstallDialog(testPage);
    const heldInstall = await holdPluginInstallResponse(testPage);
    try {
      await uploadPackage(testPage, PACKAGE_PATH);
      await heldInstall.requestSeen;

      const installButton = testPage.getByTestId("install-plugin-upload-submit");
      await expect(installButton).toBeDisabled();
      await expect(installButton).toHaveAttribute("aria-busy", "true");
      await expect(installButton.locator(".animate-spin")).toBeVisible();
      await expect(installButton).toHaveText(/Installing/);
    } finally {
      heldInstall.release();
      if (heldInstall.requestStarted()) await heldInstall.responseSettled;
      await testPage.unroute("**/api/plugins/install");
    }

    const installButton = testPage.getByTestId("install-plugin-upload-submit");
    await expect(installButton).toBeEnabled();
    await expect(installButton).toHaveText("Install");
    await expect(testPage.getByTestId("install-plugin-error")).toContainText("install failed");
  });

  test("installs via upload, loads the UI, live-updates via WS+gRPC, and uninstalls", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);
    await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      app_status_bar_enabled: true,
    });

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

    const movedOrderingId = `plugin:${PLUGIN_ID}:app-status-bar-left:0`;
    const movedContribution = testPage.locator(`[data-status-item-id="${movedOrderingId}"]`);
    const [movedBox, statusBarBox] = await Promise.all([
      movedContribution.boundingBox(),
      testPage.getByTestId("app-status-bar").boundingBox(),
    ]);
    if (!movedBox || !statusBarBox) throw new Error("plugin status drag geometry unavailable");
    const orderSaved = testPage.waitForResponse(
      (response) =>
        response.request().method() === "PATCH" && response.url().endsWith("/api/v1/user/settings"),
    );
    await testPage.keyboard.down("Meta");
    await testPage.mouse.move(movedBox.x + movedBox.width / 2, movedBox.y + movedBox.height / 2);
    await testPage.mouse.down();
    await testPage.mouse.move(
      statusBarBox.x + statusBarBox.width - 8,
      statusBarBox.y + statusBarBox.height / 2,
      { steps: 8 },
    );
    await testPage.mouse.up();
    await testPage.keyboard.up("Meta");
    expect((await orderSaved).ok()).toBe(true);
    expect(await testPage.evaluate(() => window.getSelection()?.toString() ?? "")).toBe("");
    await expect(movedContribution).toHaveAttribute("data-status-side", "right");

    await backend.restart();
    await testPage.reload();
    await expect(testPage.locator(`[data-status-item-id="${movedOrderingId}"]`)).toHaveAttribute(
      "data-status-side",
      "right",
      { timeout: 15_000 },
    );

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
    await expect(testPage.locator("#hello-status-left")).toHaveCount(0);
    await expect(testPage.locator("#hello-status-right")).toHaveCount(0);
    await testPage.goto("/");
    await expect(testPage.getByTestId(`plugin-nav-item-${NAV_ITEM_ID}`)).toHaveCount(0);
    await expect(testPage.locator("#hello-status-left")).toHaveCount(0);
    await expect(testPage.locator("#hello-status-right")).toHaveCount(0);

    // --- 6. Re-enable: nav item reappears live (no reload needed) ---
    await testPage.goto("/settings/plugins");
    await pluginRow.getByRole("button", { name: "Enable" }).click();
    await expect(pluginRow.getByText("Active", { exact: true })).toBeVisible({ timeout: 10_000 });
    await expect(testPage.locator(`[data-status-item-id="${movedOrderingId}"]`)).toHaveAttribute(
      "data-status-side",
      "right",
      { timeout: 15_000 },
    );
    await testPage.goto("/");
    await expect(testPage.getByTestId(`plugin-nav-item-${NAV_ITEM_ID}`)).toBeVisible();
    await expect(testPage.locator("#hello-status-left")).toHaveText("Hello status bar no-task");
    await expect(testPage.locator("#hello-status-right")).toHaveText("Hello status bar no-task");

    await testPage.goto("/settings/plugins");

    // --- 7. Uninstall via UI (with confirmation): row disappears, package
    // directory tree is removed from disk. ---
    await pluginRow.getByRole("button", { name: "Uninstall" }).click();
    await expect(testPage.getByTestId("plugin-uninstall-confirm-popover")).toContainText("E2E");
    await expect(testPage.locator('[data-slot="dialog-overlay"]')).toHaveCount(0);
    await testPage.getByTestId("plugin-uninstall-confirm").click();
    await expect(pluginRow).toHaveCount(0, { timeout: 10_000 });

    const pluginDir = path.join(pluginsDir, PLUGIN_ID);
    await expect.poll(() => fs.existsSync(pluginDir), { timeout: 10_000 }).toBe(false);
  });

  test("auto-update: global default persists and a per-plugin override toggles and resets", async ({
    testPage,
  }) => {
    test.setTimeout(60_000);

    await openInstallDialog(testPage);
    await uploadPackage(testPage, PACKAGE_PATH);
    const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
    await expect(pluginRow).toBeVisible({ timeout: 15_000 });

    const globalToggle = testPage.getByTestId("plugins-auto-update-default");
    const rowToggle = pluginRow.getByTestId(`plugin-auto-update-${PLUGIN_ID}`);
    const rowReset = pluginRow.getByTestId(`plugin-auto-update-reset-${PLUGIN_ID}`);

    // Default is off (opt-in); the row inherits it — unchecked, no override.
    await expect(globalToggle).toBeEnabled({ timeout: 15_000 });
    await expect(globalToggle).toHaveAttribute("aria-checked", "false");
    await expect(rowToggle).toHaveAttribute("aria-checked", "false");
    await expect(rowReset).toHaveCount(0);

    // Turn the instance-wide default on; the inheriting row reflects it.
    await globalToggle.click();
    await expect(globalToggle).toHaveAttribute("aria-checked", "true");
    await expect(rowToggle).toHaveAttribute("aria-checked", "true");
    await expect(rowReset).toHaveCount(0);

    // The default persisted server-side (plugin_settings) across a reload.
    await testPage.reload();
    await expect(globalToggle).toHaveAttribute("aria-checked", "true", { timeout: 15_000 });
    await expect(rowToggle).toHaveAttribute("aria-checked", "true");

    // Override the row OFF despite the on default → shows override + Reset.
    await rowToggle.click();
    await expect(rowToggle).toHaveAttribute("aria-checked", "false");
    await expect(rowReset).toBeVisible();

    // The per-plugin override persisted on the plugin record across a reload.
    await testPage.reload();
    await expect(rowToggle).toHaveAttribute("aria-checked", "false", { timeout: 15_000 });
    await expect(rowReset).toBeVisible();

    // Reset clears the override → the row inherits the (still on) default again.
    await rowReset.click();
    await expect(rowToggle).toHaveAttribute("aria-checked", "true");
    await expect(rowReset).toHaveCount(0);

    // Leave the instance-wide default off so sibling tests start clean.
    await globalToggle.click();
    await expect(globalToggle).toHaveAttribute("aria-checked", "false");
  });

  /**
   * Deliberate scope limit (see docs/plans/plugins/task-*): there is no
   * fixture package for a *second* signed version, so this proves the
   * operator-triggered check surfaces a highlighted Update button via a
   * route-mocked catalog, and that a manual update failing against a
   * (deliberately unreachable) mocked `package_url` renders inline without
   * disturbing the rest of the row. The real, successful reinstall path is
   * covered at the unit level (use-plugin-update-action.test.tsx).
   */
  test("marketplace update check highlights the Update button, and a failing manual update shows an inline error", async ({
    testPage,
  }) => {
    test.setTimeout(60_000);

    await openInstallDialog(testPage);
    await uploadPackage(testPage, PACKAGE_PATH);
    const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
    await expect(pluginRow).toBeVisible({ timeout: 15_000 });

    const newerVersion = "9.9.9";
    await testPage.route("**/api/plugins/marketplace", async (route) => {
      if (route.request().method() !== "GET") return route.fallback();
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          plugins: [
            {
              id: PLUGIN_ID,
              name: "E2E Hello",
              description: "",
              author: "kandev",
              categories: [],
              icon_url: "",
              repo_url: "",
              version: newerVersion,
              min_kandev_version: "",
              package_url: "https://example.invalid/kandev-plugin-e2e-9.9.9.tar.gz",
              package_sha256: "",
              stars: 0,
              updated_at: new Date(0).toISOString(),
              install_state: "update_available",
              installed_version: "1.0.0",
              source_id: "official",
              source_name: "Kandev Official",
            },
          ],
          sources: [
            {
              id: "official",
              name: "Kandev Official",
              url: "https://example.invalid",
              enabled: true,
              builtin: true,
              healthy: true,
            },
          ],
        }),
      });
    });
    await testPage.route("**/api/plugins/marketplace/refresh", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ refreshed: true }),
      }),
    );

    await testPage.getByTestId("plugins-check-updates-button").click();

    const latestVersion = pluginRow.getByTestId(`plugin-latest-version-${PLUGIN_ID}`);
    const updateButton = pluginRow.getByTestId(`plugin-update-${PLUGIN_ID}`);
    await expect(latestVersion).toContainText(newerVersion, { timeout: 15_000 });
    await expect(updateButton).toBeVisible();
    await expect(updateButton).toHaveAttribute("data-variant", "default");
    await expect(testPage.getByTestId("plugins-updates-last-checked")).toBeVisible();

    // The update tries to install from the (deliberately unreachable) mocked
    // package_url and fails — the row surfaces the error inline, keeps the
    // old version, and keeps the button clickable, without disturbing
    // enable/disable/uninstall or the rest of the row.
    await updateButton.click();
    await expect(pluginRow.getByTestId(`plugin-update-error-${PLUGIN_ID}`)).toBeVisible({
      timeout: 15_000,
    });
    await expect(updateButton).toBeEnabled();
    await expect(pluginRow.getByText("Active", { exact: true })).toBeVisible();
    await expect(pluginRow.getByRole("button", { name: "Disable" })).toBeEnabled();

    await testPage.unroute("**/api/plugins/marketplace");
    await testPage.unroute("**/api/plugins/marketplace/refresh");
  });

  test("row and detail uninstall confirmations stay local to their initiating controls", async ({
    testPage,
  }) => {
    test.setTimeout(60_000);

    await openInstallDialog(testPage);
    await uploadPackage(testPage, PACKAGE_PATH);
    const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
    await expect(pluginRow).toBeVisible({ timeout: 15_000 });

    await pluginRow.getByRole("button", { name: "Uninstall" }).click();
    const rowConfirmation = testPage.getByTestId("plugin-uninstall-confirm-popover");
    await expect(rowConfirmation).toContainText("Kandev E2E Fixture Plugin");
    await expect(testPage.locator('[data-slot="dialog-overlay"]')).toHaveCount(0);
    await rowConfirmation.getByRole("button", { name: "Cancel" }).click();
    await expect(rowConfirmation).toHaveCount(0);

    await pluginRow.click({ position: { x: 600, y: 12 } });
    await expect(testPage).toHaveURL(new RegExp(`/settings/plugins/${PLUGIN_ID}$`));
    const detail = testPage.getByTestId(`plugin-detail-${PLUGIN_ID}`);
    await expect(detail).toBeVisible();
    await detail.getByRole("button", { name: "Uninstall" }).click();

    const detailConfirmation = testPage.getByTestId("plugin-uninstall-confirm-popover");
    await expect(detailConfirmation).toContainText("Kandev E2E Fixture Plugin");
    await expect(testPage.locator('[data-slot="dialog-overlay"]')).toHaveCount(0);
    await testPage.getByTestId("plugin-uninstall-confirm").click();
    await expect(testPage).toHaveURL(/\/settings\/plugins$/);
    await expect(testPage.getByTestId(`plugin-row-${PLUGIN_ID}`)).toHaveCount(0);
  });

  test("shows boot failure diagnostics and retries an errored plugin", async ({
    testPage,
    apiClient,
    backend,
  }) => {
    test.setTimeout(120_000);

    await openInstallDialog(testPage);
    await uploadPackage(testPage, PACKAGE_PATH);
    const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
    await expect(pluginRow).toBeVisible({ timeout: 15_000 });
    await expect(pluginRow.getByText("Active", { exact: true })).toBeVisible();

    const installPath = await installedPluginPath(apiClient);
    const executablePath = pluginExecutablePath(installPath);
    const unavailablePath = `${executablePath}.unavailable`;
    fs.renameSync(executablePath, unavailablePath);

    try {
      await backend.restart();
      await expect
        .poll(
          async () => {
            const response = await apiClient.rawRequest("GET", `/api/plugins/${PLUGIN_ID}`);
            if (!response.ok) return "missing";
            return ((await response.json()) as { status?: string }).status ?? "missing";
          },
          { timeout: 20_000, intervals: [250, 500, 1000] },
        )
        .toBe("error");

      await testPage.goto("/settings/plugins");
      await expect(pluginRow.getByText("Error", { exact: true })).toBeVisible({ timeout: 15_000 });
      await expect(pluginRow.getByTestId(`plugin-error-${PLUGIN_ID}`)).toBeVisible();
      await expect(pluginRow.getByRole("button", { name: "Enable" })).toBeVisible();

      await pluginRow.getByTestId(`plugin-row-link-${PLUGIN_ID}`).click();
      const detail = testPage.getByTestId(`plugin-detail-${PLUGIN_ID}`);
      await expect(detail.getByTestId(`plugin-error-${PLUGIN_ID}`)).toBeVisible();
      await expect(detail.getByRole("button", { name: "Enable" })).toBeVisible();

      const firstDiagnostic = await detail
        .getByTestId(`plugin-error-${PLUGIN_ID}`)
        .locator("span")
        .textContent();
      if (!firstDiagnostic) throw new Error("initial plugin diagnostic was empty");

      // The first retry still sees the missing executable. The hook must
      // refetch the backend record even though the action remains failed.
      await detail.getByRole("button", { name: "Enable" }).click();
      await expect(detail.getByText("Error", { exact: true })).toBeVisible({ timeout: 15_000 });

      // Change the failure mode without restoring the real binary. Linux
      // reports an executable-format error, which must replace the original
      // missing-file diagnostic in the detail view.
      fs.writeFileSync(executablePath, "not a go-plugin executable\n");
      if (process.platform !== "win32") fs.chmodSync(executablePath, 0o755);
      await detail.getByRole("button", { name: "Enable" }).click();
      await expect(detail.getByText("Error", { exact: true })).toBeVisible({ timeout: 15_000 });
      await expect
        .poll(
          async () =>
            (await detail.getByTestId(`plugin-error-${PLUGIN_ID}`).locator("span").textContent()) ??
            "",
          { timeout: 15_000, intervals: [250, 500, 1000] },
        )
        .not.toBe(firstDiagnostic);

      fs.rmSync(executablePath, { force: true });
      fs.renameSync(unavailablePath, executablePath);
      await detail.getByRole("button", { name: "Enable" }).click();
      await expect(detail.getByText("Active", { exact: true })).toBeVisible({ timeout: 15_000 });
      await expect(detail.getByTestId(`plugin-error-${PLUGIN_ID}`)).toHaveCount(0);
    } finally {
      if (fs.existsSync(unavailablePath) && fs.existsSync(executablePath)) {
        fs.rmSync(executablePath, { force: true });
      }
      if (fs.existsSync(unavailablePath) && !fs.existsSync(executablePath)) {
        fs.renameSync(unavailablePath, executablePath);
      }
    }
  });

  test("keybinding declared in the manifest opens a host.openModal demo modal", async ({
    testPage,
  }) => {
    test.setTimeout(60_000);

    // --- Install via the upload UI, then reload so the boot payload (and
    // the manifest-driven keybinding registration) is live. ---
    await openInstallDialog(testPage);
    await uploadPackage(testPage, PACKAGE_PATH);
    const pluginRow = testPage.getByTestId(`plugin-row-${PLUGIN_ID}`);
    await expect(pluginRow).toBeVisible({ timeout: 15_000 });
    await expect(pluginRow.getByText("Active", { exact: true })).toBeVisible({ timeout: 30_000 });

    await testPage.goto("/");
    await testPage.reload();
    await waitForPluginBundleReady(testPage);

    // --- manifest.yaml declares `ui.keybindings: [{ id: open-demo, default:
    // mod+shift+j }]`; bundle.js binds it to host.openModal(...). "mod"
    // resolves to Ctrl/Cmd per-platform, matching Playwright's
    // "ControlOrMeta" pseudo-modifier. ---
    await testPage.keyboard.press("ControlOrMeta+Shift+J");
    const modal = testPage.getByTestId("hello-demo-modal");
    await expect(modal).toBeVisible();
    // toContainText, not toHaveText: the modal body also carries the tooltip
    // trigger exercised by the hover spec below, and this assertion is about
    // the keybinding reaching host.openModal, not the body's exact contents.
    await expect(modal).toContainText("Hello from the plugin modal");

    // --- The host Dialog's built-in close button dismisses the (dismissible
    // by default) modal. ---
    await testPage.getByRole("button", { name: "Close" }).click();
    await expect(modal).not.toBeVisible();
  });

  // `PluginModalHost` owns a TooltipProvider so plugin modal content remains
  // safe in both AppShell and isolated mounts. The unit test for this asserts
  // via focus, because jsdom does not
  // reliably open a Radix tooltip from synthetic hover (apps/web/CLAUDE.md) —
  // real pointer hover, and the portaled role="tooltip" it produces, are only
  // assertable in a browser.
  test("a Tooltip inside host.openModal content opens on real pointer hover", async ({
    testPage,
  }) => {
    test.setTimeout(60_000);

    await openInstallDialog(testPage);
    await uploadPackage(testPage, PACKAGE_PATH);
    await expect(testPage.getByTestId(`plugin-row-${PLUGIN_ID}`)).toBeVisible({ timeout: 15_000 });
    await expect(
      testPage.getByTestId(`plugin-row-${PLUGIN_ID}`).getByText("Active", { exact: true }),
    ).toBeVisible({ timeout: 30_000 });

    await testPage.goto("/");
    await testPage.reload();
    await waitForPluginBundleReady(testPage);

    await testPage.keyboard.press("ControlOrMeta+Shift+J");
    const modal = testPage.getByTestId("hello-demo-modal");
    await expect(modal).toBeVisible();

    // Without a provider in scope the Tooltip throws during render and
    // PluginErrorBoundary swallows the entire modal body, so the trigger
    // being present is itself part of the assertion.
    const trigger = testPage.getByTestId("hello-modal-tooltip-trigger");
    await expect(trigger).toBeVisible();

    await trigger.hover();
    await expect(
      testPage.getByRole("tooltip").filter({ hasText: "Tooltip inside a plugin modal" }),
    ).toBeVisible();
  });

  // host.toast is a per-plugin Proxy over sonner. The unit tests only ever see
  // it against a mocked sonner, so nothing has observed its runtime behavior:
  // that a real toast renders, and that `.error` does NOT file a
  // frontend-error report the way the app's own reporting seam does.
  test("host.toast.error renders a real toast and files no frontend-error report", async ({
    testPage,
  }) => {
    test.setTimeout(60_000);

    await openInstallDialog(testPage);
    await uploadPackage(testPage, PACKAGE_PATH);
    await expect(testPage.getByTestId(`plugin-row-${PLUGIN_ID}`)).toBeVisible({ timeout: 15_000 });

    await testPage.goto("/");
    await testPage.reload();
    await testPage.getByTestId(`plugin-nav-item-${NAV_ITEM_ID}`).click();

    const reportRequests: string[] = [];
    testPage.on("request", (request) => {
      if (request.url().includes("/api/v1/system/logs/frontend-errors")) {
        reportRequests.push(request.url());
      }
    });
    const consoleErrors: string[] = [];
    testPage.on("console", (message) => {
      if (message.type() === "error") consoleErrors.push(message.text());
    });

    const trigger = testPage.getByTestId("hello-toast-error");
    await expect(trigger).toBeVisible({ timeout: 15_000 });
    await trigger.click();

    // The real sonner <Toaster/> the app mounts in app/layout.tsx.
    await expect(
      testPage.locator("[data-sonner-toast]").filter({ hasText: "Plugin toast error" }),
    ).toBeVisible();

    // The load-bearing assertion: the app's reporting seam would POST here on
    // every .error, so a polling plugin would file an Error-level backend log
    // entry per cycle. The report is scheduled in a microtask and sent async,
    // so give it a real chance to fire before asserting it never did.
    await dwell(
      testPage,
      500,
      "negative-assertion",
      "the report is scheduled in a microtask and sent async, and the assertion is that it never fires; a request that must not happen has no event, so it needs a real chance to arrive",
    );
    expect(reportRequests).toEqual([]);

    // Attribution goes to the console instead, matching every other plugin
    // failure path.
    expect(consoleErrors.some((line) => line.includes(`toast.error from "${PLUGIN_ID}"`))).toBe(
      true,
    );
  });

  // host.theme is a live getter and host.onThemeChange is backed by a
  // MutationObserver on <html>'s class. jsdom's MutationObserver is a shim, so
  // only a browser proves a real theme flip reaches a plugin subscriber.
  test("host.onThemeChange fires in a real browser when the app theme flips", async ({
    testPage,
  }) => {
    test.setTimeout(60_000);

    await openInstallDialog(testPage);
    await uploadPackage(testPage, PACKAGE_PATH);
    await expect(testPage.getByTestId(`plugin-row-${PLUGIN_ID}`)).toBeVisible({ timeout: 15_000 });

    await testPage.goto("/");
    await testPage.reload();
    await testPage.getByTestId(`plugin-nav-item-${NAV_ITEM_ID}`).click();

    // The readout seeds from host.theme once, then only ever updates from an
    // onThemeChange notification — so a stale value here means the
    // subscription never fired.
    const readout = testPage.getByTestId("hello-theme-readout");
    await expect(readout).toBeVisible({ timeout: 15_000 });
    const before = await readout.textContent();
    expect(before === "light" || before === "dark").toBe(true);

    // Flip through the app's own command, not by poking the DOM, so this
    // exercises AppThemeProvider the way a user would.
    const target = before === "dark" ? "Switch to Light Mode" : "Switch to Dark Mode";
    await testPage.keyboard.press("ControlOrMeta+k");
    await testPage.getByRole("option", { name: target }).click();

    await expect(readout).toHaveText(before === "dark" ? "light" : "dark");
    await expect(readout).toHaveAttribute("data-theme-changes", "1");
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

    // The fixture declares a required api_token nobody has filled in yet, so
    // the row advertises the settings page it wants the operator to open.
    await expect(pluginRow.getByTestId(`plugin-setup-required-${PLUGIN_ID}`)).toBeVisible({
      timeout: 15_000,
    });

    // Click the card body, not the plugin name: the whole row is the target,
    // and an overlay link that stops covering it would strand the settings
    // page behind a name that no longer looks clickable.
    await pluginRow.click({ position: { x: 600, y: 12 } });
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

    // --- Saving restarted the plugin; it must be Active again, and the row
    // must stop asking for setup now that the required field is stored. ---
    await testPage.goto("/settings/plugins");
    await expect(pluginRow.getByText("Active", { exact: true })).toBeVisible({ timeout: 15_000 });
    await expect(pluginRow.getByTestId(`plugin-setup-required-${PLUGIN_ID}`)).toBeHidden();

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

    // --- Uninstall from the detail danger zone: the confirmation stays at
    // the initiating control and only successful cleanup returns to the list. ---
    await testPage.goto(`/settings/plugins/${PLUGIN_ID}`);
    const detail = testPage.getByTestId(`plugin-detail-${PLUGIN_ID}`);
    await expect(detail).toBeVisible();
    await detail.getByRole("button", { name: "Uninstall" }).click();
    await expect(testPage.getByTestId("plugin-uninstall-confirm-popover")).toContainText("E2E");
    await expect(testPage.locator('[data-slot="dialog-overlay"]')).toHaveCount(0);
    await testPage.getByTestId("plugin-uninstall-confirm").click();
    await expect(testPage).toHaveURL(/\/settings\/plugins$/);
    await expect(testPage.getByTestId(`plugin-row-${PLUGIN_ID}`)).toHaveCount(0);
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

  test("registers a sidebar-footer-section item as a footer icon, not a rail row", async ({
    testPage,
  }) => {
    test.setTimeout(60_000);

    await openInstallDialog(testPage);
    await uploadPackage(testPage, PACKAGE_PATH);
    await expect(testPage.getByTestId(`plugin-row-${PLUGIN_ID}`)).toBeVisible({ timeout: 15_000 });

    await testPage.goto("/");
    await testPage.reload();

    const footerButton = testPage.getByTestId(
      `sidebar-plugin:${PLUGIN_ID}:e2e-insights-tools-button`,
    );
    await expect(footerButton).toBeVisible({ timeout: 15_000 });
    await expect(footerButton).toHaveAttribute("aria-label", "E2E Insights Tools");
    await footerButton.click();
    await expect(testPage).toHaveURL(/\/plugins\/e2e-hello$/);

    // Moves, does not add: the same item never also renders in the rail.
    await expect(testPage.getByTestId("plugin-nav-item-e2e-insights-tools")).toHaveCount(0);
  });

  test("routes the over-budget sidebar-footer item through the overflow menu, hidden until opened", async ({
    testPage,
  }) => {
    test.setTimeout(60_000);

    // The fixture registers 4 sidebar-footer items, in registration order:
    // e2e-insights-tools (id/label "E2E Insights Tools"), then
    // -2/-3/-4 ("E2E Overflow Item 2/3/4"). Deliberately one more than the
    // desktop footer's exported MAX_INLINE_PLUGIN_FOOTER_ITEMS budget, so
    // this single install drives the real overflow trigger/menu with the
    // actual Radix DropdownMenu (spec.md#Capacity-and-overflow,
    // spec.md#The-guarantee). Unit coverage in app-sidebar-footer.test.tsx
    // mocks DropdownMenu as a pass-through, so it cannot prove the real
    // component opens on click or hides its content while closed; this is
    // the test that does.
    //
    // Per spec.md#Capacity-and-overflow, conformance tests derive their
    // expectations from the exported constant rather than hard-coding the
    // digits — which item lands inline vs. in the overflow menu is computed
    // below from MAX_INLINE_PLUGIN_FOOTER_ITEMS, not hard-coded as 3/4.
    const fixtureItemIds = [
      "e2e-insights-tools",
      "e2e-insights-tools-2",
      "e2e-insights-tools-3",
      "e2e-insights-tools-4",
    ];
    const fixtureItemLabels: Record<string, string> = {
      "e2e-insights-tools": "E2E Insights Tools",
      "e2e-insights-tools-2": "E2E Overflow Item 2",
      "e2e-insights-tools-3": "E2E Overflow Item 3",
      "e2e-insights-tools-4": "E2E Overflow Item 4",
    };
    const inlineIds = fixtureItemIds.slice(0, MAX_INLINE_PLUGIN_FOOTER_ITEMS);
    const overflowIds = fixtureItemIds.slice(MAX_INLINE_PLUGIN_FOOTER_ITEMS);
    // The fixture must register more items than the budget for this test to
    // exercise the overflow menu at all — fail loudly here rather than
    // silently degrading to an all-inline run if the budget is ever raised
    // to meet or exceed the fixture's fixed item count.
    expect(overflowIds.length).toBeGreaterThan(0);

    await openInstallDialog(testPage);
    await uploadPackage(testPage, PACKAGE_PATH);
    await expect(testPage.getByTestId(`plugin-row-${PLUGIN_ID}`)).toBeVisible({ timeout: 15_000 });

    await testPage.goto("/");
    await testPage.reload();

    for (const id of inlineIds) {
      await expect(testPage.getByTestId(`sidebar-plugin:${PLUGIN_ID}:${id}-button`)).toBeVisible({
        timeout: 15_000,
      });
    }

    const overBudgetId = overflowIds[0];
    const overBudgetTestId = `sidebar-plugin:${PLUGIN_ID}:${overBudgetId}-button`;
    const overflowTrigger = testPage.getByTestId("sidebar-plugin-overflow-button");
    await expect(overflowTrigger).toBeVisible();

    // Closed-menu guarantee: the over-budget item's button carries the same
    // testid an inline button would use (spec.md#Rendered-identity), so it
    // must be entirely absent from the DOM while the menu is closed, not
    // merely hidden — a real DropdownMenu unmounts its content when closed.
    await expect(testPage.getByTestId(overBudgetTestId)).toHaveCount(0);

    await overflowTrigger.click();

    const menuItem = testPage.getByTestId(overBudgetTestId);
    await expect(menuItem).toBeVisible();
    await expect(menuItem).toHaveText(fixtureItemLabels[overBudgetId]);
    await menuItem.click();
    await expect(testPage).toHaveURL(/\/plugins\/e2e-hello$/);
  });
});
