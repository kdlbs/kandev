import { expect, test } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { seedLinearWatcher } from "./watcher-delete-confirmation-flow";

test.describe("watcher delete confirmations", () => {
  test("uses an anchored local confirmation for a Linear watcher", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const watch = await seedLinearWatcher(apiClient, seedData);
    let nativeDialogSeen = false;
    testPage.on("dialog", (dialog) => {
      nativeDialogSeen = true;
      void dialog.dismiss();
    });

    const watchesLoaded = waitForHttp(testPage, "GET", /^\/api\/v1\/linear\/watches\/issue$/);
    await testPage.goto("/settings/integrations/linear");
    await watchesLoaded;
    const row = testPage.getByTestId(`linear-watch-row-${watch.id}`);
    await expect(row).toBeVisible();
    const trigger = row.getByTestId(`linear-watch-delete-${watch.id}`);

    await trigger.click();
    const confirmation = testPage.getByTestId("watcher-delete-confirmation");
    await expect(confirmation).toBeVisible();
    await expect(
      testPage.getByRole("dialog", { name: "Delete this Linear watcher?" }),
    ).toBeVisible();
    await prCapture.screenshot("watcher-delete-confirmation-desktop", {
      caption: "Desktop watcher delete confirmation anchored to the action",
    });
    await confirmation.getByRole("button", { name: "Cancel" }).click();
    await expect(confirmation).toBeHidden();
    await expect(row).toBeVisible();

    await trigger.click();
    await testPage.getByTestId("linear-watch-delete-confirm").click();
    await expect(row).toHaveCount(0);
    expect(nativeDialogSeen).toBe(false);
  });
});
