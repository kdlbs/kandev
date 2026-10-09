import { test, expect } from "../../fixtures/test-base";

/**
 * Remote Docker create flow on a phone.
 *
 * Mobile parity: the same user value must be reachable by touch. The page
 * composes the shipped SSH connection card, whose phone layout is already
 * proven, so this asserts the parts that are new — the route is reachable,
 * the root-authority notice is readable rather than clipped, the form is
 * usable, and the page does not scroll horizontally.
 */
test.describe("remote docker create flow on mobile", () => {
  test("reaches the form and states the authority without horizontal overflow", async ({
    testPage,
  }) => {
    await testPage.goto("/settings/executors/new/remote_docker");

    const notice = testPage.getByTestId("remote-docker-authority-notice");
    await expect(notice).toBeVisible();
    await expect(testPage.getByTestId("ssh-connection-card")).toBeVisible();

    // The notice carries the trust boundary; a clipped one is unreadable.
    const noticeBox = await notice.boundingBox();
    const viewport = testPage.viewportSize();
    expect(noticeBox).not.toBeNull();
    expect(viewport).not.toBeNull();
    if (noticeBox && viewport) {
      expect(noticeBox.x).toBeGreaterThanOrEqual(0);
      expect(noticeBox.x + noticeBox.width).toBeLessThanOrEqual(viewport.width + 1);
    }

    const overflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    );
    expect(overflow).toBeLessThanOrEqual(0);
  });

  test("keeps the primary action reachable by touch", async ({ testPage }) => {
    await testPage.goto("/settings/executors/new/remote_docker");
    await expect(testPage.getByTestId("ssh-connection-card")).toBeVisible();

    await testPage.getByTestId("ssh-input-name").fill("build-box");
    await testPage.getByTestId("ssh-input-host").fill("build-box.invalid");

    const testButton = testPage.getByTestId("ssh-test-button").first();
    await testButton.scrollIntoViewIfNeeded();
    await expect(testButton).toBeEnabled();

    // A coarse-pointer action needs a real 44px hit target, which a utility
    // class alone does not prove.
    const box = await testButton.boundingBox();
    expect(box).not.toBeNull();
    if (box) {
      expect(box.height).toBeGreaterThanOrEqual(44);
    }

    await testButton.tap();
    // The target does not resolve, so the card must report a failure rather
    // than leaving the user on a silent spinner.
    await expect(
      testPage.getByTestId("ssh-error").or(testPage.getByTestId("ssh-test-result")),
    ).toBeVisible({ timeout: 30_000 });
  });
});
