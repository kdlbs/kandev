import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { getMockAgent } from "../../helpers/agent-fixtures";
import { SessionPage } from "../../pages/session-page";
import { waitForSessionDone } from "../../helpers/session";
import { waitForFiniteAnimations } from "../../helpers/animations";

export const MCP_PREFIX = "e2e-new-session-advanced-";
export const PROMPT = "/e2e:simple-message";

export async function seedAdvancedSession(page: Page, apiClient: ApiClient, seed: SeedData) {
  const task = await apiClient.createTaskWithAgent(
    seed.workspaceId,
    "New Agent settings",
    seed.agentProfileId,
    {
      description: PROMPT,
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Missing initial session");
  await waitForSessionDone(apiClient, task.id, task.session_id, "Waiting for initial agent");
  const definitions = [];
  for (let i = 0; i < 10; i++) {
    definitions.push(
      await apiClient.createMCPServer(seed.workspaceId, {
        runtime_name: `${MCP_PREFIX}${i}`,
        display_name:
          i === 0
            ? "Inherited task documentation"
            : `Session documentation ${i} with a long localized server name for layout coverage`,
        description: "Search project documentation and retrieve detailed reference information.",
        execution_mode: "remote",
        transport: "sse",
        source: "custom",
        configuration: { url: `https://mcp.example.test/new-session-${i}` },
      }),
    );
  }
  await apiClient.replaceMCPSelections("task", task.id, seed.workspaceId, [definitions[0].id]);
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  return { task, session, definitions };
}

export async function cleanupAdvancedServers(apiClient: ApiClient, workspaceId: string) {
  for (const server of await apiClient.listMCPServers(workspaceId)) {
    if (server.runtime_name.startsWith(MCP_PREFIX))
      await apiClient.deleteMCPServer(workspaceId, server.id, server.revision);
  }
}

export async function expectContainedPicker(page: Page, picker: Locator) {
  await waitForFiniteAnimations(picker);
  const box = (await picker.boundingBox())!;
  const viewport = page.viewportSize()!;
  expect(box.x).toBeGreaterThanOrEqual(0);
  expect(box.y).toBeGreaterThanOrEqual(0);
  expect(box.x + box.width).toBeLessThanOrEqual(viewport.width + 1);
  expect(box.y + box.height).toBeLessThanOrEqual(viewport.height + 1);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
  expect(
    await picker.evaluate((el) => {
      const bounds = el.getBoundingClientRect();
      return [8, bounds.height - 8].every((offset) =>
        el.contains(document.elementFromPoint(bounds.x + bounds.width / 2, bounds.y + offset)),
      );
    }),
    "Picker edges must remain visible and receive input outside the launch form",
  ).toBe(true);
  const scroller = picker.locator(".overflow-y-auto");
  await expect(scroller).toHaveCount(1);
  if ((await picker.getAttribute("data-slot")) === "drawer-content") {
    const scrollBox = (await scroller.boundingBox())!;
    expect(scrollBox.y + scrollBox.height).toBeLessThanOrEqual(box.y + box.height - 8);
  }
  expect(await scroller.evaluate((el) => el.scrollHeight > el.clientHeight)).toBe(true);
}

export async function checkTouchPicker(page: Page, session: SessionPage, selectedId: string) {
  const dialog = session.sessionLaunchDialog();
  const advanced = session.newSessionDialogPage.advancedTrigger();
  await expect(advanced).toHaveAttribute("aria-expanded", "false");
  await session.newSessionPromptInput().fill(PROMPT);
  await advanced.tap();
  const trigger = session.newSessionDialogPage.mcpTrigger();
  for (const control of [advanced, trigger]) {
    expect((await control.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  }
  await trigger.tap();
  const picker = session.newSessionDialogPage.mcpPicker();
  await expect(picker).toHaveAttribute("data-slot", "drawer-content");
  await expectContainedPicker(page, picker);
  await expect(picker).toContainText("Inherited task documentation");
  const row = picker.getByTestId(`mcp-choice-${selectedId}`);
  await row.tap();
  await expect(row.getByRole("checkbox")).toBeChecked();
  const close = picker.getByRole("button", { name: "Close", exact: true });
  const closeBox = (await close.boundingBox())!;
  expect(closeBox.width).toBeGreaterThanOrEqual(44);
  expect(closeBox.height).toBeGreaterThanOrEqual(44);
  await close.tap();
  await expect(dialog).toBeVisible();
  await expect(trigger).toBeFocused();
  await expect(trigger).toContainText("1 selected");
  await advanced.tap();
  await advanced.tap();
  await trigger.tap();
  await expect(picker.getByTestId(`mcp-choice-${selectedId}`).getByRole("checkbox")).toBeChecked();
  await close.tap();
  await expect(session.newSessionPromptInput()).toHaveValue(PROMPT);
}

export async function expectCreatedSelection(
  apiClient: ApiClient,
  seed: SeedData,
  taskId: string,
  originalId: string,
  selectedId: string,
) {
  await expect.poll(async () => (await apiClient.listTaskSessions(taskId)).sessions.length).toBe(2);
  const { sessions } = await apiClient.listTaskSessions(taskId);
  const created = sessions.find((session) => session.id !== originalId)!;
  await expect
    .poll(
      async () =>
        (await apiClient.getMCPSelections("task_session", created.id, seed.workspaceId))
          .definition_ids,
    )
    .toEqual([selectedId]);
  await expect
    .poll(async () =>
      (await apiClient.listSessionMessages(created.id)).messages
        .filter((message) => message.author_type === "user")
        .map((message) => message.content),
    )
    .toContain(PROMPT);
  expect(
    (await apiClient.getMCPSelections("task_session", originalId, seed.workspaceId)).definition_ids,
  ).toEqual([]);
  return created;
}

export async function checkAdvancedHandoff(
  page: Page,
  apiClient: ApiClient,
  seed: SeedData,
  mobile: boolean,
) {
  const { task, session, definitions } = await seedAdvancedSession(page, apiClient, seed);
  const source = getMockAgent((await apiClient.listAgents()).agents);
  const target = await apiClient.createAgentProfile(source.id, "Advanced settings handoff", {
    model: "mock-fast",
  });
  try {
    if (mobile) await session.openMobileHandoffDialog(task.session_id!, target.id);
    else await session.openHandoffDialog(task.session_id!, target.id);
    const dialog = session.handoffDialog();
    await expect(dialog).toContainText(target.name);
    const context = dialog.getByRole("combobox").filter({ hasText: "Blank" });
    await expect(context).toBeVisible();
    await expect(session.newSessionPromptInput()).toHaveValue("");
    const controls = session.newSessionDialogPage;
    await expect(controls.advancedTrigger()).toHaveAttribute("aria-expanded", "false");
    if (mobile) {
      await checkTouchPicker(page, session, definitions[1].id);
      await session.newSessionStartButton().tap();
    } else {
      await session.newSessionPromptInput().fill(PROMPT);
      await controls.advancedTrigger().click();
      await controls.mcpTrigger().click();
      await controls.mcpPicker().getByTestId(`mcp-choice-${definitions[1].id}`).click();
      await page.keyboard.press("Escape");
      await controls.advancedTrigger().click();
      await expect(context).toBeVisible();
      await session.newSessionStartButton().click();
    }
    await expect(dialog).not.toBeVisible();
    const created = await expectCreatedSelection(
      apiClient,
      seed,
      task.id,
      task.session_id!,
      definitions[1].id,
    );
    expect(created.agent_profile_id).toBe(target.id);
    await waitForSessionDone(apiClient, task.id, created.id, "Waiting for handed off agent");
  } finally {
    await apiClient.deleteAgentProfile(target.id, true);
  }
}
