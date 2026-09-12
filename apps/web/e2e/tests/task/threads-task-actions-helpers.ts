import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { createStandardProfile } from "../../helpers/git-helper";
import { waitForLatestSessionDone } from "../../helpers/session";
import { assertNoDocumentHorizontalOverflow, requireBox } from "../../helpers/layout-assertions";

export async function withTaskActionSettings(api: ApiClient, run: () => Promise<void>) {
  const { settings } = await api.getUserSettings();
  const keys = [
    "confirm_task_archive",
    "thread_views",
    "thread_active_view_id",
    "thread_view_draft",
  ];
  const restore = Object.fromEntries(
    keys.filter((key) => key in settings).map((key) => [key, settings[key]]),
  );
  try {
    await run();
  } finally {
    expect((await api.rawRequest("PATCH", "/api/v1/user/settings", restore)).ok).toBe(true);
  }
}

export async function seedActionThreads(api: ApiClient, seed: SeedData) {
  const profile = await createStandardProfile(api, "thread-task-actions");
  const source = await api.createWorkflow(seed.workspaceId, "Thread actions source", "simple");
  const { steps } = await api.listWorkflowSteps(source.id);
  const start = steps.find((step) => step.is_start_step);
  if (!start) throw new Error("Thread action workflow has no start step");
  const tasks = [];
  for (const title of ["Task action target A", "Unaffected conversation B"]) {
    const task = await api.createTaskWithAgent(seed.workspaceId, title, profile.id, {
      description: "/e2e:simple-message",
      workflow_id: source.id,
      workflow_step_id: start.id,
      repository_ids: [seed.repositoryId],
    });
    await waitForLatestSessionDone(api, task.id, 1, `ready task ${title}`);
    tasks.push(task);
  }
  const destination = await api.createWorkflow(seed.workspaceId, "Task actions destination");
  const destinationStep = await api.createWorkflowStep(destination.id, "Incoming", 0);
  const localStep = await api.createWorkflowStep(source.id, "Task actions triage", 20);
  const emptyWorkflow = await api.createWorkflow(seed.workspaceId, "No available steps");
  return { a: tasks[0], b: tasks[1], destination, destinationStep, localStep, emptyWorkflow };
}

