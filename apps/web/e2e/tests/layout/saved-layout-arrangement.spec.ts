import type { Page } from "@playwright/test";
import { captureWorkbenchInteraction } from "../../helpers/interaction-capture";
import { test, expect } from "../../fixtures/test-base";
import { snapshotPersistedLayouts } from "../../helpers/dockview-persistence";
import { SessionPage } from "../../pages/session-page";
import { seedNavigationTasks, selectNavigationTask } from "../task/task-navigation-helpers";

async function waitForArrangement(page: Page, sessionId: string, columns: number) {
  await expect
    .poll(() =>
      page.evaluate(
        ({ sessionId, columns }) => {
          const api = (
            window as Window & { __dockviewApi__?: import("dockview-react").DockviewApi }
          ).__dockviewApi__;
          const root = api?.toJSON().grid.root;
          return (
            api?.getPanel(`session:${sessionId}`)?.api.isVisible &&
            root?.type === "branch" &&
            root.data.length === columns
          );
        },
        { sessionId, columns },
      ),
    )
    .toBe(true);
}

async function persistArrangement(page: Page, sessionId: string, columns: number) {
  await waitForArrangement(page, sessionId, columns);
  await page.evaluate(() => {
    (window as Window & { __persistDockviewLayout__: () => void }).__persistDockviewLayout__();
  });
  await expect.poll(() => snapshotPersistedLayouts(page)).toContain(sessionId);
}

test("restores different task arrangements with identical panels and preserves both drafts", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
  prCapture,
}) => {
  test.setTimeout(120_000);
  const tasks = await seedNavigationTasks(api, seed, backend);
  const environments = await Promise.all(tasks.map((task) => api.getTaskEnvironment(task.id)));
  expect(environments[0]?.id).toBeTruthy();
  expect(environments[1]?.id).not.toBe(environments[0]?.id);
  for (const [index, task] of tasks.entries()) {
    if (index === 0) await page.goto(`/t/${task.id}`);
    else await selectNavigationTask(page, task.title);
    const session = new SessionPage(page);
    await session.waitForLoad();
    await session.waitForChatIdle();
    await page.locator(".tiptap:visible").fill(`Retain ${task.title} draft`);
    await persistArrangement(page, task.session_id!, 2);
  }
  await page.evaluate((sessionId) => {
    const api = (window as Window & { __dockviewApi__: import("dockview-react").DockviewApi })
      .__dockviewApi__;
    const files = api.getPanel("files");
    const chat = api.getPanel(`session:${sessionId}`);
    if (!files || !chat) throw new Error("Arrangement fixture panels are missing");
    files.api.moveTo({ group: chat.group, position: "left" });
    files.api.setActive();
  }, tasks[1].session_id!);
  await persistArrangement(page, tasks[1].session_id!, 3);
  for (const [iteration, index] of [0, 1, 0, 1].entries()) {
    const task = tasks[index];
    await selectNavigationTask(page, task.title);
    await waitForArrangement(page, task.session_id!, 2 + index);
    await expect(page.locator(".tiptap:visible")).toHaveText(`Retain ${task.title} draft`);
    const arrangement = await page.evaluate(() => {
      const api = (window as Window & { __dockviewApi__: import("dockview-react").DockviewApi })
        .__dockviewApi__;
      return {
        filesGroup: api.getPanel("files")?.group.id,
        changesGroup: api.getPanel("changes")?.group.id,
        terminal: Boolean(api.getPanel("terminal-default")),
      };
    });
    expect(arrangement.terminal).toBe(true);
    expect(arrangement.filesGroup === arrangement.changesGroup).toBe(index === 0);
    if (iteration < 2)
      await captureWorkbenchInteraction(
        page,
        prCapture,
        `restored-${2 + index}-columns`,
        `Desktop task restores its saved ${2 + index}-column arrangement and draft.`,
      );
  }
});
