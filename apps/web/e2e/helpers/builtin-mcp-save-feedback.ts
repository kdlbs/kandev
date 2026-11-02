import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { SessionPage } from "../pages/session-page";
import { activate, chooseBuiltin } from "./builtin-mcp-management";

async function openPackages(page: Page, api: ApiClient, seed: SeedData, touch: boolean) {
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "Package save feedback",
    seed.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
    },
  );
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle();
  await activate(page.getByTestId("mcp-status-trigger"), touch);
  await activate(page.getByTestId("mcp-customize-tab"), touch);
  await expect(page.getByTestId("builtin-mcp-package-configuration")).toBeVisible();
  return task;
}

// @covers AC-AGENTS-BUILTIN-MCP-003.3, AC-AGENTS-BUILTIN-MCP-004.1
export async function checkBuiltinPendingAddition(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  touch: boolean,
) {
  const task = await openPackages(page, api, seed, touch);
  await activate(page.getByTestId("builtin-mcp-package-configuration"), touch);
  await chooseBuiltin(page, "configuration", "On", touch);
  const saved = page.waitForResponse(
    (response) =>
      response.request().method() === "PUT" &&
      response.url().endsWith(`/tasks/${task.id}/mcp-built-in/preferences`),
  );
  await activate(page.getByTestId("builtin-mcp-save"), touch);
  expect((await saved).ok()).toBe(true);
  await activate(page.getByTestId("mcp-connections-tab"), touch);
  const pending = page.getByTestId("builtin-mcp-pending-configuration");
  await expect(pending).toContainText("Configuration");
  await expect(pending).toContainText("Add");
  await expect(page.getByTestId("mcp-server-row-kandev-configuration")).toHaveCount(0);
  await activate(pending, touch);
  await expect(page.getByTestId("builtin-mcp-choice-configuration")).toContainText("On");
}

// @covers AC-AGENTS-BUILTIN-MCP-004.2, AC-AGENTS-BUILTIN-MCP-004.3, AC-AGENTS-BUILTIN-MCP-004.4
export async function checkBuiltinDirtyFeedback(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  touch: boolean,
) {
  if (touch) await page.setViewportSize({ width: 320, height: 760 });
  await openPackages(page, api, seed, touch);
  const bar = page.getByTestId("settings-floating-save");
  await expect(page.getByTestId("builtin-mcp-save")).toHaveCount(0);
  await activate(page.getByTestId("builtin-mcp-package-configuration"), touch);
  const field = page.getByTestId("builtin-mcp-choice-configuration");
  await chooseBuiltin(page, "configuration", "On", touch);
  await expect(field).toHaveAttribute("data-settings-dirty", "true");
  await expect(bar).toContainText("Unsaved changes");
  if (touch) {
    await page.setViewportSize({ width: 767, height: 760 });
    await expect(field).toContainText("On");
    await page.setViewportSize({ width: 320, height: 760 });
  }
  const reset = bar.getByRole("button", { name: "Reset", exact: true });
  if (touch) {
    expect((await reset.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    expect((await bar.boundingBox())!.y).toBeLessThan(page.viewportSize()!.height);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
  }
  await activate(reset, touch);
  await expect(field).toContainText("Inherit workspace policy");
  await expect(field).toHaveAttribute("data-settings-dirty", "false");
  await expect(bar).toHaveCount(0);
  await chooseBuiltin(page, "configuration", "On", touch);
  await activate(page.getByTestId("builtin-mcp-save"), touch);
  await expect(bar).toHaveCount(0);
  await expect(field).toHaveAttribute("data-settings-dirty", "false");
  await activate(page.getByRole("button", { name: "Back", exact: true }), touch);
  await expect(page.getByTestId("builtin-mcp-package-configuration")).toContainText("On");
}
