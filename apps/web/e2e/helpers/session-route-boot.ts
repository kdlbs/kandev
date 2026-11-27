import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { SessionPage } from "../pages/session-page";
import { observeReadTraffic, sessionReadState } from "./unread-divider";
import { dwell } from "./causal-waits";
import { waitForActiveTaskSession } from "./session-store";

async function splitBootColumn(page: Page, viewer: SessionPage, sourceId: string) {
  const bounds = await viewer.activeChat().boundingBox();
  if (!bounds) throw new Error("Source chat is not visible before splitting");
  await page
    .locator(".dv-default-tab:visible")
    .filter({ hasText: /^Files$/ })
    .dragTo(viewer.activeChat(), { targetPosition: { x: bounds.width / 2, y: bounds.height - 5 } });
  await expect
    .poll(() =>
      page.evaluate((id) => {
        const api = (window as Window & { __dockviewApi__?: import("dockview-react").DockviewApi })
          .__dockviewApi__;
        const chat = api?.getPanel(`session:${id}`)?.group.element.getBoundingClientRect();
        const files = api?.getPanel("files")?.group.element.getBoundingClientRect();
        return !!chat && !!files && Math.abs(chat.x - files.x) < 2 && files.y > chat.y;
      }, sourceId),
    )
    .toBe(true);
}

async function seedLegacyMaximize(page: Page, environmentId: string, sourceId: string) {
  await page.evaluate(
    ({ environmentId, sourceId }) => {
      const key = `kandev.dockview.env-maximize-v3.${environmentId}`;
      const saved = JSON.parse(sessionStorage.getItem(key)!);
      const overlay = saved.maximizedDockviewJson as import("dockview-react").SerializedDockview;
      const visit = (node: typeof overlay.grid.root) => {
        if (Array.isArray(node.data)) node.data.forEach(visit);
        else if (node.data.views.includes(`session:${sourceId}`)) {
          if (overlay.activeGroup === node.data.id) overlay.activeGroup = "group-1";
          node.data.id = "group-1";
        }
      };
      visit(overlay.grid.root);
      sessionStorage.setItem(key, JSON.stringify(saved));
    },
    { environmentId, sourceId },
  );
}