export class ThreadActionsPage {
  constructor(
    readonly page: Page,
    readonly mobile = false,
  ) {
    page.setDefaultTimeout(15_000);
  }
  column(taskId: string) {
    return this.page.getByTestId(`thread-column-${taskId}`);
  }
  trigger(taskId: string) {
    return this.column(taskId).getByRole("button", { name: "Task actions", exact: true });
  }
  async expectHeaderActionAlignment(taskId: string, withPicker = this.mobile) {
    const header = this.column(taskId).locator("header");
    const expand = header.getByRole("button", { name: "Open task", exact: true });
    const actions = this.trigger(taskId);
    await expect(actions).toBeVisible();
    const expandIcon = await requireBox(expand.locator("svg"), "expand icon");
    const actionsIcon = await requireBox(actions.locator("svg"), "task actions icon");
    expect(
      actionsIcon.y + actionsIcon.height / 2,
      "header icons share a vertical center",
    ).toBeCloseTo(expandIcon.y + expandIcon.height / 2, 1);
    if (!withPicker) return;
    const picker = header.getByTestId("thread-picker-trigger");
    const pickerIcon = await requireBox(picker.locator("svg"), "thread picker icon");
    const pickerGap = expandIcon.x + expandIcon.width / 2 - pickerIcon.x - pickerIcon.width / 2;
    const actionsGap = actionsIcon.x + actionsIcon.width / 2 - expandIcon.x - expandIcon.width / 2;
    expect(actionsGap, "phone header icons are evenly spaced").toBeCloseTo(pickerGap, 1);
    let previousRight = 0;
    for (const button of [picker, expand, actions]) {
      const box = await requireBox(button, "phone header touch target");
      expect(box.width).toBeGreaterThanOrEqual(44);
      expect(box.height).toBeGreaterThanOrEqual(44);
      expect(box.x, "header touch targets do not overlap").toBeGreaterThanOrEqual(previousRight);
      previousRight = box.x + box.width;
      expect(
        await button.evaluate((element) => {
          const rect = element.getBoundingClientRect();
          return element.contains(
            document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2),
          );
        }),
        "header control receives touches at its center",
      ).toBe(true);
    }
  }
  async press(locator: Locator) {
    if (this.mobile) await locator.tap();
    else await locator.click();
  }
  async open(taskId: string) {
    await this.trigger(taskId).scrollIntoViewIfNeeded();
    await this.press(this.trigger(taskId));
    await expect(
      this.page.getByTestId(this.mobile ? "task-management-drawer" : "task-management-menu"),
    ).toBeVisible();
  }
  choice(name: string) {
    return this.page.getByRole(this.mobile ? "button" : "menuitem", { name, exact: true });
  }
  async nested(name: string) {
    const option = this.choice(name);
    if (this.mobile) await option.tap();
    else {
      await this.movePointer(option);
      await option.click();
      await expect(option).toHaveAttribute("aria-expanded", "true");
    }
  }
  async pick(name: string) {
    const choice = this.choice(name);
    if (!this.mobile) await this.movePointer(choice);
    await this.press(choice);
  }
  private async movePointer(choice: Locator) {
    await expect(choice).toBeVisible();
    await choice.evaluate((element) =>
      Promise.all(
        (element.closest('[role="menu"]')?.getAnimations() ?? []).map(
          (animation) => animation.finished,
        ),
      ),
    );
    const box = await requireBox(choice, "task menu choice");
    // Radix pointer grace needs direction from intermediate moves when a submenu flips left.
    await this.page.mouse.move(box.x + box.width / 2, box.y + box.height / 2, { steps: 12 });
  }
  async cancelDelete(taskId: string) {
    await this.open(taskId);
    await this.pick("Delete");
    const dialog = this.page.getByRole("alertdialog");
    await expect(dialog).toBeVisible();
    await this.press(dialog.getByRole("button", { name: "Cancel", exact: true }));
    await expect(dialog).toBeHidden();
    await expect(this.trigger(taskId)).toBeFocused();
  }
  async delete(taskId: string) {
    await this.open(taskId);
    await this.pick("Delete");
    await this.confirmDelete();
    await expect(this.column(taskId)).toHaveCount(0);
  }
  async confirmDelete() {
    const consent = this.page.getByTestId("delete-discard-worktree-checkbox");
    if (await consent.isVisible()) await this.press(consent);
    await this.press(this.page.getByTestId("thread-delete-confirm"));
  }
  async contained(surface: Locator) {
    await expect(surface).toBeVisible();
    await surface.evaluate((element) =>
      Promise.all(
        element
          .getAnimations({ subtree: true })
          .filter((animation) => Number.isFinite(animation.effect?.getComputedTiming().endTime))
          .map((animation) => animation.finished.catch(() => undefined)),
      ),
    );
    const box = await requireBox(surface, "task actions surface");
    const viewport = this.page.viewportSize()!;
    expect(box.x).toBeGreaterThanOrEqual(0);
    expect(box.x + box.width).toBeLessThanOrEqual(viewport.width + 1);
    expect(box.y).toBeGreaterThanOrEqual(0);
    expect(box.y + box.height).toBeLessThanOrEqual(viewport.height + 1);
    await assertNoDocumentHorizontalOverflow(this.page, "Threads task actions");
  }
}

