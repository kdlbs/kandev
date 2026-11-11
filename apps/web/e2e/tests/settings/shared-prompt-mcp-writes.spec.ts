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

test("custom prompts accept requested MCP edits while preserving editor drafts", async ({
  testPage,
  apiClient,
  backend,
  prCapture,
}) => {
  await apiClient.createPrompt("review-policy", "Review changes carefully.");
  const builtIn = (await apiClient.listPrompts()).prompts.find(
    (prompt) => prompt.name === "code-review" && prompt.builtin,
  );
  expect(builtIn).toBeDefined();
  await testPage.goto("/settings/prompts");
  const row = testPage.locator(
    '[data-testid="prompt-list-item"][data-prompt-name="review-policy"]',
  );
  await row.getByTestId("prompt-edit-button").click();
  await expect(row.getByRole("switch", { name: "Allow agent edits" })).toHaveCount(0);
  await expect(row.getByText("Allow agent edits", { exact: true })).toHaveCount(0);
  const updated = await callPromptTool(backend.baseUrl, "update_shared_prompt_kandev", {
    name: "review-policy",
    content: "Check correctness and tests.",
  });
  expect(updated.isError).not.toBe(true);
  expect(
    (await apiClient.listPrompts()).prompts.find((prompt) => prompt.name === "review-policy")
      ?.content,
  ).toBe("Check correctness and tests.");
  await row.getByRole("button", { name: "Cancel" }).click();
  await expect(row).toContainText("Check correctness and tests.");

  const deniedBuiltIn = await callPromptTool(backend.baseUrl, "update_shared_prompt_kandev", {
    name: "code-review",
    content: "Do not save this built-in change.",
  });
  expect(deniedBuiltIn.isError).toBe(true);
  expect(
    (await apiClient.listPrompts()).prompts.find((prompt) => prompt.id === builtIn!.id)?.content,
  ).toBe(builtIn!.content);

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
  await prCapture.screenshot("shared-prompt-editing-desktop");
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
});

test("legacy HTTP permission input cannot disable custom prompt edits", async ({
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
  const legacyPatch = await testPage.request.patch(
    `${backend.baseUrl}/api/v1/prompts/${prompt.id}`,
    {
      data: { allow_agent_edits: false },
    },
  );
  expect(legacyPatch.ok()).toBe(true);
  const legacyResponse = await legacyPatch.json();
  expect(legacyResponse.allow_agent_edits).toBe(true);
  expect(legacyResponse.content).toBe("Original policy");
  await expect(row.getByRole("switch", { name: "Allow agent edits" })).toHaveCount(0);
  const updated = await callPromptTool(backend.baseUrl, "update_shared_prompt_kandev", {
    name: prompt.name,
    content: "Eligible after legacy input.",
  });
  expect(updated.isError).not.toBe(true);
  expect(
    (await apiClient.listPrompts()).prompts.find((item) => item.id === prompt.id)?.content,
  ).toBe("Eligible after legacy input.");
});
