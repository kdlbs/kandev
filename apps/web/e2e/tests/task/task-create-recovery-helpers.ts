import { expect, type Locator, type Page, type Route } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

export const RECOVERY_STATUS = "task-create-recovery-status";
export const RECOVERY_RETRY = "task-create-recovery-retry";
export const RECOVERY_CHECK = "task-create-recovery-check";
export const RECOVERY_OPEN = "task-create-recovery-open";
export const RECOVERY_CLOSE = "task-create-recovery-close";

export type CapturedCreate = {
  id: string;
  external_id: string;
  status?: string;
};

export async function seedRecoveryWorkflow(
  apiClient: ApiClient,
  seedData: SeedData,
  name: string,
): Promise<{ workflowId: string; stepId: string }> {
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, name);
  const step = await apiClient.createWorkflowStep(workflow.id, "Start", 0, {
    is_start_step: true,
    events: { on_enter: [], on_turn_complete: [] },
  });
  return { workflowId: workflow.id, stepId: step.id };
}

export async function openDesktopCreateDialog(page: Page) {
  const kanban = new KanbanPage(page);
  await kanban.goto();
  await kanban.createTaskButton.first().click();
  const dialog = page.getByTestId("create-task-dialog");
  await expect(dialog).toBeVisible();
  return dialog;
}

export async function openMobileCreateDialog(page: Page) {
  const mobile = new MobileKanbanPage(page);
  await mobile.goto();
  await mobile.mobileFab.tap();
  const dialog = page.getByTestId("create-task-dialog");
  await expect(dialog).toBeVisible();
  return dialog;
}

export async function selectRecoveryWorkflow(dialog: Locator, workflowId: string) {
  const trigger = dialog.getByTestId("workflow-selector-trigger");
  await expect(trigger).toBeVisible();
  await trigger.click();
  await dialog.getByTestId(`workflow-option-select-${workflowId}`).click();
  await expect(dialog.getByTestId("workflow-selector-popover")).toHaveCount(0);
}

/** Waits for the dialog's data-driven selectors before the default readiness gate. */
export async function expectCreateReady(dialog: Locator) {
  await expect(dialog.getByTestId("workflow-selector-trigger")).toBeVisible();
  await expect(dialog.getByTestId("agent-profile-selector")).toBeVisible();
  await expect(dialog.getByTestId("executor-profile-selector")).toBeVisible();
  await expect(dialog.getByTestId("repo-chip-trigger").first()).toContainText("E2E Repo");
  await expect(dialog.getByTestId("submit-start-agent")).toBeEnabled();
}

export async function fillRecoveryDraft(
  dialog: ReturnType<Page["getByTestId"]>,
  title: string,
  description: string,
) {
  await dialog.getByTestId("task-title-input").fill(title);
  await dialog.getByTestId("task-description-input").fill(description);
}

/**
 * Forwards the first native POST to the real backend, keeps its accepted task,
 * then replaces only the browser response with a failure.
 */
export async function installLostResponseRoute(page: Page, captured: CapturedCreate[]) {
  let hideNextResponse = true;
  await page.route("**/api/v1/tasks", async (route: Route) => {
    if (route.request().method() !== "POST") {
      await route.continue();
      return;
    }
    const body = route.request().postDataJSON() as { external_id?: string };
    const upstream = await route.fetch();
    const response = (await upstream.json()) as { id?: string };
    if (response.id && body.external_id) {
      captured.push({ id: response.id, external_id: body.external_id });
    }
    if (hideNextResponse) {
      hideNextResponse = false;
      await route.fulfill({ status: 502, contentType: "text/plain", body: "Bad Gateway" });
      return;
    }
    await route.fulfill({ response: upstream });
  });
  return {
    allowBrowserResponses() {
      hideNextResponse = false;
    },
  };
}

/** Makes the next read-only lookups miss, without touching the real backend. */
export async function installLookupMisses(page: Page, count = 1) {
  let remaining = count;
  await page.route("**/api/v1/workspaces/*/tasks/by-external-id?*", async (route) => {
    if (remaining <= 0) {
      await route.continue();
      return;
    }
    remaining -= 1;
    await route.fulfill({
      status: 404,
      contentType: "application/json",
      body: JSON.stringify({ error: "not found" }),
    });
  });
}

/** Forwards a lookup and overrides only its settlement projection. */
export async function installIncompleteLookupProjection(page: Page) {
  let pending = true;
  await page.route("**/api/v1/workspaces/*/tasks/by-external-id?*", async (route) => {
    if (!pending) {
      await route.continue();
      return;
    }
    const upstream = await route.fetch();
    const body = (await upstream.json()) as Record<string, unknown>;
    await route.fulfill({
      status: upstream.status(),
      contentType: "application/json",
      body: JSON.stringify({ ...body, creation_complete: false }),
    });
  });
  return {
    restore() {
      pending = false;
    },
  };
}

export async function expectOneRecoveryTask(
  apiClient: ApiClient,
  workspaceId: string,
  title: string,
  expectedTaskId: string,
) {
  await expect
    .poll(
      async () =>
        (await apiClient.listTasks(workspaceId)).tasks.filter((t) => t.title === title).length,
      { timeout: 30_000, message: "one recovered task must exist for the submission" },
    )
    .toBe(1);
  await expect
    .poll(async () => (await apiClient.listTaskSessions(expectedTaskId)).sessions.length, {
      timeout: 30_000,
      message: "the recovered task must hold the expected session",
    })
    .toBe(1);
}

export async function expectOpeningPrompt(apiClient: ApiClient, taskId: string, prompt: string) {
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        const session = sessions[0];
        if (!session) return 0;
        const { messages } = await apiClient.listSessionMessages(session.id);
        return messages.filter(
          (message) => message.author_type === "user" && message.content === prompt,
        ).length;
      },
      { timeout: 30_000, message: "the recovered submission must have one opening prompt" },
    )
    .toBe(1);
}

export async function expectNoOpeningPrompt(apiClient: ApiClient, taskId: string) {
  const { sessions } = await apiClient.listTaskSessions(taskId);
  for (const session of sessions) {
    const { messages } = await apiClient.listSessionMessages(session.id);
    expect(
      messages.filter((message) => message.author_type === "user").length,
      "create-only recovery must not submit an initial prompt",
    ).toBe(0);
  }
}
