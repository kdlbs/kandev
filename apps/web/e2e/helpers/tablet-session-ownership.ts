import { randomUUID } from "node:crypto";
import { expect, type Page } from "@playwright/test";
import { withToolFileTask } from "./tool-file-opening";
import { dwell } from "./causal-waits";
import {
  newestStoredMessageId,
  observeReadTraffic,
  sessionReadState,
  taskUnreadSessionIds,
} from "./unread-divider";
import { selectCommandTask, waitForIndexedConversation } from "./session-route-admission";
import { waitForActiveTaskSession } from "./session-store";
import { waitForSessionState } from "./session";

type Context = Parameters<typeof withToolFileTask>[0];
async function holdSavedFile(page: Page, sessionId?: string) {
  let released = false;
  const held: Array<() => void> = [];
  const requests: unknown[] = [];
  await page.routeWebSocket(
    (url) => url.pathname === "/ws",
    (socket) => {
      const server = socket.connectToServer();
      server.onMessage((message) => socket.send(message));
      socket.onMessage((message) => {
        if (typeof message !== "string") {
          server.send(message);
          return;
        }
        for (const part of message.split("\n").filter(Boolean)) {
          const frame = JSON.parse(part);
          if (frame.action === "workspace.file.get") requests.push(frame.payload);
          if (
            !released &&
            frame.action === "workspace.file.get" &&
            frame.payload?.session_id === sessionId &&
            typeof frame.payload.path === "string" &&
            /(^|\/)Dockerfile$/.test(frame.payload.path)
          ) {
            held.push(() => server.send(part));
          } else server.send(part);
        }
      });
    },
  );
  return {
    arm: (id: string) => {
      sessionId = id;
      released = false;
    },
    wait: () =>
      expect
        .poll(() => ({ blocked: held.length > 0, sessionId, requests }), { timeout: 15_000 })
        .toMatchObject({ blocked: true }),
    release: () => {
      released = true;
      held.splice(0).forEach((send) => send());
    },
  };
}

export async function expectTabletDraftDuringRestore(context: Context, screenshot: string) {
  const { testPage: page, apiClient: api } = context;
  await page.setViewportSize({ width: 820, height: 1180 });
  const reads = observeReadTraffic(page);
  await withToolFileTask(
    { ...context, extraFile: { name: "tablet-draft.txt", content: "original live file" } },
    "read",
    async (session) => {
      const taskId = new URL(page.url()).pathname.split("/").at(-1)!;
      const sessionId = (await session.activeChat().getAttribute("data-session-id"))!;
      await waitForSessionState(api, {
        taskId,
        sessionId,
        expectedState: "WAITING_FOR_INPUT",
        message: "Tablet draft fixture must settle",
      });
      const oldCursor = await newestStoredMessageId(api, sessionId);
      await expect
        .poll(async () => (await sessionReadState(api, sessionId)).cursor)
        .toBe(oldCursor);
      await session.activeChat().locator("button[title]").filter({ hasText: "Dockerfile" }).tap();
      await expect(page.getByRole("tab", { name: "Dockerfile", exact: true })).toHaveAttribute(
        "data-state",
        "active",
      );
      await expect
        .poll(() =>
          page.evaluate((id) => sessionStorage.getItem(`kandev.openFiles.${id}`), sessionId),
        )
        .toContain("Dockerfile");
      const gate = await holdSavedFile(page, sessionId);
      try {
        await page.goto(`/t/${taskId}`, { waitUntil: "domcontentloaded", timeout: 20_000 });
        await waitForActiveTaskSession(page, taskId, sessionId);
        await gate.wait();
        await page
          .getByRole("treeitem", { name: "tablet-draft.txt File actions", exact: true })
          .tap({ timeout: 10_000 });
        const editor = page.locator(
          '.cm-content[contenteditable="true"]:visible, .monaco-editor:visible',
        );
        await expect(editor).toHaveCount(1);
        await editor.click({ timeout: 10_000 });
        await page.keyboard.press("ControlOrMeta+End");
        await page.keyboard.type("\nowned unsaved draft");
        const live = page.getByRole("tab", { name: /tablet-draft\.txt/ });
        await expect(live).toContainText("*");
        const tail = await api.seedSessionMessage(sessionId, {
          type: "message",
          content: "Unread while live draft is selected",
        });
        await expect
          .poll(() => taskUnreadSessionIds(api, context.seedData.workspaceId, taskId))
          .toEqual([sessionId]);
        gate.release();
        await expect(page.getByRole("tab", { name: "Dockerfile", exact: true })).toBeVisible();
        await dwell(
          page,
          500,
          "negative-assertion",
          "Saved metadata cannot replace the live draft or acknowledge Chat",
        );
        await expect(live).toHaveAttribute("data-state", "active");
        await expect(live).toContainText("*");
        await expect(
          page.locator(".cm-content:visible, .monaco-editor:visible .view-lines"),
        ).toContainText("owned unsaved draft");
        await expect(session.activeChat()).toHaveCount(0);
        expect((await sessionReadState(api, sessionId)).cursor).toBe(oldCursor);
        expect(reads.markReads.filter((read) => read.messageId === tail.messageId)).toEqual([]);
        await page.screenshot({ path: screenshot });
        await page.getByRole("tab", { name: "Chat", exact: true }).tap();
        await expect
          .poll(async () => (await sessionReadState(api, sessionId)).cursor)
          .toBe(tail.messageId);
      } finally {
        gate.release();
      }
    },
  );
}

