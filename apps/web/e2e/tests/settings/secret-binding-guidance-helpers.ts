import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";

export type SecretGuidanceScope = "global" | "workspace";

export async function expectSecretBindingGuidance(page: Page, scope: SecretGuidanceScope) {
  const description = page
    .getByTestId("secrets-settings-body")
    .getByText(/Saving a secret stores it but does not add it to a session/i);

  await expect(description).toBeVisible();
  if (scope === "global") {
    await expect(description).toContainText(/encrypted at rest/i);
    await expect(description).toContainText(/agent profile/i);
    await expect(description).toContainText(/executor profile/i);
    await expect(description).toContainText(/repository environment/i);
    return description;
  }

  await expect(description).toContainText(/repository in this workspace/i);
  await expect(description).toContainText(/shared profiles cannot use Workspace secrets/i);
  return description;
}

export async function createSecretFromSettings(
  page: Page,
  name: string,
  value: string,
): Promise<void> {
  await page.getByRole("button", { name: "Add secret", exact: true }).click();
  await page.getByPlaceholder("Name (e.g. OpenAI Production Key)").fill(name);
  await page.getByPlaceholder("Secret value").fill(value);

  const save = page
    .getByTestId("settings-floating-save")
    .getByRole("button", { name: "Save changes" });
  await expect(save).toBeEnabled();
  await save.click();
  await expect(page.getByText(name, { exact: true })).toBeVisible();
  await expect(page.locator("body")).not.toContainText(value);
}

export async function deleteSecretByName(
  apiClient: ApiClient,
  scope: SecretGuidanceScope,
  name: string,
  workspaceId?: string,
): Promise<void> {
  const secrets = await apiClient.listSecrets({ scope, workspaceId });
  const matches = secrets.filter((secret) => secret.name === name);
  for (const secret of matches) {
    await apiClient.deleteSecretIfPresent(
      secret.id,
      scope === "workspace" ? workspaceId : undefined,
    );
  }
}
