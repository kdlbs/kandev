import { test, expect } from "../../fixtures/test-base";

test.describe("Update notifications settings", () => {
  test("channel change persists through the floating Save action and survives reload", async ({
    testPage,
  }) => {
    test.setTimeout(30_000);

    await testPage.goto("/settings/system/updates");
    await expect(testPage.getByTestId("system-page-title")).toHaveText("Updates");

    const card = testPage.getByTestId("update-notifications-card");
    await expect(card).toBeVisible();

    // Fresh worker-scoped backend: defaults to enabled + "both" (see
    // DefaultNotifySettings in internal/system/updates/notify_settings.go).
    const toggle = card.getByRole("switch", { name: "Enable update notifications" });
    await expect(toggle).toHaveAttribute("aria-checked", "true");

    const channelTrigger = card.getByRole("combobox", { name: "Update notification channel" });
    await expect(channelTrigger).toHaveText("Both");

    await channelTrigger.click();
    await testPage.getByRole("option", { name: "Desktop notification" }).click();
    await expect(channelTrigger).toHaveText("Desktop notification");
    await expect(channelTrigger).toHaveAttribute("data-settings-dirty", "true");

    await testPage.getByRole("button", { name: "Save changes" }).click();
    await expect(channelTrigger).toHaveAttribute("data-settings-dirty", "false", {
      timeout: 10_000,
    });

    await testPage.reload();
    await expect(testPage.getByTestId("update-notifications-card")).toBeVisible();
    await expect(
      testPage
        .getByTestId("update-notifications-card")
        .getByRole("combobox", { name: "Update notification channel" }),
    ).toHaveText("Desktop notification");
  });

  test("disabling hides the channel selector", async ({ testPage }) => {
    test.setTimeout(30_000);

    await testPage.goto("/settings/system/updates");
    const card = testPage.getByTestId("update-notifications-card");
    const toggle = card.getByRole("switch", { name: "Enable update notifications" });
    await expect(toggle).toBeVisible();

    await toggle.click();
    await expect(card.getByLabel("Update notification channel")).toHaveCount(0);

    await testPage.getByRole("button", { name: "Save changes" }).click();
    await expect(toggle).toHaveAttribute("data-settings-dirty", "false", { timeout: 10_000 });

    await testPage.reload();
    const reloadedToggle = testPage
      .getByTestId("update-notifications-card")
      .getByRole("switch", { name: "Enable update notifications" });
    await expect(reloadedToggle).toHaveAttribute("aria-checked", "false");
  });
});
