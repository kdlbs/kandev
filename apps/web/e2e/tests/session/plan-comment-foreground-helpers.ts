import type { Page } from "@playwright/test";
import { test, expect, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { planScript } from "../../helpers/seed-session-messages";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

type Frame = { id?: string; type?: string; action?: string; payload?: { task_id?: string } };
type Options = {
  testPage: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  mobile: boolean;
  editing: boolean;
  failedRead: boolean;
};
const DRAFT = "Keep this unfinished feedback\nincluding the second line";

async function holdForegroundRead(page: Page, taskId: string, fail: boolean) {
  let armed = false;
  let release: (() => void) | undefined;
  let mutations = 0;
  await page.routeWebSocket(/\/ws$/, (ws) => {
    const server = ws.connectToServer();
    server.onMessage((message) => ws.send(message));
    ws.onMessage((message) => {
      if (typeof message !== "string") return server.send(message);
      for (const part of message.split("\n").filter(Boolean)) {
        const frame = JSON.parse(part) as Frame;
        const ownsRequest = frame.type === "request" && frame.payload?.task_id === taskId;
        if (
          armed &&
          ownsRequest &&
          /task\.plan\.comments\.(create|update)/.test(frame.action ?? "")
        )
          mutations++;
        if (armed && !release && ownsRequest && frame.action === "task.plan.get") {
          release = () => {
            armed = false;
            if (fail)
              ws.send(
                JSON.stringify({
                  type: "error",
                  id: frame.id,
                  action: frame.action,
                  payload: { code: "INTERNAL_ERROR", message: "Injected foreground read failure" },
                }),
              );
            else server.send(part);
          };
        } else server.send(part);
      }
    });
  });
  return {
    arm: () => {
      armed = true;
    },
    held: () => Boolean(release),
    mutations: () => mutations,
    release: () => release?.(),
  };
}

async function openComment(page: Page, session: SessionPage, mobile: boolean) {
  const editor = session.planPanel.locator(".ProseMirror:visible");
  await expect(editor).toBeVisible();
  await editor.focus();
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await page.keyboard.press(`${modifier}+a`);
  if (mobile) await page.getByTestId("plan-formatting-action-comment").tap();
  else await page.keyboard.press(`${modifier}+Shift+c`);
}

export async function assertCommentSurvivesForeground(options: Options) {
  const { testPage: page, apiClient, seedData, mobile, editing, failedRead } = options;
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Foreground comment draft",
    seedData.agentProfileId,
    {
      description: planScript("## Review plan\n\nPreserve this plan comment"),
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await expect.poll(() => apiClient.getTaskPlan(task.id), { timeout: 30_000 }).not.toBeNull();
  const control = await holdForegroundRead(page, task.id, failedRead);
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle();
  await session.togglePlanMode();
  if (mobile)
    await page.getByRole("navigation").getByRole("button", { name: "Plan", exact: true }).tap();
  await openComment(page, session, mobile);
  const input = page.locator('textarea[placeholder="Add your comment or instruction..."]:visible');
  if (editing) {
    await input.fill("Original saved feedback");
    await page.getByRole("button", { name: "Add", exact: true }).click();
    const badge = session.planPanel.locator(".comment-badge[data-comment-id]");
    await expect(badge).toHaveCount(1);
    const highlight = session.planPanel
      .locator(".comment-highlight")
      .filter({ hasText: "Preserve this plan comment" });
    if (mobile) await highlight.tap();
    else await highlight.click();
    await expect(input).toHaveValue("Original saved feedback");
  }
  await input.fill(DRAFT);
  const originalInput = await input.elementHandle();
  control.arm();
  if (!mobile) {
    const otherTab = await page.context().newPage();
    try {
      await otherTab.bringToFront();
      await page.bringToFront();
    } finally {
      await otherTab.close();
    }
  }
  // Headless browsers may not emit OS focus events for bringToFront.
  // Dispatch the foreground event too; the production hook coalesces duplicates.
  await page.evaluate(() => window.dispatchEvent(new Event("focus")));
  await expect.poll(control.held).toBe(true);
  try {
    await expect(session.planPanel.locator(".ProseMirror")).toHaveAttribute(
      "contenteditable",
      "false",
    );
    await expect(input).toBeEditable();
    await expect(input).toHaveValue(DRAFT);
    expect(await originalInput!.evaluate((element) => element.isConnected)).toBe(true);
    expect(control.mutations()).toBe(0);
  } finally {
    control.release();
  }
  await expect
    .poll(() =>
      page.evaluate((taskId) => {
        const win = window as Window & {
          __KANDEV_E2E_STORE__?: {
            getState: () => { taskPlans: { loadingByTaskId: Record<string, boolean> } };
          };
        };
        return win.__KANDEV_E2E_STORE__?.getState().taskPlans.loadingByTaskId[taskId];
      }, task.id),
    )
    .toBe(false);
  await expect(input).toHaveValue(DRAFT);
  expect(await originalInput!.evaluate((element) => element.isConnected)).toBe(true);
  await expect(session.planPanel.locator(".ProseMirror")).toHaveAttribute(
    "contenteditable",
    "true",
  );
  if (mobile) await assertNoDocumentHorizontalOverflow(page, "foreground comment draft");
  await page.screenshot({ path: test.info().outputPath("foreground-comment.png") });
  await page.getByRole("button", { name: editing ? "Update" : "Add", exact: true }).click();
  await expect
    .poll(async () => {
      const snapshot = await apiClient.wsRequest<{ comments: Array<{ body: string }> }>(
        "task.plan.comments.list",
        { task_id: task.id },
      );
      return snapshot.comments.map((comment) => comment.body);
    })
    .toEqual([DRAFT]);
  await originalInput?.dispose();
}
