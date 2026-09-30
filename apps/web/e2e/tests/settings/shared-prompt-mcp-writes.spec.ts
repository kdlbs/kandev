import { test, expect } from "../../fixtures/test-base";
import { callPromptTool } from "../../helpers/shared-prompt-mcp";
import { replacePromptEditor, promptEditorText } from "../../helpers/settings-prompt-editor";

test.afterEach(async ({ apiClient }) => {
  const names = ["review-policy", "release-policy", "concurrent-review"];
  const { prompts } = await apiClient.listPrompts();
  for (const prompt of prompts) {
    if (names.includes(prompt.name)) await apiClient.deletePrompt(prompt.id);
  }
});

test("operator controls shared prompt agent edits and MCP changes appear live", async ({
  testPage,
  apiClient,
  backend,
  prCapture,
}) => {
  await apiClient.createPrompt("review-policy", "Review changes carefully.");
  await testPage.goto("/settings/prompts");
  const row = testPage.locator(
    '[data-testid="prompt-list-item"][data-prompt-name="review-policy"]',
  );
  await row.getByTestId("prompt-edit-button").click();
  const toggle = row.getByRole("switch", { name: "Allow agent edits" });
  await expect(toggle).not.toBeChecked();
  const denied = await callPromptTool(backend.baseUrl, "update_shared_prompt_kandev", {
    name: "review-policy",
    content: "Not yet",
  });
  expect(denied.isError).toBe(true);
  await toggle.click();
  await prCapture.screenshot("shared-prompt-agent-permission-desktop");
  await testPage
    .getByTestId("settings-floating-save")
    .getByRole("button", { name: "Save changes" })
    .click();
  await expect(toggle).toHaveCount(0);
  const updated = await callPromptTool(backend.baseUrl, "update_shared_prompt_kandev", {
    name: "review-policy",
    content: "Check correctness and tests.",
  });
  expect(updated.isError).not.toBe(true);
  await expect(row).toContainText("Check correctness and tests.");
  const created = await callPromptTool(backend.baseUrl, "create_shared_prompt_kandev", {
    name: "release-policy",
    content: "Run required checks.",
  });
  expect(created.isError).not.toBe(true);
  await expect(testPage.locator('[data-prompt-name="release-policy"]')).toContainText(
    "Run required checks.",
  );
  await row.getByTestId("prompt-edit-button").click();
  await replacePromptEditor(
    testPage,
    row.getByTestId("prompt-content-input"),
    "Unsaved operator draft",
  );
  expect(
    (
      await callPromptTool(backend.baseUrl, "update_shared_prompt_kandev", {
        name: "review-policy",
        content: "Remote revision",
      })
    ).isError,
  ).not.toBe(true);
  await expect(promptEditorText(row.getByTestId("prompt-content-input"))).toContainText(
    "Unsaved operator draft",
  );
  await row.getByRole("button", { name: "Cancel" }).click();
  await expect(row).toContainText("Remote revision");
  await row.getByTestId("prompt-edit-button").click();
  await toggle.click();
  await testPage
    .getByTestId("settings-floating-save")
    .getByRole("button", { name: "Save changes" })
    .click();
  await expect(toggle).toHaveCount(0);
  expect(
    (
      await callPromptTool(backend.baseUrl, "update_shared_prompt_kandev", {
        name: "review-policy",
        content: "Blocked again",
      })
    ).isError,
  ).toBe(true);
  await expect(row).toContainText("Remote revision");
});

test("content saves keep permission revoked by another operator", async ({
  testPage,
  apiClient,
  backend,
}) => {
  await callPromptTool(backend.baseUrl, "create_shared_prompt_kandev", {
    name: "concurrent-review",
    content: "Original policy",
  });
  const prompt = (await apiClient.listPrompts()).prompts.find(
    (item) => item.name === "concurrent-review",
  )!;
  await testPage.goto("/settings/prompts");
  const row = testPage.locator('[data-prompt-name="concurrent-review"]');
  await row.getByTestId("prompt-edit-button").click();
  const toggle = row.getByRole("switch", { name: "Allow agent edits" });
  await expect(toggle).toBeChecked();
  await replacePromptEditor(
    testPage,
    row.getByTestId("prompt-content-input"),
    "Local operator draft",
  );
  const revoke = await testPage.request.patch(`${backend.baseUrl}/api/v1/prompts/${prompt.id}`, {
    data: { allow_agent_edits: false },
  });
  expect(revoke.ok()).toBe(true);
  await expect(toggle).not.toBeChecked();
  await expect(promptEditorText(row.getByTestId("prompt-content-input"))).toContainText(
    "Local operator draft",
  );
  const saveRequest = testPage.waitForRequest(
    (request) => request.method() === "PATCH" && request.url().endsWith(`/prompts/${prompt.id}`),
  );
  await testPage
    .getByTestId("settings-floating-save")
    .getByRole("button", { name: "Save changes" })
    .click();
  expect((await saveRequest).postDataJSON()).not.toHaveProperty("allow_agent_edits");
  await expect(toggle).toHaveCount(0);
  const saved = (await apiClient.listPrompts()).prompts.find((item) => item.id === prompt.id)!;
  expect(saved.content).toBe("Local operator draft");
  expect(
    (
      await callPromptTool(backend.baseUrl, "update_shared_prompt_kandev", {
        name: prompt.name,
        content: "Still forbidden",
      })
    ).isError,
  ).toBe(true);
});
