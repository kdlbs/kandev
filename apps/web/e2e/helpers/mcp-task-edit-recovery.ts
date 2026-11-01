import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { KanbanPage } from "../pages/kanban-page";
import { MobileKanbanPage } from "../pages/mobile-kanban-page";
import { waitForHttp } from "./causal-waits";
import { waitForFiniteAnimations } from "./animations";
import { PrAssetCapture } from "./pr-asset-capture";

// @covers AC-AGENTS-MCP-ASSIGNMENT-003.1, AC-AGENTS-MCP-ASSIGNMENT-003.4
export async function checkTaskMCPRecovery(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  phone: boolean,
) {
  const server = await api.createMCPServer(seed.workspaceId, {
    runtime_name: "e2e-mcp-task-edit-recovery",
    display_name: "Documentation tools",
    execution_mode: "remote",
    transport: "streamable_http",
    configuration: { url: "https://mcp.example.test/task-edit" },
    source: "custom",
  });
  try {
    const task = await api.createTask(seed.workspaceId, "Review documentation tools", {
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
    });
    const endpoint = `/api/v1/tasks/${task.id}/mcp-selections`;
    let writes = 0;
    await page.route(`**${endpoint}*`, async (route) => {
      if (route.request().method() === "PUT" && ++writes === 1) {
        await route.fulfill({ status: 500, json: { error: "Selection save rejected" } });
      } else await route.continue();
    });
    const board = phone ? new MobileKanbanPage(page) : new KanbanPage(page);
    await board.goto();
    await board.taskCard(task.id).getByRole("button", { name: "More options" }).click();
    await page.getByRole("menuitem", { name: "Edit", exact: true }).click();
    const dialog = page.getByTestId("create-task-dialog");
    await dialog.getByTestId("task-create-advanced-settings-trigger").click();
    await dialog.getByTestId("task-create-mcp-select").click();
    const choice = page.getByRole("checkbox", { name: server.display_name, exact: true });
    await choice.click();
    const options = page.getByTestId("task-create-mcp-options");
    if (phone) await options.getByRole("button", { name: "Close", exact: true }).click();
    else await choice.press("Escape");
    const submit = async () => {
      if (phone) await dialog.getByTestId("mobile-create-without-agent").click();
      else {
        await dialog.getByTestId("submit-start-agent-chevron").click();
        await page.getByTestId("submit-create-without-agent").click();
      }
    };
    const rejected = waitForHttp(page, "PUT", new RegExp(`${endpoint}$`), {
      predicate: (response) => response.status() === 500,
    });
    await submit();
    await rejected;
    const reload = dialog.getByRole("button", { name: "Reload choices", exact: true });
    await expect(reload).toBeVisible();
    await reload.scrollIntoViewIfNeeded();
    await waitForFiniteAnimations(dialog);
    if (phone) expect((await reload.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    const assets = new PrAssetCapture(
      page,
      phone ? "mobile-mcp-task-edit-recovery.spec.ts" : "mcp-task-edit-recovery.spec.ts",
    );
    if (assets.capturing)
      await assets.screenshot("rejected-save", {
        caption: "Reload saved MCP choices without discarding the task draft",
      });
    assets.flush();
    const refreshed = waitForHttp(page, "GET", new RegExp(`${endpoint}$`));
    await reload.click();
    await refreshed;
    await expect(reload).toHaveCount(0);
    expect(writes).toBe(1);
    expect((await api.getMCPSelections("task", task.id, seed.workspaceId)).definition_ids).toEqual(
      [],
    );
    await dialog.getByTestId("task-create-mcp-select").click();
    await expect(choice).toBeChecked();
    if (phone) await options.getByRole("button", { name: "Close", exact: true }).click();
    else await choice.press("Escape");
    const saved = waitForHttp(page, "PUT", new RegExp(`${endpoint}$`), {
      predicate: (response) => response.ok(),
    });
    await submit();
    await saved;
    await expect(dialog).toHaveCount(0);
    expect(writes).toBe(2);
    expect((await api.getMCPSelections("task", task.id, seed.workspaceId)).definition_ids).toEqual([
      server.id,
    ]);
  } finally {
    await api.deleteMCPServer(seed.workspaceId, server.id, server.revision);
  }
}
