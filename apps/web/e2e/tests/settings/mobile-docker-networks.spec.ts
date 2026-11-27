import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";

/**
 * Docker executor profile editing on a phone.
 *
 * Mobile parity for the container-network card and the Remote Docker
 * connection section: on a phone the additional-network rows stack as bordered
 * blocks with 44px touch targets, so this drives them by touch, saves, and
 * checks that the page never scrolls sideways.
 */

async function expectNoHorizontalOverflow(page: Page) {
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
  );
  expect(overflow).toBeLessThanOrEqual(0);
}

async function expectTouchTarget(page: Page, testLocator: ReturnType<Page["locator"]>) {
  await testLocator.scrollIntoViewIfNeeded();
  const box = await testLocator.boundingBox();
  expect(box).not.toBeNull();
  if (box) {
    expect(box.height).toBeGreaterThanOrEqual(44);
    expect(box.width).toBeGreaterThanOrEqual(44);
  }
  await expectNoHorizontalOverflow(page);
}

test.describe("docker profile editing on mobile", () => {
  test("configures container networks by touch and saves them", async ({ testPage, apiClient }) => {
    test.setTimeout(60_000);
    const exec = await apiClient.createExecutor("e2e-mobile-networks", "local_docker");
    const profile = await apiClient.createExecutorProfile(exec.id, {
      name: "networks",
      config: { image_tag: "kandev/e2e:test", dockerfile: "FROM busybox\n" },
    });

    try {
      await testPage.goto(`/settings/executors/${profile.id}`);
      const primary = testPage.locator("#docker-primary-network");
      await expect(primary).toBeVisible();
      await primary.fill("kandev-bridge");

      const addNetwork = testPage.getByRole("button", { name: "Add network" });
      await expectTouchTarget(testPage, addNetwork);
      await addNetwork.tap();
      await addNetwork.tap();
      await testPage.locator("#docker-additional-network-0").fill("lan");
      await testPage.locator("#docker-additional-priority-0").fill("10");
      await testPage.locator("#docker-additional-network-1").fill("discarded");

      const removeSecond = testPage.getByRole("button", { name: "Remove this network" }).nth(1);
      await expectTouchTarget(testPage, removeSecond);
      await removeSecond.tap();
      await expect(testPage.locator("#docker-additional-network-1")).toHaveCount(0);

      const saveButton = testPage
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: "Save changes" });

      // A fractional priority would save and then fail every launch.
      await testPage.locator("#docker-additional-priority-0").fill("1.5");
      await expect(saveButton).toBeDisabled();
      await expect(testPage.getByText("Gateway priority must be a whole number.")).toBeVisible();
      await testPage.locator("#docker-additional-priority-0").fill("10");

      await expect(saveButton).toBeEnabled();
      await saveButton.tap();

      await expect
        .poll(async () => (await apiClient.getExecutorProfile(exec.id, profile.id)).config)
        .toMatchObject({
          docker_network: "kandev-bridge",
          docker_additional_networks: JSON.stringify([{ name: "lan", gw_priority: 10 }]),
        });
      await expectNoHorizontalOverflow(testPage);
    } finally {
      await apiClient.deleteExecutorProfile(profile.id).catch(() => {});
      await apiClient.deleteExecutor(exec.id).catch(() => {});
    }
  });

  test("shows the remote Docker connection section without clipping", async ({
    testPage,
    apiClient,
  }) => {
    const exec = await apiClient.createExecutor("e2e-mobile-remote-docker", "remote_docker", {
      ssh_host: "build-box.invalid",
      ssh_user: "kandev",
      ssh_identity_source: "agent",
      ssh_host_fingerprint: "SHA256:e2e-pinned",
    });
    const profile = await apiClient.createExecutorProfile(exec.id, { name: "remote" });

    try {
      await testPage.goto(`/settings/executors/${profile.id}`);
      const notice = testPage.getByTestId("remote-docker-authority-notice");
      await expect(notice).toBeVisible();
      await expect(testPage.getByTestId("ssh-input-host")).toHaveValue("build-box.invalid");

      const noticeBox = await notice.boundingBox();
      const viewport = testPage.viewportSize();
      expect(noticeBox).not.toBeNull();
      expect(viewport).not.toBeNull();
      if (noticeBox && viewport) {
        expect(noticeBox.x).toBeGreaterThanOrEqual(0);
        expect(noticeBox.x + noticeBox.width).toBeLessThanOrEqual(viewport.width + 1);
      }

      const testButton = testPage.getByTestId("ssh-test-button").first();
      await testButton.scrollIntoViewIfNeeded();
      const box = await testButton.boundingBox();
      expect(box).not.toBeNull();
      if (box) expect(box.height).toBeGreaterThanOrEqual(44);
      await expectNoHorizontalOverflow(testPage);
    } finally {
      await apiClient.deleteExecutorProfile(profile.id).catch(() => {});
      await apiClient.deleteExecutor(exec.id).catch(() => {});
    }
  });
});
