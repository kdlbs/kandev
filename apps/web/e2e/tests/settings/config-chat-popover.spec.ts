import { type Locator, type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";

async function openConfigChatFromSettings(page: Page): Promise<Locator> {
  await page.goto("/settings/agents");
  await page.waitForLoadState("networkidle");
  const fab = page.getByRole("button", { name: "Configuration Chat" });
  await expect(fab).toBeVisible({ timeout: 10_000 });
  await fab.click();
  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  await expect(dialog).toBeVisible({ timeout: 10_000 });
  return dialog;
}

async function startConfigChat(dialog: Locator, prompt: string) {
  const setup = dialog.getByTestId("config-chat-setup");
  await expect(setup).toBeVisible({ timeout: 10_000 });
  await expect(
    setup.getByText(/manage workflows, agent profiles, and MCP configuration/i),
  ).toBeVisible();
  await expect(setup.getByText(/repositories/i)).toHaveCount(0);
  const input = setup.getByPlaceholder("Ask anything about your configuration...");
  await input.fill(prompt);
  await setup.getByRole("button", { name: "Start configuration chat" }).click();
  await expect(dialog.getByRole("img", { name: "Configuration chat" })).toBeVisible({
    timeout: 15_000,
  });
}

async function sendMessage(dialog: Locator, text: string) {
  const editor = dialog.getByTestId("chat-input-editor");
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 30_000 });
  await editor.fill(text);
  await editor.press(`${modifier}+Enter`);
}

test.describe("Configuration Chat in Quick Chat", () => {
  test.beforeEach(async ({ apiClient, seedData }) => {
    await apiClient.updateWorkspace(seedData.workspaceId, {
      default_config_agent_profile_id: seedData.agentProfileId,
    });
  });

  test("launches in the large modal, restores after refresh, continues, and deletes", async ({
    testPage,
  }) => {
    const dialog = await openConfigChatFromSettings(testPage);
    const viewport = testPage.viewportSize();
    const box = await dialog.boundingBox();
    expect(box).not.toBeNull();
    expect(viewport).not.toBeNull();
    expect(box!.width).toBeGreaterThan(viewport!.width * 0.7);
    expect(box!.height).toBeGreaterThan(viewport!.height * 0.75);

    await startConfigChat(dialog, "/e2e:simple-message");
    await expect(
      dialog.getByText("simple mock response for e2e testing", { exact: false }),
    ).toBeVisible({ timeout: 30_000 });

    await testPage.reload();
    await testPage.waitForLoadState("networkidle");
    await expect(testPage.getByRole("dialog", { name: "Quick Chat" })).not.toBeVisible();
    await testPage.getByRole("button", { name: "Configuration Chat" }).click();
    const restored = testPage.getByRole("dialog", { name: "Quick Chat" });
    await expect(restored.getByRole("img", { name: "Configuration chat" })).toBeVisible({
      timeout: 10_000,
    });
    await expect(
      restored.getByText("simple mock response for e2e testing", { exact: false }),
    ).toBeVisible({ timeout: 20_000 });

    await sendMessage(restored, 'e2e:message("continued config response")');
    await expect(restored.getByText("continued config response", { exact: true })).toBeVisible({
      timeout: 30_000,
    });

    await restored.locator("button[aria-label^='Close ']").first().click();
    const deleteDialog = testPage.getByRole("alertdialog");
    await expect(deleteDialog).toContainText("Delete Quick Chat?");
    const deleteResponse = testPage.waitForResponse(
      (response) => response.request().method() === "DELETE" && response.ok(),
    );
    await deleteDialog.getByRole("button", { name: "Delete" }).click();
    await deleteResponse;
    await expect(restored).not.toBeVisible();

    await testPage.reload();
    await testPage.waitForLoadState("networkidle");
    await testPage.getByRole("button", { name: "Configuration Chat" }).click();
    await expect(
      testPage.getByRole("dialog", { name: "Quick Chat" }).getByTestId("config-chat-setup"),
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
    const dialog = await openConfigChatFromSettings(testPage);
    await startConfigChat(dialog, 'e2e:message("context before clarification")');
    const messageList = dialog.locator(".chat-message-list");
    await expect(
      messageList.getByText("context before clarification", { exact: true }),
    ).toBeVisible({
      timeout: 30_000,
    });

    await sendMessage(dialog, "/ask-single");
    const clarification = dialog.getByTestId("clarification-overlay-container");
    await expect(clarification).toContainText("Which database", { timeout: 30_000 });
    const historyBox = await messageList.boundingBox();
    expect(historyBox).not.toBeNull();
    expect(historyBox!.height).toBeGreaterThan(100);
    await expect(
      messageList.getByText("context before clarification", { exact: true }),
    ).toBeVisible();

    await dialog.getByRole("button", { name: "Collapse clarification" }).click();
    await expect(clarification.getByText("Which database", { exact: false })).not.toBeVisible();
    await dialog.getByRole("button", { name: "Expand clarification" }).click();
    await expect(clarification.getByText("Which database", { exact: false })).toBeVisible();
    await clarification.getByText("PostgreSQL", { exact: true }).click();
    await expect(clarification).not.toBeVisible({ timeout: 30_000 });
  });
});
