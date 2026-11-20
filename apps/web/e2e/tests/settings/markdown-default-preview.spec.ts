import { checkMarkdownOpening } from "../../helpers/markdown-default-preview";
import { expect, test } from "../../fixtures/test-base";

test("persists and resets the Markdown initial-mode preference", async ({
  testPage,
  apiClient,
}) => {
  const original = await apiClient.rawRequest("GET", "/api/v1/user/settings");
  const originalValue = (await original.json()).settings.markdown_preview_by_default;
  try {
    await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      markdown_preview_by_default: true,
    });
    await testPage.goto("/settings/preferences/terminal-editors");
    const toggle = testPage.getByRole("switch", {
      name: "Open Markdown files in Preview",
      exact: true,
    });
    await expect(toggle).toBeChecked();
    await expect(toggle).toHaveAccessibleDescription(
      "Choose the initial mode for newly opened Markdown files. You can switch modes at any time.",
    );
    await toggle.click();
    const save = testPage.getByTestId("settings-floating-save");
    await save.getByRole("button", { name: "Reset", exact: true }).click();
    await expect(toggle).toBeChecked();
    await toggle.click();
    await save.getByRole("button", { name: "Save changes", exact: true }).click();
    await expect(save).not.toBeVisible();
    await testPage.reload();
    await expect(toggle).not.toBeChecked();
    const saved = await apiClient.rawRequest("GET", "/api/v1/user/settings");
    expect((await saved.json()).settings.markdown_preview_by_default).toBe(false);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
  } finally {
    await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      markdown_preview_by_default: originalValue,
    });
  }
});

test("opens Markdown with both saved defaults and allows mode switching", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(120000);
  await checkMarkdownOpening({ testPage, apiClient, seedData, backend, mobile: false });
});
