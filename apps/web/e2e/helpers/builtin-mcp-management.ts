import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { PrAssetCapture } from "./pr-asset-capture";
import { waitForFiniteAnimations } from "./animations";
import type { SeedData } from "../fixtures/test-base";
import type { BuiltinMCPManagementView } from "@/lib/types/builtin-mcp";
import { SessionPage } from "../pages/session-page";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";

export async function builtinView(
  api: ApiClient,
  scope: "tasks" | "workspaces",
  id: string,
): Promise<BuiltinMCPManagementView> {
  const response = await api.rawRequest("GET", `/api/v1/${scope}/${id}/mcp-built-in`);
  expect(response.status).toBe(200);
  return response.json();
}
export async function activate(locator: Locator, touch: boolean) {
  await expect(locator).toHaveCount(1);
  if (touch) await locator.tap();
  else await locator.click();
}
export async function chooseBuiltin(page: Page, id: string, label: string, touch: boolean) {
  if (label === "Inherit workspace policy") {
    await activate(page.getByTestId(`builtin-mcp-inherit-${id}`), touch);
    return;
  }
  const choice = page.getByTestId(`builtin-mcp-choice-${id}`).getByRole("switch");
  const enabled = label !== "Off" && label !== "Blocked";
  if ((await choice.isChecked()) !== enabled) await activate(choice, touch);
  if (label === "On by default" || label === "Available") {
    const defaultChoice = page.getByTestId(`builtin-mcp-default-${id}`).getByRole("switch");
    if ((await defaultChoice.isChecked()) !== (label === "On by default"))
      await activate(defaultChoice, touch);
  }
}
export async function checkBuiltinWorkspace(page: Page, api: ApiClient, touch: boolean) {
  const workspace = await api.createWorkspace(`Built-in packages ${Date.now()}`);
  try {
    await page.goto(`/settings/workspaces/${workspace.id}/mcp-servers`);
    await expect(page.getByTestId("builtin-mcp-packages")).toContainText("Core");
    await activate(page.getByTestId("builtin-mcp-package-configuration"), touch);
    const detail = page.getByTestId("builtin-mcp-detail");
    await expect(detail).toContainText("Configuration chats always receive Core and Configuration");
    await expect(page.getByTestId("builtin-mcp-default-configuration")).toHaveCount(0);
    await activate(detail.getByRole("button", { name: "Back to packages", exact: true }), touch);
    await expect(page.getByTestId("builtin-mcp-package-configuration")).toBeFocused();
    await expect(page.getByTestId("builtin-mcp-detail")).toHaveCount(0);
    await chooseBuiltin(page, "diagnostics", "Blocked", touch);
    await expect(page.getByTestId("builtin-mcp-choice-diagnostics")).toHaveAttribute(
      "data-settings-dirty",
      "true",
    );
    const save = page
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes", exact: true });
    const box = await save.boundingBox();
    expect(box).not.toBeNull();
    if (touch) expect(box!.height).toBeGreaterThanOrEqual(44);
    else expect(box!.height).toBeCloseTo(28, 0);
    await activate(save, touch);
    await expect
      .poll(
        async () =>
          (await builtinView(api, "workspaces", workspace.id)).policy.packages.diagnostics,
      )
      .toBe("blocked");
    await page.reload();
    await expect(page.getByTestId("builtin-mcp-detail")).toHaveCount(0);
    await expect(page.getByTestId("builtin-mcp-choice-diagnostics")).toContainText("Off");
    await expect(page.getByTestId("builtin-mcp-choice-diagnostics")).toHaveAttribute(
      "data-settings-dirty",
      "false",
    );
    const defaultChoice = page.getByTestId("builtin-mcp-default-diagnostics").getByRole("switch");
    await expect(defaultChoice).toBeDisabled();
    await chooseBuiltin(page, "diagnostics", "Available", touch);
    await expect(defaultChoice).toBeEnabled();
    await expect(defaultChoice).not.toBeChecked();
    await chooseBuiltin(page, "diagnostics", "On by default", touch);
    await expect(defaultChoice).toBeChecked();
    await page.getByTestId("builtin-mcp-packages").screenshot({
      path: `../../artifacts/mcp-switches/list-workspace-${touch ? "phone" : "desktop"}.png`,
    });
    await activate(save, touch);
    await expect
      .poll(
        async () =>
          (await builtinView(api, "workspaces", workspace.id)).policy.packages.diagnostics,
      )
      .toBe("default");
    await assertNoDocumentHorizontalOverflow(page, "built-in package detail");
  } finally {
    await api.deleteWorkspace(workspace.id, workspace.name);
  }
}
export async function checkBuiltinTask(page: Page, api: ApiClient, seed: SeedData, touch: boolean) {
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "Built-in consent task",
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
  await session.waitForChatIdle({ timeout: 30_000 });
  await activate(page.getByTestId("mcp-status-trigger"), touch);
  const explorer = page.getByTestId("mcp-server-explorer");
  await activate(explorer.getByTestId("mcp-customize-tab"), touch);
  await activate(explorer.getByTestId("builtin-mcp-package-configuration"), touch);
  await expect(explorer.getByTestId("builtin-mcp-effective")).toContainText("Off");
  await expect(explorer.getByTestId("builtin-mcp-detail")).toContainText(
    "shared Kandev settings and resources",
  );
  await chooseBuiltin(page, "configuration", "On", touch);
  const save = explorer.getByTestId("builtin-mcp-save");
  await save.scrollIntoViewIfNeeded();
  if (touch) expect((await save.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await activate(save, touch);
  await expect
    .poll(
      async () => (await builtinView(api, "tasks", task.id)).preferences?.packages.configuration,
    )
    .toBe("on");
  await expect(save).toHaveCount(0);
  await activate(explorer.getByRole("button", { name: "Back to packages", exact: true }), touch);
  await activate(explorer.getByTestId("builtin-mcp-package-walkthroughs"), touch);
  await chooseBuiltin(page, "walkthroughs", "Off", touch);
  await activate(save, touch);
  await expect
    .poll(async () => (await builtinView(api, "tasks", task.id)).preferences?.packages.walkthroughs)
    .toBe("off");
  await expect(explorer).toContainText("Running or resumed agents keep their current tools");
  const { sessions } = await api.listTaskSessions(task.id);
  const current = sessions.find((entry) => entry.is_primary)!;
  const evidenceResponse = await api.rawRequest(
    "GET",
    `/api/v1/task-sessions/${current.id}/mcp-built-in`,
  );
  expect(evidenceResponse.status).toBe(200);
  const evidence = await evidenceResponse.json();
  expect(evidence.desired.packages).toContain("configuration");
  expect(evidence.desired.packages).not.toContain("walkthroughs");
  expect(evidence.bound.manifest.packages).not.toContain("configuration");
  expect(evidence.bound.manifest.packages).toContain("walkthroughs");

  await assertNoDocumentHorizontalOverflow(page, "task built-in packages");
  if (touch) {
    const viewport = page.viewportSize()!;
    await expect
      .poll(async () => (await explorer.boundingBox())!.height)
      .toBeGreaterThanOrEqual(viewport.height * 0.9);
    const scroll = explorer.getByTestId("mcp-customization-scroll");
    expect(await scroll.evaluate((node) => getComputedStyle(node).overflowY)).toBe("auto");
  }
  await page.keyboard.press("Escape");
  await page.reload();
  await session.waitForLoad();
  await activate(page.getByTestId("mcp-status-trigger"), touch);
  await activate(page.getByTestId("mcp-customize-tab"), touch);
  await activate(page.getByTestId("builtin-mcp-package-configuration"), touch);
  await expect(page.getByTestId("builtin-mcp-choice-configuration")).toContainText("On");
  await checkBuiltinTaskReopen(page, api, task.id, touch);
}

async function checkBuiltinTaskReopen(page: Page, api: ApiClient, taskId: string, touch: boolean) {
  await page.keyboard.press("Escape");
  const current = await builtinView(api, "tasks", taskId);
  const saved = await api.rawRequest("PUT", `/api/v1/tasks/${taskId}/mcp-built-in/preferences`, {
    version: 1,
    expected_revision: current.preferences!.revision,
    expected_workspace_policy_revision: current.policy.revision,
    packages: { ...current.preferences!.packages, configuration: "off" },
  });
  expect(saved.status).toBe(200);
  await activate(page.getByTestId("mcp-status-trigger"), touch);
  await activate(page.getByTestId("mcp-customize-tab"), touch);
  await expect(page.getByTestId("builtin-mcp-choice-configuration")).toContainText("Off");
  await chooseBuiltin(page, "configuration", "On", touch);
  await page.keyboard.press("Escape");
  await activate(page.getByTestId("mcp-status-trigger"), touch);
  await activate(page.getByTestId("mcp-customize-tab"), touch);
  await expect(page.getByTestId("builtin-mcp-choice-configuration")).toContainText("On");
}

async function openCreationMCP(page: Page, dialog: Locator, touch: boolean) {
  await expect(dialog.getByTestId("task-create-builtin-packages")).toBeHidden();
  await activate(dialog.getByTestId("task-create-advanced-settings-trigger"), touch);
  await expect(dialog.getByTestId("task-create-builtin-packages")).toBeHidden();
  const trigger = dialog.getByTestId("task-create-mcp-select");
  await activate(trigger, touch);
  const options = page.getByTestId("task-create-mcp-options");
  const packages = options.getByTestId("task-create-builtin-packages");
  await expect(packages.getByTestId("builtin-mcp-package-core")).toBeVisible();
  await expect(options.getByText("Built-in packages", { exact: true })).toHaveCount(1);
  await expect(
    options.getByRole("heading", { name: "Built-in packages", exact: true }),
  ).toHaveCount(1);
  await assertNoDocumentHorizontalOverflow(page, "new-task MCP packages");
  await waitForFiniteAnimations(options);
  for (const row of await packages
    .locator('[data-testid^="builtin-mcp-package-"][data-testid$="-row"]')
    .all()) {
    const box = await row.evaluate((element) => {
      const bounds = element.getBoundingClientRect();
      const label = element.firstElementChild!.getBoundingClientRect();
      return {
        top: bounds.top,
        bottom: bounds.bottom,
        labelTop: label.top,
        labelBottom: label.bottom,
      };
    });
    expect(box.labelTop).toBeGreaterThanOrEqual(box.top + 8);
    expect(box.labelBottom).toBeLessThanOrEqual(box.bottom - 8);
  }
  if (touch) {
    expect((await trigger.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    const box = (await options.boundingBox())!;
    expect(box.x).toBeGreaterThanOrEqual(0);
    expect(box.x + box.width).toBeLessThanOrEqual(page.viewportSize()!.width + 1);
    expect(box.y + box.height).toBeLessThanOrEqual(page.viewportSize()!.height + 1);
  }
  return { packages, options };
}

async function closeCreationMCP(page: Page, options: Locator, touch: boolean) {
  if (touch) await options.getByRole("button", { name: "Close", exact: true }).tap();
  else await page.keyboard.press("Escape");
  await expect(options).toBeHidden();
}

export async function checkBuiltinCreationLoading(page: Page, touch: boolean) {
  if (touch) await page.setViewportSize({ width: 320, height: 852 });
  await page.goto("/");
  const catalog = "**/api/v1/workspaces/*/mcp-servers";
  const release = Promise.withResolvers<void>();
  const received = Promise.withResolvers<void>();
  const completed = Promise.withResolvers<void>();
  await page.route(catalog, async (route) => {
    received.resolve();
    await release.promise;
    await route.continue();
    completed.resolve();
  });
  try {
    const create = touch
      ? page.getByRole("button", { name: "Add task", exact: true })
      : page.getByTestId("create-task-button").filter({ visible: true });
    await activate(create, touch);
    await received.promise;
    const dialog = page.getByTestId("create-task-dialog");
    await activate(dialog.getByTestId("task-create-advanced-settings-trigger"), touch);
    const trigger = dialog.getByTestId("task-create-mcp-select");
    await expect(trigger).toBeEnabled();
    await activate(trigger, touch);
    const options = page.getByTestId("task-create-mcp-options");
    await expect(options.getByRole("status")).toContainText("Loading");
    await activate(options.getByTestId("builtin-mcp-package-configuration"), touch);
    await chooseBuiltin(page, "configuration", "On", touch);
    await expect(options.getByTestId("builtin-mcp-choice-configuration")).toContainText("On");
    const response = page.waitForResponse((entry) =>
      /\/api\/v1\/workspaces\/[^/]+\/mcp-servers$/.test(new URL(entry.url()).pathname),
    );
    release.resolve();
    expect((await response).ok()).toBe(true);
    await expect(options.getByRole("status")).toBeHidden();
    await expect(options.getByTestId("builtin-mcp-choice-configuration")).toContainText("On");
    await closeCreationMCP(page, options, touch);
  } finally {
    release.resolve();
    await completed.promise;
    await page.unroute(catalog);
  }
}

export async function checkBuiltinCreation(page: Page, api: ApiClient, touch: boolean) {
  await page.goto("/");
  const create = touch
    ? page.getByRole("button", { name: "Add task", exact: true })
    : page.getByTestId("create-task-button").filter({ visible: true });
  await expect(create).toHaveCount(1);
  await activate(create, touch);
  const dialog = page.getByTestId("create-task-dialog");
  await dialog.getByTestId("task-title-input").fill("Creation package consent");
  await dialog.getByTestId("task-description-input").fill("/e2e:simple-message");
  const { packages, options } = await openCreationMCP(page, dialog, touch);
  await expect(
    packages.getByTestId("builtin-mcp-choice-configuration").getByRole("switch"),
  ).not.toBeChecked();
  await chooseBuiltin(page, "configuration", "On", touch);
  await expect(packages.getByTestId("builtin-mcp-detail")).toHaveCount(0);
  await packages.screenshot({
    path: `../../artifacts/mcp-switches/list-new-task-${touch ? "phone" : "desktop"}.png`,
  });
  await closeCreationMCP(page, options, touch);
  await activate(dialog.getByTestId("task-create-advanced-settings-trigger"), touch);
  const reopened = await openCreationMCP(page, dialog, touch);
  await activate(reopened.packages.getByTestId("builtin-mcp-package-configuration"), touch);
  await expect(reopened.packages.getByTestId("builtin-mcp-choice-configuration")).toContainText(
    "On",
  );
  await closeCreationMCP(page, reopened.options, touch);
  const created = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      /\/api\/v1\/tasks$/.test(new URL(response.url()).pathname),
  );
  await activate(dialog.getByTestId("submit-start-agent"), touch);
  const response = await created;
  expect(response.ok(), await response.text()).toBeTruthy();
  const body = await response.json();
  const id = body.task?.id ?? body.id;
  expect(typeof id).toBe("string");
  expect((await builtinView(api, "tasks", id)).preferences?.packages.configuration).toBe("on");
  await page.goto(`/t/${id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  await session.sendMessageViaButton("e2e:mcp:kandev-configuration:list_shared_prompts_kandev({})");
  await session.waitForChatIdle({ timeout: 30_000 });
  const { sessions } = await api.listTaskSessions(id);
  const primary = sessions.find((entry) => entry.is_primary)!;
  const evidenceResponse = await api.rawRequest(
    "GET",
    `/api/v1/task-sessions/${primary.id}/mcp-built-in`,
  );
  const evidence = await evidenceResponse.json();
  const configuration = evidence.history?.current?.servers?.find(
    (server: { name: string }) => server.name === "kandev-configuration",
  );
  expect(configuration?.status, JSON.stringify(configuration)).not.toBe("failed");
  await activate(page.getByTestId("mcp-status-trigger"), touch);
  const explorer = page.getByTestId("mcp-server-explorer");
  await expect(explorer.getByTestId("mcp-server-row-kandev-configuration")).toContainText("Active");
  await activate(explorer.getByTestId("mcp-server-row-kandev-configuration"), touch);
  await expect(explorer.getByTestId("mcp-tool-row-list_shared_prompts_kandev")).toBeVisible();
}

export async function checkBuiltinCreationBack(
  page: Page,
  touch: boolean,
  capture: PrAssetCapture,
) {
  await page.goto("/");
  const create = touch
    ? page.getByRole("button", { name: "Add task", exact: true })
    : page.getByTestId("create-task-button").filter({ visible: true });
  await activate(create, touch);
  const dialog = page.getByTestId("create-task-dialog");
  await dialog.getByTestId("task-title-input").fill("Unfinished package selection");
  await dialog.getByTestId("task-description-input").fill("/e2e:simple-message");
  const { packages } = await openCreationMCP(page, dialog, touch);
  if (capture.capturing) {
    await capture.screenshot(
      touch ? "phone-new-task-mcp-packages" : "desktop-new-task-mcp-packages",
      {
        caption:
          "New Task groups built-in packages inside Advanced settings and MCP servers, with one package heading.",
      },
    );
  }
  await activate(packages.getByTestId("builtin-mcp-package-configuration"), touch);
  const form = dialog.locator("form");
  await form.evaluate((node) => {
    node.dataset.submissions = "0";
    node.addEventListener(
      "submit",
      (event) => {
        node.dataset.submissions = String(Number(node.dataset.submissions) + 1);
        event.preventDefault();
        event.stopPropagation();
      },
      true,
    );
  });
  await activate(packages.getByRole("button", { name: "Back to packages", exact: true }), touch);
  await expect(packages.getByTestId("builtin-mcp-package-configuration")).toBeFocused();
  await expect(form).toHaveAttribute("data-submissions", "0");
  await expect(dialog).toBeVisible();
  await page.keyboard.press("Escape");
}
