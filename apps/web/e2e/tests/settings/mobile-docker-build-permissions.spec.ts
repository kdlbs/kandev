import { test, expect } from "../../fixtures/test-base";

/**
 * Mobile parity for the admin gating on the Dockerfile build control.
 *
 * The desktop coverage lives in docker-profile-persistence.spec.ts, which the
 * `chromium` project runs at desktop viewport; only `mobile-*.spec.ts` files
 * reach the Pixel 5 `mobile-chrome` project. The gating swaps the build status
 * badge for a sentence of explanation inside a horizontal flex row, so the
 * narrow viewport is where that row can overflow, and it is asserted here
 * rather than assumed from the desktop run.
 *
 * The security boundary is the backend's authn.RequireAdmin() on
 * POST /api/v1/docker/build; this covers the UI not offering a control that
 * can only 403.
 */
test.describe("Docker build permissions on mobile", () => {
  test("member sees the build control disabled and readable at phone width", async ({
    testPage,
  }) => {
    await testPage.goto("/settings/executors/new/local_docker");
    await expect(testPage.locator("#profile-name")).toHaveValue("Docker", { timeout: 10_000 });
    await testPage.getByRole("button", { name: "Use defaults" }).click();

    const buildButton = testPage.getByRole("button", { name: "Build Image" });
    await expect(buildButton).toBeEnabled();

    await testPage.waitForFunction(() => Boolean(window.__KANDEV_E2E_STORE__));
    await testPage.evaluate(() => {
      const store = window.__KANDEV_E2E_STORE__;
      if (!store) throw new Error("E2E store bridge is unavailable");
      store.getState().setAuthState({
        mode: "enabled",
        authenticated: true,
        user: {
          id: "e2e-member",
          email: "member@e2e.dev",
          display_name: "E2E Member",
          role: "member",
          status: "active",
        },
        ssoProviders: [],
      });
    });

    await expect(buildButton).toBeDisabled();
    const explanation = testPage.getByText("Only administrators can build images.");
    await expect(explanation).toBeVisible();

    // The explanation must sit inside the viewport, not run off the side of
    // the flex row it shares with the button.
    const viewportWidth = testPage.viewportSize()?.width ?? 0;
    expect(viewportWidth).toBeGreaterThan(0);
    const explanationBox = await explanation.boundingBox();
    expect(explanationBox).not.toBeNull();
    expect(explanationBox!.x).toBeGreaterThanOrEqual(0);
    expect(explanationBox!.x + explanationBox!.width).toBeLessThanOrEqual(viewportWidth);

    // And it must not introduce document-level horizontal scrolling.
    const horizontalOverflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    );
    expect(horizontalOverflow).toBeLessThanOrEqual(0);
  });

  test("admin keeps the build control usable at phone width", async ({ testPage }) => {
    await testPage.goto("/settings/executors/new/local_docker");
    await expect(testPage.locator("#profile-name")).toHaveValue("Docker", { timeout: 10_000 });
    await testPage.getByRole("button", { name: "Use defaults" }).click();

    await testPage.waitForFunction(() => Boolean(window.__KANDEV_E2E_STORE__));
    await testPage.evaluate(() => {
      const store = window.__KANDEV_E2E_STORE__;
      if (!store) throw new Error("E2E store bridge is unavailable");
      store.getState().setAuthState({
        mode: "enabled",
        authenticated: true,
        user: {
          id: "e2e-admin",
          email: "admin@e2e.dev",
          display_name: "E2E Admin",
          role: "admin",
          status: "active",
        },
        ssoProviders: [],
      });
    });

    await expect(testPage.getByRole("button", { name: "Build Image" })).toBeEnabled();
    await expect(testPage.getByText("Only administrators can build images.")).toHaveCount(0);
  });
});
