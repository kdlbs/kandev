import { expect } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { backendFixture as test } from "../../fixtures/backend";
import { setupAdmin } from "../../helpers/auth";

/**
 * Captures the pre-auth login page with a plugin-contributed "Continue with
 * Google" SSO button into e2e/.auth-screenshots/. Fully real end-to-end: it
 * installs the actual kandev-plugin-google-oidc package, so the button comes
 * from the real boot payload (auth.ssoProviders, populated by the backend's
 * plugins.SSOProviders() from the installed plugin's manifest).
 *
 * Runs in the `auth` project (backend restarted with auth required). Plugins
 * are already enabled in the e2e profile.
 */
const SHOT_DIR = path.join(__dirname, "..", "..", ".auth-screenshots");
const PLUGIN_PKG = path.join(
  process.env.HOME ?? "",
  "kandev-plugins/kandev-plugin-google-oidc/kandev-plugin-google-oidc-0.1.0.tar.gz",
);
const PLUGIN_ID = "kandev-plugin-google-oidc";

test.describe.serial("sso login screenshot", () => {
  const ADMIN = { email: "admin@demo.dev", password: "adminpass123", displayName: "Ada Admin" };

  test.beforeAll(async ({ backend }) => {
    fs.mkdirSync(SHOT_DIR, { recursive: true });
    await backend.restart({ KANDEV_FEATURES_AUTH: "true" });
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("login page shows the Google SSO button", async ({ browser, backend }) => {
    // Local capture only: needs the out-of-tree plugin package built via
    // `make package-host` in kandev-plugin-google-oidc. Skip where it's absent
    // (e.g. CI) rather than fail.
    test.skip(!fs.existsSync(PLUGIN_PKG), `plugin package not found at ${PLUGIN_PKG}`);

    // 1. Become admin (setup mode -> enabled). Install requires an admin.
    const adminCtx = await browser.newContext({ baseURL: backend.frontendUrl });
    await setupAdmin(adminCtx, backend.baseUrl, ADMIN);

    // 2. Install the real Google OIDC plugin package as admin.
    const pkg = fs.readFileSync(PLUGIN_PKG);
    const install = await adminCtx.request.post(`${backend.baseUrl}/api/plugins/install`, {
      multipart: {
        package: { name: path.basename(PLUGIN_PKG), mimeType: "application/gzip", buffer: pkg },
      },
    });
    expect(
      install.ok(),
      `install failed: ${install.status()} ${await install.text()}`,
    ).toBeTruthy();

    // 3. Wait for the plugin subprocess to spawn and go active.
    await expect
      .poll(
        async () => {
          const res = await adminCtx.request.get(`${backend.baseUrl}/api/plugins/${PLUGIN_ID}`);
          if (!res.ok()) return "missing";
          return ((await res.json()) as { status?: string }).status ?? "unknown";
        },
        { timeout: 20_000, intervals: [500] },
      )
      .toBe("active");

    // 4. The anonymous boot payload now advertises the provider.
    const stateRes = await adminCtx.request.get(`${backend.baseUrl}/api/v1/app-state?path=/`);
    const boot = (await stateRes.json()) as {
      initialState?: { auth?: { ssoProviders?: Array<{ id: string; displayName: string }> } };
    };
    const providers = boot.initialState?.auth?.ssoProviders ?? [];
    expect(providers.map((p) => p.id)).toContain("google");
    await adminCtx.close();

    // 5. Fresh anonymous browser -> the login page renders the SSO button.
    const ctx = await browser.newContext({
      baseURL: backend.frontendUrl,
      viewport: { width: 1280, height: 900 },
    });
    const page = await ctx.newPage();
    await page.goto("/");
    await expect(page.getByTestId("login-email")).toBeVisible({ timeout: 15_000 });
    const ssoButton = page.getByTestId("login-sso-google");
    await expect(ssoButton).toBeVisible();
    await expect(ssoButton).toHaveText(/Continue with Google/);

    await page.screenshot({ path: path.join(SHOT_DIR, "13-sso-login.png"), fullPage: true });
    await ctx.close();
  });
});
