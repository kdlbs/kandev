import { randomUUID } from "node:crypto";
import { expect, type Page, type Locator } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import { SessionPage } from "../pages/session-page";
import { openMCPComposerCustomization } from "./mcp-composer";
import { expectMCPCustomizationScrolling } from "./mcp-customization-scrolling";

async function activate(button: Locator, touch: boolean) {
  await button.scrollIntoViewIfNeeded();
  if (touch) await button.tap();
  else await button.press("Enter");
}

async function rejectThenReload(
  page: Page,
  surface: Locator,
  sessionId: string,
  button: Locator,
  touch: boolean,
) {
  const route = `**/api/v1/task-sessions/${sessionId}/mcp-selections`;
  await page.route(route, async (request) => {
    if (request.request().method() !== "PUT") return request.continue();
    await request.fulfill({
      status: 409,
      contentType: "application/json",
      body: JSON.stringify({ code: "mcp_selection_conflict", error: "Changed" }),
    });
  });
  try {
    await activate(button, touch);
    await expect(surface.getByRole("alert")).toBeVisible();
    await expect(button).toBeDisabled();
  } finally {
    await page.unroute(route);
  }
  await surface.getByRole("button", { name: "Reload choices", exact: true }).click();
  await expect(button).toBeEnabled();
  await expect(
    surface.getByRole("button", { name: "Restore inheritance", exact: true }),
  ).toHaveCount(0);
}

async function seedDefinitions(api: ApiClient, workspaceId: string) {
  const suffix = randomUUID().slice(0, 8);
  const definition = await api.createMCPServer(workspaceId, {
    runtime_name: `exclude-docs-${suffix}`,
    display_name: "Inherited exclusion docs",
    execution_mode: "remote",
    transport: "sse",
    configuration: { url: "https://mcp.example.test/docs" },
    source: "custom",
  });
  for (let i = 0; i < 8; i++)
    await api.createMCPServer(workspaceId, {
      runtime_name: `additional-${suffix}-${i}`,
      display_name: `Additional server ${i}`,
      execution_mode: "remote",
      transport: "sse",
      configuration: { url: "https://mcp.example.test/addition" },
      source: "custom",
    });
  return definition;
}

export async function checkMCPSessionExclusions(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  touch: boolean,
) {
  const definition = await seedDefinitions(api, seed.workspaceId);
  await api.replaceMCPSelections("repository", seed.repositoryId, seed.workspaceId, [
    definition.id,
  ]);
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "Session exclusions",
    seed.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Missing session");
  const sessionId = task.session_id;
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  await session.sendMessageViaButton("/e2e:cancel-hold");
  await expect
    .poll(
      async () =>
        (await api.listTaskSessions(task.id)).sessions.find((s) => s.id === sessionId)?.state,
    )
    .toBe("RUNNING");
  let surface = await openMCPComposerCustomization(page);
  const row = () => surface.getByTestId(`mcp-inherited-server-${definition.id}`);
  const exclude = row().getByRole("button", { name: "Exclude for this session", exact: true });
  try {
    await expect(exclude).toBeVisible();
    const bounds = await exclude.boundingBox();
    if (touch || page.viewportSize()!.width < 768) {
      expect(bounds!.height).toBeGreaterThanOrEqual(44);
      expect(bounds!.width).toBeGreaterThanOrEqual(44);
    }
    await rejectThenReload(page, surface, sessionId, exclude, touch);
    await activate(exclude, touch);
    await expect(
      row().getByRole("button", { name: "Restore inheritance", exact: true }),
    ).toBeVisible();
    await expect(row()).toContainText("Saved: excluded from this session");
    const saved = await api.getMCPSelections("task_session", sessionId, seed.workspaceId);
    expect(saved.excluded_definition_ids).toEqual([definition.id]);
    expect(saved.mcp_state?.apply_state).toBe("pending_idle");
    expect(saved.mcp_state?.comparison?.changes?.map((c) => c.action)).toContain("remove");
    expect(
      (await api.getMCPSelections("repository", seed.repositoryId, seed.workspaceId))
        .definition_ids,
    ).toEqual([definition.id]);
    await expectMCPCustomizationScrolling(page, surface, touch);
    await page.reload();
    await session.waitForLoad();
    surface = await openMCPComposerCustomization(page);
    const restore = row().getByRole("button", { name: "Restore inheritance", exact: true });
    await expect(restore).toBeVisible();
    await activate(restore, touch);
    await expect(
      row().getByRole("button", { name: "Exclude for this session", exact: true }),
    ).toBeVisible();
    expect(
      (await api.getMCPSelections("task_session", sessionId, seed.workspaceId))
        .excluded_definition_ids,
    ).toEqual([]);
    await activate(
      row().getByRole("button", { name: "Exclude for this session", exact: true }),
      touch,
    );
    await expect(
      row().getByRole("button", { name: "Restore inheritance", exact: true }),
    ).toBeVisible();
    await api.updateMCPServer(seed.workspaceId, definition.id, {
      enabled: false,
      expected_revision: definition.revision,
    });
    await api.replaceMCPSelections("repository", seed.repositoryId, seed.workspaceId, []);
    await page.reload();
    await session.waitForLoad();
    surface = await openMCPComposerCustomization(page);
    const dormant = surface.getByTestId(`mcp-dormant-exclusion-${definition.id}`);
    await expect(dormant).toContainText("No longer inherited");
    await expect(dormant).toContainText("Restoring inheritance does not enable a disabled server.");
    await activate(
      dormant.getByRole("button", { name: "Restore inheritance", exact: true }),
      touch,
    );
    await expect(dormant).toHaveCount(0);
    const restored = await api.getMCPSelections("task_session", sessionId, seed.workspaceId);
    expect(restored.excluded_definition_ids).toEqual([]);
    expect(restored.definition_ids).toEqual([]);
    expect(
      (await api.listMCPServers(seed.workspaceId)).find((item) => item.id === definition.id)
        ?.enabled,
    ).toBe(false);
    await expect
      .poll(() => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth))
      .toBe(true);
    await surface.getByTestId("mcp-explorer-close").click();
    await expect(page.getByTestId("mcp-status-trigger")).toBeFocused();
  } finally {
    if (await surface.isVisible()) await surface.getByTestId("mcp-explorer-close").click();
    if (await session.cancelAgentButton().isVisible()) await session.cancelAgentButton().click();
  }
}
