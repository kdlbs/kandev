import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

export async function findCopilotProfileId(apiClient: ApiClient): Promise<string> {
  const { agents } = await apiClient.listAgents();
  const copilot = agents.find((agent) => agent.name === "copilot-acp");
  if (!copilot) {
    throw new Error("Copilot agent is unavailable after enabling KANDEV_E2E_MOCK_COPILOT");
  }
  const existing = copilot.profiles?.[0]?.id;
  if (existing) {
    await apiClient.updateAgentProfile(existing, { model: "mock-fast" });
    return existing;
  }
  const profile = await apiClient.createAgentProfile(
    copilot.id,
    `Copilot native ${Date.now().toString(36)}`,
    { model: "mock-fast" },
  );
  return profile.id;
}

export async function seedCopilotNativeTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  prompt: string,
): Promise<{ taskId: string; sessionId: string }> {
  const profileId = await findCopilotProfileId(apiClient);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Copilot native compaction",
    profileId,
    {
      description: prompt,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Expected the Copilot task to start a session");

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return sessions.find((candidate) => candidate.id === task.session_id)?.state;
      },
      { timeout: 30_000, message: "Waiting for the Copilot session to become idle" },
    )
    .toBe("WAITING_FOR_INPUT");

  // Reload from durable state so the assertion also proves hydration, not just
  // a live in-memory patch.
  await testPage.reload();
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });

  return { taskId: task.id, sessionId: task.session_id };
}

export async function openContextTooltip(testPage: Page) {
  const trigger = testPage.getByRole("button", { name: /Context window: / });
  await expect(trigger).toBeVisible();
  const tooltip = testPage
    .locator('[data-slot="tooltip-content"]:not([data-state="closed"])')
    .filter({ has: testPage.getByTestId("context-window-usage") });
  const usage = tooltip.getByTestId("context-window-usage").first();
  if (!(await usage.isVisible().catch(() => false))) {
    await trigger.click();
  }
  await expect(usage).toBeVisible();
  return { trigger, tooltip, usage };
}

export async function ensureContextTooltipOpen(
  testPage: Page,
): Promise<Awaited<ReturnType<typeof openContextTooltip>>> {
  return openContextTooltip(testPage);
}

export async function expectNativeCompactionCount(
  tooltip: Awaited<ReturnType<typeof openContextTooltip>>["tooltip"],
  count: number,
) {
  const usage = tooltip.getByTestId("context-window-usage").first();
  const row = usage.getByTestId("context-window-compactions-row");
  await expect(row.getByText("Compactions", { exact: true }).first()).toBeVisible();
  await expect(row.getByText(String(count), { exact: true }).first()).toBeVisible();
}

export async function sendCopilotLiveCompaction(testPage: Page): Promise<void> {
  const session = new SessionPage(testPage);
  await session.sendMessage("/e2e:copilot-native-live");
  await expect(session.chat).toContainText("/e2e:copilot-native-live");
  await session.waitForChatIdle({ timeout: 30_000 });
}
