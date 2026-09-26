import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { useManagedNPMUpdates } from "./updates-channel-helpers";

const LONG_NIGHTLY_VERSION = "999999999.999999999.999999999-nightly.shaabcdef123456";
const LONG_NIGHTLY_TAG = `v${LONG_NIGHTLY_VERSION}`;

test.describe("System update channel on mobile", () => {
  test("selects, saves, and reloads Nightly with touch-safe rows", async ({
    backend,
    testPage,
  }, testInfo) => {
    test.setTimeout(60_000);
    const capture = new PrAssetCapture(testPage, testInfo.file);
    const fixture = await useManagedNPMUpdates(backend, LONG_NIGHTLY_VERSION);
    try {
      await testPage.goto("/settings/system/updates");
      const nightly = testPage.getByRole("radio", { name: /^Nightly/ });
      const nightlyRow = testPage.getByTestId("system-updates-channel-nightly");
      const rowBox = await nightlyRow.boundingBox();
      expect(rowBox).not.toBeNull();
      expect(rowBox!.height).toBeGreaterThanOrEqual(44);
      await assertNoDocumentHorizontalOverflow(testPage, "Updates before channel selection");

      await nightlyRow.tap();
      await expect(nightly).toBeChecked();
      const saved = testPage.waitForResponse(
        (response) =>
          response.request().method() === "PATCH" &&
          new URL(response.url()).pathname === "/api/v1/system/updates/channel",
      );
      await testPage
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: "Save changes" })
        .tap();
      expect((await saved).status()).toBe(200);
      await expect(testPage.getByTestId("system-updates-latest")).toHaveText(LONG_NIGHTLY_TAG);
      expect(fixture.registryRequests()).toBeGreaterThanOrEqual(1);

      await testPage.reload();
      await expect(nightly).toBeChecked();
      await expect(testPage.getByTestId("system-updates-latest")).toHaveText(LONG_NIGHTLY_TAG);
      await assertNoDocumentHorizontalOverflow(testPage, "Updates after Nightly reload");
      await capture.screenshot("mobile-nightly-update-channel", {
        caption: "Mobile: saved Nightly channel and exact target",
      });
    } finally {
      try {
        capture.flush();
      } finally {
        await fixture.release();
      }
    }
  });

  test("explains an unsupported Nightly channel without horizontal overflow", async ({
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
    await assertNoDocumentHorizontalOverflow(testPage, "Unsupported Updates channel");
  });
});
