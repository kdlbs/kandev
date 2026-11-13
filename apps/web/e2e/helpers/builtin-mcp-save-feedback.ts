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
  await expect(page.getByTestId("builtin-mcp-source-configuration")).toContainText(
    "Inherited from workspace: Off",
  );
  await expect(field).toHaveAttribute("data-settings-dirty", "false");
  await expect(bar).toHaveCount(0);
  await chooseBuiltin(page, "configuration", "On", touch);
  await activate(page.getByTestId("builtin-mcp-save"), touch);
  await expect(bar).toHaveCount(0);
  await expect(field).toHaveAttribute("data-settings-dirty", "false");
  await activate(page.getByRole("button", { name: "Back to packages", exact: true }), touch);
  await expect(page.getByTestId("builtin-mcp-package-configuration-row")).toContainText("On");
}

// @covers AC-AGENTS-BUILTIN-MCP-004.2, AC-AGENTS-BUILTIN-MCP-004.4
export async function checkBuiltinPackageDetail(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  touch: boolean,
) {
  await page.addInitScript(() => localStorage.setItem("theme", "dark"));
  if (touch) await page.setViewportSize({ width: 320, height: 760 });
  await openPackages(page, api, seed, touch);
  const explorer = page.getByTestId("mcp-server-explorer");
  await expect(explorer.getByText("Saved session exclusions: 0", { exact: true })).toHaveCount(0);
  await activate(page.getByTestId("builtin-mcp-package-configuration"), touch);
  const detail = page.getByTestId("builtin-mcp-detail");
  await expect(detail.getByText("Built-in package", { exact: true })).toBeVisible();
  await expect(detail.getByRole("heading", { name: "Configuration", exact: true })).toBeVisible();
  const back = detail.getByRole("button", { name: "Back to packages", exact: true });
  await expect(back).toBeVisible();
  await expect(
    detail.getByText(
      "Applies when a new agent process starts. Running or resumed agents keep their current tools.",
      { exact: true },
    ),
  ).toHaveCount(1);
  const metadata = detail.getByRole("button", { name: "Package details", exact: true });
  await expect(metadata).toHaveAttribute("aria-expanded", "false");
  await expect(detail.getByText("kandev-configuration", { exact: true })).not.toBeVisible();
  await activate(metadata, touch);
  await expect(detail.getByText("kandev-configuration", { exact: true })).toBeVisible();
  await activate(metadata, touch);
  await expect(detail.getByText("kandev-configuration", { exact: true })).not.toBeVisible();
  if (touch) {
    expect((await back.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    await page.setViewportSize({ width: 393, height: 852 });
  }
  await page
    .getByTestId("mcp-customization-scroll")
    .evaluate((node) => node.scrollTo({ top: 0, behavior: "instant" }));
  await expect(detail.getByText("Built-in package", { exact: true })).toBeVisible();
}

// @covers AC-AGENTS-BUILTIN-MCP-003.3, AC-AGENTS-BUILTIN-MCP-004.1, AC-AGENTS-BUILTIN-MCP-004.5
export async function checkBuiltinUnavailableAcceptance(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  touch: boolean,
) {
  const task = await openPackages(page, api, seed, touch);
  const path = `**/api/v1/task-sessions/${task.session_id}/mcp-built-in`;
  await page.route(path, async (route) => {
    const response = await route.fetch();
    const view = await response.json();
    await route.fulfill({ response, json: { ...view, bound: null, binding_state: "legacy" } });
  });
  try {
    await activate(page.getByTestId("builtin-mcp-package-configuration"), touch);
    await chooseBuiltin(page, "configuration", "On", touch);
    const saved = page.waitForResponse(
      (response) =>
        response.request().method() === "PUT" &&
        response.url().endsWith(`/tasks/${task.id}/mcp-built-in/preferences`),
    );
    await activate(page.getByTestId("builtin-mcp-save"), touch);
    expect((await saved).ok()).toBe(true);
    await expect(page.getByTestId("settings-floating-save")).toHaveCount(0);
    await activate(page.getByTestId("mcp-connections-tab"), touch);
    await expect(page.getByTestId("builtin-mcp-pending-configuration")).toContainText(
      "Configuration",
    );
    await expect(page.getByTestId("builtin-mcp-pending-configuration")).toContainText("Selected");
    await expect(page.getByTestId("builtin-mcp-pending-changes")).toContainText(
      "Current agent packages are unknown",
    );
    await expect(page.getByTestId("mcp-server-row-kandev-configuration")).toHaveCount(0);
  } finally {
    await page.unroute(path);
  }
}

// @covers AC-AGENTS-BUILTIN-MCP-005.2, AC-AGENTS-BUILTIN-MCP-005.5
export async function checkBuiltinInheritance(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  touch: boolean,
) {
  if (!touch) await page.setViewportSize({ width: 1280, height: 1000 });
  const task = await openPackages(page, api, seed, touch);
  await expect(page.getByTestId("builtin-mcp-detail")).toHaveCount(0);
  const source = page.getByTestId("builtin-mcp-source-diagnostics");
  const control = page.getByTestId("builtin-mcp-choice-diagnostics");
  const toggle = control.getByRole("switch");
  await expect(source).toHaveText("Inherited from workspace: On");
  await expect(toggle).toBeChecked();
  if (touch) expect((await control.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await toggle.focus();
  await page.keyboard.press("Space");
  await expect(toggle).not.toBeChecked();
  await page.keyboard.press("Space");
  await expect(source).toContainText("Task override: On");
  await activate(page.getByTestId("builtin-mcp-save"), touch);
  await expect(page.getByTestId("settings-floating-save")).toHaveCount(0);
  const getChoice = async () => {
    const response = await api.rawRequest("GET", `/api/v1/tasks/${task.id}/mcp-built-in`);
    return (await response.json()).preferences.packages.diagnostics;
  };
  await expect.poll(getChoice).toBe("on");
  await page.getByTestId(touch ? "mcp-server-explorer" : "builtin-mcp-packages").screenshot({
    path: `../../artifacts/mcp-switches/list-task-override-${touch ? "phone" : "desktop"}.png`,
  });
  await activate(page.getByTestId("builtin-mcp-inherit-diagnostics"), touch);
  await expect(toggle).toBeChecked();
  await expect(source).toHaveText("Inherited from workspace: On");
  await expect(control).toHaveAttribute("data-settings-dirty", "true");
  await activate(page.getByTestId("builtin-mcp-save"), touch);
  await expect.poll(getChoice).toBeUndefined();
  await expect(page.getByTestId("settings-floating-save")).toHaveCount(0);
  await page.getByTestId(touch ? "mcp-server-explorer" : "builtin-mcp-packages").screenshot({
    path: `../../artifacts/mcp-switches/list-task-inherited-${touch ? "phone" : "desktop"}.png`,
  });
}
