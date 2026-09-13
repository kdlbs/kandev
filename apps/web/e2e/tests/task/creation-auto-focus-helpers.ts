import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { KanbanPage } from "../../pages/kanban-page";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { SessionPage } from "../../pages/session-page";
import { waitForHttp } from "../../helpers/causal-waits";

async function saveFocusPreference(page: Page, enabled: boolean, mobile: boolean) {
  await page.goto("/settings/preferences/task-behavior");
  const card = page.getByTestId("creation-auto-focus-card");
  const toggle = card.getByRole("switch", { name: "Auto-focus new tasks" });
  await expect(toggle).toBeChecked({ checked: !enabled });
  await toggle.scrollIntoViewIfNeeded();
  if (mobile) {
    const box = await toggle.boundingBox();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    expect(box!.width).toBeGreaterThanOrEqual(44);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
      await page.evaluate(() => document.documentElement.clientWidth),
    );
  }
  await toggle.click();
  await page
    .getByTestId("settings-floating-save")
    .getByRole("button", { name: "Save changes" })
    .click();
  await expect(page.getByTestId("settings-floating-save")).toBeHidden();
  await page.reload();
  await expect(toggle).toBeChecked({ checked: enabled });
}

async function submitTask(page: Page, title: string, withAgent: boolean, mobile: boolean) {
  const dialog = page.getByTestId("create-task-dialog");
  await expect(dialog).toBeVisible();
  await dialog.getByTestId("task-title-input").fill(title);
  await dialog.getByTestId("task-description-input").fill("/e2e:simple-message");
  if (!withAgent && !mobile) {
    await expect(dialog.getByTestId("submit-start-agent")).toBeEnabled();
    await dialog.getByTestId("submit-start-agent-chevron").click();
  }
  const submit =
    !withAgent && mobile
      ? dialog.getByRole("button", { name: "Create only", exact: true })
      : page.getByTestId(withAgent ? "submit-start-agent" : "submit-create-without-agent");
  await expect(submit).toBeEnabled();
  await submit.click();
  await expect(dialog).toBeHidden();
}

async function openCreationDialog(page: Page, open: () => Promise<void>) {
  // Local repository status supplies the default branch used by submit eligibility.
  const repositoryReady = waitForHttp(page, "GET", /\/repositories\/local-status$/);
  await open();
  const response = await repositoryReady;
  expect(response.ok()).toBe(true);
  await response.finished();
}

async function openFromTask(page: Page, mobile: boolean) {
  if (mobile) {
    await page.getByTestId("mobile-session-menu").click();
    await page
      .getByRole("dialog", { name: "Tasks", exact: true })
      .getByRole("button", { name: "New", exact: true })
      .click();
  } else {
    await page.getByTestId("create-task-button").click();
  }
}

async function taskID(api: ApiClient, workspaceID: string, title: string) {
  let id: string | undefined;
  await expect
    .poll(async () => {
      id = (await api.listTasks(workspaceID)).tasks.find((task) => task.title === title)?.id;
      return id;
    })
    .toBeTruthy();
  return id!;
}

// @covers AC-TASKS-CREATION-AUTO-FOCUS-001.1 through .5
export async function verifyCreationAutoFocus(
  page: Page,
  api: ApiClient,
  workspaceID: string,
  mobile: boolean,
) {
  const { settings: baseline } = await api.getUserSettings();
  expect(baseline.auto_focus_new_tasks).toBe(true);
  await api.saveUserSettings({ agent_generated_task_titles: false });
  try {
    await saveFocusPreference(page, false, mobile);
    const board = mobile ? new MobileKanbanPage(page) : new KanbanPage(page);
    await board.goto();
    const listingURL = page.url();
    const opener = mobile
      ? (board as MobileKanbanPage).mobileFab
      : page.getByTestId("create-task-button");
    await openCreationDialog(page, () => opener.click());
    await submitTask(page, "Background task one", false, mobile);
    await expect(page).toHaveURL(listingURL);
    await expect(opener).toBeFocused();
    const firstID = await taskID(api, workspaceID, "Background task one");
    // The task is discoverable and can still be opened deliberately.
    if (mobile) {
      await (board as MobileKanbanPage).taskCard(firstID).click();
    } else {
      await new SessionPage(page).sidebarTaskItem("Background task one").click();
    }
    await expect(page).toHaveURL(new RegExp(`/t/${firstID}`));
    await new SessionPage(page).waitForLoad();
    const activeURL = page.url();
    await openCreationDialog(page, () => openFromTask(page, mobile));
    await submitTask(page, "Background task with agent", true, mobile);
    await expect(page).toHaveURL(activeURL);
    if (mobile) await expect(page.getByTestId("mobile-session-menu")).toBeFocused();
    const secondID = await taskID(api, workspaceID, "Background task with agent");
    await expect
      .poll(
        async () =>
          (await api.listTaskSessions(secondID)).sessions.some((session) =>
            ["RUNNING", "WAITING_FOR_INPUT", "COMPLETED"].includes(session.state),
          ),
        { timeout: 30_000 },
      )
      .toBe(true);
    await saveFocusPreference(page, true, mobile);
    await page.goto(activeURL);
    await new SessionPage(page).waitForLoad();
    await openCreationDialog(page, () => openFromTask(page, mobile));
    await submitTask(page, "Focused task again", false, mobile);
    const focusedID = await taskID(api, workspaceID, "Focused task again");
    await expect(page).toHaveURL(new RegExp(`/t/${focusedID}`));
  } finally {
    await api.saveUserSettings({
      auto_focus_new_tasks: baseline.auto_focus_new_tasks as boolean,
      agent_generated_task_titles: baseline.agent_generated_task_titles ?? true,
    });
  }
}
