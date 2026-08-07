import { test, expect } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { NIGHTLY_TAG, NIGHTLY_VERSION, useManagedNPMUpdates } from "./updates-channel-helpers";

test.describe("System update channel", () => {
  test("selects and persists Nightly before offering the exact target", async ({
    backend,
    testPage,
  }, testInfo) => {
    test.setTimeout(60_000);
    const capture = new PrAssetCapture(testPage, testInfo.file);
    const fixture = await useManagedNPMUpdates(backend);
    try {
      await testPage.goto("/settings/system/updates");
      const stable = testPage.getByRole("radio", { name: /^Stable/ });
      const nightly = testPage.getByRole("radio", { name: /^Nightly/ });
      await expect(stable).toBeChecked();
      await expect(nightly).toBeEnabled();

      const saved = testPage.waitForResponse(
        (response) =>
          response.request().method() === "PATCH" &&
          new URL(response.url()).pathname === "/api/v1/system/updates/channel",
      );
      await nightly.click();
      await testPage
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: "Save changes" })
        .click();
      expect((await saved).status()).toBe(200);

      await expect(nightly).toBeChecked();
      await expect(testPage.getByTestId("system-updates-latest")).toHaveText(NIGHTLY_TAG);
      expect(fixture.registryRequests()).toBeGreaterThanOrEqual(1);

      await testPage.reload();
      await expect(nightly).toBeChecked();
      await expect(testPage.getByTestId("system-updates-latest")).toHaveText(NIGHTLY_TAG);

      await capture.screenshot("desktop-nightly-update-channel", {
        caption: "Desktop: managed npm service following the Nightly channel",
      });

      await makeExactNightlyAvailable(testPage);
      let reportTargetVersion = false;
      let applyBody: unknown;
      await testPage.route("**/api/v1/system/updates/apply", async (route) => {
        applyBody = route.request().postDataJSON();
        reportTargetVersion = true;
        await route.fulfill({
          status: 202,
          contentType: "application/json",
          body: JSON.stringify({ job_id: "nightly-update-1" }),
        });
      });
      let jobRequests = 0;
      await testPage.route("**/api/v1/system/jobs/nightly-update-1", async (route) => {
        jobRequests += 1;
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            id: "nightly-update-1",
            kind: "self-update",
            state: "succeeded",
            started_at: new Date().toISOString(),
          }),
        });
      });
      await testPage.route("**/api/v1/system/info", async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            version: reportTargetVersion ? NIGHTLY_TAG : "v1.0.0",
            commit: "abcdef123456",
            build_time: new Date().toISOString(),
            go_version: "go1.26",
            os: "linux",
            arch: "amd64",
          }),
        });
      });
      const completedReload = testPage.waitForRequest(
        (request) =>
          request.isNavigationRequest() &&
          request.frame() === testPage.mainFrame() &&
          new URL(request.url()).pathname === "/settings/system/updates",
      );
      await testPage.getByTestId("system-updates-apply").click();
      await expect(testPage.getByRole("alertdialog")).toContainText(NIGHTLY_TAG);
      await testPage.getByTestId("system-updates-apply-confirm").click();
      await expect
        .poll(() => applyBody)
        .toEqual({
          confirm: "UPDATE",
          target_version: NIGHTLY_TAG,
        });
      await expect.poll(() => jobRequests).toBeGreaterThan(0);
      await completedReload;
      await testPage.waitForLoadState("domcontentloaded");
      await expect(testPage.getByTestId("system-page-title")).toHaveText("Updates");
      await expect(testPage.getByTestId("system-updates-latest")).toHaveText(NIGHTLY_TAG);
      await expect(testPage.getByTestId("system-updates-progress")).toHaveCount(0);
    } finally {
      try {
        capture.flush();
      } finally {
        await fixture.release();
      }
    }
  });

  test("keeps unsupported installs on Stable with no Nightly mutation path", async ({
    testPage,
  }) => {
    let channelMutations = 0;
    testPage.on("request", (request) => {
      if (
        request.method() === "PATCH" &&
        new URL(request.url()).pathname === "/api/v1/system/updates/channel"
      ) {
        channelMutations += 1;
      }
    });

    await testPage.goto("/settings/system/updates");

    await expect(testPage.getByRole("radio", { name: /^Stable/ })).toBeChecked();
    await expect(testPage.getByRole("radio", { name: /^Nightly/ })).toBeDisabled();
    await expect(testPage.getByTestId("system-updates-channel-reason")).toContainText(
      /managed npm or npx user service/i,
    );
    await expect(testPage.getByTestId("settings-floating-save")).toHaveCount(0);
    await testPage.waitForLoadState("networkidle");
    expect(channelMutations).toBe(0);
  });
});

async function makeExactNightlyAvailable(testPage: Page): Promise<void> {
  await testPage.route("**/api/v1/system/updates/check", async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        current: "v1.0.0",
        latest: NIGHTLY_TAG,
        latest_url: `https://www.npmjs.com/package/kandev/v/${NIGHTLY_VERSION}`,
        latest_checked_at: new Date().toISOString(),
        update_available: true,
        channel: "nightly",
        channel_editable: true,
        channel_unsupported_reason: "",
        install: {
          running_as_service: true,
          managed_service: true,
          mode: "user",
          manager: "systemd",
          kind: "npm",
        },
        apply_supported: true,
      }),
    });
  });
  await testPage.getByTestId("system-updates-check").click();
  await expect(testPage.getByTestId("system-updates-apply")).toBeVisible();
}
