import { type Locator, type Page } from "@playwright/test";
import { test, expect, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";
import type { CreateTaskResponse } from "../../../lib/types/http";
import type {
  EntityReference,
  EntityReferenceSearchResponse,
} from "../../../lib/types/entity-reference";

const REFERENCE_QUERY = "E2E Reference";
const LINEAR_SCOPE = "mock-org";

function taskReference(workspaceId: string, taskId: string, title: string): EntityReference {
  return {
    version: 1,
    ref: `mention:v1:kandev:task:${workspaceId}:${taskId}`,
    provider: "kandev",
    kind: "task",
    id: taskId,
    title,
    url: `/t/${taskId}`,
    scope: workspaceId,
  };
}

function linearReference(id: string, key: string, title: string): EntityReference {
  return {
    version: 1,
    ref: `mention:v1:linear:issue:${LINEAR_SCOPE}:${id}`,
    provider: "linear",
    kind: "issue",
    id,
    key,
    title,
    url: `https://linear.app/${LINEAR_SCOPE}/issue/${key}`,
    scope: LINEAR_SCOPE,
  };
}

function externalSearchResponse(
  query: string,
  references: EntityReference[],
  includeTimedOutProvider = false,
): EntityReferenceSearchResponse {
  return {
    query,
    groups: [
      {
        source: "kandev_tasks",
        provider: "kandev",
        kind: "task",
        display_name: "Kandev tasks",
        kind_label: "Task",
        status: "ok",
        results: [taskReference("workspace-stale", "task-stale", "Hidden Kandev task")],
      },
      {
        source: "linear_issues",
        provider: "linear",
        kind: "issue",
        display_name: "Linear",
        kind_label: "Issue",
        status: "ok",
        results: references,
      },
      ...(includeTimedOutProvider
        ? [
            {
              source: "github_issues",
              provider: "github",
              kind: "issue",
              display_name: "GitHub issues",
              kind_label: "Issue",
              status: "timeout" as const,
              results: [],
            },
          ]
        : []),
    ],
  };
}

async function createReadyTask(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<CreateTaskResponse> {
  return apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
}

async function openTaskChat(page: Page, taskId: string): Promise<SessionPage> {
  await page.goto(`/t/${taskId}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return session;
}

function visibleEditor(scope: Locator | Page): Locator {
  return scope.locator(".tiptap.ProseMirror:visible").first();
}

function referenceOption(page: Page, title: string): Locator {
  return page.getByRole("option").filter({ hasText: title });
}

async function expectMenuAnchoredToEditor(menu: Locator, editor: Locator): Promise<void> {
  const [menuBox, editorBox] = await Promise.all([menu.boundingBox(), editor.boundingBox()]);
  expect(menuBox).not.toBeNull();
  expect(editorBox).not.toBeNull();
  expect(Math.abs(menuBox!.y + menuBox!.height - editorBox!.y)).toBeLessThanOrEqual(4);
}

async function typeReferenceQuery(editor: Locator, query: string): Promise<void> {
  await editor.click();
  await editor.fill("");
  await editor.pressSequentially(`#${query}`);
}

async function expectPersistedReference(
  apiClient: ApiClient,
  sessionId: string,
  referenceId: string,
): Promise<EntityReference> {
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        return messages.some((message) => {
          const references = message.metadata?.entity_references;
          return (
            message.author_type === "user" &&
            Array.isArray(references) &&
            references.some(
              (reference) =>
                typeof reference === "object" &&
                reference !== null &&
                (reference as Record<string, unknown>).id === referenceId,
            )
          );
        });
      },
      { timeout: 15_000, message: `Wait for persisted reference ${referenceId}` },
    )
    .toBe(true);

  const { messages } = await apiClient.listSessionMessages(sessionId);
  for (const message of messages) {
    const references = message.metadata?.entity_references;
    if (!Array.isArray(references)) continue;
    const reference = references.find(
      (candidate) =>
        typeof candidate === "object" &&
        candidate !== null &&
        (candidate as Record<string, unknown>).id === referenceId,
    );
    if (reference) return reference as EntityReference;
  }
  throw new Error(`Persisted entity reference ${referenceId} disappeared`);
}

async function configureLinear(apiClient: ApiClient, workspaceId: string): Promise<void> {
  await apiClient.setLinearConfig({ secret: "lin_api_entity_refs", workspaceId });
  await apiClient.waitForIntegrationAuthHealthy("linear", { workspaceId });
}

async function openQuickChatWithAgent(page: Page): Promise<{
  dialog: Locator;
  sessionId: string;
}> {
  await page.goto("/");
  await page.waitForLoadState("networkidle");
  await page.getByTestId("sidebar-quick-chat-shortcut").click();

  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  await expect(dialog).toBeVisible({ timeout: 10_000 });
  const setup = dialog.getByTestId("quick-chat-setup");
  if (!(await setup.isVisible({ timeout: 1_000 }).catch(() => false))) {
    await dialog.getByLabel("Start new chat").click();
  }
  await expect(setup).toBeVisible({ timeout: 5_000 });

  const agentSelector = dialog.getByTestId("agent-profile-selector");
  if (
    await agentSelector
      .getByText("Select agent", { exact: false })
      .isVisible()
      .catch(() => false)
  ) {
    await agentSelector.click();
    await page.getByRole("option").first().click();
  }

  const started = page.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      new URL(response.url()).pathname.endsWith("/quick-chat"),
  );
  await dialog.getByTestId("quick-chat-start").click();
  const payload = (await started).json() as Promise<{ session_id: string }>;
  const { session_id: sessionId } = await payload;

  const editor = visibleEditor(dialog);
  await expect(editor).toBeVisible({ timeout: 15_000 });
  await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 30_000 });
  return { dialog, sessionId };
}

