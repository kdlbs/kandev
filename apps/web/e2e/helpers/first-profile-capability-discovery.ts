import { expect, type Page, type Request, type Response } from "@playwright/test";

import type { ApiClient } from "./api-client";
import type { BackendContext } from "../fixtures/backend";
import { dwell } from "./causal-waits";

export type DiscoveryRequestEvidence = {
  url: string;
  body: Record<string, unknown>;
  status?: number;
  failure?: string;
};

const AMP_AGENT_ID = "amp-acp";

/**
 * The worker must start without an Amp installation. Fail before mutating a
 * pre-existing one instead of deleting user/worker data that this test does not
 * own.
 */
export async function expectAmpBaselineAbsent(apiClient: ApiClient): Promise<void> {
  expect(
    await ampAgentRow(apiClient),
    "a pre-existing Amp installation is present; use a dedicated worker instead of deleting it",
  ).toBeUndefined();
}

/** Enables the disposable Amp mock provider for one worker restart. */
export async function enableAmpMockProvider(backend: BackendContext): Promise<void> {
  await backend.restart({ KANDEV_MOCK_PROVIDERS: AMP_AGENT_ID });
}

async function ampAgentRow(apiClient: ApiClient) {
  const { agents } = await apiClient.listAgents();
  return agents.find((agent) => agent.name === AMP_AGENT_ID);
}

/**
 * Removes only the Amp saved parent/profile rows while retaining the registry
 * entry. The boot profile reconciler may seed a default profile for a newly
 * available agent, so delete until the row stays absent across a settled read.
 */
export async function removeDisposableAmpRows(apiClient: ApiClient): Promise<void> {
  for (let attempt = 0; attempt < 12; attempt += 1) {
    const agent = await ampAgentRow(apiClient);
    if (agent) {
      await apiClient.rawRequest("DELETE", `/api/v1/agents/${agent.id}`);
      continue;
    }
    await dwell(
      500,
      "negative-assertion",
      "boot profile reconciliation may reseed the disposable identity",
    );
    if (!(await ampAgentRow(apiClient))) return;
  }
  expect(await ampAgentRow(apiClient)).toBeUndefined();
}

/** Requires Amp to be available for a first-profile draft while unsaved. */
export async function expectAmpAvailableWithoutSavedRow(
  apiClient: ApiClient,
  agentName = AMP_AGENT_ID,
): Promise<void> {
  const { agents } = await apiClient.listAvailableAgents();
  expect(agents.some((agent) => agent.name === agentName)).toBe(true);
  const { agents: saved } = await apiClient.listAgents();
  expect(saved.some((agent) => agent.name === agentName)).toBe(false);
}

/** Records real probe/resolve traffic without fulfilling any response. */
export function observeProfileDiscoveryRequests(page: Page, agentName = AMP_AGENT_ID) {
  const requests: DiscoveryRequestEvidence[] = [];
  const pending = new Map<Request, DiscoveryRequestEvidence>();
  const record = (request: Request) => {
    if (!request.url().includes(`/api/v1/agent-models/${agentName}/`)) return;
    const body = request.postDataJSON() as Record<string, unknown> | null;
    const evidence: DiscoveryRequestEvidence = { url: request.url(), body: body ?? {} };
    requests.push(evidence);
    pending.set(request, evidence);
  };
  const settle = (request: Request, status: number) => {
    const evidence = pending.get(request);
    if (evidence) evidence.status = status;
  };
  const fail = (request: Request) => {
    const evidence = pending.get(request);
    if (evidence) evidence.failure = request.failure()?.errorText ?? "request failed";
  };
  const onResponse = (response: Response) => settle(response.request(), response.status());
  page.on("request", record);
  page.on("response", onResponse);
  page.on("requestfailed", fail);
  return {
    requests,
    dispose: () => {
      page.off("request", record);
      page.off("response", onResponse);
      page.off("requestfailed", fail);
    },
  };
}