export async function expectTabletWorkflowFocus(context: Context, screenshot: string) {
  const { testPage: page, apiClient: api, seedData: seed } = context;
  await page.setViewportSize({ width: 820, height: 1180 });
  const { settings } = await api.getUserSettings();
  await api.saveUserSettings({ prevent_auto_start_agent_on_open: true });
  let workflowId: string | undefined;
  const reads = observeReadTraffic(page);
  const treeGate = await holdSavedFile(page);
  try {
    const workflow = await api.createWorkflow(seed.workspaceId, `Tablet focus ${randomUUID()}`);
    workflowId = workflow.id;
    const start = await api.createWorkflowStep(workflow.id, "Tablet start", 0, {
      is_start_step: true,
    });
    const target = await api.createWorkflowStep(workflow.id, "Tablet committed", 1);
    await api.updateWorkflowStep(target.id, {
      session_target: { kind: "initial" },
      profile_session_end_policy: "park",
      prompt: 'e2e:message("Tablet workflow turn finished")',
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });
    await withToolFileTask(
      { ...context, seedData: { ...seed, workflowId: workflow.id, startStepId: start.id } },
      "read",
      async (session) => {
        const taskId = new URL(page.url()).pathname.split("/").at(-1)!;
        const sessionId = (await session.activeChat().getAttribute("data-session-id"))!;
        await waitForSessionState(api, {
          taskId,
          sessionId,
          expectedState: "WAITING_FOR_INPUT",
          message: "Tablet workflow fixture must settle",
        });
        await session.activeChat().locator("button[title]").filter({ hasText: "Dockerfile" }).tap();
        const sibling = await api.seedTaskSession(taskId, {
          sessionId: randomUUID(),
          agentProfileId: seed.agentProfileId,
          state: "WAITING_FOR_INPUT",
        });
        const query = `tabletsource${randomUUID().replaceAll("-", "")}`;
        const source = await api.seedSessionMessage(sibling.session_id, {
          type: "message",
          content: query,
        });
        await waitForIndexedConversation(api, seed.workspaceId, query, {
          sessionId: sibling.session_id,
          messageId: source.messageId,
        });
        await selectCommandTask(page, query, true);
        await expect(session.activeChat()).toHaveAttribute("data-session-id", sibling.session_id);
        await expect
          .poll(async () => (await sessionReadState(api, sibling.session_id)).cursor)
          .toBe(source.messageId);
        await api.seedSessionMessage(sessionId, {
          type: "message",
          content: "Tablet committed recipient unread output",
        });
        const response = page.waitForResponse(
          (value) =>
            value.request().method() === "POST" &&
            new URL(value.url()).pathname === `/api/v1/tasks/${taskId}/move`,
        );
        const initialStarts = reads.agentStarts().length;
        const minimal = page.getByTestId("workflow-stepper-minimal");
        if (await minimal.isVisible()) {
          await minimal.tap();
          await page.getByTestId(`workflow-step-disclosure-move-${target.id}`).tap();
        } else {
          await page.getByTestId("workflow-step-Tablet committed").hover();
          await page.getByTestId("workflow-step-move-here").click();
        }
        expect((await response).ok()).toBe(true);
        await waitForActiveTaskSession(page, taskId, sessionId);
        await expect(session.activeChat()).toHaveAttribute("data-session-id", sessionId);
        await expect(
          session
            .activeChat()
            .getByText("Tablet committed recipient unread output", { exact: true }),
        ).toBeVisible();
        await expect(
          session.activeChat().getByText("Tablet workflow turn finished", { exact: true }),
        ).toBeVisible();
        await waitForSessionState(api, {
          taskId,
          sessionId,
          expectedState: "WAITING_FOR_INPUT",
          message: "Committed tablet turn must settle",
        });
        let expectedTail = await newestStoredMessageId(api, sessionId);
        await expect
          .poll(async () => (await sessionReadState(api, sessionId)).cursor)
          .toBe(expectedTail);
        await expect
          .poll(() =>
            page.evaluate(
              () =>
                (
                  window as Window & {
                    __KANDEV_E2E_STORE__?: {
                      getState: () => { workflowSessionFocus: { request: unknown } };
                    };
                  }
                ).__KANDEV_E2E_STORE__?.getState().workflowSessionFocus.request,
            ),
          )
          .toBeNull();
        expect((await sessionReadState(api, sibling.session_id)).cursor).toBe(source.messageId);
        await page.screenshot({ path: screenshot });
        const moveStarts = reads.agentStarts().slice(initialStarts);
        expect(moveStarts.filter((action) => action !== `session.launch:${sessionId}`)).toEqual([]);
        expect(moveStarts.length).toBeLessThanOrEqual(1);
        const startsAfterMove = reads.agentStarts().length;
        treeGate.arm(sessionId);
        await page.getByRole("treeitem", { name: "Dockerfile File actions", exact: true }).tap();
        await treeGate.wait();
        const repeatedQuery = `tablettree${randomUUID().replaceAll("-", "")}`;
        const repeated = await api.seedSessionMessage(sessionId, {
          type: "message",
          content: repeatedQuery,
        });
        await waitForIndexedConversation(api, seed.workspaceId, repeatedQuery, {
          sessionId,
          messageId: repeated.messageId,
        });
        await selectCommandTask(page, repeatedQuery, true);
        await expect(session.activeChat()).toHaveAttribute("data-session-id", sessionId);
        treeGate.release();
        await expect
          .poll(async () => (await sessionReadState(api, sessionId)).cursor)
          .toBe(repeated.messageId);
        await dwell(
          page,
          500,
          "negative-assertion",
          "An older Files-tree open cannot replace the explicitly activated tablet conversation",
        );
        await expect(page.getByRole("tab", { name: "Dockerfile", exact: true })).toHaveAttribute(
          "data-state",
          "inactive",
        );
        await expect(session.activeChat()).toHaveAttribute("data-session-id", sessionId);
        expect((await sessionReadState(api, sibling.session_id)).cursor).toBe(source.messageId);
        expectedTail = repeated.messageId;
        await page.getByRole("tab", { name: "Dockerfile", exact: true }).tap();
        const later = await api.seedSessionMessage(sessionId, {
          type: "message",
          content: "After tablet workflow focus was consumed",
        });
        await expect
          .poll(() => taskUnreadSessionIds(api, seed.workspaceId, taskId))
          .toEqual([sessionId]);
        await dwell(
          page,
          500,
          "negative-assertion",
          "Acknowledged workflow focus must not replay over a later file choice",
        );
        await expect(session.activeChat()).toHaveCount(0);
        expect((await sessionReadState(api, sessionId)).cursor).toBe(expectedTail);
        expect(reads.agentStarts().slice(startsAfterMove)).toEqual([]);
        expect(reads.markReads.filter((read) => read.messageId === later.messageId)).toEqual([]);
      },
    );
  } finally {
    treeGate.release();
    try {
      if (workflowId) await api.deleteWorkflow(workflowId);
    } finally {
      await api.saveUserSettings({
        prevent_auto_start_agent_on_open: settings.prevent_auto_start_agent_on_open === true,
      });
    }
  }
}