async function createPassthroughProfile(apiClient: ApiClient): Promise<string> {
  const { agents } = await apiClient.listAgents();
  const agent = agents[0];
  if (!agent) throw new Error("No E2E agent registered");
  const profile = await apiClient.createAgentProfile(agent.id, "Entity Reference Passthrough", {
    model: "mock-fast",
    cli_passthrough: true,
  });
  return profile.id;
}

test.describe("Entity reference composer", () => {
  test("keeps Kandev task suggestions under @", async ({ testPage, apiClient, seedData }) => {
    const targetTitle = "At-Mention-Task-Target";
    await apiClient.seedTask(seedData.workspaceId, targetTitle, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    const active = await createReadyTask(apiClient, seedData, "At Mention Active Task");
    const session = await openTaskChat(testPage, active.id);
    const editor = visibleEditor(session.activeChat());

    await editor.fill("");
    await editor.pressSequentially(`@${targetTitle}`);

    await expect(testPage.getByRole("option").filter({ hasText: targetTitle })).toBeVisible({
      timeout: 10_000,
    });
  });

  test("task chat restores a keyboard-selected draft and explicitly sends durable metadata", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const alpha = linearReference("linear-alpha", "ENG-101", `${REFERENCE_QUERY} Alpha`);
    const beta = linearReference("linear-beta", "ENG-102", `${REFERENCE_QUERY} Beta`);
    await configureLinear(apiClient, seedData.workspaceId);
    const active = await createReadyTask(apiClient, seedData, "Entity Reference Active Task");
    if (!active.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    const pageErrors: string[] = [];
    testPage.on("pageerror", (error) => pageErrors.push(error.message));
    const session = await openTaskChat(testPage, active.id);
    const editor = visibleEditor(session.activeChat());
    await testPage.route("**/api/v1/workspaces/*/mentions/search?*", async (route) => {
      const query = new URL(route.request().url()).searchParams.get("q") ?? "";
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(externalSearchResponse(query, [alpha, beta])),
      });
    });
    await typeReferenceQuery(editor, REFERENCE_QUERY);

    const menu = testPage.getByTestId("entity-reference-menu");
    await expect(menu).toBeVisible({ timeout: 10_000 });
    await expect(menu.getByRole("listbox", { name: "Reference work items" })).toBeVisible();
    await expect(referenceOption(testPage, alpha.title)).toHaveAttribute("aria-selected", "true");
    await expect(referenceOption(testPage, "Hidden Kandev task")).toHaveCount(0);
    await expectMenuAnchoredToEditor(menu, editor);

    await editor.press("ArrowDown");
    const betaOption = referenceOption(testPage, beta.title);
    await expect(betaOption).toHaveAttribute("aria-selected", "true");
    await editor.press("Tab");

    await expect(menu).not.toBeVisible();
    await expect(editor).toBeFocused();
    await expect(editor.getByTestId("entity-reference-chip")).toContainText(beta.key ?? beta.title);
    await editor.pressSequentially("needs follow-up");
    await expect(editor).toHaveText(/ENG-102\s+needs follow-up/);
    await expect(
      session
        .activeChat()
        .locator(".chat-message-list:visible")
        .getByText("needs follow-up", { exact: false }),
    ).not.toBeVisible();

    await testPage.reload();
    await session.waitForLoad();
    const restoredEditor = visibleEditor(session.activeChat());
    // A restored non-empty draft intentionally has no idle placeholder, so
    // wait on editability instead of SessionPage.waitForChatIdle().
    await expect(restoredEditor).toHaveAttribute("contenteditable", "true", {
      timeout: 30_000,
    });
    await expect(restoredEditor.getByTestId("entity-reference-chip")).toContainText(
      beta.key ?? beta.title,
    );
    await expect(restoredEditor).toHaveText(/ENG-102\s+needs follow-up/);

    await session.activeChat().getByTestId("submit-message-button").click();
    const persisted = await expectPersistedReference(apiClient, active.session_id, beta.id);
    expect(persisted).toMatchObject({
      version: 1,
      provider: "linear",
      kind: "issue",
      id: beta.id,
      key: beta.key,
      title: beta.title,
      url: beta.url,
      scope: LINEAR_SCOPE,
    });

    const sentChip = session
      .activeChat()
      .locator(".chat-message-list:visible")
      .getByTestId("entity-reference-chip")
      .filter({ hasText: beta.key });
    await expect(sentChip).toBeVisible({ timeout: 10_000 });
    await expect(sentChip).toHaveAttribute("href", beta.url);
    expect(pageErrors).toEqual([]);
  });

  test("keeps successful results visible when another provider times out", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const target = linearReference("linear-partial", "ENG-201", "Partial Result Target");
    const active = await createReadyTask(apiClient, seedData, "Partial Result Active Task");
    await openTaskChat(testPage, active.id);
    await testPage.route("**/api/v1/workspaces/*/mentions/search?*", async (route) => {
      const query = new URL(route.request().url()).searchParams.get("q") ?? "";
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(externalSearchResponse(query, [target], true)),
      });
    });

    await typeReferenceQuery(visibleEditor(testPage), "Partial Result");
    const menu = testPage.getByTestId("entity-reference-menu");
    await expect(referenceOption(testPage, target.title)).toBeVisible({ timeout: 10_000 });
    await expect(referenceOption(testPage, "Hidden Kandev task")).toHaveCount(0);
    await expect(menu.getByText("GitHub issues", { exact: true })).toBeVisible();
    await expect(menu.getByText("Search timed out", { exact: true })).toBeVisible();
  });

  test("Quick Chat searches, inserts without auto-send, and explicitly sends an external reference", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const target = linearReference("linear-quick-chat", "ENG-301", "Quick Chat Reference Target");
    await configureLinear(apiClient, seedData.workspaceId);
    const { dialog, sessionId } = await openQuickChatWithAgent(testPage);
    const editor = visibleEditor(dialog);

    await testPage.route("**/api/v1/workspaces/*/mentions/search?*", async (route) => {
      const query = new URL(route.request().url()).searchParams.get("q") ?? "";
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(externalSearchResponse(query, [target])),
      });
    });

    await typeReferenceQuery(editor, "Quick Chat Reference");
    await expect(referenceOption(testPage, target.title)).toBeVisible({ timeout: 10_000 });
    await expect(referenceOption(testPage, "Hidden Kandev task")).toHaveCount(0);
    await editor.press("Enter");

    await expect(editor.getByTestId("entity-reference-chip")).toContainText(
      target.key ?? target.title,
    );
    await expect(editor).toBeFocused();
    await expect(
      dialog
        .getByTestId("quick-chat-messages")
        .getByTestId("entity-reference-chip")
        .filter({ hasText: target.key ?? target.title }),
    ).not.toBeVisible();

    await dialog.getByTestId("submit-message-button").click();
    await expectPersistedReference(apiClient, sessionId, target.id);
    await expect(
      dialog
        .getByTestId("quick-chat-messages")
        .getByTestId("entity-reference-chip")
        .filter({ hasText: target.key ?? target.title }),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("passthrough keeps hash text literal and never starts entity search", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const profileId = await createPassthroughProfile(apiClient);
    const active = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Entity Reference Passthrough Task",
      profileId,
      {
        description: "initial passthrough prompt",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await testPage.goto(`/t/${active.id}`);
    const session = new SessionPage(testPage);
    await session.waitForPassthroughLoad(20_000);
    await session.waitForPassthroughLoaded(20_000);
    await session.expectPassthroughHasText("Processed:", 20_000);

    await testPage.getByTestId("passthrough-toggle-composer").click();
    const editor = testPage.getByTestId("passthrough-composer").getByTestId("chat-input-editor");
    await expect(editor).toBeVisible({ timeout: 5_000 });
    const searchObserved = testPage
      .waitForRequest((request) => new URL(request.url()).pathname.endsWith("/mentions/search"), {
        timeout: 750,
      })
      .then(
        () => true,
        () => false,
      );
    await editor.fill("#Literal passthrough reference");

    expect(await searchObserved).toBe(false);
    await expect(testPage.getByTestId("entity-reference-menu")).toHaveCount(0);
    await expect(editor).toHaveText("#Literal passthrough reference");
  });
});
