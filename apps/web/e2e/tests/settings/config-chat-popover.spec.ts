import { type Locator, type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";

async function openConfigChatFromSettings(page: Page): Promise<Locator> {
  const pageErrors: string[] = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));
  await page.goto("/settings/agents");
  await page.waitForLoadState("networkidle");
  expect(pageErrors, "settings page should render without client errors").toEqual([]);
  const fab = page.getByRole("button", { name: "Configuration Chat" });
  await expect(fab).toBeVisible({ timeout: 10_000 });
  await fab.click();
  const popover = page.getByTestId("config-chat-popover");
  await expect(popover).toBeVisible({ timeout: 10_000 });
  await expect(page.getByRole("dialog", { name: "Quick Chat" })).not.toBeVisible();
  return popover;
}

async function startConfigChat(dialog: Locator, prompt: string) {
  const setup = dialog.getByTestId("config-chat-setup");
  await expect(setup).toBeVisible({ timeout: 10_000 });
  await expect(setup.getByText(/repositories/i)).toHaveCount(0);
  const input = setup.getByPlaceholder("Ask anything about your configuration...");
  await input.fill(prompt);
  await setup.getByRole("button", { name: "Start configuration chat" }).click();
  await expect(setup).not.toBeVisible({ timeout: 15_000 });
  await expect(dialog.getByTestId("chat-input-editor")).toBeVisible({ timeout: 15_000 });
}

async function sendMessage(dialog: Locator, text: string) {
  const editor = dialog.getByTestId("chat-input-editor");
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 30_000 });
  await editor.fill(text);
  await editor.press(`${modifier}+Enter`);
}