/** Requires the complete draft snapshot and omitted profile id on both calls. */
export function expectFirstDraftRequestEvidence(requests: DiscoveryRequestEvidence[]): void {
  const probe = requests.find((request) => request.url.endsWith("/probe"));
  const resolve = requests.find((request) => request.url.endsWith("/resolve"));
  expect(probe, "the editor must probe provider capabilities").toBeDefined();
  expect(resolve, "the editor must resolve model-dependent options").toBeDefined();
  for (const request of [probe, resolve]) {
    expect(request?.body.profile_id ?? "").toBe("");
    expect(request?.body.launch_settings).toMatchObject({
      env_vars: expect.any(Array),
      cli_flags: expect.any(Array),
      command_prefix: expect.any(String),
    });
  }
  expect(probe?.status).toBe(200);
  expect(resolve?.status).toBe(200);
}

/** Opens the create-profile page for an agent with no saved parent row. */
export async function openFirstProfileDraft(page: Page, agentName = AMP_AGENT_ID): Promise<void> {
  await page.goto(`/settings/agents/${agentName}?mode=create`);
  await expect(page.getByTestId("profile-name-input")).toBeVisible({ timeout: 20_000 });
}

/** Waits for the first-draft capability discovery to settle successfully. */
export async function expectReadyDiscovery(page: Page): Promise<void> {
  await expect(page.getByTestId("profile-capability-status")).toHaveAttribute(
    "data-status",
    "ready",
    { timeout: 30_000 },
  );
}

/** Chooses the first model advertised by the draft probe. */
export async function chooseAdvertisedModel(page: Page): Promise<string> {
  const selector = page.getByRole("button", { name: "Profile start model settings" });
  await expect(selector).toBeVisible({ timeout: 20_000 });
  await selector.click();
  // Desktop opens the existing popover; phones open the shared sheet.
  await expect(
    page
      .locator('[data-slot="popover-content"]:visible, [data-testid="model-config-sheet-scroll"]')
      .first(),
  ).toBeVisible({ timeout: 20_000 });
  const sheet = page.getByTestId("model-config-sheet-scroll");
  const surface =
    (await sheet.count()) > 0 ? sheet : page.locator('[data-slot="popover-content"]:visible');
  const options = surface.getByRole("option");
  await expect(options.first()).toBeVisible({ timeout: 20_000 });
  const count = await options.count();
  const target = count > 1 ? options.nth(1) : options.first();
  const value = (await target.getAttribute("data-value")) ?? "";
  const name = (await target.getAttribute("aria-label")) ?? (await target.textContent()) ?? "";
  await target.click();
  await dismissModelPicker(page);
  return (value || name).trim();
}

/** Closes whichever picker surface the viewport opened. */
export async function dismissModelPicker(page: Page): Promise<void> {
  const sheetClose = page.getByTestId("model-config-sheet-close");
  if (await sheetClose.isVisible().catch(() => false)) {
    await sheetClose.click();
    await expect(page.getByTestId("model-config-sheet-scroll")).toHaveCount(0);
    return;
  }
  await page.keyboard.press("Escape");
  await expect(page.locator('[data-slot="popover-content"]:visible')).toHaveCount(0);
}

/** Saves the open settings page through the shared floating action. */
export async function saveSettingsPage(page: Page): Promise<void> {
  // Create mode renders "Save"; a dirty saved profile renders "Save changes".
  const saved = page.getByRole("button", { name: /^Save( changes)?$/i }).first();
  await expect(saved).toBeEnabled({ timeout: 20_000 });
  await saved.click();
}

/** Persisted parent and profile model for the first save. */
export async function expectPersistedFirstProfile(
  apiClient: ApiClient,
  profileName: string,
  expectedModel: string,
  agentName = AMP_AGENT_ID,
): Promise<{ agentId: string; profileId: string; model: string }> {
  await expect
    .poll(
      async () => {
        const { agents } = await apiClient.listAgents();
        return agents
          .find((item) => item.name === agentName)
          ?.profiles.find((item) => item.name === profileName)?.model;
      },
      { timeout: 20_000 },
    )
    .toBe(expectedModel);
  const { agents } = await apiClient.listAgents();
  const agent = agents.find((item) => item.name === agentName);
  const profile = agent?.profiles.find((item) => item.name === profileName);
  if (!agent || !profile) throw new Error("persisted first profile missing");
  return { agentId: agent.id, profileId: profile.id, model: profile.model ?? "" };
}

/** Removes only the owned Amp parent after a test, then restarts the worker. */
export async function cleanupFirstProfile(
  apiClient: ApiClient,
  backend: BackendContext,
): Promise<void> {
  try {
    await removeDisposableAmpRows(apiClient);
  } finally {
    await backend.restart();
  }
}