// @covers AC-TASKS-THREADS-ACTIONS-001.1 through AC-TASKS-THREADS-ACTIONS-001.6
export async function allTaskActionOutcomes(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  mobile: boolean,
) {
  const fixture = await seedActionThreads(api, seed);
  await api.updateRepository(seed.repositoryId, {
    provider: "github",
    provider_owner: "testorg",
    provider_name: "testrepo",
    provider_host: "github.com",
  });
  const { a, b, destination, destinationStep, localStep, emptyWorkflow } = fixture;
  const ui = new ThreadActionsPage(page, mobile);
  await api.saveUserSettings({ confirm_task_archive: true });
  await page.goto(`/threads?workspace=${seed.workspaceId}&taskId=${a.id}`);
  const siblingBefore = await api.getTask(b.id);
  const deletions: string[] = [];
  page.on("request", (request) => {
    if (request.method() === "DELETE" && request.url().includes(`/tasks/${a.id}`))
      deletions.push(request.url());
  });
  await ui.cancelDelete(a.id);
  expect(deletions).toEqual([]);

  await ui.open(a.id);
  const root = page.getByTestId(mobile ? "task-management-scroll" : "task-management-menu");
  await expect(root.getByRole(mobile ? "button" : "menuitem")).toHaveText([
    "Priority",
    "Move to",
    "Send to workflow",
    "Link",
    "Archive",
    "Delete",
  ]);
  await page.keyboard.press("Escape");

  await ui.open(a.id);
  await ui.nested("Send to workflow");
  await expect(page.getByTestId(`task-context-workflow-${emptyWorkflow.id}`)).toBeDisabled();
  await page.keyboard.press("Escape");
  if (mobile) await page.keyboard.press("Escape");

  await ui.open(a.id);
  await ui.nested("Priority");
  await ui.pick("Critical");
  await expect.poll(async () => (await api.getTask(a.id)).priority).toBe("critical");

  await ui.open(a.id);
  await ui.nested("Link");
  await ui.pick("GitHub Issue");
  await expect(page.getByTestId("task-management-drawer")).toHaveCount(0);
  await page
    .getByTestId("task-github-issue-input")
    .fill("https://github.com/testorg/testrepo/issues/901");
  await ui.contained(page.getByRole("dialog"));
  await ui.press(page.getByTestId("task-github-issue-submit"));
  await expect.poll(async () => (await api.getTask(a.id)).metadata?.issue_number).toBe(901);
  await expect(ui.trigger(a.id)).toBeFocused();

  await ui.open(a.id);
  await ui.nested("Move to");
  const current = await api.getTask(a.id);
  await expect(page.getByTestId(`task-context-step-${current.workflow_step_id}`)).toBeDisabled();
  await ui.pick("Task actions triage");
  await expect.poll(async () => (await api.getTask(a.id)).workflow_step_id).toBe(localStep.id);

  await ui.open(a.id);
  await ui.nested("Send to workflow");
  await ui.nested(destination.name);
  await ui.pick("Incoming");
  await expect
    .poll(async () => (await api.getTask(a.id)).workflow_step_id)
    .toBe(destinationStep.id);
  await expect(page).toHaveURL(/\/threads\?/);
  await page.reload();
  await expect(ui.column(a.id)).toBeVisible();
  expect((await api.getTask(a.id)).priority).toBe("critical");
  expect((await api.getTask(a.id)).metadata?.issue_number).toBe(901);
  expect(await api.getTask(b.id)).toMatchObject({
    priority: siblingBefore.priority,
    workflow_step_id: siblingBefore.workflow_step_id,
  });

  await ui.open(a.id);
  await ui.pick("Archive");
  await expect(page.getByTestId("archive-task-confirm")).toBeVisible();
  await ui.press(page.getByRole("button", { name: "Cancel", exact: true }));
  await expect(ui.column(a.id)).toBeVisible();
  await ui.open(a.id);
  await ui.pick("Archive");
  await ui.press(page.getByTestId("archive-task-confirm"));
  await expect(ui.column(a.id)).toHaveCount(0);
  await expect(ui.trigger(b.id)).toBeFocused();
  await expect.poll(async () => (await api.getTask(a.id)).archived_at).toBeTruthy();
  await ui.delete(b.id);
  await expect(page.getByTestId("threads-empty-state")).toBeVisible();
  await expect(page.getByTestId("threads-empty-state")).toBeFocused();
  await expect(page).toHaveURL(/\/threads/);
  expect((await api.rawRequest("GET", `/api/v1/tasks/${b.id}`)).status).toBe(404);
}

export async function seedLinkedIssue(api: ApiClient) {
  await api.mockGitHubSetUser("thread-actions-user");
  await api.mockGitHubAddIssues([
    {
      number: 901,
      title: "Issue linked from Threads",
      state: "open",
      author_login: "test-user",
      repo_owner: "testorg",
      repo_name: "testrepo",
      html_url: "https://github.com/testorg/testrepo/issues/901",
    },
  ]);
}
