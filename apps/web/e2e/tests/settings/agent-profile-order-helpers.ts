import { expect, type Locator, type Page } from "@playwright/test";
import type { Agent } from "../../../lib/types/http";
import type { ApiClient } from "../../helpers/api-client";
import { getMockAgent } from "../../helpers/agent-fixtures";
import { loadInterimSettingsInterlockToken } from "../../helpers/interim-settings-interlock";
import { settledBoundingBox } from "../../helpers/settled-box";

export async function seedProfileOrder(apiClient: ApiClient, prefix: string, count = 2) {
  const { agents } = await apiClient.listAgents();
  const agent = getMockAgent(agents);
  const createdIds: string[] = [];
  for (let index = 0; index < count; index++) {
    const profile = await apiClient.createAgentProfile(
      agent.id,
      `${prefix} ${index} ${Date.now()}`,
      { model: "mock-fast" },
    );
    createdIds.push(profile.id);
  }
  const current = (await apiClient.listAgents()).agents.find((item) => item.id === agent.id)!;
  return { original: agent, current, createdIds };
}

export function profileRow(page: Page, agent: Agent, profileId: string) {
  return page
    .getByTestId(`agent-profiles-${agent.name}`)
    .getByTestId("agent-profile-row")
    .filter({
      has: page
        .getByTestId("agent-profile-row-link")
        .and(page.locator(`[href$="/profiles/${profileId}"]`)),
    });
}

export async function visibleProfileNames(group: Locator) {
  return group
    .getByTestId("agent-profile-row-link")
    .evaluateAll((links) => links.map((link) => link.getAttribute("aria-label")));
}

export async function pointerDragProfile(page: Page, source: Locator, target: Locator) {
  const handle = source.getByTestId("agent-profile-drag-handle");
  await expect(handle).toBeVisible({ timeout: 15_000 });
  await expect(target).toBeVisible({ timeout: 15_000 });
  await target.scrollIntoViewIfNeeded();
  const from = await settledBoundingBox(handle);
  await expect(handle).toBeInViewport();
  await expect(target).toBeInViewport();
  const to = await target.boundingBox();
  if (!to) throw new Error("Target profile row has no bounding box");
  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2);
  await page.mouse.down();
  await page.mouse.move(from.x + from.width / 2 + 14, from.y + from.height / 2);
  await expect(source.locator("xpath=..")).toHaveClass(/opacity-70/);
  await page.mouse.move(to.x + to.width / 2, to.y + to.height / 2, { steps: 12 });
  await page.mouse.up();
}

export async function restoreProfileOrder(page: Page, baseUrl: string, agent: Agent) {
  const token = await loadInterimSettingsInterlockToken(baseUrl);
  const response = await page.request.put(`${baseUrl}/api/v1/agents/${agent.id}/profiles/order`, {
    headers: { "X-Kandev-Interim-Settings-Interlock": token },
    data: { profile_ids: agent.profiles.map((profile) => profile.id) },
  });
  expect(response.ok(), await response.text()).toBe(true);
}

export async function selectorProfileNames(page: Page, selector: Locator, names: string[]) {
  await selector.click();
  const listbox = page.getByRole("listbox");
  await expect(listbox).toBeVisible();
  for (const name of names) {
    await expect(listbox.getByRole("option").filter({ hasText: name })).toBeVisible();
  }
  const labels = await listbox.getByRole("option").allTextContents();
  await page.keyboard.press("Escape");
  await expect(listbox).not.toBeVisible();
  return labels.flatMap((label) => names.filter((name) => label.includes(name)));
}
