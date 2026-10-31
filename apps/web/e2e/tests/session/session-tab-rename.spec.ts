import { dwell, watchWs } from "../../helpers/causal-waits";
import { visitOtherBrowserTab } from "../../helpers/external-focus";
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { composeTabRename } from "../../helpers/tab-rename-composition";

test.use({ channel: "chromium" });

// @covers AC-UI-EDIT-FOCUS-001.4
test("keeps session rename open during composition until a separate Enter", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Session rename composition",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Missing session ID");
  const renames: unknown[] = [];
  testPage.on("websocket", (socket) => {
    if (new URL(socket.url()).pathname !== "/ws") return;
    socket.on("framesent", ({ payload }) => {
      const frame = JSON.parse(String(payload));
      if (frame.action === "session.rename") renames.push(frame.payload);
    });
  });
  const ws = watchWs(testPage);
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  const tab = session.sessionTabBySessionId(task.session_id);
  await tab.click({ button: "right" });
  await session.contextMenuItem("Rename").click();
  const input = testPage.getByTestId("session-tab-rename-input");
  await composeTabRename(input);
  expect(renames).toEqual([]);
  const saved = ws.waitForResponse("session.rename");
  await input.press("Enter");
  await saved;
  await expect(input).toHaveCount(0);
  expect(renames).toEqual([{ session_id: task.session_id, name: "日本語" }]);
  await expect(tab).toContainText("日本語");
  await testPage.reload();
  await session.waitForLoad();
  await expect(tab).toContainText("日本語");
  expect(
    (await apiClient.listTaskSessions(task.id)).sessions.find((row) => row.id === task.session_id)
      ?.name,
  ).toBe("日本語");
});

test.describe("Session tab rename", () => {
  // @covers AC-UI-TASK-AGENT-TAB-RECONCILIATION-002.1 through .3
  test("keeps rename-input double-clicks out of the maximize handler", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Agent Tab Rename Double-click",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    const tab = session.sessionTabBySessionId(task.session_id);
    await expect(tab).toBeVisible();
    const header = testPage.locator(
      `.dv-tabs-and-actions-container:has([data-testid="session-tab-${task.session_id}"])`,
    );
    const maximizeButton = header.getByTestId("dockview-maximize-btn");
    await expect(maximizeButton.locator(".tabler-icon-arrows-maximize")).toBeVisible();

    await tab.click({ button: "right" });
    await session.contextMenuItem("Rename").click();
    const input = testPage.getByTestId("session-tab-rename-input");
    await input.click();
    await expect(input).toBeFocused();
    await input.fill("implementation");
    // @covers AC-UI-EDIT-FOCUS-001.1
    await visitOtherBrowserTab(testPage, async () => {
      await expect(input).toHaveValue("implementation");
      await expect(input).toBeVisible();
    });
    await input.dblclick();
    await expect(maximizeButton.locator(".tabler-icon-arrows-maximize")).toBeVisible();
    const selection = await input.evaluate((element: HTMLInputElement) => ({
      start: element.selectionStart,
      end: element.selectionEnd,
      length: element.value.length,
    }));
    expect(selection).toEqual({ start: 0, end: 14, length: 14 });

    await testPage.keyboard.press("Escape");
    await expect(input).not.toBeVisible();
    await tab.dblclick();
    await expect(maximizeButton.locator(".tabler-icon-arrows-minimize")).toBeVisible();

    await tab.click({ button: "right" });
    await session.contextMenuItem("Rename").click();
    await input.click();
    await expect(input).toBeFocused();
    await input.fill("implementation");
    await input.dblclick();
    await expect(maximizeButton.locator(".tabler-icon-arrows-minimize")).toBeVisible();
    await testPage.keyboard.press("Enter");
    await expect(input).not.toBeVisible();
    await expect(tab).toContainText("implementation");
  });
});

// @covers AC-UI-TASK-AGENT-TAB-RECONCILIATION-002.5 and .6
test("preserves legal Unicode names and edits at the server code-point limit", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Unicode session rename",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Missing session ID");
  const id = task.session_id;
  const initial = "🧭".repeat(120);
  await apiClient.renameSession(id, initial);
  const renames: Array<{ session_id: string; name: string }> = [];
  testPage.on("websocket", (socket) => {
    if (new URL(socket.url()).pathname !== "/ws") return;
    socket.on("framesent", ({ payload }) => {
      const frame = JSON.parse(String(payload));
      if (frame.action === "session.rename") renames.push(frame.payload);
    });
  });
  const ws = watchWs(testPage);
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  const tab = session.sessionTabBySessionId(id);
  const input = testPage.getByTestId("session-tab-rename-input");
  async function openRename() {
    await tab.click({ button: "right" });
    await session.contextMenuItem("Rename").click();
    await expect(input).toHaveCount(1);
    await expect(input).toBeFocused();
  }
  await expect(tab).toContainText(initial);
  await openRename();
  await expect(input).toHaveValue(initial);
  await input.press("Enter");
  await expect(input).toHaveCount(0);
  await dwell(
    testPage,
    250,
    "negative-assertion",
    "Unchanged rename has no completion event; observe transport for an accidental write",
  );
  expect(renames).toEqual([]);

  await openRename();
  const boundary = `${"x".repeat(119)}🧭`;
  await input.fill(boundary);
  await expect(input).toHaveValue(boundary);
  await input.press("End");
  await testPage.keyboard.insertText("extra");
  await expect(input).toHaveValue(boundary);
  await input.press("Home");
  await testPage.keyboard.insertText("Z");
  await expect(input).toHaveValue(boundary);
  await input.press("Home");
  await input.press("ArrowRight");
  expect(await input.evaluate((element: HTMLInputElement) => element.selectionStart)).toBe(1);
  await testPage.keyboard.insertText("Z");
  await expect(input).toHaveValue(boundary);
  const saved = ws.waitForResponse("session.rename");
  await input.press("Enter");
  await saved;
  expect(renames).toEqual([{ session_id: id, name: boundary }]);
  await expect(tab).toContainText(boundary);
  await testPage.reload();
  await session.waitForLoad();
  await expect(tab).toContainText(boundary);
  expect(
    (await apiClient.listTaskSessions(task.id)).sessions.find((row) => row.id === id)?.name,
  ).toBe(boundary);

  await openRename();
  await input.fill("Cancelled 🧭");
  await input.press("Escape");
  await expect(input).toHaveCount(0);
  await expect(tab).toContainText(boundary);

  await openRename();
  const changed = "漢🧭".repeat(60);
  await input.fill(changed);
  await expect(input).toHaveValue(changed);
  const blurred = ws.waitForResponse("session.rename");
  await input.press("Tab");
  await blurred;
  await expect(input).toHaveCount(0);
  await expect(tab).toContainText(changed);
  expect(renames).toEqual([
    { session_id: id, name: boundary },
    { session_id: id, name: changed },
  ]);
  await testPage.screenshot({ path: test.info().outputPath("unicode-session-desktop.png") });
});