/** A saved Changes preference on a clean tablet workspace settles to Chat from the tablet's own refresh. */
export async function expectTabletCleanChangesFallback(context: Context, screenshot: string) {
  const { testPage: page, apiClient: api } = context;
  await page.setViewportSize({ width: 820, height: 1180 });
  const reads = observeReadTraffic(page);
  await withToolFileTask(context, "read", async (session) => {
    const taskId = new URL(page.url()).pathname.split("/").at(-1)!;
    const sessionId = (await session.activeChat().getAttribute("data-session-id"))!;
    await waitForSessionState(api, {
      taskId,
      sessionId,
      expectedState: "WAITING_FOR_INPUT",
      message: "Clean tablet fixture must settle",
    });
    await page.evaluate(
      (id) => sessionStorage.setItem(`kandev.activeTab.${id}`, "changes"),
      sessionId,
    );
    await page.goto(`/t/${taskId}`, { waitUntil: "domcontentloaded", timeout: 20_000 });
    await waitForActiveTaskSession(page, taskId, sessionId);
    await expect(page.getByRole("tab", { name: "Chat", exact: true })).toHaveAttribute(
      "data-state",
      "active",
      { timeout: 15_000 },
    );
    await expect(page.getByRole("tab", { name: "All changes", exact: true })).toHaveCount(0);
    // Reload may send the existing session_focus signal; reading the fallback Chat must not start.
    const startsBefore = reads.agentStarts().length;
    const tail = await api.seedSessionMessage(sessionId, {
      type: "message",
      content: "Clean tablet output after Changes fallback",
    });
    await expect
      .poll(async () => (await sessionReadState(api, sessionId)).cursor)
      .toBe(tail.messageId);
    await expect
      .poll(() => taskUnreadSessionIds(api, context.seedData.workspaceId, taskId))
      .toEqual([]);
    expect(reads.agentStarts().slice(startsBefore)).toEqual([]);
    await page.screenshot({ path: screenshot });
  });
}
