import { expect, test } from "../../fixtures/test-base";

const PATH = "/settings/preferences/task-behavior?tab=conversation";

// @covers AC-UI-PROMPT-SUGGEST-001.1 AC-UI-PROMPT-SUGGEST-001.3 AC-UI-PROMPT-SUGGEST-001.4
test("the prompt suggestion switch persists and explains native-only support", async ({
  testPage,
  apiClient,
}) => {
  try {
    await testPage.goto(PATH);
    const card = testPage.getByTestId("task-behavior-settings");
    const main = card.locator("#prompt-suggestions");
    await expect(main).toHaveAttribute("aria-checked", "false");
    await expect(card.locator("#prompt-suggestions-fallback")).toHaveCount(0);

    await card.getByRole("button", { name: "About Suggest next prompt" }).hover();
    await expect(testPage.getByRole("tooltip")).toContainText("native suggestions");
    await testPage.mouse.move(0, 0, { steps: 5 });

    await main.click();
    await expect(card.locator("#prompt-suggestions-fallback")).toHaveCount(0);
    await expect(card.getByTestId("prompt-suggestion-profile-picker")).toHaveCount(0);

    const floatingSave = testPage.getByTestId("settings-floating-save");
    await floatingSave.getByRole("button", { name: "Save changes" }).click();
    await expect(floatingSave).not.toBeVisible({ timeout: 10_000 });

    await expect
      .poll(async () => {
        const response = await apiClient.rawRequest("GET", "/api/v1/user/settings");
        const body = await response.json();
        return body.settings.prompt_suggestions;
      })
      .toBe(true);

    await testPage.reload();
    await expect(card.locator("#prompt-suggestions")).toHaveAttribute("aria-checked", "true");
  } finally {
    await apiClient.rawRequest("PATCH", "/api/v1/user/settings", { prompt_suggestions: false });
  }
});
