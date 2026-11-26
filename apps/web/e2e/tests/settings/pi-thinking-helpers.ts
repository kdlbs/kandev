import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { BackendContext } from "../../fixtures/backend";
import { waitForSessionState } from "../../helpers/session";

export async function createPiProfile(backend: BackendContext, api: ApiClient) {
  await backend.restart({ KANDEV_E2E_MOCK_PI: "true" });
  const { agents } = await api.listAgents();
  const pi = agents.find((agent) => agent.name === "pi-acp");
  expect(pi).toBeDefined();
  return api.createAgentProfile(pi!.id, "Thinking regression", {
    model: "mock-fast",
    mode: "medium",
    require_exact_model: true,
  });
}

export async function clearPiThinking(page: Page, profileId: string, mobile: boolean) {
  await page.goto(`/settings/agents/pi-acp/profiles/${profileId}`);
  const field = page.getByTestId("profile-thinking-field");
  const trigger = field.getByRole("button", { name: "Thinking level", exact: true });
  await expect(trigger).toBeEnabled();
  await expect(field).toContainText("medium (unavailable)");
  await trigger.click();
  if (mobile) {
    const sheet = page.getByRole("dialog");
    await expect(sheet.getByRole("heading", { name: "Thinking level" })).toBeVisible();
    await expect(sheet.getByTestId("model-config-sheet-scroll")).toBeVisible();
    const close = sheet.getByRole("button", { name: "Close", exact: true });
    expect((await close.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await close.click();
    await expect(sheet).not.toBeVisible();
    await expect(trigger).toBeFocused();
    await trigger.click();
    expect(
      (await sheet.getByRole("option", { name: "Use provider default", exact: true }).boundingBox())
        ?.height,
    ).toBeGreaterThanOrEqual(44);
  }
  await page.getByRole("option", { name: "Use provider default", exact: true }).click();
  await expect(trigger).toHaveText("Use provider default");
  const saved = page.waitForResponse(
    (response) =>
      response.url().endsWith(`/agent-profiles/${profileId}`) &&
      response.request().method() === "PATCH",
  );
  const save = page.getByRole("button", { name: /^Save( changes)?$/i });
  await expect(save).toHaveCount(1);
  await save.click();
  expect((await saved).ok()).toBe(true);
  await page.reload();
  await expect(field.getByRole("button", { name: "Thinking level", exact: true })).toHaveText(
    "Use provider default",
  );
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
}

export async function assertPiStartup(
  page: Page,
  api: ApiClient,
  seed: { workspaceId: string; workflowId: string; startStepId: string; repositoryId: string },
  profileId: string,
  fails: boolean,
) {
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    fails ? "Unsupported Pi thinking" : "Default Pi thinking",
    profileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Creating the Pi task did not return a session ID");
  const sessionId = task.session_id;
  if (fails) {
    await waitForSessionState(api, {
      taskId: task.id,
      sessionId,
      expectedState: "FAILED",
      message: "Waiting for the unsupported Pi thinking level to fail startup",
      timeout: 60_000,
    });
    const { session } = await api.getTaskSession(sessionId);
    expect(session.metadata?.last_agent_error).toMatchObject({
      causes: expect.arrayContaining([
        expect.objectContaining({
          code: "thinking_level_unavailable",
          requested_setting: "medium",
          prompt_not_sent: true,
        }),
      ]),
    });
    await page.goto(`/t/${task.id}`);
    const card = page.getByTestId("session-recovery-card");
    await expect(card).toBeVisible();
    await expect(card).toContainText("Thinking level");
    await expect(card).toContainText("No prompt was sent");
    await expect(card.getByRole("link", { name: "Edit profile", exact: true })).toHaveAttribute(
      "href",
      `/settings/agents/pi-acp/profiles/${profileId}`,
    );
    await page.reload();
    await expect(card).toContainText("Thinking level");
    await expect(card).toContainText("No prompt was sent");
    const editProfile = card.getByRole("link", { name: "Edit profile", exact: true });
    await editProfile.click();
    await expect(page).toHaveURL(new RegExp(`/settings/agents/pi-acp/profiles/${profileId}$`));
  } else {
    await waitForSessionState(api, {
      taskId: task.id,
      sessionId,
      expectedState: "WAITING_FOR_INPUT",
      message: "Waiting for Pi to receive its prompt with the provider default",
      timeout: 60_000,
    });
    await page.goto(`/t/${task.id}`);
    const { messages } = await api.listSessionMessages(sessionId);
    expect(
      messages.some(
        (message) =>
          message.author_type === "agent" &&
          message.content.includes("This is a simple mock response"),
      ),
    ).toBe(true);
    await expect(
      page.getByText("This is a simple mock response for e2e testing.", { exact: true }),
    ).toBeVisible();
    await expect(page.getByTestId("session-recovery-card")).toHaveCount(0);
    const { session } = await api.getTaskSession(sessionId);
    expect(session.metadata?.acp_model_state).toMatchObject({ current_model_id: "mock-fast" });
  }
}
