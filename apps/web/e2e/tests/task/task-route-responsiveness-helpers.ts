import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import type { BackendContext } from "../../fixtures/backend";
import { SessionPage } from "../../pages/session-page";
import { seedNavigationTasks, selectNavigationTask } from "./task-navigation-helpers";

export async function assertImmediateTaskReturn(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  backend: BackendContext,
  mobile: boolean,
) {
  const [a, b] = await seedNavigationTasks(api, seed, backend);
  for (const task of [a, b]) {
    await expect
      .poll(
        async () =>
          (await api.listTaskSessions(task.id)).sessions.find(
            (session) => session.id === task.session_id,
          )?.state,
      )
      .toBe("WAITING_FOR_INPUT");
    await api.seedSessionMessage(task.session_id!, {
      type: "message",
      authorType: "agent",
      content: `Cached conversation for ${task.title}`,
    });
  }
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(`/t/${a.id}`);
  await new SessionPage(page).waitForLoad();
  const chat = page.getByTestId("session-chat").filter({ visible: true });
  await expect(chat).toContainText(`Cached conversation for ${a.title}`);
  await selectNavigationTask(page, b.title, mobile);
  await expect(chat).toContainText(`Cached conversation for ${b.title}`);

  let release!: () => void;
  const responseGate = new Promise<void>((resolve) => {
    release = resolve;
  });
  let held = 0;
  const forwards: Promise<void>[] = [];
  const taskUrl = `**/api/v1/tasks/${a.id}`;
  await page.route(taskUrl, (route) => {
    held++;
    const forward = responseGate.then(() => route.continue());
    forwards.push(forward);
    return forward;
  });
  try {
    await selectNavigationTask(page, a.title, mobile);
    await expect.poll(() => held).toBeGreaterThan(0);
    // The task details response is still held: this proves cached content is usable.
    await expect(chat).toContainText(`Cached conversation for ${a.title}`);
    await expect(
      chat.getByText(`Cached conversation for ${a.title}`, { exact: true }),
    ).toBeVisible();
    await expect(page.getByRole("status").filter({ hasText: "Loading task" })).toHaveCount(0);
    await expect(page.getByTestId("task-loading-state")).toHaveCount(0);
    await expect(chat.locator("xpath=ancestor::*[@inert]")).toHaveCount(0);
    if (mobile) {
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
        true,
      );
      await page.getByTestId("mobile-task-picker-trigger").tap();
      await expect(page.getByRole("dialog", { name: "Tasks", exact: true })).toBeVisible();
      await page.keyboard.press("Escape");
    } else {
      await expect(page.getByTestId("task-topbar-title")).toHaveText(a.title);
    }
    expect(errors).toEqual([]);
  } finally {
    release();
    await Promise.all(forwards);
    await page.unroute(taskUrl);
  }
  await expect(chat).toContainText(`Cached conversation for ${a.title}`);
}
