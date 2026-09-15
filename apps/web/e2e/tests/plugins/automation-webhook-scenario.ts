import { expect, type Page } from "@playwright/test";
import { AutomationsPage } from "../../pages/automations-page";
import { createHmac } from "node:crypto";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";

export async function selectPluginCondition(page: Page, seed: SeedData, apiClient: ApiClient) {
  await installFixturePlugin(page);
  const automations = new AutomationsPage(page, seed.workspaceId);
  await automations.gotoNew();
  await automations.addConditionButton.click();
  await page.getByRole("option", { name: "Fixture signed event" }).click();
  const card = page.getByTestId("trigger-card-plugin_event");
  await expect(card).toBeVisible();
  await card.getByRole("button", { name: "signed-event", exact: true }).click();
  await expect(card.getByText("Fixture signed event", { exact: true })).toBeVisible();
  await expect(card.getByRole("button", { name: "Configure webhook" })).toBeDisabled();
  await expect(page.getByTestId("schedule-frequency")).toHaveCount(0);
  await expect(card).toBeInViewport();
}

export async function exerciseSavedWebhook(page: Page, seed: SeedData, apiClient: ApiClient) {
  const automation = await apiClient.wsRequest<{ id: string }>("automation.create", {
    workspace_id: seed.workspaceId,
    name: "Signed fixture automation",
    agent_profile_id: seed.agentProfileId,
    prompt: "Summarize this event: {{webhook.body}}",
    repository_mode: "none",
    triggers: [
      {
        type: "plugin_event",
        enabled: true,
        config: {
          plugin_id: PLUGIN_ID,
          condition_key: "signed-event",
          config_version: 1,
          settings: {},
        },
      },
    ],
  });
  page.on("dialog", (dialog) => dialog.accept());
  await page.goto(`/settings/workspaces/${seed.workspaceId}/automations/${automation.id}`);
  const card = page.getByTestId("trigger-card-plugin_event");
  await card.getByRole("button", { name: "signed-event", exact: true }).click();
  await card.getByRole("button", { name: "Configure webhook", exact: true }).click();
  await expect(card.getByLabel("Webhook URL", { exact: true })).toBeVisible();
  const url = await card.getByLabel("Webhook URL", { exact: true }).inputValue();
  await card.getByRole("button", { name: "Reveal secret", exact: true }).click();
  const secret = await card.getByLabel("Signing secret", { exact: true }).inputValue();
  const body = '{ "message": "こんにちは" }';
  const signature = "sha256=" + createHmac("sha256", secret).update(body).digest("hex");
  expect((await page.request.post(url, { data: body })).status()).toBe(401);
  expect(
    (
      await page.request.post(url, { data: body, headers: { "x-fixture-signature": signature } })
    ).status(),
  ).toBe(202);
  expect(
    (
      await page.request.post(url, { data: body, headers: { "x-fixture-signature": signature } })
    ).status(),
  ).toBe(200);
  await expect
    .poll(
      async () => {
        const rows = await apiClient.wsRequest<
          Array<{ state: string; run_id?: string; task_id?: string }>
        >("automation.webhook_binding", { automation_id: automation.id, operation: "receipts" });
        return (
          rows.length === 1 &&
          Boolean(rows[0].run_id) &&
          rows[0].state === "admitted" &&
          Boolean(rows[0].task_id)
        );
      },
      { timeout: 30000 },
    )
    .toBe(true);
  await card.getByRole("button", { name: "Revoke webhook", exact: true }).click();
  await expect(card.getByLabel("Webhook URL", { exact: true })).toBeHidden();
  expect(
    (
      await page.request.post(url, { data: body, headers: { "x-fixture-signature": signature } })
    ).status(),
  ).toBe(401);
}
