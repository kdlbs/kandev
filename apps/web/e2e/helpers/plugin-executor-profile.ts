import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";
import { waitForHttp } from "./causal-waits";
import { installFixturePlugin, PLUGIN_ID } from "./plugin-fixture";

export const FIXTURE_EXECUTOR_PROVIDER_KEY = "remote-sandbox";
export const FIXTURE_PROFILE_SECRET = "fixture-profile-secret-never-return-this";

export type FixtureExecutor = {
  id: string;
  name: string;
  type: string;
  provider?: { plugin_id?: string; key?: string };
};

export async function installFixtureExecutorProvider(page: Page, apiClient: ApiClient) {
  await installFixturePlugin(page);
  let executor: FixtureExecutor | undefined;
  await expect
    .poll(async () => {
      const response = await apiClient.listExecutors();
      executor = response.executors.find(
        (item) =>
          item.provider?.plugin_id === PLUGIN_ID &&
          item.provider.key === FIXTURE_EXECUTOR_PROVIDER_KEY,
      );
      return executor?.id ?? null;
    })
    .not.toBeNull();
  if (!executor) throw new Error("The packaged fixture must register its remote executor provider");
  return executor;
}

export async function createFixtureExecutorProfile(
  page: Page,
  executor: FixtureExecutor,
  touch: boolean,
) {
  const card = page.getByTestId(`executor-profiles-card-${executor.id}`);
  await expect(card).toBeVisible();
  const add = card.getByRole("button", { name: "Add" });
  if (touch) await add.tap();
  else await add.click();

  const form = page.getByTestId("plugin-executor-profile-create-form");
  await expect(form).toBeVisible();
  await page.locator("#plugin-executor-profile-name").fill("E2E remote sandbox profile");
  const create = page.getByRole("button", { name: "Create Profile" });
  if (touch) await create.tap();
  else await create.click();
  await expect(form.getByRole("alert")).toContainText("Region");

  const region = page.locator("#executor-profile-region");
  if (touch) await region.tap();
  else await region.click();
  const regionOption = page.getByRole("option", { name: "eu-west-1" });
  if (touch) await regionOption.tap();
  else await regionOption.click();
  await page.getByLabel("Credential").fill(FIXTURE_PROFILE_SECRET);
  await page.getByLabel(/Environment label with additional context/).fill("fixture-workspace");
  await page.getByLabel("Image").fill("fixture-agent:stable");

  const created = waitForHttp(
    page,
    "POST",
    new RegExp(`/api/v1/executors/${executor.id}/profiles$`),
    { predicate: (response) => response.ok() },
  );
  if (touch) await create.tap();
  else await create.click();
  const response = await created;
  const profile = (await response.json()) as { id: string; name: string };
  await expect(page).toHaveURL(`/settings/executors/${encodeURIComponent(profile.id)}`);
  return profile;
}

export async function readFixtureProfile(
  apiClient: ApiClient,
  executorId: string,
  profileId: string,
) {
  const response = await apiClient.rawRequest(
    "GET",
    `/api/v1/executors/${executorId}/profiles/${profileId}`,
  );
  expect(response.ok).toBe(true);
  return response.json() as Promise<{
    id: string;
    config?: Record<string, string>;
    secret_fields?: Record<string, boolean>;
  }>;
}
