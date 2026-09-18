import { expect, type Page } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { selectExampleAssistant } from "./personal-assistant";

export async function exerciseWorkspaceLinks(
  page: Page,
  backend: BackendContext,
  api: ApiClient,
  seed: SeedData,
) {
  const binding = await selectExampleAssistant(page, backend, api, seed);
  const linked = await api.createWorkspace("Example linked workspace");
  await page.reload();
  await page.getByRole("tab", { name: "Details", exact: true }).click();
  const panel = page.getByRole("region", { name: "Linked workspaces", exact: true });
  const form = panel.getByTestId("workspace-grant-form");
  await expect(form).toBeVisible();
  await form.getByRole("combobox", { name: "Workspace", exact: true }).selectOption(linked.id);
  const submit = form.getByRole("button", { name: "Confirm workspace access" });
  await expect(submit).toBeDisabled();
  await form
    .getByLabel("I confirm this workspace context may be sent to the displayed profile.")
    .check();
  const base = `${backend.baseUrl}/api/v1/orchestration/assistant/workspace-links/${linked.id}`;
  const saved = page.waitForResponse(
    (response) => response.url() === base && response.request().method() === "PUT",
  );
  await submit.click();
  expect((await saved).status()).toBe(200);
  const card = panel.getByTestId("workspace-grant-card");
  await expect(card).toContainText("Access active");
  await expect(card).toContainText("Task titles, states and attention references");
  await card.scrollIntoViewIfNeeded();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
  const forgotten = page.waitForResponse(
    (response) => response.url() === `${base}/forget` && response.request().method() === "POST",
  );
  await card.getByRole("button", { name: "Forget stored handoffs" }).click();
  expect((await forgotten).status()).toBe(200);
  await expect(card).toContainText("grant revision 2");
  const revoked = page.waitForResponse(
    (response) => response.url() === base && response.request().method() === "DELETE",
  );
  await card.getByRole("button", { name: "Revoke workspace access" }).click();
  expect((await revoked).status()).toBe(200);
  await expect(card).toContainText("Access revoked");
  await page.reload();
  await page.getByRole("tab", { name: "Details", exact: true }).click();
  await expect(card).toContainText("Access revoked");
  await expect(panel).toContainText("text already in chat or sent to a provider remains");
  const response = await page.request.get(`${backend.baseUrl}/api/v1/orchestration/assistant`);
  expect((await response.json()).conversation_id).toBe(binding.conversation_id);
  expect((await api.listTaskSessions(binding.conversation_id)).sessions).toEqual([]);
}
