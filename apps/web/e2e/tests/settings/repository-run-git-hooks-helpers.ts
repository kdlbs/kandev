import { expect, type Page } from "@playwright/test";

export async function openRepositoryCard(page: Page, workspaceId: string) {
  await page.goto(`/settings/workspace/${workspaceId}/repositories`);
  await page.getByRole("heading", { name: "E2E Repo", exact: true }).click();
  const field = page.getByTestId("repository-run-git-hooks");
  await field.scrollIntoViewIfNeeded();
  await expect(field).toBeVisible();
  return field.getByTestId("repository-run-git-hooks-switch");
}

export async function saveSettings(page: Page) {
  const save = page
    .getByTestId("settings-floating-save")
    .getByRole("button", { name: "Save changes" });
  await expect(save).toBeEnabled();
  await save.click();
  await expect(save).toBeHidden();
}
