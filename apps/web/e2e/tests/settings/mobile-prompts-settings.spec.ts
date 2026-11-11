import { test, expect } from "../../fixtures/test-base";
import { callPromptTool } from "../../helpers/shared-prompt-mcp";
import { promptEditorText, replacePromptEditor } from "../../helpers/settings-prompt-editor";

test.use({ trace: "retain-on-failure" });

test.afterEach(async ({ apiClient }) => {
  const { prompts } = await apiClient.listPrompts();
  for (const prompt of prompts) {
    if (!prompt.builtin) {
      await apiClient.deletePrompt(prompt.id).catch(() => undefined);
    }
  }
});

test.describe("Prompts settings on a phone", () => {
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

test("edits a custom prompt on a phone and accepts a later MCP update", async ({
  testPage,
  apiClient,
  backend,
  prCapture,
}) => {
  await apiClient.createPrompt("mobile-review-policy", "Check correctness and tests.");
  await testPage.goto("/settings/prompts");
  const row = testPage.locator(
    '[data-testid="prompt-list-item"][data-prompt-name="mobile-review-policy"]',
  );
  await row.getByTestId("prompt-edit-button").tap();
  await expect(row.getByRole("switch", { name: "Allow agent edits" })).toHaveCount(0);
  await expect(row.getByText("Allow agent edits", { exact: true })).toHaveCount(0);
  await replacePromptEditor(testPage, row.getByTestId("prompt-content-input"), "Saved from phone");
  const saveButton = testPage
    .getByTestId("settings-floating-save")
    .getByRole("button", { name: "Save changes" });
  await expect(saveButton).toBeVisible();
  expect((await saveButton.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await prCapture.screenshot("shared-prompt-editing-mobile");
  await testPage
    .getByTestId("settings-floating-save")
    .getByRole("button", { name: "Save changes" })
    .tap();
  await expect(row).toContainText("Saved from phone");
  await row.getByTestId("prompt-edit-button").tap();
  await expect(promptEditorText(row.getByTestId("prompt-content-input"))).toContainText(
    "Saved from phone",
  );
  await row.getByRole("button", { name: "Cancel" }).tap();
  const updated = await callPromptTool(backend.baseUrl, "update_shared_prompt_kandev", {
    name: "mobile-review-policy",
    content: "Updated through MCP.",
  });
  expect(updated.isError).not.toBe(true);
  await expect(row).toContainText("Updated through MCP.");
  expect(
    await testPage.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
  ).toBe(false);
});
