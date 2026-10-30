import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";
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
  await activate(page.getByTestId(`builtin-mcp-choice-${id}`), touch);
  await activate(
    page.getByRole("listbox").getByRole("option", { name: label, exact: true }),
    touch,
  );
}
export async function checkBuiltinWorkspace(page: Page, api: ApiClient, touch: boolean) {
  const workspace = await api.createWorkspace(`Built-in packages ${Date.now()}`);
  try {
    await page.goto(`/settings/workspaces/${workspace.id}/mcp-servers`);
    await expect(page.getByTestId("builtin-mcp-packages")).toContainText("Core");
    await activate(page.getByTestId("builtin-mcp-package-configuration"), touch);
    const detail = page.getByTestId("builtin-mcp-detail");
    await expect(detail).toContainText("Configuration chats always receive Core and Configuration");
    await activate(page.getByTestId("builtin-mcp-choice-configuration"), touch);
    await expect(
      page.getByRole("listbox").getByRole("option", { name: "On by default", exact: true }),
    ).toHaveCount(0);
    await page.keyboard.press("Escape");
    await activate(detail.getByRole("button", { name: "Back", exact: true }), touch);
    await expect(page.getByTestId("builtin-mcp-package-configuration")).toBeFocused();
    await activate(page.getByTestId("builtin-mcp-package-diagnostics"), touch);
    await chooseBuiltin(page, "diagnostics", "Blocked", touch);
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
    await activate(page.getByTestId("builtin-mcp-package-diagnostics"), touch);
    await expect(page.getByTestId("builtin-mcp-choice-diagnostics")).toContainText("Blocked");
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
  await expect(save).toBeDisabled();
  await activate(explorer.getByRole("button", { name: "Back", exact: true }), touch);
  await activate(explorer.getByTestId("builtin-mcp-package-walkthroughs"), touch);
  await chooseBuiltin(page, "walkthroughs", "Off", touch);
  await activate(save, touch);
  await expect
    .poll(async () => (await builtinView(api, "tasks", task.id)).preferences?.packages.walkthroughs)
    .toBe("off");
  await expect(explorer).toContainText(
    "Running and resumed processes keep their accepted packages",
  );
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
  const packages = dialog.getByTestId("task-create-builtin-packages");
  await activate(packages.locator(":scope > summary"), touch);
  await activate(packages.getByTestId("builtin-mcp-package-configuration"), touch);
  await expect(packages.getByTestId("builtin-mcp-effective")).toContainText("Off");
  await chooseBuiltin(page, "configuration", "On", touch);
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
