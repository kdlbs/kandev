/**
 * E2E: `PluginComposerCapability` against the real native composers
 * (docs/specs/plugins/requirements/voice-extraction-host.md).
 *
 * These are the prerequisites the extracted Voice Mode plugin stands on, so
 * everything here goes through a genuinely registered plugin slot and then
 * observes what the *native* path produced — a created task, a sent message,
 * a launched session. A test that called the host's submit callback directly
 * would prove nothing about native validation, steering, or draft ownership.
 *
 * The fixture plugin registers one composer action on all three slots; see
 * apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js. It exposes
 * insert / capture / insert-captured / submit / focus buttons plus the live
 * slot props as data attributes.
 */
import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";
import { SessionPage } from "../../pages/session-page";
import { KanbanPage } from "../../pages/kanban-page";
import type { ApiClient } from "../../helpers/api-client";
import type { Locator, Page } from "@playwright/test";

const DICTATED = "DICTATED";
const CREATED_TITLE = "Composer created task";

async function uninstallViaApi(apiClient: ApiClient): Promise<void> {
  await apiClient.rawRequest("DELETE", `/api/plugins/${PLUGIN_ID}`).catch(() => undefined);
}

/** The fixture's action inside a specific composer (dialog, chat, ...). */
function action(scope: Page | Locator): Locator {
  return scope.getByTestId("e2e-composer-action").first();
}

/**
 * Places the caret in the middle of an already-typed draft, so an insertion
 * that merely appends is distinguishable from one that honours the selection.
 *
 * Walks left from the end rather than using Control+Home: `fill` leaves the
 * caret at the end on every platform, while Control+Home is a no-op under
 * Android emulation and silently leaves the caret where it was, which makes
 * the assertion vacuous instead of failing.
 */
async function caretBackFromEnd(target: Locator, steps: number): Promise<void> {
  await target.click();
  for (let i = 0; i < steps; i++) await target.press("ArrowLeft");
}

