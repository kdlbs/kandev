import { expect, test } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { seedLinearWatcher } from "./watcher-delete-confirmation-flow";

test.describe("watcher delete confirmations (mobile)", () => {
  test("keeps Linear delete inline, touch-sized, and inside the viewport", async ({
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
    const row = testPage.getByTestId(`linear-watch-mobile-row-${watch.id}`);
    await expect(row).toBeVisible();
    await expect(row).toContainText("E2E Workspace");
    const trigger = row.getByTestId(`linear-watch-delete-${watch.id}`);
    await expect(trigger).toHaveCSS("height", "44px");

    await trigger.tap();
    const confirmation = row.getByTestId("watcher-delete-confirmation");
    await expect(confirmation).toBeVisible();
    await expect(testPage.getByRole("dialog")).toHaveCount(0);
    await expect(confirmation.getByRole("button", { name: "Cancel" })).toHaveCSS("height", "44px");
    await expect(confirmation.getByRole("button", { name: "Delete" })).toHaveCSS("height", "44px");
    await prCapture.screenshot("watcher-delete-confirmation-mobile", {
      caption: "Mobile inline watcher delete confirmation with touch-sized actions",
    });

    const viewportFits = await testPage.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    );
    expect(viewportFits).toBe(true);

    await confirmation.getByRole("button", { name: "Cancel" }).tap();
    await expect(confirmation).toBeHidden();
    await trigger.tap();
    await row.getByTestId("linear-watch-delete-confirm").tap();
    await expect(row).toHaveCount(0);
    expect(nativeDialogSeen).toBe(false);
  });
});
