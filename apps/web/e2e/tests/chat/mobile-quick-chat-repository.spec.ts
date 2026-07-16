import { test, expect } from "../../fixtures/test-base";

async function assertNoHorizontalOverflow(page: import("@playwright/test").Page) {
  const dimensions = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(dimensions.scrollWidth).toBeLessThanOrEqual(dimensions.clientWidth + 1);
}

test.describe("Quick Chat repository context on mobile", () => {
  test("selects an agent and repository branch without hidden actions", async ({ testPage }) => {
    test.setTimeout(90_000);
    await testPage.goto("/");
    await testPage.waitForLoadState("networkidle");
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${modifier}+Shift+q`);

    const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
    await expect(dialog.getByTestId("quick-chat-setup")).toBeVisible({ timeout: 10_000 });
    await expect(dialog.getByTestId("quick-chat-resize-left")).not.toBeVisible();
    await expect(dialog.getByTestId("quick-chat-resize-right")).not.toBeVisible();
    await expect(
      dialog.getByText("Chat with an agent about an idea, question, or codebase."),
    ).toBeVisible();

    const agentSelector = dialog.getByTestId("agent-profile-selector");
    if (await agentSelector.getByText("Select agent", { exact: false }).isVisible()) {
      await agentSelector.click();
      await testPage.getByRole("option").first().click();
    }
    await dialog.getByTestId("add-repository").click();
    await dialog.getByTestId("repo-chip-trigger").click();
    await testPage.getByRole("option").first().click();
    await expect(dialog.getByTestId("branch-chip-trigger")).toContainText("main", {
      timeout: 10_000,
    });
    await assertNoHorizontalOverflow(testPage);

    await dialog.getByTestId("quick-chat-start").click();
    await expect(dialog.locator(".tiptap.ProseMirror")).toBeVisible({ timeout: 30_000 });
    await assertNoHorizontalOverflow(testPage);

    await dialog.getByLabel("Start new chat").click();
    await expect(dialog.getByTestId("quick-chat-setup")).toBeVisible({ timeout: 5_000 });
    const secondAgentSelector = dialog.getByTestId("agent-profile-selector");
    if (await secondAgentSelector.getByText("Select agent", { exact: false }).isVisible()) {
      await secondAgentSelector.click();
      await testPage.getByRole("option").first().click();
    }
    await dialog.getByTestId("quick-chat-start").click();
    await expect(dialog.locator(".tiptap.ProseMirror")).toBeVisible({ timeout: 30_000 });

    const originalTabs = dialog.getByTestId("quick-chat-tab");
    await expect(originalTabs).toHaveCount(2);
    const originalNames = await originalTabs.locator("span").allTextContents();

    await testPage.reload();
    await testPage.waitForLoadState("networkidle");
    await testPage.keyboard.press(`${modifier}+Shift+q`);

    const restoredDialog = testPage.getByRole("dialog", { name: "Quick Chat" });
    const restoredTabs = restoredDialog.getByTestId("quick-chat-tab");
    await expect(restoredTabs).toHaveCount(2);
    await expect(restoredTabs.locator("span")).toHaveText([...originalNames].reverse());
    await expect(restoredDialog.getByTestId("quick-chat-setup")).not.toBeVisible();
    await assertNoHorizontalOverflow(testPage);
  });
});