test.describe("Configuration Chat", () => {
  test.beforeEach(async ({ apiClient, seedData }) => {
    await apiClient.updateWorkspace(seedData.workspaceId, {
      default_config_agent_profile_id: seedData.agentProfileId,
    });
  });

  test("keeps the floating Save action clear of the closed and open config chat", async ({
    testPage,
    apiClient,
  }) => {
    const initial = await apiClient.getUserSettings();
    const initialLayout = initial.settings.changes_panel_layout === "tree" ? "tree" : "flat";
    const nextLayout = initialLayout === "tree" ? "flat" : "tree";

    try {
      await testPage.goto("/settings/general/appearance");
      const layout = testPage.getByTestId("changes-panel-layout-select");
      await layout.click();
      await testPage
        .getByRole("option", { name: nextLayout === "tree" ? "Tree" : "Flat list" })
        .click();

      const floatingSave = testPage.getByTestId("settings-floating-save");
      const saveButton = floatingSave.getByRole("button", { name: "Save changes" });
      const configChatButton = testPage.getByRole("button", { name: "Configuration Chat" });
      await expect(saveButton).toHaveClass(/bg-success/);
      await expectElementsNotToIntersect(saveButton, configChatButton);

      await configChatButton.click();
      const configChatPopover = testPage.getByTestId("config-chat-popover");
      await expect(configChatPopover).toBeVisible();
      await expectElementAbove(saveButton, configChatPopover);
    } finally {
      await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
        changes_panel_layout: initialLayout,
      });
    }
  });

  test("starts floating, expands the same session, restores, continues, and deletes", async ({
    testPage,
  }) => {
    await testPage.setViewportSize({ width: 900, height: 520 });
    const popover = await openConfigChatFromSettings(testPage);
    const viewport = testPage.viewportSize();
    const box = await popover.boundingBox();
    expect(box).not.toBeNull();
    expect(viewport).not.toBeNull();
    expect(box!.width).toBeLessThan(viewport!.width * 0.7);
    const setup = popover.getByTestId("config-chat-setup");
    const input = setup.getByPlaceholder("Ask anything about your configuration...");
    const scrollRegion = setup.getByTestId("config-chat-guidance");
    await expect.poll(() => scrollRegion.evaluate((element) => element.scrollTop)).toBe(0);
    await expect(setup.getByRole("heading", { name: "Configuration Chat" })).toHaveCount(0);
    await expect(setup.getByRole("button", { name: "Cancel" })).toHaveCount(0);
    await expect(setup.locator("footer")).toHaveCount(0);
    const inputBox = await input.boundingBox();
    expect(inputBox).not.toBeNull();
    await expect(setup.getByRole("button", { name: "Start configuration chat" })).toBeInViewport({
      ratio: 1,
    });

    await startConfigChat(popover, "/e2e:simple-message");
    await expect(popover.getByTestId("quick-chat-tab")).toHaveCount(0);
    await expect(popover.getByRole("button", { name: "Start new configuration chat" })).toHaveCount(
      0,
    );
    await expect(
      popover.getByText("simple mock response for e2e testing", { exact: false }),
    ).toBeVisible({ timeout: 30_000 });
    await popover.getByRole("button", { name: "Open in Quick Chat" }).click();
    const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
    await expect(dialog).toBeVisible({ timeout: 10_000 });
    await expect(
      dialog.getByText("simple mock response for e2e testing", { exact: false }),
    ).toBeVisible();

    await testPage.reload();
    await testPage.waitForLoadState("networkidle");
    await expect(testPage.getByRole("dialog", { name: "Quick Chat" })).not.toBeVisible();
    await testPage.getByRole("button", { name: "Configuration Chat" }).click();
    const restored = testPage.getByTestId("config-chat-popover");
    await expect(restored.getByTestId("chat-input-editor")).toBeVisible({ timeout: 10_000 });
    await expect(
      restored.getByText("simple mock response for e2e testing", { exact: false }),
    ).toBeVisible({ timeout: 20_000 });

    await sendMessage(restored, 'e2e:message("continued config response")');
    await expect(restored.getByText("continued config response", { exact: true })).toBeVisible({
      timeout: 30_000,
    });

    await restored.getByRole("button", { name: "Open in Quick Chat" }).click();
    const restoredDialog = testPage.getByRole("dialog", { name: "Quick Chat" });
    await restoredDialog
      .getByTestId("quick-chat-tab")
      .getByRole("button", { name: /^Close / })
      .click();
    const deleteDialog = testPage.getByRole("alertdialog");
    await expect(deleteDialog).toContainText("Delete Quick Chat?");
    const deleteResponse = testPage.waitForResponse(
      (response) => response.request().method() === "DELETE" && response.ok(),
    );
    await deleteDialog.getByRole("button", { name: "Delete" }).click();
    await deleteResponse;
    await expect(restoredDialog).not.toBeVisible();

    await testPage.reload();
    await testPage.waitForLoadState("networkidle");
    await testPage.getByRole("button", { name: "Configuration Chat" }).click();
    await expect(
      testPage.getByTestId("config-chat-popover").getByTestId("config-chat-setup"),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("opens the same typed setup from the command palette", async ({ testPage }) => {
    await testPage.goto("/");
    await expect(testPage.getByTestId("kanban-board")).toBeVisible({ timeout: 15_000 });
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${modifier}+k`);
    const palette = testPage.getByRole("dialog", { name: "Command Palette" });
    await expect(palette).toBeVisible({ timeout: 5_000 });
    await palette.locator("input").fill("Configuration Chat");
    await palette.getByText("Configuration Chat", { exact: true }).click();

    const dialog = testPage.getByRole("dialog", { name: "Quick Chat" });
    await expect(dialog.getByTestId("config-chat-setup")).toBeVisible({ timeout: 10_000 });
    await expect(dialog.getByRole("img", { name: "Configuration chat" })).toBeVisible();
  });

  test("keeps conversation context visible around an inline clarification", async ({
    testPage,
  }) => {
    const popover = await openConfigChatFromSettings(testPage);
    await startConfigChat(popover, 'e2e:message("context before clarification")');
    const messageList = popover.locator(".chat-message-list");
    await expect(
      messageList.getByText("context before clarification", { exact: true }),
    ).toBeVisible({
      timeout: 30_000,
    });

    await sendMessage(popover, "/ask-single");
    const clarification = popover.getByTestId("clarification-overlay-container");
    await expect(clarification).toContainText("Which database", { timeout: 30_000 });
    const historyBox = await messageList.boundingBox();
    expect(historyBox).not.toBeNull();
    expect(historyBox!.height).toBeGreaterThan(40);
    await expect(
      messageList.getByText("context before clarification", { exact: true }),
    ).toBeVisible();

    await popover.getByRole("button", { name: "Collapse clarification" }).click();
    await expect(clarification.getByText("Which database", { exact: false })).not.toBeVisible();
    await popover.getByRole("button", { name: "Expand clarification" }).click();
    await expect(clarification.getByText("Which database", { exact: false })).toBeVisible();
    await clarification.getByText("PostgreSQL", { exact: true }).click();
    await expect(clarification).not.toBeVisible({ timeout: 30_000 });
  });
});

async function expectElementsNotToIntersect(
  first: import("@playwright/test").Locator,
  second: import("@playwright/test").Locator,
) {
  const [firstBox, secondBox] = await Promise.all([first.boundingBox(), second.boundingBox()]);
  expect(firstBox).not.toBeNull();
  expect(secondBox).not.toBeNull();
  const intersects =
    firstBox!.x < secondBox!.x + secondBox!.width &&
    firstBox!.x + firstBox!.width > secondBox!.x &&
    firstBox!.y < secondBox!.y + secondBox!.height &&
    firstBox!.y + firstBox!.height > secondBox!.y;
  expect(intersects).toBe(false);
}

async function expectElementAbove(
  upper: import("@playwright/test").Locator,
  lower: import("@playwright/test").Locator,
) {
  const [upperBox, lowerBox] = await Promise.all([upper.boundingBox(), lower.boundingBox()]);
  expect(upperBox).not.toBeNull();
  expect(lowerBox).not.toBeNull();
  expect(upperBox!.y + upperBox!.height).toBeLessThanOrEqual(lowerBox!.y);
}