/** Real prepared sessions retain the same environment across a new Go-boot document. */
export async function expectBootConversationRoute(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  maximized: boolean | "parked" | "hidden" | "tree" | "legacy" = false,
) {
  const liveOverlay = maximized === true || maximized === "tree" || maximized === "legacy";
  const task = await api.createTask(seed.workspaceId, "Boot explicit conversation", {
    prepare_session: true,
    agent_profile_id: seed.agentProfileId,
    executor_profile_id: seed.worktreeExecutorProfileId,
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
    repository_ids: [seed.repositoryId],
  });
  const sourceId = task.session_id;
  if (!sourceId) throw new Error("Boot fixture did not prepare its source session");
  await expect
    .poll(async () => (await api.getTaskEnvironment(task.id))?.status, { timeout: 60_000 })
    .toBe("ready");
  const environment = await api.getTaskEnvironment(task.id);
  if (!environment) throw new Error("Prepared environment disappeared after readiness");
  const requested = await api.launchSession({
    task_id: task.id,
    agent_profile_id: seed.agentProfileId,
    executor_profile_id: seed.worktreeExecutorProfileId,
    workflow_step_id: seed.startStepId,
    intent: "prepare",
    prompt: "",
    launch_workspace: true,
  });
  expect(requested.session_id).not.toBe(sourceId);
  expect((await api.getTaskSession(requested.session_id)).session.task_environment_id).toBe(
    environment.id,
  );
  await api.setPrimarySession(sourceId);
  await api.seedSessionMessage(sourceId, {
    type: "message",
    content: "Saved active boot conversation",
  });
  const viewer = new SessionPage(page);
  await page.goto(`/t/${task.id}?sessionId=${sourceId}`);
  await expect(viewer.activeChat()).toHaveAttribute("data-session-id", sourceId);
  if (maximized === "hidden") {
    await viewer.sessionTabCloseButton(requested.session_id).click();
    await expect(viewer.sessionTabBySessionId(requested.session_id)).toHaveCount(0);
  }
  if (maximized) {
    if (maximized === "tree") await splitBootColumn(page, viewer, sourceId);
    if (maximized === "parked") {
      await viewer.clickTab("Files");
      await viewer
        .sessionTabBySessionId(requested.session_id)
        .dragTo(page.locator(".dv-default-tab:visible").filter({ hasText: /^Files$/ }));
      await expect(
        page.locator(
          `[data-testid="session-chat"][data-session-id="${requested.session_id}"]:visible`,
        ),
      ).toBeVisible();
    }
    await page
      .locator(".dv-tabs-and-actions-container")
      .filter({ has: viewer.sessionTabBySessionId(sourceId) })
      .getByTestId("dockview-maximize-btn")
      .click();
  } else {
    await viewer.sessionTabCloseButton(requested.session_id).click();
    await expect(viewer.sessionTabBySessionId(requested.session_id)).toHaveCount(0);
  }
  await expect
    .poll(() =>
      page.evaluate(
        ({ environmentId, active, hidden, maximized }) => {
          const raw = sessionStorage.getItem(`kandev.dockview.env-layout-v3.${environmentId}`);
          const savedMax = sessionStorage.getItem(
            `kandev.dockview.env-maximize-v3.${environmentId}`,
          );
          return (
            !!raw &&
            raw.includes(`"activeView":"session:${active}"`) &&
            (maximized
              ? (maximized === "hidden" || raw.includes(`"session:${hidden}"`)) &&
                !!savedMax?.includes('"maximizedDockviewJson"')
              : !raw.includes(`"session:${hidden}"`))
          );
        },
        {
          environmentId: environment.id,
          active: sourceId,
          hidden: requested.session_id,
          maximized,
        },
      ),
    )
    .toBe(true);
  await page.goto("/");
  if (maximized === "legacy") await seedLegacyMaximize(page, environment.id, sourceId);
  const requestedTail = await api.seedSessionMessage(requested.session_id, {
    type: "message",
    content: "Unread explicitly requested boot conversation",
  });
  const requestedBefore = (await sessionReadState(api, requested.session_id)).cursor;
  await page.goto(`/t/${task.id}`);
  await expect(viewer.activeChat()).toHaveAttribute("data-session-id", sourceId);
  await expect(viewer.sessionTabBySessionId(requested.session_id)).toHaveCount(liveOverlay ? 1 : 0);
  expect((await sessionReadState(api, requested.session_id)).cursor).toBe(requestedBefore);
  const sourceBefore = (await sessionReadState(api, sourceId)).cursor;
  await page.goto("/");
  await api.seedSessionMessage(sourceId, {
    type: "message",
    content: "Unread saved tab must stay unread",
  });
  const traffic = observeReadTraffic(page);
  const origin = await page.evaluate(() => performance.timeOrigin);
  await page.goto(`/t/${task.id}?sessionId=${requested.session_id}`);
  expect(await page.evaluate(() => performance.timeOrigin)).not.toBe(origin);
  const requestedChat = page.locator(
    `[data-testid="session-chat"][data-session-id="${requested.session_id}"]:visible`,
  );
  await expect(requestedChat).toBeVisible();
  await expect(
    requestedChat.getByText("Unread explicitly requested boot conversation", { exact: true }),
  ).toBeVisible();
  await expect
    .poll(async () => (await sessionReadState(api, requested.session_id)).cursor)
    .toBe(requestedTail.messageId);
  await dwell(
    page,
    500,
    "negative-assertion",
    "Boot explicit focus must not read the restored sibling",
  );
  if (liveOverlay) {
    await page.getByTestId("dockview-maximize-btn").first().hover();
    await expect(page.getByRole("tooltip")).toHaveText("Restore");
    await page.getByTestId("dockview-maximize-btn").first().click();
    await expect(requestedChat).toBeVisible();
    await dwell(page, 500, "negative-assertion", "Restore must not read the saved sibling");
  }
  if (maximized === "parked") {
    await expect
      .poll(() =>
        page.evaluate(
          ({ a, b }) => {
            const api = (
              window as Window & { __dockviewApi__?: import("dockview-react").DockviewApi }
            ).__dockviewApi__;
            const left = api?.getPanel(`session:${a}`);
            const right = api?.getPanel(`session:${b}`);
            return !!left && !!right && left.group.id !== right.group.id;
          },
          { a: sourceId, b: requested.session_id },
        ),
      )
      .toBe(true);
  } else {
    expect((await sessionReadState(api, sourceId)).cursor).toBe(sourceBefore);
    expect(traffic.markReads.filter((row) => row.sessionId === sourceId)).toEqual([]);
  }
  await waitForActiveTaskSession(page, task.id, requested.session_id);
  return { requestedId: requested.session_id, requestedTail: requestedTail.messageId };
}
