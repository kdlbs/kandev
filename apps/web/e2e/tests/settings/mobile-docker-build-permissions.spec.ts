import { test, expect } from "../../fixtures/test-base";
import { setStoreRole } from "../../helpers/session-store";

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

    await setStoreRole(testPage, "member");

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

    // And it must not introduce document-level horizontal scrolling. The DOM
    // guarantees scrollWidth >= clientWidth, so the only passing value is 0;
    // asserting that exactly keeps the failure message readable.
    const horizontalOverflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    );
    expect(horizontalOverflow).toBe(0);
  });

  test("admin keeps the build control usable at phone width", async ({ testPage }) => {
    await testPage.goto("/settings/executors/new/local_docker");
    await expect(testPage.locator("#profile-name")).toHaveValue("Docker", { timeout: 10_000 });
    await testPage.getByRole("button", { name: "Use defaults" }).click();

    await setStoreRole(testPage, "admin");

    await expect(testPage.getByRole("button", { name: "Build Image" })).toBeEnabled();
    await expect(testPage.getByText("Only administrators can build images.")).toHaveCount(0);
  });
});
