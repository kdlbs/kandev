import { expect, type Page } from "@playwright/test";
import type { AgentProfile } from "../../../lib/types/http-agents";
import type { ApiClient } from "../../helpers/api-client";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForSessionDone } from "../../helpers/session";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import { readModelSelectionWarnings } from "../session/model-mismatch-warning-helpers";

export async function launchExecutorOnlyModelProfile(
  page: Page,
  apiClient: ApiClient,
  profile: AgentProfile,
  mobile: boolean,
) {
  await expect
    .poll(async () => {
      const { agents } = await apiClient.listAvailableAgents();
      return agents.find((agent) => agent.name === "mock-agent")?.model_config.status;
    })
    .toBe("ok");
  const { agents } = await apiClient.listAvailableAgents();
  const hostModels = agents
    .find((agent) => agent.name === "mock-agent")!
    .model_config.available_models.map((model) => model.id);
  expect(hostModels.length).toBeGreaterThan(0);
  expect(hostModels).not.toContain(profile.model);

  const kanban = new KanbanPage(page);
  await kanban.goto();
  await page.reload();
  if (mobile) await page.getByRole("button", { name: "Add task" }).tap();
  else await kanban.createTaskButton.first().click();
  const dialog = page.getByTestId("create-task-dialog");
  const selector = dialog.getByTestId("agent-profile-selector");
  await selector.click();
  const option = page.getByRole("listbox").getByRole("option", { name: profile.name });
  await expect(option).toBeEnabled();
  await expect(option.locator("button, .tabler-icon-alert-triangle")).toHaveCount(0);
  if (mobile) await option.tap();
  else {
    const search = page.locator("[cmdk-input]");
    await search.fill(profile.name);
    await search.press("Enter");
  }
  await expect(page.getByRole("listbox")).not.toBeVisible();
  await expect(selector).toContainText(profile.name);
  await expect(selector.locator("button, .tabler-icon-alert-triangle")).toHaveCount(0);
  await dialog.getByTestId("task-title-input").fill("Use my saved model");
  await dialog.getByTestId("task-description-input").fill("/e2e:simple-message");
  await dialog.getByTestId("submit-start-agent").click();
  if (mobile) await kanban.taskCardByTitle("Use my saved model").tap();
  await expect(page).toHaveURL(/\/t\/[^/?]+/);
  const taskId = new URL(page.url()).pathname.split("/")[2];
  await expect.poll(async () => (await apiClient.listTaskSessions(taskId)).sessions.length).toBe(1);
  const { sessions } = await apiClient.listTaskSessions(taskId);
  const sessionId = sessions[0].id;
  expect(sessions[0].agent_profile_id).toBe(profile.id);
  await waitForSessionDone(apiClient, taskId, sessionId, "Waiting for requested executor model");

  const session = new SessionPage(page);
  await session.waitForLoad();
  await assertRequestedModel(page, apiClient, taskId, sessionId, profile);
  await page.reload();
  await session.waitForLoad();
  await assertRequestedModel(page, apiClient, taskId, sessionId, profile);
  if (mobile) await assertNoDocumentHorizontalOverflow(page, "requested model after reload");
}

async function assertRequestedModel(
  page: Page,
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
  profile: AgentProfile,
) {
  const { sessions } = await apiClient.listTaskSessions(taskId);
  const current = sessions.find((session) => session.id === sessionId)!;
  expect(current.metadata?.runtime_config).toMatchObject({
    model: profile.model,
    config_options: { effort: "high" },
  });
  expect(current.metadata?.acp_model_state).toMatchObject({
    current_model_id: profile.model,
    models: expect.arrayContaining([expect.objectContaining({ model_id: profile.model })]),
  });
  expect(await readModelSelectionWarnings(apiClient, sessionId)).toHaveLength(0);
  const modelTrigger = page.getByRole("button", { name: "Session model settings" });
  await expect(modelTrigger).toContainText("Opus (270k)");
  await expect(modelTrigger).toContainText("High");
  const saved = await apiClient.getAgentProfile(profile.id);
  expect(saved.model).toBe(profile.model);
  expect(saved.config_options).toEqual(profile.config_options);
  expect(saved.fallback_model).toBe(profile.fallback_model);
  expect(saved.auto_fallback).toBe(profile.auto_fallback);
}