test.describe("Plugins — composer capability", () => {
  test.afterEach(async ({ apiClient }) => {
    await uninstallViaApi(apiClient);
  });

  test("task chat: inserts at the selection and submits through the native handler", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await installFixturePlugin(testPage);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Composer capability task chat",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const editor = await session.composerReady();

    const composerAction = action(session.activeChat());
    await expect(composerAction).toBeVisible();
    await expect(composerAction).toHaveAttribute("data-surface", "task-chat");
    await expect(composerAction).toHaveAttribute("data-presentation", "desktop");
    await expect(composerAction).toHaveAttribute("data-task-id", task.id);

    // An empty composer is not submittable; the props are live, not a
    // snapshot taken when the slot first mounted.
    await expect(composerAction).toHaveAttribute("data-submittable", "false");

    await editor.fill("head tail");
    await expect(composerAction).toHaveAttribute("data-submittable", "true");

    // Caret between "head" and " tail" — a plain append would land at the end.
    await caretBackFromEnd(editor, " tail".length);
    await composerAction.getByTestId("e2e-composer-insert").click();
    await expect(composerAction).toHaveAttribute("data-status", "inserted");
    await expect(editor).toHaveText(`head ${DICTATED} tail`);

    await composerAction.getByTestId("e2e-composer-submit").click();
    await expect(composerAction).toHaveAttribute("data-status", "submitted");

    // The native submit path is what actually sent it: the message exists on
    // the session, and the composer was cleared by the host, not by us.
    await expect(editor).toHaveText("");
    await session.expectChatResponseVisible(`head ${DICTATED} tail`);
  });

  test("task chat: a capability captured before an edit still inserts and submits", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await installFixturePlugin(testPage);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Composer capability stability",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const editor = await session.composerReady();
    const composerAction = action(session.activeChat());

    // This is the voice plugin's actual shape: grab the capability when
    // recording starts, then use it after the user has typed (and the host
    // has re-rendered the composer many times, flipping `submittable`).
    await composerAction.getByTestId("e2e-composer-capture").click();
    await expect(composerAction).toHaveAttribute("data-status", "captured");

    await editor.fill("typed while recording");
    await expect(composerAction).toHaveAttribute("data-submittable", "true");

    await composerAction.getByTestId("e2e-composer-insert-captured").click();
    await expect(composerAction).toHaveAttribute("data-status", "inserted");
    await expect(editor).toHaveText(`typed while recording ${DICTATED}`);

    await composerAction.getByTestId("e2e-composer-submit").click();
    await expect(composerAction).toHaveAttribute("data-status", "submitted");
    await session.expectChatResponseVisible(`typed while recording ${DICTATED}`);
  });

  test("task chat: submitting an empty composer is blocked and keeps the draft", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await installFixturePlugin(testPage);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Composer capability blocked submit",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const editor = await session.composerReady();
    const composerAction = action(session.activeChat());

    await expect(composerAction).toHaveAttribute("data-submittable", "false");
    await composerAction.getByTestId("e2e-composer-submit").click();
    await expect(composerAction).toHaveAttribute("data-status", "blocked");
    await expect(editor).toHaveText("");
  });

  test("task creation: inserts at the textarea selection and the native form creates the task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await installFixturePlugin(testPage);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    const description = dialog.getByTestId("task-description-input");
    await expect(description).toBeVisible();

    const composerAction = action(dialog);
    await expect(composerAction).toBeVisible();
    await expect(composerAction).toHaveAttribute("data-surface", "task-create");
    await expect(composerAction).toHaveAttribute("data-submittable", "false");

    await description.fill("head tail");
    await expect(composerAction).toHaveAttribute("data-submittable", "true");
    await caretBackFromEnd(description, " tail".length);

    await composerAction.getByTestId("e2e-composer-insert").click();
    await expect(composerAction).toHaveAttribute("data-status", "inserted");
    await expect(description).toHaveValue(`head ${DICTATED} tail`);

    // The title still comes from the native field: the capability owns the
    // prompt, not the whole form.
    await dialog.getByTestId("task-title-input").fill(CREATED_TITLE);
    await expect(dialog.getByTestId("submit-start-agent")).toBeEnabled({ timeout: 30_000 });

    await composerAction.getByTestId("e2e-composer-submit").click();

    // The native creation handler ran, not a plugin-built request: the app
    // navigated to a real task and the dictated prompt reached the agent.
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 30_000 });
    await expect
      .poll(
        async () => {
          const { tasks } = await apiClient.listTasks(seedData.workspaceId);
          return tasks.some((t) => t.title === CREATED_TITLE);
        },
        { timeout: 30_000 },
      )
      .toBe(true);
  });

  test("new session: inserts at the selection and the native handler launches the session", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await installFixturePlugin(testPage);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Composer capability new session",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const before = (await apiClient.listTaskSessions(task.id)).sessions.length;

    await session.openNewSessionDialog();
    const dialog = session.newSessionDialog();
    await expect(dialog).toBeVisible();

    const description = dialog.getByTestId("task-description-input");
    await expect(description).toBeVisible();

    const composerAction = action(dialog);
    await expect(composerAction).toBeVisible();
    await expect(composerAction).toHaveAttribute("data-surface", "new-session");

    await description.fill("head tail");
    await caretBackFromEnd(description, " tail".length);
    await composerAction.getByTestId("e2e-composer-insert").click();
    await expect(description).toHaveValue(`head ${DICTATED} tail`);

    await composerAction.getByTestId("e2e-composer-submit").click();
    await expect(composerAction).toHaveAttribute("data-status", "submitted");

    await expect
      .poll(async () => (await apiClient.listTaskSessions(task.id)).sessions.length, {
        timeout: 45_000,
      })
      .toBeGreaterThan(before);
  });

  test("an uninstalled plugin leaves no composer action behind", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await installFixturePlugin(testPage);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Composer capability cleanup",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.composerReady();
    await expect(action(session.activeChat())).toBeVisible();

    await uninstallViaApi(apiClient);
    await testPage.reload();
    await session.waitForLoad();
    await session.composerReady();

    await expect(testPage.getByTestId("e2e-composer-action")).toHaveCount(0);
  });
});
