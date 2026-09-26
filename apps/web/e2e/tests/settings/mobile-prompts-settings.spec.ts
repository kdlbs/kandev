import { test, expect } from "../../fixtures/test-base";

test.describe("Prompts settings on a phone", () => {
  test.afterEach(async ({ apiClient }) => {
    const { prompts } = await apiClient.listPrompts();
    for (const prompt of prompts) {
      if (!prompt.builtin) {
        await apiClient.deletePrompt(prompt.id).catch(() => undefined);
      }
    }
  });

  test("opens prompt deletion in a touch-sized confirmation sheet", async ({
    testPage,
    apiClient,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await apiClient.createPrompt("mobile-delete-prompt", "delete me");

    await testPage.goto("/settings/prompts");

    const row = testPage.locator(
      '[data-testid="prompt-list-item"][data-prompt-name="mobile-delete-prompt"]',
    );
    await expect(row).toBeVisible();
    await row.getByTestId("prompt-delete-button").tap();

    const inline = testPage.getByRole("dialog", { name: "Delete prompt" });
    await expect(inline).toBeVisible();
    await expect(testPage.getByTestId("prompt-delete-confirm-popover")).toHaveCount(0);
    await expect(testPage.getByRole("alertdialog")).toHaveCount(0);

    const [cancelBox, confirmBox] = await Promise.all([
      inline.getByRole("button", { name: "Cancel" }).boundingBox(),
      inline.getByTestId("prompt-delete-confirm").boundingBox(),
    ]);
    expect(cancelBox).not.toBeNull();
    expect(confirmBox).not.toBeNull();
    expect(cancelBox!.height).toBeGreaterThanOrEqual(44);
    expect(cancelBox!.width).toBeGreaterThanOrEqual(44);
    expect(confirmBox!.height).toBeGreaterThanOrEqual(44);
    expect(confirmBox!.width).toBeGreaterThanOrEqual(44);

    const hasDocumentOverflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth,
    );
    expect(hasDocumentOverflow).toBe(false);

    await inline.getByRole("button", { name: "Cancel" }).tap();
    await expect(row).toBeVisible();

    await row.getByTestId("prompt-delete-button").tap();
    await testPage.getByTestId("prompt-delete-confirm").tap();
    await expect(row).toHaveCount(0);
    expect(
      (await apiClient.listPrompts()).prompts.some(
        (prompt) => prompt.name === "mobile-delete-prompt",
      ),
    ).toBe(false);
  });
});
