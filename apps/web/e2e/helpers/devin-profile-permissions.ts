import { expect, type Page } from "@playwright/test";

import type { ApiClient } from "./api-client";
import type { BackendContext } from "../fixtures/backend";
import { dwell } from "./causal-waits";

export const DEVIN_AGENT_ID = "devin-acp";
const DEVIN_SANDBOX_ENV_KEY = "DEVIN_SANDBOX";

/** Registers the disposable Devin settings identity for one worker restart. */
export async function enableDevinMockProvider(backend: BackendContext): Promise<void> {
  await backend.restart({ KANDEV_E2E_MOCK_DEVIN: "true" });
}

export async function devinAgent(apiClient: ApiClient) {
  const { agents } = await apiClient.listAgents();
  return agents.find((agent) => agent.name === DEVIN_AGENT_ID);
}

export async function removeDisposableDevinRows(apiClient: ApiClient): Promise<void> {
  for (let attempt = 0; attempt < 12; attempt += 1) {
    const agent = await devinAgent(apiClient);
    if (agent) {
      await apiClient.rawRequest("DELETE", `/api/v1/agents/${agent.id}`);
      continue;
    }
    await dwell(
      500,
      "negative-assertion",
      "boot profile reconciliation may reseed the disposable identity",
    );
    if (!(await devinAgent(apiClient))) return;
  }
  expect(await devinAgent(apiClient)).toBeUndefined();
}

/** Opens the create-profile page for the disposable Devin identity. */
export async function openDevinDraft(page: Page): Promise<void> {
  await page.goto(`/settings/agents/${DEVIN_AGENT_ID}?mode=create`);
  await expect(page.getByTestId("profile-name-input")).toBeVisible({ timeout: 20_000 });
  await expect(page.getByTestId("devin-sandbox-preference")).toBeVisible({ timeout: 20_000 });
}

/** Sandbox preference rendered by the shared select control. */
export function sandboxControl(page: Page) {
  return page.getByRole("combobox", { name: /Devin sandbox/ });
}

export async function chooseSandbox(
  page: Page,
  label: "Use CLI settings" | "Disabled" | "Enabled",
) {
  await sandboxControl(page).click();
  await page.getByRole("option", { name: label }).click();
}

/** Persisted Devin values for the first saved profile. */
export async function expectPersistedDevinProfile(
  apiClient: ApiClient,
): Promise<{ model: string; mode: string; sandbox?: string }> {
  await expect
    .poll(async () => (await devinAgent(apiClient))?.profiles.length ?? 0, { timeout: 20_000 })
    .toBeGreaterThan(0);
  const agent = await devinAgent(apiClient);
  const profile = agent?.profiles[0];
  if (!profile) throw new Error("Devin profile missing after save");
  return {
    model: profile.model,
    mode: profile.mode ?? "",
    sandbox: profile.envVars?.find((entry) => entry.key === DEVIN_SANDBOX_ENV_KEY)?.value,
  };
}

/** Deletes only the owned Devin rows and restores the worker environment. */
export async function cleanupDevinProfile(apiClient: ApiClient, backend: BackendContext) {
  try {
    await removeDisposableDevinRows(apiClient);
  } finally {
    await backend.restart();
  }
}
